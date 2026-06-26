package domain

import "time"

// SilenceMatcher khớp alert theo label. Alertmanager là nơi thực thi matching
// thực tế — LogMon KHÔNG reimplement engine match; đây chỉ là VO mang dữ liệu.
type SilenceMatcher struct {
	name    string
	value   string
	isRegex bool
	isEqual bool
}

// NewSilenceMatcher dựng một matcher (dùng cả khi tạo silence lẫn khi parse
// read model từ Alertmanager). isEqual=false nghĩa là phủ định (negative match).
func NewSilenceMatcher(name, value string, isRegex, isEqual bool) SilenceMatcher {
	return SilenceMatcher{name: name, value: value, isRegex: isRegex, isEqual: isEqual}
}

// Name trả về tên label của matcher.
func (m SilenceMatcher) Name() string { return m.name }

// Value trả về giá trị (hoặc regex) so khớp.
func (m SilenceMatcher) Value() string { return m.value }

// IsRegex cho biết value là regex.
func (m SilenceMatcher) IsRegex() bool { return m.isRegex }

// IsEqual cho biết matcher khớp dương (true) hay phủ định (false).
func (m SilenceMatcher) IsEqual() bool { return m.isEqual }

// MatcherInput là dữ liệu thô tạo một matcher (từ HTTP request).
type MatcherInput struct {
	Name    string
	Value   string
	IsRegex bool
	IsEqual bool
}

// NewSilenceInput gom dữ liệu tạo một silence.
type NewSilenceInput struct {
	Matchers  []MatcherInput
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy string
	Comment   string
}

// Silence là VO mô tả yêu cầu tắt thông báo, proxy sang Alertmanager. Bất biến.
// LogMon KHÔNG lưu silence — Alertmanager là source of truth; VO này chỉ validate
// + mang dữ liệu sang gateway.
type Silence struct {
	matchers  []SilenceMatcher
	startsAt  time.Time
	endsAt    time.Time
	createdBy string
	comment   string
}

// NewSilence validate input và dựng Silence bất biến. Trả về ValidationError nếu
// thiếu matcher, matcher rỗng tên, thiếu người tạo/ghi chú, hoặc cửa sổ thời gian
// không hợp lệ (endsAt phải sau startsAt).
func NewSilence(in NewSilenceInput) (Silence, error) {
	if len(in.Matchers) == 0 {
		return Silence{}, newValidationError("matchers", "must have at least one matcher")
	}
	matchers := make([]SilenceMatcher, 0, len(in.Matchers))
	for _, m := range in.Matchers {
		if m.Name == "" {
			return Silence{}, newValidationError("matchers", "matcher name must not be empty")
		}
		matchers = append(matchers, NewSilenceMatcher(m.Name, m.Value, m.IsRegex, m.IsEqual))
	}
	if in.CreatedBy == "" {
		return Silence{}, newValidationError("createdBy", "must not be empty")
	}
	if in.Comment == "" {
		return Silence{}, newValidationError("comment", "must not be empty")
	}
	if !in.EndsAt.After(in.StartsAt) {
		return Silence{}, newValidationError("endsAt", "must be after startsAt")
	}
	return Silence{
		matchers:  matchers,
		startsAt:  in.StartsAt,
		endsAt:    in.EndsAt,
		createdBy: in.CreatedBy,
		comment:   in.Comment,
	}, nil
}

// Matchers trả về bản sao slice matcher (copy tại boundary, tránh mutation ngoài).
func (s Silence) Matchers() []SilenceMatcher {
	out := make([]SilenceMatcher, len(s.matchers))
	copy(out, s.matchers)
	return out
}

// StartsAt trả về thời điểm bắt đầu tắt thông báo.
func (s Silence) StartsAt() time.Time { return s.startsAt }

// EndsAt trả về thời điểm hết hiệu lực.
func (s Silence) EndsAt() time.Time { return s.endsAt }

// CreatedBy trả về userID người tạo silence.
func (s Silence) CreatedBy() string { return s.createdBy }

// Comment trả về ghi chú lý do tắt thông báo.
func (s Silence) Comment() string { return s.comment }

// SilenceView là read model của một silence lấy từ Alertmanager (kèm id + trạng
// thái). Plain struct — không validate, chỉ phục vụ hiển thị (CQRS read side).
type SilenceView struct {
	ID        string
	Status    string
	Matchers  []SilenceMatcher
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy string
	Comment   string
}
