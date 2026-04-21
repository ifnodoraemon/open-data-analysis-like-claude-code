package handler

import "github.com/ifnodoraemon/openDataAnalysis/session"

var sessionManager *session.Manager

func StopSessionCleanup() {
	if sessionManager != nil {
		sessionManager.StopPeriodicCleanup()
	}
}
