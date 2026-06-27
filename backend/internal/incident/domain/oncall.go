package domain

import (
	"sort"
	"strings"
	"time"
)

// On-call & escalation (doc_v2/06 §1.4). Điểm cốt lõi: "ai đang on-call" là một
// PURE FUNCTION từ schedule config + thời điểm — không cần DB state, test xác định.
// Override (swap/nghỉ phép) ghi đè primary trong một khoảng thời gian.

const _maxParticipants = 50

// RotationType là chu kỳ luân phiên on-call: daily hoặc weekly.
type RotationType struct {
	value string
}

// Hai kiểu rotation hỗ trợ (doc_v2/06 §1.4).
var (
	RotationDaily  = RotationType{"daily"}
	RotationWeekly = RotationType{"weekly"}
)

var _validRotations = map[string]RotationType{
	RotationDaily.value:  RotationDaily,
	RotationWeekly.value: RotationWeekly,
}

// NewRotationType validate và bọc một rotation string.
func NewRotationType(raw string) (RotationType, error) {
	if r, ok := _validRotations[raw]; ok {
		return r, nil
	}
	return RotationType{}, newValidationError("rotation", "must be one of daily|weekly")
}

// String trả về biểu diễn chuỗi của rotation type.
func (r RotationType) String() string { return r.value }

// period trả về độ dài một chu kỳ luân phiên.
func (r RotationType) period() time.Duration {
	if r == RotationWeekly {
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// ScheduleID là value object định danh on-call schedule (UUID dạng chuỗi).
type ScheduleID struct {
	value string
}

// NewScheduleID validate và bọc định danh schedule không rỗng.
func NewScheduleID(raw string) (ScheduleID, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ScheduleID{}, newValidationError("schedule_id", "must not be empty")
	}
	return ScheduleID{value: v}, nil
}

// String trả về biểu diễn chuỗi của ScheduleID.
func (id ScheduleID) String() string { return id.value }

// Schedule là aggregate cấu hình lịch on-call. Bất biến: mọi field private,
// truy cập qua accessor; "ai on-call" tính bằng WhoIsOnCall (pure). Anchor đã gộp
// handoff time theo timezone tại lúc tạo nên period boundary là tuyệt đối
// (instant), độc lập tz khi tính — đủ chính xác cho tz không DST (vd Asia/*);
// với tz có DST handoff wall-clock có thể lệch tối đa 1h (chấp nhận được).
type Schedule struct {
	id           ScheduleID
	workspaceID  string
	name         string
	rotation     RotationType
	participants []string // thứ tự luân phiên; index 0 bắt đầu tại anchor
	timezone     string   // IANA, vd "Asia/Ho_Chi_Minh"
	anchor       time.Time
	createdAt    time.Time
	updatedAt    time.Time
}

// NewScheduleInput gom tham số tạo Schedule (tránh long param list).
type NewScheduleInput struct {
	ID           string
	WorkspaceID  string
	Name         string
	Rotation     string
	Participants []string
	Timezone     string // IANA; rỗng → UTC
	HandoffHour  int    // 0-23, giờ bàn giao trong ngày
	HandoffMin   int    // 0-59
	StartDate    time.Time
	Now          time.Time
}

// NewSchedule tạo schedule mới sau khi validate. Anchor = StartDate (theo
// timezone) tại HandoffHour:HandoffMin — mốc bắt đầu chu kỳ của participant[0].
func NewSchedule(in NewScheduleInput) (Schedule, error) {
	id, err := NewScheduleID(in.ID)
	if err != nil {
		return Schedule{}, err
	}
	if strings.TrimSpace(in.WorkspaceID) == "" {
		return Schedule{}, newValidationError("workspace_id", "must not be empty")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Schedule{}, newValidationError("name", "must not be empty")
	}
	rotation, err := NewRotationType(in.Rotation)
	if err != nil {
		return Schedule{}, err
	}
	participants, err := normalizeParticipants(in.Participants)
	if err != nil {
		return Schedule{}, err
	}
	if in.HandoffHour < 0 || in.HandoffHour > 23 {
		return Schedule{}, newValidationError("handoff_hour", "must be in range 0..23")
	}
	if in.HandoffMin < 0 || in.HandoffMin > 59 {
		return Schedule{}, newValidationError("handoff_min", "must be in range 0..59")
	}
	tz := strings.TrimSpace(in.Timezone)
	loc, err := loadLocation(tz)
	if err != nil {
		return Schedule{}, err
	}
	if in.StartDate.IsZero() {
		return Schedule{}, newValidationError("start_date", "must not be empty")
	}
	y, m, d := in.StartDate.In(loc).Date()
	anchor := time.Date(y, m, d, in.HandoffHour, in.HandoffMin, 0, 0, loc)
	return Schedule{
		id:           id,
		workspaceID:  in.WorkspaceID,
		name:         name,
		rotation:     rotation,
		participants: participants,
		timezone:     tz,
		anchor:       anchor,
		createdAt:    in.Now.UTC(),
		updatedAt:    in.Now.UTC(),
	}, nil
}

// normalizeParticipants trim, loại rỗng, validate số lượng (giữ thứ tự).
func normalizeParticipants(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, newValidationError("participants", "must have at least one participant")
	}
	if len(out) > _maxParticipants {
		return nil, newValidationError("participants", "too many participants")
	}
	return out, nil
}

// loadLocation parse IANA timezone (rỗng → UTC), trả ValidationError nếu sai.
func loadLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, newValidationError("timezone", "unknown IANA timezone")
	}
	return loc, nil
}

// Accessors (read-only) ----------------------------------------------------

// ID trả về định danh schedule.
func (s Schedule) ID() ScheduleID { return s.id }

// WorkspaceID trả về workspace sở hữu schedule.
func (s Schedule) WorkspaceID() string { return s.workspaceID }

// Name trả về tên schedule.
func (s Schedule) Name() string { return s.name }

// Rotation trả về kiểu luân phiên.
func (s Schedule) Rotation() RotationType { return s.rotation }

// Participants trả về BẢN SAO danh sách participant (copy tại boundary).
func (s Schedule) Participants() []string {
	out := make([]string, len(s.participants))
	copy(out, s.participants)
	return out
}

// Timezone trả về IANA timezone string ("" nghĩa là UTC).
func (s Schedule) Timezone() string { return s.timezone }

// Anchor trả về mốc bắt đầu chu kỳ (đã gộp handoff time).
func (s Schedule) Anchor() time.Time { return s.anchor }

// CreatedAt trả về thời điểm tạo (UTC).
func (s Schedule) CreatedAt() time.Time { return s.createdAt }

// UpdatedAt trả về thời điểm cập nhật cuối (UTC).
func (s Schedule) UpdatedAt() time.Time { return s.updatedAt }

// onCallIndex tính index participant đang trực tại thời điểm `at`.
func (s Schedule) onCallIndex(at time.Time) int {
	n := len(s.participants)
	if n == 0 {
		return 0
	}
	elapsed := at.Sub(s.anchor)
	if elapsed < 0 {
		return 0
	}
	periods := int64(elapsed / s.rotation.period())
	return int(periods % int64(n))
}

// ReconstructScheduleInput hydrate Schedule từ DB (đã được validate khi ghi).
type ReconstructScheduleInput struct {
	ID           string
	WorkspaceID  string
	Name         string
	Rotation     string
	Participants []string
	Timezone     string
	Anchor       time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ReconstructSchedule dựng lại Schedule từ dữ liệu persistence (bỏ qua validate
// nặng — dữ liệu đã hợp lệ khi ghi). Vẫn chuẩn hoá rotation để an toàn.
func ReconstructSchedule(in ReconstructScheduleInput) Schedule {
	rotation := RotationDaily
	if r, ok := _validRotations[in.Rotation]; ok {
		rotation = r
	}
	participants := make([]string, len(in.Participants))
	copy(participants, in.Participants)
	return Schedule{
		id:           ScheduleID{value: in.ID},
		workspaceID:  in.WorkspaceID,
		name:         in.Name,
		rotation:     rotation,
		participants: participants,
		timezone:     in.Timezone,
		anchor:       in.Anchor,
		createdAt:    in.CreatedAt.UTC(),
		updatedAt:    in.UpdatedAt.UTC(),
	}
}

// Override là một ghi đè on-call (swap/nghỉ phép): trong [StartAt, EndAt)
// participant chỉ định thay thế primary của schedule.
type Override struct {
	id          string
	scheduleID  ScheduleID
	participant string
	startAt     time.Time
	endAt       time.Time
	createdAt   time.Time
}

// NewOverrideInput gom tham số tạo Override.
type NewOverrideInput struct {
	ID          string
	ScheduleID  string
	Participant string
	StartAt     time.Time
	EndAt       time.Time
	Now         time.Time
}

// NewOverride tạo override mới sau khi validate khoảng thời gian.
func NewOverride(in NewOverrideInput) (Override, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return Override{}, newValidationError("id", "must not be empty")
	}
	sid, err := NewScheduleID(in.ScheduleID)
	if err != nil {
		return Override{}, err
	}
	participant := strings.TrimSpace(in.Participant)
	if participant == "" {
		return Override{}, newValidationError("participant", "must not be empty")
	}
	if in.StartAt.IsZero() || in.EndAt.IsZero() {
		return Override{}, newValidationError("interval", "start and end must be set")
	}
	if !in.EndAt.After(in.StartAt) {
		return Override{}, newValidationError("interval", "end must be after start")
	}
	return Override{
		id:          id,
		scheduleID:  sid,
		participant: participant,
		startAt:     in.StartAt.UTC(),
		endAt:       in.EndAt.UTC(),
		createdAt:   in.Now.UTC(),
	}, nil
}

// ID trả về định danh override.
func (o Override) ID() string { return o.id }

// ScheduleID trả về schedule mà override áp dụng.
func (o Override) ScheduleID() ScheduleID { return o.scheduleID }

// Participant trả về người thay thế.
func (o Override) Participant() string { return o.participant }

// StartAt trả về thời điểm bắt đầu hiệu lực (UTC).
func (o Override) StartAt() time.Time { return o.startAt }

// EndAt trả về thời điểm kết thúc hiệu lực (UTC, exclusive).
func (o Override) EndAt() time.Time { return o.endAt }

// CreatedAt trả về thời điểm tạo override (UTC).
func (o Override) CreatedAt() time.Time { return o.createdAt }

// covers cho biết override có hiệu lực tại thời điểm `at` ([startAt, endAt)).
func (o Override) covers(at time.Time) bool {
	return !at.Before(o.startAt) && at.Before(o.endAt)
}

// ReconstructOverride dựng lại Override từ persistence.
func ReconstructOverride(id, scheduleID, participant string, startAt, endAt, createdAt time.Time) Override {
	return Override{
		id:          id,
		scheduleID:  ScheduleID{value: scheduleID},
		participant: participant,
		startAt:     startAt.UTC(),
		endAt:       endAt.UTC(),
		createdAt:   createdAt.UTC(),
	}
}

// OnCall là kết quả "ai đang on-call" tại một thời điểm: primary (đang trực) +
// secondary (người kế trong rotation, để escalation). Secondary rỗng nếu chỉ có
// 1 participant. OverrideID khác rỗng khi primary đến từ một override.
type OnCall struct {
	Primary    string
	Secondary  string
	OverrideID string
}

// WhoIsOnCall là PURE FUNCTION trả về on-call tại thời điểm `at`: tính primary
// từ rotation, secondary = người kế tiếp; nếu có override phủ `at` thì primary
// lấy theo override (override mới nhất theo StartAt thắng). Không chạm DB.
func WhoIsOnCall(s Schedule, overrides []Override, at time.Time) OnCall {
	n := len(s.participants)
	if n == 0 {
		return OnCall{}
	}
	idx := s.onCallIndex(at)
	result := OnCall{Primary: s.participants[idx]}
	if n >= 2 {
		result.Secondary = s.participants[(idx+1)%n]
	}

	// Override: chọn override đang phủ `at`, ưu tiên StartAt mới nhất.
	active := make([]Override, 0, len(overrides))
	for _, o := range overrides {
		if o.scheduleID == s.id && o.covers(at) {
			active = append(active, o)
		}
	}
	if len(active) > 0 {
		sort.Slice(active, func(i, j int) bool {
			return active[i].startAt.Before(active[j].startAt)
		})
		winner := active[len(active)-1]
		result.Primary = winner.participant
		result.OverrideID = winner.id
	}
	return result
}
