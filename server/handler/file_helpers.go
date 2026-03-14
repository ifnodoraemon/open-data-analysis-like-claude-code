package handler

import "github.com/ifnodoraemon/openDataAnalysis/domain"

func serializeFile(file domain.File) map[string]interface{} {
	return map[string]interface{}{
		"fileId":   file.ID,
		"name":     file.DisplayName,
		"purpose":  file.Purpose,
		"size":     file.SizeBytes,
		"status":   file.Status,
		"mimeType": file.ContentType,
	}
}
