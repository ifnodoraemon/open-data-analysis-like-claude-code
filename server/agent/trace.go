package agent

import "context"

type TraceMetadata struct {
	WorkspaceID string
	SessionID   string
	RunID       string
}

type traceContextKey string

const traceMetadataKey traceContextKey = "llm-trace-metadata"

func WithTraceMetadata(ctx context.Context, meta TraceMetadata) context.Context {
	return context.WithValue(ctx, traceMetadataKey, meta)
}

func TraceMetadataFromContext(ctx context.Context) TraceMetadata {
	meta, _ := ctx.Value(traceMetadataKey).(TraceMetadata)
	return meta
}
