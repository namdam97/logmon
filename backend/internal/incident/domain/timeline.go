package domain

import (
	"strings"
	"time"
)

const maxNoteLength = 2000

// TimelineKind phân loại một mục trên dòng thời gian incident (event sourcing nhẹ
// cho audit — doc_v2/06 §1.1). Mỗi chuyển trạng thái ghi một mục.
type TimelineKind string

// Các loại mục timeline.
const (
	KindCreated           TimelineKind = "created"
	KindTriaged           TimelineKind = "triaged"
	KindAssigned          TimelineKind = "assigned"
	KindMitigating        TimelineKind = "mitigating"
	KindResolved          TimelineKind = "resolved"
	KindPostmortemPending TimelineKind = "postmortem_pending"
	KindClosed            TimelineKind = "closed"
	KindNote              TimelineKind = "note"
)

// TimelineEntry là một mục bất biến trên dòng thời gian incident.
type TimelineEntry struct {
	id         string
	incidentID string
	kind       TimelineKind
	fromStatus Status
	toStatus   Status
	actor      string
	note       string
	at         time.Time
}

// NewTimelineEntryInput gom tham số tạo mục timeline.
type NewTimelineEntryInput struct {
	ID         string
	IncidentID string
	Kind       TimelineKind
	FromStatus Status
	ToStatus   Status
	Actor      string
	Note       string
	At         time.Time
}

// NewTimelineEntry tạo mục timeline đã validate note length.
func NewTimelineEntry(in NewTimelineEntryInput) (TimelineEntry, error) {
	note := strings.TrimSpace(in.Note)
	if len(note) > maxNoteLength {
		return TimelineEntry{}, newValidationError("note", "exceeds maximum length")
	}
	return TimelineEntry{
		id:         in.ID,
		incidentID: in.IncidentID,
		kind:       in.Kind,
		fromStatus: in.FromStatus,
		toStatus:   in.ToStatus,
		actor:      strings.TrimSpace(in.Actor),
		note:       note,
		at:         in.At,
	}, nil
}

// ReconstructTimelineEntry hydrate mục timeline từ DB.
func ReconstructTimelineEntry(in NewTimelineEntryInput) TimelineEntry {
	return TimelineEntry{
		id:         in.ID,
		incidentID: in.IncidentID,
		kind:       in.Kind,
		fromStatus: in.FromStatus,
		toStatus:   in.ToStatus,
		actor:      in.Actor,
		note:       in.Note,
		at:         in.At,
	}
}

// ID trả về định danh mục timeline.
func (e TimelineEntry) ID() string { return e.id }

// IncidentID trả về incident chứa mục này.
func (e TimelineEntry) IncidentID() string { return e.incidentID }

// Kind trả về loại mục timeline.
func (e TimelineEntry) Kind() TimelineKind { return e.kind }

// FromStatus trả về trạng thái trước (zero nếu không phải chuyển trạng thái).
func (e TimelineEntry) FromStatus() Status { return e.fromStatus }

// ToStatus trả về trạng thái sau (zero nếu là note).
func (e TimelineEntry) ToStatus() Status { return e.toStatus }

// Actor trả về người/hệ thống thực hiện hành động.
func (e TimelineEntry) Actor() string { return e.actor }

// Note trả về ghi chú đính kèm.
func (e TimelineEntry) Note() string { return e.note }

// At trả về thời điểm xảy ra.
func (e TimelineEntry) At() time.Time { return e.at }
