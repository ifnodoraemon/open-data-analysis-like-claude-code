package handler

import (
	"database/sql"
	"errors"
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
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return true
}
