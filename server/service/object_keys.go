package service

import (
	"fmt"
	"path/filepath"
)

func SourceFileKey(workspaceID, fileID, fileName string) string {
	return fmt.Sprintf("workspaces/%s/files/%s/source/%s", workspaceID, fileID, filepath.Base(fileName))
}

func ReportHTMLKey(workspaceID, runID string) string {
	return fmt.Sprintf("workspaces/%s/runs/%s/report/report.html", workspaceID, runID)
}

func ArtifactKey(workspaceID, runID, name string) string {
	return fmt.Sprintf("workspaces/%s/runs/%s/artifacts/%s", workspaceID, runID, filepath.Base(name))
}
