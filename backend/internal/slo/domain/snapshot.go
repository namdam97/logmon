package domain

import "time"

// Snapshot là read model của một lần đo budget SLO (ghi định kỳ bởi budget
// snapshot job). Bất biến — tạo qua NewSnapshot.
type Snapshot struct {
	sloID                  SLOID
	currentSLI             float64
	budgetRemainingPercent float64
	burnRate1h             float64
	burnRate6h             float64
	burnRate24h            float64
	recordedAt             time.Time
}

// NewSnapshotInput gom tham số tạo snapshot.
type NewSnapshotInput struct {
	SLOID                  SLOID
	CurrentSLI             float64
	BudgetRemainingPercent float64
	BurnRate1h             float64
	BurnRate6h             float64
	BurnRate24h            float64
	RecordedAt             time.Time
}

// NewSnapshot tạo snapshot read model.
func NewSnapshot(in NewSnapshotInput) Snapshot {
	return Snapshot{
		sloID:                  in.SLOID,
		currentSLI:             in.CurrentSLI,
		budgetRemainingPercent: in.BudgetRemainingPercent,
		burnRate1h:             in.BurnRate1h,
		burnRate6h:             in.BurnRate6h,
		burnRate24h:            in.BurnRate24h,
		recordedAt:             in.RecordedAt,
	}
}

// SLOID trả về SLO của snapshot.
func (s Snapshot) SLOID() SLOID { return s.sloID }

// CurrentSLI trả về SLI hiện tại (vd 0.9993).
func (s Snapshot) CurrentSLI() float64 { return s.currentSLI }

// BudgetRemainingPercent trả về phần trăm budget còn lại (0..100).
func (s Snapshot) BudgetRemainingPercent() float64 { return s.budgetRemainingPercent }

// BurnRate1h trả về burn rate cửa sổ 1h.
func (s Snapshot) BurnRate1h() float64 { return s.burnRate1h }

// BurnRate6h trả về burn rate cửa sổ 6h.
func (s Snapshot) BurnRate6h() float64 { return s.burnRate6h }

// BurnRate24h trả về burn rate cửa sổ 24h.
func (s Snapshot) BurnRate24h() float64 { return s.burnRate24h }

// RecordedAt trả về thời điểm ghi snapshot.
func (s Snapshot) RecordedAt() time.Time { return s.recordedAt }
