// logger.go — wrapper mỏng quanh zerolog cho demo-order service.
// Hệ thống chỉ log qua đây, KHÔNG dùng log.Println / fmt.Print.
package main

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// serviceLogger bọc zerolog, thêm field cố định service="demo-order".
type serviceLogger struct {
	zl zerolog.Logger
}

// newLogger tạo serviceLogger ghi ra w với level cho trước.
// level rỗng/không hợp lệ → info.
func newLogger(w io.Writer, level string) *serviceLogger {
	if w == nil {
		w = os.Stdout
	}
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || level == "" {
		lvl = zerolog.InfoLevel
	}
	// Timestamp ISO8601 UTC theo spec.
	zerolog.TimeFieldFormat = time.RFC3339
	zl := zerolog.New(w).
		Level(lvl).
		With().
		Timestamp().
		Str("service", "demo-order").
		Logger()
	return &serviceLogger{zl: zl}
}

// Info log message mức info, nhận thêm key-value pairs tuỳ chọn.
func (l *serviceLogger) Info(msg string, kvs ...string) {
	e := l.zl.Info()
	for i := 0; i+1 < len(kvs); i += 2 {
		e = e.Str(kvs[i], kvs[i+1])
	}
	e.Msg(msg)
}

// Error log lỗi mức error kèm err context.
func (l *serviceLogger) Error(msg string, err error, kvs ...string) {
	e := l.zl.Error().Err(err)
	for i := 0; i+1 < len(kvs); i += 2 {
		e = e.Str(kvs[i], kvs[i+1])
	}
	e.Msg(msg)
}

// Request log một HTTP request đã hoàn tất. KHÔNG log body.
func (l *serviceLogger) Request(method, path string, status, durationMS int) {
	l.zl.Info().
		Str("method", method).
		Str("path", path).
		Int("status", status).
		Int("duration_ms", durationMS).
		Msg("request")
}
