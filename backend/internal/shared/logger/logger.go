// Package logger là wrapper mỏng quanh zerolog — toàn hệ thống chỉ log qua đây,
// KHÔNG dùng log.Println / fmt.Print. Hỗ trợ gắn trace_id vào context.
package logger

import (
	"context"
	"io"
	"os"

	"github.com/rs/zerolog"
)

type ctxKey int

const traceIDKey ctxKey = iota

// Logger bọc zerolog.Logger, expose API tối thiểu cần dùng.
type Logger struct {
	zl zerolog.Logger
}

// New tạo Logger ghi ra w với log level cho trước. level rỗng/không hợp lệ → info.
func New(w io.Writer, level string) *Logger {
	if w == nil {
		w = os.Stdout
	}
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || level == "" {
		lvl = zerolog.InfoLevel
	}
	zl := zerolog.New(w).Level(lvl).With().Timestamp().Logger()
	return &Logger{zl: zl}
}

// ContextWithTraceID gắn trace_id vào context để các tầng dưới lấy ra khi log.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext lấy trace_id đã gắn; trả về "" nếu chưa có.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}
	return ""
}

// withCtx thêm trace_id (nếu có) vào một event.
func (l *Logger) withCtx(ctx context.Context, e *zerolog.Event) *zerolog.Event {
	if tid := TraceIDFromContext(ctx); tid != "" {
		e = e.Str("trace_id", tid)
	}
	return e
}

// Info log một sự kiện mức info kèm trace_id từ context.
func (l *Logger) Info(ctx context.Context, msg string) {
	l.withCtx(ctx, l.zl.Info()).Msg(msg)
}

// Error log một lỗi mức error kèm trace_id và error context.
func (l *Logger) Error(ctx context.Context, err error, msg string) {
	l.withCtx(ctx, l.zl.Error()).Err(err).Msg(msg)
}

// Infof log info có structured field key=value.
func (l *Logger) Infof(ctx context.Context, msg, key, value string) {
	l.withCtx(ctx, l.zl.Info()).Str(key, value).Msg(msg)
}
