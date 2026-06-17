package domain

import "time"

// Severity là value object cho mức độ alert (ADR-024: severity = hành động).
type Severity struct {
	value string
}

// Các severity hợp lệ.
var (
	SeverityCritical = Severity{"critical"} // page (đánh thức người)
	SeverityWarning  = Severity{"warning"}  // ticket (giờ hành chính)
	SeverityInfo     = Severity{"info"}     // ghi nhận, không notify
)

// NewSeverity validate và bọc một severity string.
func NewSeverity(raw string) (Severity, error) {
	switch raw {
	case SeverityCritical.value, SeverityWarning.value, SeverityInfo.value:
		return Severity{raw}, nil
	default:
		return Severity{}, newValidationError("severity", "must be one of critical|warning|info")
	}
}

// String trả về biểu diễn chuỗi của severity.
func (s Severity) String() string { return s.value }

// MinForDuration là `for` tối thiểu chống alert fatigue (doc_v2/05 §1):
// critical ≥ 1m, warning ≥ 5m, info ≥ 0.
func (s Severity) MinForDuration() time.Duration {
	switch s.value {
	case SeverityCritical.value:
		return time.Minute
	case SeverityWarning.value:
		return 5 * time.Minute
	default:
		return 0
	}
}
