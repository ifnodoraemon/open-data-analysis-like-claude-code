package agent

import "context"

type ChildRunStart struct {
	ParentRunID  string
	RoleName     string
	InputMessage string
	GoalID       string
	AllowedTools []string
}

type DelegateRunPersistence interface {
	StartChildRun(ctx context.Context, input ChildRunStart) (string, error)
	AppendChildEvent(ctx context.Context, childRunID string, ev WSEvent) error
	UpdateChildRunStatus(ctx context.Context, childRunID string, status string, errMsg *string) error
	UpdateChildRunSummary(ctx context.Context, childRunID, summary string) error
}

type delegatePersistenceKey struct{}

func WithDelegateRunPersistence(ctx context.Context, persistence DelegateRunPersistence) context.Context {
	if persistence == nil {
		return ctx
	}
	return context.WithValue(ctx, delegatePersistenceKey{}, persistence)
}

func DelegateRunPersistenceFromContext(ctx context.Context) DelegateRunPersistence {
	persistence, _ := ctx.Value(delegatePersistenceKey{}).(DelegateRunPersistence)
	return persistence
}
