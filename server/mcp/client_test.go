package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func TestMCPSyncedToolSetExecutionContext(t *testing.T) {
	t.Parallel()

	tool := &MCPSyncedTool{}
	ctx, cancel := context.WithCancel(context.Background())

	tool.SetExecutionContext(ctx)
	if tool.parentCtx != ctx {
		t.Fatal("SetExecutionContext did not store context")
	}

	cancel()
	if tool.parentCtx.Err() == nil {
		t.Fatal("expected cancelled context")
	}
}

func TestMCPSyncedToolExecuteUsesParentContext(t *testing.T) {
	t.Parallel()

	parentCtx, parentCancel := context.WithCancel(context.Background())
	parentCancel()

	client := NewClient()
	client.RegisterServer("test", "http://127.0.0.1:0", "")
	tool := &MCPSyncedTool{
		Schema:     ToolSchema{Name: "test_tool"},
		ServerName: "test",
		Client:     client,
	}
	tool.SetExecutionContext(parentCtx)

	_, err := tool.Execute(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestMCPSyncedToolExecuteFallsBackToBackground(t *testing.T) {
	t.Parallel()

	client := NewClient()
	tool := &MCPSyncedTool{
		Schema:     ToolSchema{Name: "test_tool"},
		ServerName: "nonexistent",
		Client:     client,
	}

	_, err := tool.Execute(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error (no server registered)")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Logf("got error: %v", err)
	}
}

func TestClientRegisterAndRemoveServer(t *testing.T) {
	t.Parallel()

	client := NewClient()
	client.RegisterServer("test", "http://localhost:8088", "token123")

	if len(client.configs) != 1 {
		t.Fatalf("expected 1 server, got %d", len(client.configs))
	}

	client.RemoveServer("test")
	if len(client.configs) != 0 {
		t.Fatalf("expected 0 servers after remove, got %d", len(client.configs))
	}
}

func TestClientListServers(t *testing.T) {
	t.Parallel()

	client := NewClient()
	client.RegisterServer("a", "http://a:8088", "")
	client.RegisterServer("b", "http://b:8088", "")

	servers := client.ListServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
}

func TestClientConcurrency(t *testing.T) {
	t.Parallel()

	client := NewClient()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			client.RegisterServer("srv"+string(rune('A'+n%26)), "http://localhost:8088", "")
		}(i)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.ListServers()
		}()
	}
	wg.Wait()
}

type testTool struct{ name string }

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string { return "test" }
func (t *testTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *testTool) Execute(args json.RawMessage) (string, error) {
	return "ok", nil
}

func TestRegistrySyncRemoveOrphaned(t *testing.T) {
	t.Parallel()

	client := NewClient()
	sync := &RegistrySync{
		Client: client,
		Target: tools.NewRegistry(),
		synced: make(map[string]struct{}),
	}

	sync.Target.Register(&testTool{name: "keep_me"})
	sync.synced["keep_me"] = struct{}{}
	sync.synced["orphan"] = struct{}{}

	removed := sync.RemoveOrphaned(context.Background())
	if removed != 1 {
		t.Fatalf("expected 1 orphan removed, got %d", removed)
	}
	if _, ok := sync.synced["orphan"]; ok {
		t.Fatal("orphan should be removed from synced map")
	}
	if _, ok := sync.synced["keep_me"]; !ok {
		t.Fatal("keep_me should still be in synced map")
	}
}

func TestRegistrySyncCachedToolsForWithMutex(t *testing.T) {
	t.Parallel()

	client := NewClient()
	client.RegisterServer("test", "http://127.0.0.1:0", "")
	rsync := NewRegistrySync(client, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rsync.cachedToolsFor("nonexistent")
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		client.RegisterServer("concurrent", "http://127.0.0.1:0", "")
	}()
	wg.Wait()
}

func TestClientDiscoverToolsEmpty(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	tools, err := client.DiscoverTools(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools from empty registry, got %d", len(tools))
	}
}
