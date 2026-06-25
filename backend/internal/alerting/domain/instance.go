package domain

import "time"

// InstanceStatus là trạng thái vòng đời của một alert instance.
type InstanceStatus string

// Các trạng thái instance hợp lệ (ack ở GĐ2.4).
const (
	InstanceFiring       InstanceStatus = "firing"
	InstanceAcknowledged InstanceStatus = "acknowledged"
	InstanceResolved     InstanceStatus = "resolved"
)

// _maxFingerprintLen khớp VARCHAR(64) của cột fingerprint.
const _maxFingerprintLen = 64

// Fingerprint là khóa dedup ổn định của một alert (do Alertmanager sinh) — value
// object đảm bảo non-empty và ≤ 64 ký tự.
type Fingerprint struct{ value string }

// NewFingerprint validate và bọc một fingerprint string.
func NewFingerprint(raw string) (Fingerprint, error) {
	if raw == "" {
		return Fingerprint{}, newValidationError("fingerprint", "must not be empty")
	}
	if len(raw) > _maxFingerprintLen {
		return Fingerprint{}, newValidationError("fingerprint", "must be at most 64 characters")
	}
	return Fingerprint{raw}, nil
}

// String trả về biểu diễn chuỗi của fingerprint.
func (f Fingerprint) String() string { return f.value }

// AlertInstance là một lần firing của alert nhận từ Alertmanager (aggregate
// alerting BC). Bất biến — mọi thay đổi trả về bản sao mới.
type AlertInstance struct {
	id          string
	workspaceID string
	fingerprint Fingerprint
	status      InstanceStatus
	firedAt     time.Time
	resolvedAt  time.Time // zero nếu chưa resolved
	labels      map[string]string
}

// NewFiringInstanceInput gom tham số khởi tạo một instance ở trạng thái firing.
type NewFiringInstanceInput struct {
	ID          string
	WorkspaceID string
	Fingerprint Fingerprint
	FiredAt     time.Time
	Labels      map[string]string
}

// NewFiringInstance tạo instance firing mới sau khi kiểm bất biến.
func NewFiringInstance(in NewFiringInstanceInput) (AlertInstance, error) {
	if in.ID == "" {
		return AlertInstance{}, newValidationError("id", "must not be empty")
	}
	if in.WorkspaceID == "" {
		return AlertInstance{}, newValidationError("workspaceId", "must not be empty")
	}
	if in.Fingerprint == (Fingerprint{}) {
		return AlertInstance{}, newValidationError("fingerprint", "must not be empty")
	}
	if in.FiredAt.IsZero() {
		return AlertInstance{}, newValidationError("firedAt", "must not be zero")
	}
	return AlertInstance{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		fingerprint: in.Fingerprint,
		status:      InstanceFiring,
		firedAt:     in.FiredAt,
		labels:      copyMap(in.Labels),
	}, nil
}

// ReconstructInstanceInput hydrate một instance từ storage (KHÔNG validate lại
// — dữ liệu đã hợp lệ khi ghi).
type ReconstructInstanceInput struct {
	ID          string
	WorkspaceID string
	Fingerprint Fingerprint
	Status      InstanceStatus
	FiredAt     time.Time
	ResolvedAt  time.Time
	Labels      map[string]string
}

// ReconstructInstance dựng lại AlertInstance từ dữ liệu đã lưu (read side).
func ReconstructInstance(in ReconstructInstanceInput) AlertInstance {
	return AlertInstance{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		fingerprint: in.Fingerprint,
		status:      in.Status,
		firedAt:     in.FiredAt,
		resolvedAt:  in.ResolvedAt,
		labels:      copyMap(in.Labels),
	}
}

// Resolve trả về bản sao đã chuyển sang trạng thái resolved tại thời điểm at.
func (i AlertInstance) Resolve(at time.Time) AlertInstance {
	c := i
	c.status = InstanceResolved
	c.resolvedAt = at
	c.labels = copyMap(i.labels)
	return c
}

// ID trả về định danh instance.
func (i AlertInstance) ID() string { return i.id }

// WorkspaceID trả về workspace sở hữu instance.
func (i AlertInstance) WorkspaceID() string { return i.workspaceID }

// Fingerprint trả về khóa dedup của instance.
func (i AlertInstance) Fingerprint() Fingerprint { return i.fingerprint }

// Status trả về trạng thái hiện tại của instance.
func (i AlertInstance) Status() InstanceStatus { return i.status }

// FiredAt trả về thời điểm alert bắt đầu firing.
func (i AlertInstance) FiredAt() time.Time { return i.firedAt }

// ResolvedAt trả về thời điểm resolved (zero nếu chưa resolved).
func (i AlertInstance) ResolvedAt() time.Time { return i.resolvedAt }

// Labels trả về bản copy labels của instance.
func (i AlertInstance) Labels() map[string]string { return copyMap(i.labels) }
