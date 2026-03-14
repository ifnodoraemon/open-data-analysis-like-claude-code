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
		return nil, "", fmt.Errorf("报告 HTML 不能为空")
	}

	tempRoot := strings.TrimSpace(s.TempDir)
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return nil, "", fmt.Errorf("创建导出临时目录失败: %w", err)
	}

	workdir, err := os.MkdirTemp(tempRoot, "report-export-*")
	if err != nil {
		return nil, "", fmt.Errorf("创建导出工作目录失败: %w", err)
	}
	defer os.RemoveAll(workdir)

	inputPath := filepath.Join(workdir, "report.html")
	outputPath := filepath.Join(workdir, "report.docx")
	if err := os.WriteFile(inputPath, []byte(html), 0o644); err != nil {
		return nil, "", fmt.Errorf("写入临时 HTML 失败: %w", err)
	}

	cmd := exec.CommandContext(ctx, "pandoc", inputPath, "--from=html", "--to=docx", "--output", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, "", fmt.Errorf("pandoc 转换 DOCX 失败: %s", message)
	}

	body, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, "", fmt.Errorf("读取 DOCX 输出失败: %w", err)
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
