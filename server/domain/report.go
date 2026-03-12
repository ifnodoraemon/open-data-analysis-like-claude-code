package domain

import (
	"encoding/json"
	"time"
)

type Report struct {
	ID                  string
	RunID               string
	WorkspaceID         string
	Title               string
	Author              string
	HTMLStorageProvider string
	HTMLBucket          string
	HTMLStorageKey      string
	SnapshotJSON        string
	CreatedAt           time.Time
}

type ReportSnapshot struct {
	Version     string                  `json:"version"`
	GeneratedAt time.Time               `json:"generatedAt"`
	Title       string                  `json:"title"`
	Author      string                  `json:"author,omitempty"`
	Sections    []ReportSnapshotSection `json:"sections"`
	Charts      []ReportSnapshotChart   `json:"charts"`
}

type ReportSnapshotSection struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type ReportSnapshotChart struct {
	ID     string          `json:"id"`
	Option json.RawMessage `json:"option"`
	Width  string          `json:"width,omitempty"`
	Height string          `json:"height,omitempty"`
}
