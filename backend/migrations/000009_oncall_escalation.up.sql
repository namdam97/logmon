-- 000009_oncall_escalation: on-call & escalation (GĐ3.4, doc_v2/06 §1.4).
-- Schedule (rotation daily/weekly, timezone-aware) + override (swap/nghỉ phép) +
-- escalation policy (1/workspace) + escalation state (bậc đã thông báo/incident).
-- "Ai đang on-call" tính bằng pure function nên schedule chỉ lưu config, không
-- lưu trạng thái trực; escalation state idempotent theo (incident_id, level).

CREATE TABLE IF NOT EXISTS oncall_schedules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID         NOT NULL,
    name         VARCHAR(120) NOT NULL,
    rotation     VARCHAR(10)  NOT NULL CHECK (rotation IN ('daily', 'weekly')),
    participants TEXT[]       NOT NULL,
    timezone     VARCHAR(64)  NOT NULL DEFAULT '',
    anchor       TIMESTAMPTZ  NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_oncall_schedules_ws ON oncall_schedules (workspace_id, created_at ASC);

CREATE TABLE IF NOT EXISTS oncall_overrides (
    id          UUID PRIMARY KEY,
    schedule_id UUID        NOT NULL REFERENCES oncall_schedules (id) ON DELETE CASCADE,
    participant TEXT        NOT NULL,
    start_at    TIMESTAMPTZ NOT NULL,
    end_at      TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_at > start_at)
);

-- Truy vấn override hiệu lực tại một thời điểm cho một schedule.
CREATE INDEX IF NOT EXISTS idx_oncall_overrides_active
    ON oncall_overrides (schedule_id, start_at, end_at);

CREATE TABLE IF NOT EXISTS escalation_policies (
    id           UUID PRIMARY KEY,
    workspace_id UUID         NOT NULL UNIQUE,
    name         VARCHAR(120) NOT NULL,
    team_lead    TEXT         NOT NULL DEFAULT '',
    levels       JSONB        NOT NULL
);

CREATE TABLE IF NOT EXISTS incident_escalations (
    incident_id UUID        NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    level       INT         NOT NULL,
    recipient   TEXT        NOT NULL DEFAULT '',
    notified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, level)
);
