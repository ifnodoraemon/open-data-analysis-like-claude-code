package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type sessionDeletionPlan struct {
	sourceFileIDs []string
	reportFileIDs []string
	storageKeys   []string
}

func deleteSessionResources(ctx context.Context, session domain.Session) error {
	if metadataStore == nil || metadataStore.DB == nil {
		return fmt.Errorf("metadata store is not initialized")
	}

	plan, err := buildSessionDeletionPlan(ctx, metadataStore.DB, session.ID)
	if err != nil {
		return err
	}

	if sessionManager != nil {
		if err := sessionManager.Stop(session.ID, session.WorkspaceID, session.UserID); err != nil {
			return err
		}
	}

	tx, err := metadataStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM run_messages WHERE session_id = ?`, session.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reports WHERE run_id IN (SELECT id FROM analysis_runs WHERE session_id = ?)`, session.ID); err != nil {
		return err
	}
	if len(plan.reportFileIDs) > 0 {
		if err := deleteFilesByIDs(ctx, tx, plan.reportFileIDs); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analysis_runs WHERE session_id = ?`, session.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_files WHERE session_id = ?`, session.ID); err != nil {
		return err
	}
	if len(plan.sourceFileIDs) > 0 {
		if err := deleteFilesByIDs(ctx, tx, plan.sourceFileIDs); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, session.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	CloseSessionWebSockets(session.ID)
	if sessionManager != nil {
		if err := sessionManager.Delete(session.ID, session.WorkspaceID, session.UserID); err != nil {
			log.Printf("delete in-memory session failed session_id=%s err=%v", session.ID, err)
		}
	}

	for _, key := range plan.storageKeys {
		if strings.TrimSpace(key) == "" || fileService == nil || fileService.Storage == nil {
			continue
		}
		if err := fileService.Storage.Delete(ctx, key); err != nil {
			log.Printf("delete session storage object failed after metadata removal session_id=%s key=%s err=%v", session.ID, key, err)
		}
	}
	return nil
}

func buildSessionDeletionPlan(ctx context.Context, db *sql.DB, sessionID string) (sessionDeletionPlan, error) {
	sourceFileIDs, err := queryStrings(ctx, db, `
		SELECT DISTINCT sf1.file_id
		FROM session_files sf1
		JOIN files f ON f.id = sf1.file_id
		WHERE sf1.session_id = ?
		  AND f.visibility = 'private'
		  AND NOT EXISTS (
		      SELECT 1 FROM session_files sf2 
		      WHERE sf2.file_id = sf1.file_id AND sf2.session_id != ?
		  )
	`, sessionID, sessionID)
	if err != nil {
		return sessionDeletionPlan{}, err
	}
	reportFileIDs, err := queryStrings(ctx, db, `
		SELECT DISTINCT report_file_id
		FROM analysis_runs
		WHERE session_id = ? AND report_file_id IS NOT NULL AND report_file_id != ''
	`, sessionID)
	if err != nil {
		return sessionDeletionPlan{}, err
	}
	storageKeys, err := queryStrings(ctx, db, `
		SELECT DISTINCT storage_key
		FROM files
		WHERE id IN (
			SELECT sf1.file_id FROM session_files sf1 
			JOIN files f ON f.id = sf1.file_id
			WHERE sf1.session_id = ? 
			  AND f.visibility = 'private'
			  AND NOT EXISTS (
			      SELECT 1 FROM session_files sf2 
			      WHERE sf2.file_id = sf1.file_id AND sf2.session_id != ?
			  )
			UNION
			SELECT report_file_id FROM analysis_runs WHERE session_id = ? AND report_file_id IS NOT NULL AND report_file_id != ''
		)
	`, sessionID, sessionID, sessionID)
	if err != nil {
		return sessionDeletionPlan{}, err
	}
	reportHTMLKeys, err := queryStrings(ctx, db, `
		SELECT DISTINCT html_storage_key
		FROM reports
		WHERE run_id IN (SELECT id FROM analysis_runs WHERE session_id = ?)
	`, sessionID)
	if err != nil {
		return sessionDeletionPlan{}, err
	}
	return sessionDeletionPlan{
		sourceFileIDs: sourceFileIDs,
		reportFileIDs: reportFileIDs,
		storageKeys:   uniqueStrings(append(storageKeys, reportHTMLKeys...)),
	}, nil
}

func queryStrings(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0, 8)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return uniqueStrings(items), rows.Err()
}

func deleteFilesByIDs(ctx context.Context, tx *sql.Tx, ids []string) error {
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id IN (`+placeholders+`)`, args...)
	return err
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	unique := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		unique = append(unique, item)
	}
	return unique
}
