// Package ports khai báo interfaces tầng app của alerting BC phụ thuộc (DIP).
// Implementation ở adapters. Transaction dùng pattern tx-in-context: TxManager
// mở tx và gắn vào ctx; RuleRepository + EventPublisher đọc tx từ ctx → rule
// INSERT và outbox INSERT nằm chung một TX (transactional outbox).
package ports

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

// TxManager chạy fn trong một transaction (tx mang trong ctx ở adapter).
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// RuleRepository ghi alert rule. Save chạy trong tx của ctx (gọi bên trong
// TxManager.WithinTx). ExistsByName kiểm trùng tên trong workspace.
type RuleRepository interface {
	Save(ctx context.Context, r domain.AlertRule) error
	ExistsByName(ctx context.Context, workspaceID, name string) (bool, error)
}

// RuleReader là read side (CQRS) — truy vấn rule, có thể tối ưu riêng.
type RuleReader interface {
	ByID(ctx context.Context, id domain.RuleID) (domain.AlertRule, error)
	List(ctx context.Context, workspaceID string) ([]domain.AlertRule, error)
}

// EventPublisher ghi domain event vào outbox (trong tx của ctx). Trừu tượng hoá
// outbox khỏi alerting app.
type EventPublisher interface {
	Publish(ctx context.Context, aggregateType, aggregateID, eventType string, payload any) error
}

// RuleValidator validate cú pháp biểu thức PromQL (adapter dùng prometheus parser).
type RuleValidator interface {
	ValidateExpression(expr string) error
}

// IDGenerator sinh định danh rule (UUID).
type IDGenerator interface {
	NewID() string
}

// Clock cung cấp thời gian hiện tại — inject để test xác định.
type Clock interface {
	Now() time.Time
}
