package handler

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/metadata"
	sqliterepo "github.com/ifnodoraemon/openDataAnalysis/repository/sqlite"
	"github.com/ifnodoraemon/openDataAnalysis/session"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func TestAttachRunRuntimeStateUsesSessionScopedState(t *testing.T) {
	ctx := context.Background()
	store, err := metadata.Open(t.TempDir() + "/metadata.db")
	if err != nil {
		t.Fatalf("open metadata: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DB.Close()
	})

	prevRunRepo := runRepo
	prevMessageRepo := messageRepo
	prevSessionManager := sessionManager
	t.Cleanup(func() {
		runRepo = prevRunRepo
		messageRepo = prevMessageRepo
		sessionManager = prevSessionManager
	})

	userRepo := sqliterepo.NewUserRepository(store.DB)
	workspaceRepo := sqliterepo.NewWorkspaceRepository(store.DB)
	sessionRepo := sqliterepo.NewSessionRepository(store.DB)
	runRepo = sqliterepo.NewRunRepository(store.DB)
	messageRepo = sqliterepo.NewMessageRepository(store.DB)
	sessionManager = nil

	now := time.Now()
	rootID := "run_root"
	childID := "run_child"

	if err := userRepo.Create(ctx, &domain.User{
		ID:           "user_1",
		Email:        "user@example.com",
		PasswordHash: "hash",
		Name:         "User",
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := workspaceRepo.CreateWorkspace(ctx, &domain.Workspace{
		ID:          "ws_1",
		Name:        "Workspace",
		Slug:        "workspace",
		OwnerUserID: "user_1",
		Status:      domain.WorkspaceStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if err := workspaceRepo.AddMember(ctx, &domain.WorkspaceMember{
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		Role:        domain.WorkspaceRoleOwner,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("add workspace member: %v", err)
	}

	if err := sessionRepo.Create(ctx, &domain.Session{
		ID:          "sess_1",
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		Title:       "Session",
		Status:      domain.SessionStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastSeenAt:  now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := runRepo.Create(ctx, &domain.AnalysisRun{
		ID:           rootID,
		SessionID:    "sess_1",
		WorkspaceID:  "ws_1",
		UserID:       "user_1",
		RunKind:      domain.RunKindRoot,
		Status:       domain.RunStatusCompleted,
		InputMessage: "root",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create root run: %v", err)
	}

	if err := runRepo.Create(ctx, &domain.AnalysisRun{
		ID:           childID,
		SessionID:    "sess_1",
		WorkspaceID:  "ws_1",
		UserID:       "user_1",
		ParentRunID:  &rootID,
		RunKind:      domain.RunKindDelegate,
		Status:       domain.RunStatusCompleted,
		InputMessage: "child",
		CreatedAt:    now.Add(time.Second),
		UpdatedAt:    now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create child run: %v", err)
	}

	callRoot := "call_root_memory"
	callChild := "call_child_goal"
	success := true

	if err := messageRepo.Create(ctx, &domain.RunMessage{
		ID:          "msg_1",
		RunID:       rootID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventToolCall),
		Name:        "memory_save_fact",
		ToolCallID:  &callRoot,
		Content:     `{"key":"confirmed_metric","fact":"GMV uses settled orders"}`,
		CreatedAt:   now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("create root tool call: %v", err)
	}

	if err := messageRepo.Create(ctx, &domain.RunMessage{
		ID:          "msg_2",
		RunID:       rootID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventToolResult),
		Name:        "memory_save_fact",
		ToolCallID:  &callRoot,
		Content:     `{"ok":true}`,
		Success:     &success,
		CreatedAt:   now.Add(3 * time.Second),
	}); err != nil {
		t.Fatalf("create root tool result: %v", err)
	}

	if err := messageRepo.Create(ctx, &domain.RunMessage{
		ID:          "msg_3",
		RunID:       childID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventToolCall),
		Name:        "goal_manage",
		ToolCallID:  &callChild,
		Content:     `{"action":"add","description":"Inspect revenue quality"}`,
		CreatedAt:   now.Add(4 * time.Second),
	}); err != nil {
		t.Fatalf("create child tool call: %v", err)
	}

	if err := messageRepo.Create(ctx, &domain.RunMessage{
		ID:          "msg_4",
		RunID:       childID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventToolResult),
		Name:        "goal_manage",
		ToolCallID:  &callChild,
		Content:     `{"ok":true,"goal_id":"goal_123"}`,
		Success:     &success,
		CreatedAt:   now.Add(5 * time.Second),
	}); err != nil {
		t.Fatalf("create child tool result: %v", err)
	}

	if err := messageRepo.Create(ctx, &domain.RunMessage{
		ID:          "msg_5",
		RunID:       childID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventReportUpdate),
		Content: `{
			"report_snapshot":{
				"title":"共享草稿",
				"needsFinalize":true,
				"blocks":[{"id":"blk_1","kind":"markdown","content":"共享草稿已持久化"}],
				"charts":[]
			}
		}`,
		CreatedAt: now.Add(6 * time.Second),
	}); err != nil {
		t.Fatalf("create report update: %v", err)
	}

	resp := map[string]interface{}{}
	attachRunRuntimeState(ctx, resp, domain.AnalysisRun{
		ID:          childID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		UserID:      "user_1",
	})

	runtimeState, ok := resp["runtimeState"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected runtimeState map, got %#v", resp["runtimeState"])
	}

	memory, ok := runtimeState["memory"].(map[string]string)
	if !ok {
		t.Fatalf("expected memory map, got %#v", runtimeState["memory"])
	}
	if memory["confirmed_metric"] != "GMV uses settled orders" {
		t.Fatalf("expected session-scoped memory fact from root run, got %#v", memory)
	}

	subgoals, ok := runtimeState["subgoals"].([]agent.Subgoal)
	if !ok {
		t.Fatalf("expected subgoals slice, got %#v", runtimeState["subgoals"])
	}
	if len(subgoals) != 1 || subgoals[0].ID != "goal_123" {
		t.Fatalf("expected child subgoal to be preserved in session runtime state, got %#v", subgoals)
	}

	reportHTML, ok := runtimeState["report_html"].(string)
	if !ok || reportHTML == "" {
		t.Fatalf("expected persisted report_html, got %#v", runtimeState["report_html"])
	}
	if !strings.Contains(reportHTML, "共享草稿已持久化") {
		t.Fatalf("expected rendered report_html to contain draft body, got %q", reportHTML)
	}
	reportSnapshot, ok := runtimeState["report_snapshot"].(*domain.ReportSnapshot)
	if !ok || reportSnapshot == nil {
		t.Fatalf("expected persisted report_snapshot, got %#v", runtimeState["report_snapshot"])
	}
	if !reportSnapshot.NeedsFinalize || len(reportSnapshot.Blocks) != 1 || reportSnapshot.Blocks[0].ID != "blk_1" {
		t.Fatalf("expected structured draft snapshot, got %#v", reportSnapshot)
	}
}

func TestHydrateSessionFromPersistenceRestoresStructuredReportState(t *testing.T) {
	ctx := context.Background()
	store, err := metadata.Open(t.TempDir() + "/metadata.db")
	if err != nil {
		t.Fatalf("open metadata: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DB.Close()
	})

	setupRuntimeStateRepos(t, ctx, store)

	now := time.Now()
	rootID := "run_report_root"
	success := true

	mustCreateRunMessage(t, ctx, &domain.AnalysisRun{
		ID:           rootID,
		SessionID:    "sess_1",
		WorkspaceID:  "ws_1",
		UserID:       "user_1",
		RunKind:      domain.RunKindRoot,
		Status:       domain.RunStatusCompleted,
		InputMessage: "draft",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	mustCreateRunMessage(t, ctx, &domain.RunMessage{
		ID:          "msg_report_call",
		RunID:       rootID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventToolCall),
		Name:        "memory_save_fact",
		ToolCallID:  strPtr("call_report_memory"),
		Content:     `{"key":"report_scope","fact":"draft report should survive restart"}`,
		CreatedAt:   now.Add(time.Second),
	})
	mustCreateRunMessage(t, ctx, &domain.RunMessage{
		ID:          "msg_report_result",
		RunID:       rootID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventToolResult),
		Name:        "memory_save_fact",
		ToolCallID:  strPtr("call_report_memory"),
		Content:     `{"ok":true}`,
		Success:     &success,
		CreatedAt:   now.Add(2 * time.Second),
	})
	mustCreateRunMessage(t, ctx, &domain.RunMessage{
		ID:          "msg_report_update",
		RunID:       rootID,
		SessionID:   "sess_1",
		WorkspaceID: "ws_1",
		Type:        string(agent.EventReportUpdate),
		Content: `{
			"html":"<p>draft body</p>",
			"report_snapshot":{
				"title":"恢复后的草稿",
				"author":"Analyst",
				"needsFinalize":true,
				"layout":{"customCss":"body { color: red; }"},
				"blocks":[{"id":"blk_1","kind":"markdown","title":"概览","content":"draft body"}],
				"charts":[{"id":"chart_1","option":{"series":[{"type":"bar","data":[1]}]}}]
			}
		}`,
		CreatedAt: now.Add(3 * time.Second),
	})

	sess := &session.Session{
		ID:          "sess_1",
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		ReportState: &tools.ReportState{},
		EditState:   &tools.ReportEditState{},
		Memory:      agent.NewWorkingMemory(),
		Subgoals:    agent.NewSubgoalManager(),
	}

	if err := hydrateSessionFromPersistence(ctx, sess); err != nil {
		t.Fatalf("hydrate session: %v", err)
	}

	if got := sess.Memory.Snapshot()["report_scope"]; got != "draft report should survive restart" {
		t.Fatalf("expected working memory to be restored, got %q", got)
	}
	if sess.ReportState.FinalTitle != "恢复后的草稿" || sess.ReportState.FinalAuthor != "Analyst" {
		t.Fatalf("expected report metadata to be restored, got %#v", sess.ReportState)
	}
	if !sess.ReportState.NeedsFinalize {
		t.Fatalf("expected draft report to remain draft after hydrate, got %#v", sess.ReportState)
	}
	if len(sess.ReportState.Blocks) != 1 || sess.ReportState.Blocks[0].ID != "blk_1" {
		t.Fatalf("expected report blocks to be restored, got %#v", sess.ReportState.Blocks)
	}
	if len(sess.ReportState.Charts) != 1 || sess.ReportState.Charts[0].ID != "chart_1" {
		t.Fatalf("expected report charts to be restored, got %#v", sess.ReportState.Charts)
	}
}

func TestLoadSessionRuntimeStateFromPersistenceDoesNotTruncateLongHistory(t *testing.T) {
	ctx := context.Background()
	store, err := metadata.Open(t.TempDir() + "/metadata.db")
	if err != nil {
		t.Fatalf("open metadata: %v", err)
	}
	t.Cleanup(func() {
		_ = store.DB.Close()
	})

	setupRuntimeStateRepos(t, ctx, store)

	now := time.Now()
	success := true
	oldestRunID := ""
	for i := 0; i < 1001; i++ {
		runID := "run_bulk_" + strconv.Itoa(i)
		createdAt := now.Add(time.Duration(i) * time.Second)
		mustCreateRunMessage(t, ctx, &domain.AnalysisRun{
			ID:           runID,
			SessionID:    "sess_1",
			WorkspaceID:  "ws_1",
			UserID:       "user_1",
			RunKind:      domain.RunKindRoot,
			Status:       domain.RunStatusCompleted,
			InputMessage: runID,
			CreatedAt:    createdAt,
			UpdatedAt:    createdAt,
		})
		if i == 0 {
			oldestRunID = runID
			mustCreateRunMessage(t, ctx, &domain.RunMessage{
				ID:          "msg_old_call",
				RunID:       runID,
				SessionID:   "sess_1",
				WorkspaceID: "ws_1",
				Type:        string(agent.EventToolCall),
				Name:        "memory_save_fact",
				ToolCallID:  strPtr("call_old_fact"),
				Content:     `{"key":"oldest_fact","fact":"must survive beyond 1000 roots"}`,
				CreatedAt:   createdAt.Add(100 * time.Millisecond),
			})
			mustCreateRunMessage(t, ctx, &domain.RunMessage{
				ID:          "msg_old_result",
				RunID:       runID,
				SessionID:   "sess_1",
				WorkspaceID: "ws_1",
				Type:        string(agent.EventToolResult),
				Name:        "memory_save_fact",
				ToolCallID:  strPtr("call_old_fact"),
				Content:     `{"ok":true}`,
				Success:     &success,
				CreatedAt:   createdAt.Add(200 * time.Millisecond),
			})
		}
	}

	memory, subgoals, reportSnapshot, reportHTML := loadSessionRuntimeStateFromPersistence(ctx, "sess_1")
	if memory["oldest_fact"] != "must survive beyond 1000 roots" {
		t.Fatalf("expected oldest root fact from %s to survive unlimited replay, got %#v", oldestRunID, memory)
	}
	if len(subgoals) != 0 {
		t.Fatalf("expected no subgoals in bulk replay test, got %#v", subgoals)
	}
	if reportSnapshot != nil || reportHTML != "" {
		t.Fatalf("expected no report state in bulk replay test, got snapshot=%#v html=%q", reportSnapshot, reportHTML)
	}
}

func setupRuntimeStateRepos(t *testing.T, ctx context.Context, store *metadata.Store) {
	t.Helper()

	prevRunRepo := runRepo
	prevMessageRepo := messageRepo
	prevSessionRepo := sessionRepo
	prevSessionManager := sessionManager
	t.Cleanup(func() {
		runRepo = prevRunRepo
		messageRepo = prevMessageRepo
		sessionRepo = prevSessionRepo
		sessionManager = prevSessionManager
	})

	userRepo := sqliterepo.NewUserRepository(store.DB)
	workspaceRepo := sqliterepo.NewWorkspaceRepository(store.DB)
	sessionRepo = sqliterepo.NewSessionRepository(store.DB)
	runRepo = sqliterepo.NewRunRepository(store.DB)
	messageRepo = sqliterepo.NewMessageRepository(store.DB)
	sessionManager = nil

	now := time.Now()
	if err := userRepo.Create(ctx, &domain.User{
		ID:           "user_1",
		Email:        "user@example.com",
		PasswordHash: "hash",
		Name:         "User",
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := workspaceRepo.CreateWorkspace(ctx, &domain.Workspace{
		ID:          "ws_1",
		Name:        "Workspace",
		Slug:        "workspace",
		OwnerUserID: "user_1",
		Status:      domain.WorkspaceStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := workspaceRepo.AddMember(ctx, &domain.WorkspaceMember{
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		Role:        domain.WorkspaceRoleOwner,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("add workspace member: %v", err)
	}
	if err := sessionRepo.Create(ctx, &domain.Session{
		ID:          "sess_1",
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		Title:       "Session",
		Status:      domain.SessionStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastSeenAt:  now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func mustCreateRunMessage(t *testing.T, ctx context.Context, value interface{}) {
	t.Helper()
	switch item := value.(type) {
	case *domain.AnalysisRun:
		if err := runRepo.Create(ctx, item); err != nil {
			t.Fatalf("create run: %v", err)
		}
	case *domain.RunMessage:
		if err := messageRepo.Create(ctx, item); err != nil {
			t.Fatalf("create run message: %v", err)
		}
	default:
		t.Fatalf("unsupported test fixture type %T", value)
	}
}

func strPtr(v string) *string {
	return &v
}
