package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
)

func isRepoNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "不存在") || strings.Contains(text, "not found")
}

func writeRepoLookupError(w http.ResponseWriter, err error, notFoundMessage string) bool {
	if err == nil {
		return false
	}
	if isRepoNotFound(err) {
		http.Error(w, notFoundMessage, http.StatusNotFound)
		return true
	}
	log.Printf("internal repo error: %v", err)
	http.Error(w, "内部服务错误", http.StatusInternalServerError)
	return true
}
