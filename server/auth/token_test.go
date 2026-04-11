package auth

import (
	"testing"
	"time"
)

func TestTokenManager_SignAndParse(t *testing.T) {
	tm := NewTokenManager("test-secret-key")
	identity := Identity{
		UserID:      "user1",
		UserName:    "Test User",
		UserEmail:   "test@example.com",
		WorkspaceID: "ws1",
		Workspace:   "Default",
	}

	token, err := tm.Sign(identity, 1*time.Hour)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	parsed, err := tm.Parse(token)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if parsed.UserID != identity.UserID {
		t.Errorf("UserID mismatch: got %q, want %q", parsed.UserID, identity.UserID)
	}
	if parsed.WorkspaceID != identity.WorkspaceID {
		t.Errorf("WorkspaceID mismatch: got %q, want %q", parsed.WorkspaceID, identity.WorkspaceID)
	}
}

func TestTokenManager_ExpiredToken(t *testing.T) {
	tm := NewTokenManager("test-secret-key")
	identity := Identity{UserID: "user1", UserName: "Test", UserEmail: "t@e.com", WorkspaceID: "ws1", Workspace: "W"}

	token, err := tm.Sign(identity, -1*time.Second)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	_, err = tm.Parse(token)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestTokenManager_InvalidSignature(t *testing.T) {
	tm1 := NewTokenManager("secret-1")
	tm2 := NewTokenManager("secret-2")
	identity := Identity{UserID: "user1", UserName: "Test", UserEmail: "t@e.com", WorkspaceID: "ws1", Workspace: "W"}

	token, _ := tm1.Sign(identity, 1*time.Hour)
	_, err := tm2.Parse(token)
	if err == nil {
		t.Error("expected error for invalid signature, got nil")
	}
}

func TestTokenManager_MalformedToken(t *testing.T) {
	tm := NewTokenManager("secret")

	_, err := tm.Parse("not.a.valid-token-format")
	if err == nil {
		t.Error("expected error for malformed token, got nil")
	}

	_, err = tm.Parse("no-dot")
	if err == nil {
		t.Error("expected error for token without dots, got nil")
	}
}
