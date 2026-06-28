// Package domain chứa entity, value object và domain error của reporting BC
// (scheduled reports + async export — GĐ4.3). Chỉ import stdlib + shared/errors.
package domain

import (
	"errors"
	"strings"
	"time"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// Domain sentinel errors.
var (
	// ErrReportScheduleNotFound: không có schedule theo id.
	ErrReportScheduleNotFound = errors.New("report schedule not found")
	// ErrExportJobNotFound: không có export job theo id.
	ErrExportJobNotFound = errors.New("export job not found")
	// ErrExportNotPending: job không ở trạng thái pending (không thể process lại).
	ErrExportNotPending = errors.New("export job not pending")
)

const _maxRecipients = 50

// ReportType là loại báo cáo định kỳ.
type ReportType int

// Enum bắt đầu từ 1.
const (
	// ReportSLOWeekly: tổng hợp SLO tuần.
	ReportSLOWeekly ReportType = iota + 1
	// ReportIncidentSummary: tổng hợp sự cố.
	ReportIncidentSummary
	// ReportCostMonthly: chi phí/usage tháng.
	ReportCostMonthly
)

// String trả về biểu diễn khớp cột DB.
func (t ReportType) String() string {
	switch t {
	case ReportSLOWeekly:
		return "slo_weekly"
	case ReportIncidentSummary:
		return "incident_summary"
	case ReportCostMonthly:
		return "cost_monthly"
	default:
		return "unknown"
	}
}

// ParseReportType chuyển chuỗi thành ReportType.
func ParseReportType(raw string) (ReportType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "slo_weekly":
		return ReportSLOWeekly, nil
	case "incident_summary":
		return ReportIncidentSummary, nil
	case "cost_monthly":
		return ReportCostMonthly, nil
	default:
		return 0, apperrors.NewValidationError("reportType", "must be slo_weekly|incident_summary|cost_monthly")
	}
}

// ReportFormat là định dạng xuất báo cáo.
type ReportFormat int

const (
	// FormatPDF báo cáo PDF.
	FormatPDF ReportFormat = iota + 1
	// FormatCSV dữ liệu CSV.
	FormatCSV
)

// String trả về biểu diễn khớp cột DB.
func (f ReportFormat) String() string {
	switch f {
	case FormatPDF:
		return "pdf"
	case FormatCSV:
		return "csv"
	default:
		return "unknown"
	}
}

// ParseReportFormat chuyển chuỗi thành ReportFormat.
func ParseReportFormat(raw string) (ReportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pdf":
		return FormatPDF, nil
	case "csv":
		return FormatCSV, nil
	default:
		return 0, apperrors.NewValidationError("format", "must be pdf|csv")
	}
}

// ReportSchedule là aggregate cấu hình báo cáo định kỳ. Bất biến — đổi trạng thái
// trả bản sao mới. cron_expression validate ở tầng adapter (cron parser), domain
// chỉ đảm bảo non-empty (giữ domain thuần stdlib).
type ReportSchedule struct {
	id          string
	workspaceID string
	reportType  ReportType
	cronExpr    string
	timezone    string
	format      ReportFormat
	recipients  []string
	channelID   string // optional
	enabled     bool
	lastRunAt   *time.Time
	createdAt   time.Time
}

// NewScheduleInput là dữ liệu tạo schedule.
type NewScheduleInput struct {
	ID          string
	WorkspaceID string
	ReportType  ReportType
	CronExpr    string
	Timezone    string
	Format      ReportFormat
	Recipients  []string
	ChannelID   string
	Now         time.Time
}

// NewReportSchedule tạo schedule đã validate.
func NewReportSchedule(in NewScheduleInput) (ReportSchedule, error) {
	if strings.TrimSpace(in.ID) == "" {
		return ReportSchedule{}, apperrors.NewValidationError("id", "must not be empty")
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return ReportSchedule{}, apperrors.NewValidationError("workspaceId", "must not be empty")
	}
	if !in.ReportType.valid() {
		return ReportSchedule{}, apperrors.NewValidationError("reportType", "invalid")
	}
	if !in.Format.valid() {
		return ReportSchedule{}, apperrors.NewValidationError("format", "invalid")
	}
	if strings.TrimSpace(in.CronExpr) == "" {
		return ReportSchedule{}, apperrors.NewValidationError("cronExpression", "must not be empty")
	}
	if len(in.Recipients) == 0 {
		return ReportSchedule{}, apperrors.NewValidationError("recipients", "at least one required")
	}
	if len(in.Recipients) > _maxRecipients {
		return ReportSchedule{}, apperrors.NewValidationError("recipients", "too many")
	}
	if in.Now.IsZero() {
		return ReportSchedule{}, apperrors.NewValidationError("createdAt", "must be set")
	}
	tz := strings.TrimSpace(in.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	return ReportSchedule{
		id:          in.ID,
		workspaceID: in.WorkspaceID,
		reportType:  in.ReportType,
		cronExpr:    strings.TrimSpace(in.CronExpr),
		timezone:    tz,
		format:      in.Format,
		recipients:  copyStrings(in.Recipients),
		channelID:   strings.TrimSpace(in.ChannelID),
		enabled:     true,
		createdAt:   in.Now,
	}, nil
}

// ReconstructSchedule dựng lại từ storage — KHÔNG validate lại.
func ReconstructSchedule(id, workspaceID string, rt ReportType, cronExpr, tz string, format ReportFormat, recipients []string, channelID string, enabled bool, lastRunAt *time.Time, createdAt time.Time) ReportSchedule {
	return ReportSchedule{
		id: id, workspaceID: workspaceID, reportType: rt, cronExpr: cronExpr, timezone: tz,
		format: format, recipients: copyStrings(recipients), channelID: channelID,
		enabled: enabled, lastRunAt: lastRunAt, createdAt: createdAt,
	}
}

// ID trả về định danh schedule.
func (s ReportSchedule) ID() string { return s.id }

// WorkspaceID trả về workspace.
func (s ReportSchedule) WorkspaceID() string { return s.workspaceID }

// ReportType trả về loại báo cáo.
func (s ReportSchedule) ReportType() ReportType { return s.reportType }

// CronExpr trả về biểu thức cron.
func (s ReportSchedule) CronExpr() string { return s.cronExpr }

// Timezone trả về timezone (IANA).
func (s ReportSchedule) Timezone() string { return s.timezone }

// Format trả về định dạng xuất.
func (s ReportSchedule) Format() ReportFormat { return s.format }

// Recipients trả về bản sao danh sách người nhận (copy tại boundary).
func (s ReportSchedule) Recipients() []string { return copyStrings(s.recipients) }

// ChannelID trả về kênh gửi (rỗng nếu không dùng).
func (s ReportSchedule) ChannelID() string { return s.channelID }

// Enabled báo schedule có đang bật không.
func (s ReportSchedule) Enabled() bool { return s.enabled }

// LastRunAt trả về lần chạy gần nhất (nil nếu chưa chạy).
func (s ReportSchedule) LastRunAt() *time.Time { return s.lastRunAt }

// CreatedAt trả về thời điểm tạo (UTC).
func (s ReportSchedule) CreatedAt() time.Time { return s.createdAt }

// WithEnabled trả bản sao bật/tắt schedule.
func (s ReportSchedule) WithEnabled(enabled bool) ReportSchedule {
	cp := s
	cp.enabled = enabled
	return cp
}

// WithLastRun trả bản sao đã đánh dấu thời điểm chạy gần nhất.
func (s ReportSchedule) WithLastRun(at time.Time) ReportSchedule {
	cp := s
	t := at
	cp.lastRunAt = &t
	return cp
}

func (t ReportType) valid() bool   { return t >= ReportSLOWeekly && t <= ReportCostMonthly }
func (f ReportFormat) valid() bool { return f == FormatPDF || f == FormatCSV }

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
