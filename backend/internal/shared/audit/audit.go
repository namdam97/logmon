// Package audit ghi nhật ký audit immutable cho các thao tác nhạy cảm (member
// changes, workspace create, mode switch...). Bảng audit_logs KHÔNG UPDATE/DELETE
// — doc_v2/09 §8. Recorder là interface để handler/app inject (ISP).
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry mô tả một sự kiện audit. WorkspaceID/UserID/IPAddress có thể rỗng.
type Entry struct {
	WorkspaceID  string
	UserID       string
	Action       string // 'member.add', 'member.update', 'workspace.create'...
	ResourceType string
	ResourceID   string
	Details      map[string]any
	IPAddress    string
}

// Recorder ghi một entry audit. Lỗi ghi audit KHÔNG nên chặn nghiệp vụ chính —
// caller log lỗi nhưng vẫn tiếp tục (best-effort).
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// PostgresRecorder ghi audit vào bảng audit_logs qua pgx (parameterized).
type PostgresRecorder struct {
	pool *pgxpool.Pool
}

// Verify compliance tại compile time.
var _ Recorder = (*PostgresRecorder)(nil)

// NewPostgresRecorder tạo recorder với pool đã khởi tạo.
func NewPostgresRecorder(pool *pgxpool.Pool) *PostgresRecorder {
	return &PostgresRecorder{pool: pool}
}

// Record chèn một dòng audit. Cột nullable (workspace_id UUID, user_id, ip_address
// INET) nhận NULL khi rỗng để tránh lỗi ép kiểu.
func (r *PostgresRecorder) Record(ctx context.Context, e Entry) error {
	var details []byte
	if e.Details != nil {
		b, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("marshal audit details: %w", err)
		}
		details = b
	}
	const q = `INSERT INTO audit_logs
	           (workspace_id, user_id, action, resource_type, resource_id, details, ip_address)
	           VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, q,
		nullStr(e.WorkspaceID), nullStr(e.UserID), e.Action, e.ResourceType, e.ResourceID,
		details, nullStr(e.IPAddress))
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

// nullStr trả về nil cho chuỗi rỗng (→ SQL NULL), ngược lại trả con trỏ chuỗi.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NopRecorder bỏ qua mọi entry — dùng khi audit chưa cấu hình hoặc trong test.
type NopRecorder struct{}

// Record không làm gì.
func (NopRecorder) Record(context.Context, Entry) error { return nil }
