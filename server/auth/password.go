package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

func HashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func VerifyPassword(password, encoded string) bool {
	hashed := HashPassword(password)
	return subtle.ConstantTimeCompare([]byte(hashed), []byte(encoded)) == 1
}
