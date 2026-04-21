package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerDeleteRemovesCacheFilesForUnloadedSession(t *testing.T) {
	t.Parallel()

	cacheRoot := t.TempDir()
	sessionID := "s_cache"
	for _, suffix := range []string{".db", ".db-wal", ".db-shm"} {
		path := filepath.Join(cacheRoot, sessionID+suffix)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write cache file %s: %v", path, err)
		}
	}

	manager := NewManager(cacheRoot, nil, nil)
	if err := manager.Delete(sessionID, "w_1", "u_1"); err != nil {
		t.Fatalf("delete session cache: %v", err)
	}

	for _, suffix := range []string{".db", ".db-wal", ".db-shm"} {
		path := filepath.Join(cacheRoot, sessionID+suffix)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", path, err)
		}
	}
}

func TestManagerStopWaitsForActiveRunToFinish(t *testing.T) {
	t.Parallel()

	sess := &Session{
		ID:          "s_live",
		WorkspaceID: "w_1",
		UserID:      "u_1",
	}
	sess.ActiveRun = &RunState{
		RunID:  "r_1",
		Status: "running",
		Cancel: func() {
			go func() {
				time.Sleep(50 * time.Millisecond)
				sess.FinishRun("r_1", "cancelled")
			}()
		},
		StartedAt: time.Now(),
	}

	manager := NewManager(t.TempDir(), nil, nil)
	manager.sessions[sess.ID] = sess

	if err := manager.Stop(sess.ID, sess.WorkspaceID, sess.UserID); err != nil {
		t.Fatalf("stop session: %v", err)
	}
	if !sess.WaitUntilIdle(10 * time.Millisecond) {
		t.Fatal("expected session to become idle after stop")
	}
}
