package domain

import (
	"regexp"
	"strings"
	"time"
)

const (
	maxNameLength    = 100
	maxServiceLength = 100

	// DefaultWindowDays là cửa sổ SLO mặc định (28d = bội số tuần, tránh dao
	// động theo ngày trong tuần — doc_v2/05 §4.1).
	DefaultWindowDays = 28
	minWindowDays     = 1
	maxWindowDays     = 90
)

// _safeIdentPattern giới hạn ký tự cho name/service trước khi nhúng vào PromQL
// selector/label (chống injection — điều kiện hội đồng GĐ3): chữ, số, khoảng
// trắng, '_' '-' '.'; KHÔNG cho phép '"' '\' hay ký tự điều khiển.
var _safeIdentPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.\-]*$`)

// SyncStatus phản ánh trạng thái đồng bộ rule SLO sang Prometheus.
type SyncStatus string

// Các trạng thái sync.
const (
	SyncPending SyncStatus = "pending"
	SyncSynced  SyncStatus = "synced"
	SyncError   SyncStatus = "error"
)

// SLIType là value object cho loại SLI (doc_v2/05 §4.1).
type SLIType struct {
	value string
}

// Các SLI type hợp lệ GĐ3.
var (
	SLIAvailability = SLIType{"availability"} // 1 − error ratio
	SLILatency      = SLIType{"latency"}      // tỉ lệ request nhanh hơn threshold
)

// NewSLIType validate và bọc một SLI type string.
func NewSLIType(raw string) (SLIType, error) {
	switch raw {
	case SLIAvailability.value, SLILatency.value:
		return SLIType{raw}, nil
	default:
		return SLIType{}, newValidationError("sliType", "must be one of availability|latency")
	}
}

// String trả về biểu diễn chuỗi của SLI type.
func (t SLIType) String() string { return t.value }

// IsLatency cho biết SLI có phải loại latency không.
func (t SLIType) IsLatency() bool { return t.value == SLILatency.value }

// SLOID là value object định danh SLO (UUID dạng chuỗi).
type SLOID struct {
	value string
}

// NewSLOID validate và bọc định danh SLO không rỗng.
func NewSLOID(raw string) (SLOID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return SLOID{}, newValidationError("id", "must not be empty")
	}
	return SLOID{value: v}, nil
}

// String trả về biểu diễn chuỗi của SLOID.
func (id SLOID) String() string { return id.value }

// SLO là aggregate root của slo BC. Field không export để giữ bất biến — chỉ
// tạo qua NewSLO, chuyển trạng thái qua method trả bản copy mới.
type SLO struct {
	id           SLOID
	workspaceID  string
	name         string
	service      string
	sliType      SLIType
	latencyMs    int // chỉ dùng khi sliType=latency; 0 với availability
	target       float64
	windowDays   int
	syncStatus   SyncStatus
	syncErrorMsg string
	createdAt    time.Time
	updatedAt    time.Time
}

// NewSLOInput gom tham số tạo SLO (đã đi qua value object SLOID/SLIType).
type NewSLOInput struct {
	ID                 SLOID
	WorkspaceID        string
	Name               string
	Service            string
	SLIType            SLIType
	LatencyThresholdMs int
	Target             float64
	WindowDays         int
	CreatedAt          time.Time
}

// sloFields gom field nội dung để validate chung giữa NewSLO và Update.
type sloFields struct {
	name        string
	workspaceID string
	service     string
	sliType     SLIType
	latencyMs   int
	target      float64
	windowDays  int
}

// validateSLOFields kiểm mọi invariant nội dung SLO, trả về giá trị đã chuẩn hóa
// (name/service trim, windowDays default, latencyMs đã chuẩn theo sliType).
func validateSLOFields(f sloFields) (name, service string, windowDays, latencyMs int, err error) {
	name = strings.TrimSpace(f.name)
	switch {
	case name == "":
		return "", "", 0, 0, newValidationError("name", "must not be empty")
	case len(name) > maxNameLength:
		return "", "", 0, 0, newValidationError("name", "exceeds maximum length")
	case !_safeIdentPattern.MatchString(name):
		return "", "", 0, 0, newValidationError("name", "contains invalid characters")
	}
	if strings.TrimSpace(f.workspaceID) == "" {
		return "", "", 0, 0, newValidationError("workspaceId", "must not be empty")
	}
	service = strings.TrimSpace(f.service)
	switch {
	case service == "":
		return "", "", 0, 0, newValidationError("service", "must not be empty")
	case len(service) > maxServiceLength:
		return "", "", 0, 0, newValidationError("service", "exceeds maximum length")
	case !_safeIdentPattern.MatchString(service):
		return "", "", 0, 0, newValidationError("service", "contains invalid characters")
	}
	if f.target <= 0 || f.target >= 1 {
		return "", "", 0, 0, newValidationError("target", "must be between 0 and 1 (exclusive)")
	}

	windowDays = f.windowDays
	if windowDays == 0 {
		windowDays = DefaultWindowDays
	}
	if windowDays < minWindowDays || windowDays > maxWindowDays {
		return "", "", 0, 0, newValidationError("windowDays", "must be between 1 and 90")
	}

	// latency phải có threshold > 0; availability không được set threshold.
	if f.sliType.IsLatency() {
		if f.latencyMs <= 0 {
			return "", "", 0, 0, newValidationError("latencyThresholdMs", "must be positive for latency SLI")
		}
		latencyMs = f.latencyMs
	} else if f.latencyMs != 0 {
		return "", "", 0, 0, newValidationError("latencyThresholdMs", "must not be set for availability SLI")
	}
	return name, service, windowDays, latencyMs, nil
}

// NewSLO tạo SLO mới đã validate đầy đủ invariant.
func NewSLO(in NewSLOInput) (SLO, error) {
	name, service, windowDays, latencyMs, err := validateSLOFields(sloFields{
		name:        in.Name,
		workspaceID: in.WorkspaceID,
		service:     in.Service,
		sliType:     in.SLIType,
		latencyMs:   in.LatencyThresholdMs,
		target:      in.Target,
		windowDays:  in.WindowDays,
	})
	if err != nil {
		return SLO{}, err
	}
	if in.CreatedAt.IsZero() {
		return SLO{}, newValidationError("createdAt", "must be set")
	}

	return SLO{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		name:        name,
		service:     service,
		sliType:     in.SLIType,
		latencyMs:   latencyMs,
		target:      in.Target,
		windowDays:  windowDays,
		syncStatus:  SyncPending,
		createdAt:   in.CreatedAt,
		updatedAt:   in.CreatedAt,
	}, nil
}

// UpdateInput gom field người dùng có thể sửa trên một SLO đã tồn tại.
type UpdateInput struct {
	Name               string
	Service            string
	SLIType            SLIType
	LatencyThresholdMs int
	Target             float64
	WindowDays         int
}

// Update trả về bản copy SLO đã đổi field nội dung (đã validate), đặt lại sync về
// pending để render lại rules. Giữ nguyên id/workspaceId/createdAt.
func (s SLO) Update(in UpdateInput, now time.Time) (SLO, error) {
	name, service, windowDays, latencyMs, err := validateSLOFields(sloFields{
		name:        in.Name,
		workspaceID: s.workspaceID,
		service:     in.Service,
		sliType:     in.SLIType,
		latencyMs:   in.LatencyThresholdMs,
		target:      in.Target,
		windowDays:  in.WindowDays,
	})
	if err != nil {
		return SLO{}, err
	}
	c := s
	c.name = name
	c.service = service
	c.sliType = in.SLIType
	c.latencyMs = latencyMs
	c.target = in.Target
	c.windowDays = windowDays
	c.syncStatus = SyncPending
	c.syncErrorMsg = ""
	c.updatedAt = now
	return c, nil
}

// ReconstructInput dựng lại SLO từ dữ liệu DB đã hợp lệ — KHÔNG áp default.
type ReconstructInput struct {
	ID          SLOID
	WorkspaceID string
	Name        string
	Service     string
	SLIType     SLIType
	LatencyMs   int
	Target      float64
	WindowDays  int
	SyncStatus  SyncStatus
	SyncError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Reconstruct hydrate SLO từ persistence (repository). Tin dữ liệu DB.
func Reconstruct(in ReconstructInput) SLO {
	return SLO{
		id:           in.ID,
		workspaceID:  in.WorkspaceID,
		name:         in.Name,
		service:      in.Service,
		sliType:      in.SLIType,
		latencyMs:    in.LatencyMs,
		target:       in.Target,
		windowDays:   in.WindowDays,
		syncStatus:   in.SyncStatus,
		syncErrorMsg: in.SyncError,
		createdAt:    in.CreatedAt,
		updatedAt:    in.UpdatedAt,
	}
}

// MarkSynced đánh dấu SLO đã sync rules thành công sang Prometheus.
func (s SLO) MarkSynced(now time.Time) SLO {
	c := s
	c.syncStatus = SyncSynced
	c.syncErrorMsg = ""
	c.updatedAt = now
	return c
}

// MarkSyncError đánh dấu SLO sync lỗi kèm thông điệp.
func (s SLO) MarkSyncError(message string, now time.Time) SLO {
	c := s
	c.syncStatus = SyncError
	c.syncErrorMsg = message
	c.updatedAt = now
	return c
}

// ErrorBudget trả về error budget = 1 − target (vd target 0.999 → budget 0.001).
func (s SLO) ErrorBudget() float64 { return 1 - s.target }

// ID trả về định danh SLO.
func (s SLO) ID() SLOID { return s.id }

// WorkspaceID trả về workspace sở hữu SLO.
func (s SLO) WorkspaceID() string { return s.workspaceID }

// Name trả về tên SLO (duy nhất trong workspace).
func (s SLO) Name() string { return s.name }

// Service trả về service mà SLO áp dụng.
func (s SLO) Service() string { return s.service }

// SLIType trả về loại SLI.
func (s SLO) SLIType() SLIType { return s.sliType }

// LatencyThresholdMs trả về ngưỡng latency (ms); 0 nếu SLI là availability.
func (s SLO) LatencyThresholdMs() int { return s.latencyMs }

// Target trả về mục tiêu SLO (0 < target < 1).
func (s SLO) Target() float64 { return s.target }

// WindowDays trả về cửa sổ rolling (ngày).
func (s SLO) WindowDays() int { return s.windowDays }

// SyncStatus trả về trạng thái sync rules sang Prometheus.
func (s SLO) SyncStatus() SyncStatus { return s.syncStatus }

// SyncErrorMessage trả về thông điệp lỗi sync gần nhất (rỗng nếu không lỗi).
func (s SLO) SyncErrorMessage() string { return s.syncErrorMsg }

// CreatedAt trả về thời điểm tạo SLO.
func (s SLO) CreatedAt() time.Time { return s.createdAt }

// UpdatedAt trả về thời điểm cập nhật SLO gần nhất.
func (s SLO) UpdatedAt() time.Time { return s.updatedAt }
