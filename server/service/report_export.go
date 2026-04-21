package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (s *FileService) ConvertHTMLToDOCX(ctx context.Context, title, html string) ([]byte, string, error) {
	if strings.TrimSpace(html) == "" {
		return nil, "", fmt.Errorf("report HTML cannot be empty")
	}

	tempRoot := strings.TrimSpace(s.TempDir)
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return nil, "", fmt.Errorf("failed to create export temp directory: %w", err)
	}

	workdir, err := os.MkdirTemp(tempRoot, "report-export-*")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create export working directory: %w", err)
	}
	defer os.RemoveAll(workdir)

	inputPath := filepath.Join(workdir, "report.html")
	outputPath := filepath.Join(workdir, "report.docx")
	if err := os.WriteFile(inputPath, []byte(html), 0o644); err != nil {
		return nil, "", fmt.Errorf("failed to write temp HTML: %w", err)
	}

	cmd := exec.CommandContext(ctx, "pandoc", inputPath, "--from=html", "--to=docx", "--output", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, "", fmt.Errorf("pandoc DOCX conversion failed: %s", message)
	}

	body, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read DOCX output: %w", err)
	}

	filename := sanitizeFilename(strings.TrimSpace(title))
	if filename == "" || filename == "upload.bin" {
		filename = "report"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".docx" {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".docx"
	}

	return body, filename, nil
}
