package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordUsesBcrypt(t *testing.T) {
	t.Parallel()

	encoded, err := HashPassword("admin@123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !strings.HasPrefix(encoded, "$2") {
		t.Fatalf("expected bcrypt hash, got %q", encoded)
	}
	if !VerifyPassword("admin@123", encoded) {
		t.Fatal("expected password verification to succeed")
	}
	if VerifyPassword("wrong", encoded) {
		t.Fatal("expected wrong password to fail")
	}
}
