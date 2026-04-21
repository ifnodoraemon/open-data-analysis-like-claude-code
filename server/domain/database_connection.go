package domain

import "time"

type DatabaseConnection struct {
	SourceID        string
	Driver          string
	Host            string
	Port            int
	DatabaseName    string
	DefaultSchema   string
	SSLMode         string
	Username        string
	SecretCiphertext []byte
	AllowlistJSON   string
	LastTestedAt    *time.Time
	LastTestStatus  string
	LastErrorMessage *string
}
