package agent

import "context"

type TraceMetadata struct {
	WorkspaceID string
	SessionID   string
	RunID       string
	TraceID     string
}

type traceContextKey string

const traceMetadataKey traceContextKey = "llm-trace-metadata"

func WithTraceMetadata(ctx context.Context, meta TraceMetadata) context.Context {
	if meta.TraceID == "" {
		if meta.RunID != "" {
			meta.TraceID = meta.RunID
		} else {
			meta.TraceID = "trace"
		}
	}
	return context.WithValue(ctx, traceMetadataKey, meta)
}

func TraceMetadataFromContext(ctx context.Context) TraceMetadata {
	meta, _ := ctx.Value(traceMetadataKey).(TraceMetadata)
	return meta
}
