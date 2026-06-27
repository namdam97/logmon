-- 000008_incidents: incident BC (GĐ3.3). State machine 7 trạng thái + timeline
-- (event sourcing nhẹ cho audit) + severity SEV1-4. workspace_id dùng workspace
-- mặc định (multi-tenancy đầy đủ ở 3.6). severity NULL = chưa phân loại (set khi
-- Triage). assigned_at/resolved_at là nguồn tính MTTA/MTTR (doc_v2/06 §1.1-1.3).

CREATE TABLE IF NOT EXISTS incidents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID         NOT NULL,
    title        VARCHAR(200) NOT NULL,
    description  TEXT         NOT NULL DEFAULT '',
    service      VARCHAR(100) NOT NULL,
    severity     VARCHAR(10)  CHECK (severity IN ('SEV1', 'SEV2', 'SEV3', 'SEV4')),
    status       VARCHAR(20)  NOT NULL CHECK (status IN
                     ('open', 'triaged', 'assigned', 'mitigating',
                      'resolved', 'postmortem_pending', 'closed')),
    source       VARCHAR(20)  NOT NULL CHECK (source IN ('manual', 'alert', 'slo_budget')),
    source_ref   TEXT         NOT NULL DEFAULT '',
    assignee     TEXT         NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    assigned_at  TIMESTAMPTZ,
    resolved_at  TIMESTAMPTZ,
    closed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_incidents_ws_status ON incidents (workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_incidents_ws_created ON incidents (workspace_id, created_at DESC);

-- Dedup auto-create ở DB level: tối đa một incident ĐANG ACTIVE cho mỗi
-- (workspace, source, source_ref) khi có ref — chặn race khi event lặp
-- (BudgetExhausted/AlertFired). Loại trừ ref rỗng (incident manual).
CREATE UNIQUE INDEX IF NOT EXISTS uq_incidents_active_source_ref
    ON incidents (workspace_id, source, source_ref)
    WHERE source_ref <> '' AND status IN ('open', 'triaged', 'assigned', 'mitigating');

CREATE TABLE IF NOT EXISTS incident_timeline (
    id          UUID PRIMARY KEY,
    incident_id UUID        NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    kind        VARCHAR(30) NOT NULL,
    from_status VARCHAR(20),
    to_status   VARCHAR(20),
    actor       TEXT        NOT NULL DEFAULT '',
    note        TEXT        NOT NULL DEFAULT '',
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_incident_timeline ON incident_timeline (incident_id, at ASC);
