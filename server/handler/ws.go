package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/agent"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/data"
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
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 每个连接独立的 session ID
	sessionID := uuid.New().String()[:8]
	log.Printf("新会话: %s", sessionID)

	// 初始化数据导入引擎
	uploadDir := "./uploads"
	ingester := data.NewIngester("./data/cache")
	if err := ingester.InitDB(sessionID); err != nil {
		log.Printf("数据库初始化失败: %v", err)
		return
	}

	// 研报章节存储
	var reportSections []tools.ReportSection

	// 创建工具注册表
	registry := tools.NewRegistry()
	registry.Register(&tools.LoadDataTool{Ingester: ingester, UploadDir: uploadDir})
	registry.Register(&tools.ListTablesTool{Ingester: ingester})
	registry.Register(&tools.DescribeDataTool{Ingester: ingester})
	registry.Register(&tools.QueryDataTool{Ingester: ingester})
	registry.Register(&tools.WriteSectionTool{ReportSections: &reportSections})
	registry.Register(&tools.FinalizeReportTool{
		ReportSections: &reportSections,
		OnReport: func(html string) {
			sendEvent(conn, &writeMu, agent.WSEvent{
				Type: agent.EventReportFinal,
				Data: agent.ReportUpdateData{HTML: html},
			})
		},
	})

	// 事件发射器
	emitter := func(event agent.WSEvent) {
		sendEvent(conn, &writeMu, event)

		// 如果是 write_section 的结果，推送研报增量更新
		if event.Type == agent.EventToolResult {
			if result, ok := event.Data.(agent.ToolResultData); ok && result.Name == "write_section" {
				partialHTML := generatePartialReport(reportSections)
				sendEvent(conn, &writeMu, agent.WSEvent{
					Type: agent.EventReportUpdate,
					Data: agent.ReportUpdateData{HTML: partialHTML},
				})
			}
		}
	}

	// 读取消息循环
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("会话 %s: WebSocket 连接关闭", sessionID)
			} else {
				log.Printf("会话 %s: 读取消息失败: %v", sessionID, err)
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
			json.Unmarshal(dataBytes, &userMsg)

			// 构建用户指令（含文件上下文）
			userContent := userMsg.Content
			if len(userMsg.Files) > 0 {
				fileList := strings.Join(userMsg.Files, ", ")
				userContent = fmt.Sprintf("用户已上传以下数据文件: [%s]\n\n用户指令: %s", fileList, userMsg.Content)
			}

			// 在 goroutine 中运行 Agent
			go func() {
				engine := agent.NewEngine(registry, emitter)
				engine.Run(ctx, userContent)
			}()

		case agent.EventStop:
			cancel()
			ctx, cancel = context.WithCancel(r.Context())

		default:
			log.Printf("未知事件类型: %s", event.Type)
		}
	}
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

func generatePartialReport(sections []tools.ReportSection) string {
	title := "数据分析报告"
	for _, s := range sections {
		if s.Type == "title" {
			title = s.Title
			break
		}
	}

	var html string
	html = "<html><body style='font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 2rem;'>"
	html += "<h1 style='color: #1a365d;'>" + title + "</h1><hr>"
	for _, sec := range sections {
		if sec.Type == "title" {
			continue
		}
		html += "<h2 style='color: #2a4a7f; margin-top: 2rem;'>" + sec.Title + "</h2>"
		html += "<div style='line-height: 1.8;'>" + sec.Content + "</div>"
	}
	html += "</body></html>"
	return html
}
