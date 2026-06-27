-- 000010_postmortems: postmortem blameless + action items (GĐ3.5, doc_v2/06 §1.5).
-- Postmortem 1-1 với incident (unique incident_id). Impact = số liệu từ chính
-- LogMon (thời lượng/error count/budget tiêu thụ). Action item track assignee +
-- due date + trạng thái — nguồn dữ liệu thật cho "fewer repeat incidents".

CREATE TABLE IF NOT EXISTS postmortems (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id             UUID        NOT NULL UNIQUE REFERENCES incidents (id) ON DELETE CASCADE,
    workspace_id            UUID        NOT NULL,
    status                  VARCHAR(20) NOT NULL CHECK (status IN ('draft', 'published')),
    root_cause              TEXT        NOT NULL DEFAULT '',
    impact_duration_seconds BIGINT      NOT NULL DEFAULT 0,
    impact_error_count      BIGINT      NOT NULL DEFAULT 0,
    impact_budget_percent   DOUBLE PRECISION NOT NULL DEFAULT 0,
    impact_summary          TEXT        NOT NULL DEFAULT '',
    timeline_summary        TEXT        NOT NULL DEFAULT '',
    lessons_learned         TEXT        NOT NULL DEFAULT '',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at            TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS action_items (
    id            UUID PRIMARY KEY,
    postmortem_id UUID         NOT NULL REFERENCES postmortems (id) ON DELETE CASCADE,
    title         VARCHAR(500) NOT NULL,
    assignee      TEXT         NOT NULL DEFAULT '',
    due_date      TIMESTAMPTZ,
    status        VARCHAR(20)  NOT NULL CHECK (status IN ('open', 'in_progress', 'done')),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_action_items_postmortem ON action_items (postmortem_id, created_at ASC);

-- Reminder auto-PostmortemPending (24h): quét incident SEV1/2 đã Resolved quá hạn.
CREATE INDEX IF NOT EXISTS idx_incidents_resolved_postmortem
    ON incidents (resolved_at)
    WHERE status = 'resolved' AND severity IN ('SEV1', 'SEV2');
