package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/agent"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/service"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSHandler WebSocket 连接处理
func WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade 失败: %v", err)
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	identity, _ := auth.FromContext(r.Context())
	sess, _, err := sessionManager.GetOrCreate(sessionID, identity.WorkspaceID, identity.UserID)
	if err != nil {
		log.Printf("创建会话失败: %v", err)
		return
	}

	sess.SetEmitter(func(event agent.WSEvent) {
		sendSessionEvent(conn, &writeMu, sess.ID, "", event)
	})

	sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
		Type: agent.EventSessionReady,
		Data: agent.SessionReadyData{
			SessionID: sess.ID,
			Files:     sess.FilesForClient(),
		},
	})

	if len(sess.ReportState.Sections) > 0 {
		sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
			Type: agent.EventReportUpdate,
			Data: agent.ReportUpdateData{
				HTML: tools.RenderReportHTML(sess.ReportState.FinalTitle, sess.ReportState.FinalAuthor, sess.ReportState),
			},
		})
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("会话 %s: WebSocket 连接关闭", sess.ID)
			} else {
				log.Printf("会话 %s: 读取消息失败: %v", sess.ID, err)
			}
			break
		}

		var event agent.WSEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			log.Printf("消息解析失败: %v", err)
			continue
		}

		switch event.Type {
		case agent.EventUserMessage:
			dataBytes, _ := json.Marshal(event.Data)
			var userMsg agent.UserMessage
			if err := json.Unmarshal(dataBytes, &userMsg); err != nil {
				sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
					Type: agent.EventError,
					Data: agent.ErrorData{Message: "用户消息解析失败"},
				})
				continue
			}

			runID, ctx, err := sess.StartRun(r.Context())
			if err != nil {
				sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
					Type: agent.EventError,
					Data: agent.ErrorData{Message: err.Error()},
				})
				continue
			}

			now := time.Now()
			if err := runRepo.Create(r.Context(), &domain.AnalysisRun{
				ID:           runID,
				SessionID:    sess.ID,
				WorkspaceID:  sess.WorkspaceID,
				UserID:       identity.UserID,
				Status:       domain.RunStatusRunning,
				InputMessage: strings.TrimSpace(userMsg.Content),
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
			}); err != nil {
				sess.CancelRun(runID)
				sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
					Type: agent.EventError,
					Data: agent.ErrorData{Message: "创建任务记录失败: " + err.Error()},
				})
				continue
			}
			_ = sessionRepo.UpdateLastRun(r.Context(), sess.ID, runID)

			sendSessionEvent(conn, &writeMu, sess.ID, runID, agent.WSEvent{
				Type: agent.EventRunStarted,
				Data: agent.RunStartedData{RunID: runID},
			})

			runEmitter := func(runID string) func(agent.WSEvent) {
				return func(ev agent.WSEvent) {
					sendSessionEvent(conn, &writeMu, sess.ID, runID, ev)

					if ev.Type == agent.EventToolResult {
						if result, ok := ev.Data.(agent.ToolResultData); ok {
							switch result.Name {
							case "write_section":
								sendSessionEvent(conn, &writeMu, sess.ID, runID, agent.WSEvent{
									Type: agent.EventReportUpdate,
									Data: agent.ReportUpdateData{
										HTML: tools.RenderReportHTML("", "", sess.ReportState),
									},
								})
							case "finalize_report":
								finalHTML := tools.RenderReportHTML(sess.ReportState.FinalTitle, sess.ReportState.FinalAuthor, sess.ReportState)
								if reportFile, err := fileService.SaveReportHTML(r.Context(), service.SaveReportInput{
									UserID:      identity.UserID,
									WorkspaceID: sess.WorkspaceID,
									SessionID:   sess.ID,
									RunID:       runID,
									Title:       sess.ReportState.FinalTitle,
									HTML:        finalHTML,
								}); err == nil {
									_ = runRepo.BindReportFile(r.Context(), runID, reportFile.ID)
								} else {
									sendSessionEvent(conn, &writeMu, sess.ID, runID, agent.WSEvent{
										Type: agent.EventError,
										Data: agent.ErrorData{Message: "保存最终报告失败: " + err.Error()},
									})
								}
								sendSessionEvent(conn, &writeMu, sess.ID, runID, agent.WSEvent{
									Type: agent.EventReportFinal,
									Data: agent.ReportUpdateData{
										HTML: finalHTML,
									},
								})
							}
						}
					}

					switch ev.Type {
					case agent.EventRunCompleted:
						sess.FinishRun(runID, "completed")
						_ = runRepo.UpdateStatus(r.Context(), runID, domain.RunStatusCompleted, nil)
					case agent.EventRunCancelled:
						sess.FinishRun(runID, "cancelled")
						_ = runRepo.UpdateStatus(r.Context(), runID, domain.RunStatusCancelled, nil)
					case agent.EventError:
						sess.FinishRun(runID, "failed")
						if errData, ok := ev.Data.(agent.ErrorData); ok {
							msg := errData.Message
							_ = runRepo.UpdateStatus(r.Context(), runID, domain.RunStatusFailed, &msg)
						} else {
							_ = runRepo.UpdateStatus(r.Context(), runID, domain.RunStatusFailed, nil)
						}
					}
				}
			}(runID)

			sess.SetEmitter(runEmitter)

			userContent := strings.TrimSpace(userMsg.Content)
			if fileContext := sess.BuildFileContext(); fileContext != "" {
				userContent = fileContext + "\n用户指令: " + userContent
			}

			go sess.Engine.Run(ctx, userContent)

		case agent.EventStop:
			dataBytes, _ := json.Marshal(event.Data)
			var req agent.StopRunRequest
			_ = json.Unmarshal(dataBytes, &req)
			if !sess.CancelRun(req.RunID) {
				sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
					Type: agent.EventError,
					Data: agent.ErrorData{Message: "当前没有可停止的任务"},
				})
			}

		case agent.EventReset:
			dataBytes, _ := json.Marshal(event.Data)
			req := agent.ResetSessionRequest{KeepFiles: true}
			_ = json.Unmarshal(dataBytes, &req)
			if err := sess.Reset(req.KeepFiles); err != nil {
				sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
					Type: agent.EventError,
					Data: agent.ErrorData{Message: err.Error()},
				})
				continue
			}
			sendSessionEvent(conn, &writeMu, sess.ID, "", agent.WSEvent{
				Type: agent.EventSessionReset,
				Data: agent.SessionResetData{
					KeepFiles: req.KeepFiles,
					Files:     sess.FilesForClient(),
				},
			})

		default:
			log.Printf("未知事件类型: %s", event.Type)
		}
	}
}

func sendSessionEvent(conn *websocket.Conn, mu *sync.Mutex, sessionID, runID string, event agent.WSEvent) {
	event.SessionID = sessionID
	if runID != "" && event.RunID == "" {
		event.RunID = runID
	}
	sendEvent(conn, mu, event)
}

func sendEvent(conn *websocket.Conn, mu *sync.Mutex, event agent.WSEvent) {
	mu.Lock()
	defer mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("序列化事件失败: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("发送事件失败: %v", err)
	}
}
