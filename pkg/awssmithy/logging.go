package awssmithy

import (
	"context"
	"fmt"
	"log/slog"

	smithylogging "github.com/aws/smithy-go/logging"
)

// SmithyLogger is an interface that combines [smithylogging.Logger] and [smithylogging.ContextLogger].
type SmithyLogger interface {
	smithylogging.Logger
	smithylogging.ContextLogger
}

type smithyLogger struct {
	ctx context.Context
}

var _ SmithyLogger = (*smithyLogger)(nil)

// AdaptLogger returns a new [SmithyLogger] that wraps the provided [slog.Logger].
func AdaptLogger(ctx context.Context) SmithyLogger {
	return &smithyLogger{
		ctx: ctx,
	}
}

// Logf implements [smithylogging.Logger].
func (l smithyLogger) Logf(classification smithylogging.Classification, format string, v ...any) {
	msg := fmt.Sprintf(format, v...)

	switch classification {
	case smithylogging.Warn:
		slog.WarnContext(l.ctx, msg)
	case smithylogging.Debug:
		slog.DebugContext(l.ctx, msg)
	}
}

// WithContext implements [smithylogging.ContextLogger].
func (l smithyLogger) WithContext(ctx context.Context) smithylogging.Logger {
	return &smithyLogger{
		ctx: ctx,
	}
}
