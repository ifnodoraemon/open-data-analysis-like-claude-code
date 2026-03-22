package session

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupExpiredSessions 清理超过 TTL 的空闲 session。
// 在真正删除前会在锁内二次验证状态，避免扫描与删除之间的竞态。
// 如果 Manager.FullDeleteFunc 已设置，则通过全链路路径清理（包含文件和存储对象）；
// 否则退化到 Session.Destroy + repo.Delete。
func (m *Manager) CleanupExpiredSessions(ttlHours int) int {
	if ttlHours <= 0 {
		return 0
	}

	cutoff := time.Now().Add(-time.Duration(ttlHours) * time.Hour)

	// 第一步：在锁内收集候选 ID
	m.mu.Lock()
	var candidates []string
	for id, sess := range m.sessions {
		sess.mu.Lock()
		isIdle := sess.ActiveRun == nil || (sess.ActiveRun.Status != "running" && sess.ActiveRun.Status != "waiting_user_input")
		lastSeen := sess.LastSeenAt
		sess.mu.Unlock()
		if isIdle && lastSeen.Before(cutoff) {
			candidates = append(candidates, id)
		}
	}
	m.mu.Unlock()

	cleaned := 0
	for _, id := range candidates {
		// 第二步：在锁内再次验证状态（防止扫描期间会话被激活）
		m.mu.Lock()
		sess, ok := m.sessions[id]
		if ok {
			sess.mu.Lock()
			isStillIdle := sess.ActiveRun == nil || (sess.ActiveRun.Status != "running" && sess.ActiveRun.Status != "waiting_user_input")
			stillExpired := sess.LastSeenAt.Before(cutoff)
			sess.mu.Unlock()
			if !isStillIdle || !stillExpired {
				// 状态已改变，跳过该会话
				m.mu.Unlock()
				continue
			}
			delete(m.sessions, id)
		}
		m.mu.Unlock()

		if !ok {
			continue
		}

		// 第三步：执行清理
		if m.FullDeleteFunc != nil {
			// 全链路删除：停止运行 + 删除文件记录 + 删除存储对象 + 删除数据库行
			if err := m.FullDeleteFunc(context.Background(), id); err != nil {
				log.Printf("cleanup session %s full-delete error: %v", id, err)
				continue
			}
		} else {
			// 退化路径：销毁本地 SQLite 缓存
			if err := sess.Destroy(); err != nil {
				log.Printf("cleanup session %s destroy error: %v", id, err)
				continue
			}
			if m.sessionRepo != nil {
				_ = m.sessionRepo.Delete(context.Background(), id)
			}
		}
		cleaned++
	}
	return cleaned
}

// CleanupOldTraces 清理超过保留天数的 LLM debug trace 目录
func CleanupOldTraces(traceDir string, retentionDays int) int {
	if retentionDays <= 0 || traceDir == "" {
		return 0
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(traceDir)
	if err != nil {
		return 0
	}

	cleaned := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// trace 目录名格式: YYYY-MM-DD
		name := entry.Name()
		if len(name) != 10 || !isDateDirName(name) {
			continue
		}
		parsed, err := time.Parse("2006-01-02", name)
		if err != nil {
			continue
		}
		if parsed.Before(cutoff) {
			fullPath := filepath.Join(traceDir, name)
			if err := os.RemoveAll(fullPath); err != nil {
				log.Printf("cleanup trace dir %s: %v", fullPath, err)
			} else {
				cleaned++
			}
		}
	}
	return cleaned
}

// CleanupTempDir 清理临时目录中的所有文件
func CleanupTempDir(tempDir string) error {
	if tempDir == "" {
		return nil
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(tempDir, entry.Name()))
	}
	return nil
}

func isDateDirName(name string) bool {
	// 快速检查 YYYY-MM-DD 格式
	for i, ch := range name {
		if i == 4 || i == 7 {
			if ch != '-' {
				return false
			}
		} else if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// StartPeriodicCleanup 启动后台清理协程
func (m *Manager) StartPeriodicCleanup(sessionTTLHours, traceRetentionDays int, traceDir, tempDir string, tempCleanupOnStart bool) {
	// 如果没有任何周期性任务需要跑，并且也没有启动时 temp 清理，直接返回
	periodicEnabled := sessionTTLHours > 0 || traceRetentionDays > 0 || tempDir != ""
	if !periodicEnabled && !tempCleanupOnStart {
		return
	}

	// 仅在明确配置了 TEMP_CLEANUP_ON_START 时才做启动时清理
	if tempCleanupOnStart && tempDir != "" {
		if entries, err := os.ReadDir(tempDir); err == nil && len(entries) > 0 {
			log.Printf("cleanup: clearing %d temp entries on start", len(entries))
			_ = CleanupTempDir(tempDir)
		}
	}

	if !periodicEnabled {
		return
	}

	go func() {
		// 每小时检查一次
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if n := m.CleanupExpiredSessions(sessionTTLHours); n > 0 {
				log.Printf("cleanup: removed %d expired sessions", n)
			}
			if n := CleanupOldTraces(traceDir, traceRetentionDays); n > 0 {
				log.Printf("cleanup: removed %d old trace directories", n)
			}
			if tempDir != "" {
				// 清理超过 4 小时未修改的 temp 文件
				cleanupStaleTemp(tempDir, 4*time.Hour)
			}
		}
	}()

	var parts []string
	if sessionTTLHours > 0 {
		parts = append(parts, "session_ttl="+itoa(sessionTTLHours)+"h")
	}
	if traceRetentionDays > 0 {
		parts = append(parts, "trace_retention="+itoa(traceRetentionDays)+"d")
	}
	log.Printf("cleanup: periodic cleanup started (%s)", strings.Join(parts, ", "))
}

func cleanupStaleTemp(tempDir string, maxAge time.Duration) {
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(tempDir, entry.Name()))
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
