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
	return strings.Contains(text, "does not exist") || strings.Contains(text, "not found")
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
	http.Error(w, "internal server error", http.StatusInternalServerError)
	return true
}
