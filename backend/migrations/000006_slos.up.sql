-- 000006_slos: slo BC (GĐ3). SLO definition + budget snapshots (read model).
-- workspace_id dùng workspace mặc định (multi-tenancy đầy đủ ở 3.6). sync_status
-- đóng vòng rule-sync pipeline (recording + MWMB alerting rules → Prometheus).

CREATE TABLE IF NOT EXISTS slos (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id         UUID         NOT NULL,
    name                 VARCHAR(100) NOT NULL,
    service              VARCHAR(100) NOT NULL,
    sli_type             VARCHAR(20)  NOT NULL CHECK (sli_type IN ('availability', 'latency')),
    latency_threshold_ms INT,
    target               DOUBLE PRECISION NOT NULL CHECK (target > 0 AND target < 1),
    window_days          INT          NOT NULL DEFAULT 28,
    sync_status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    sync_error           TEXT,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX IF NOT EXISTS idx_slos_ws_svc ON slos (workspace_id, service);

CREATE TABLE IF NOT EXISTS slo_snapshots (
    id                       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slo_id                   UUID NOT NULL REFERENCES slos (id) ON DELETE CASCADE,
    current_sli              DOUBLE PRECISION NOT NULL,
    budget_remaining_percent DOUBLE PRECISION NOT NULL,
    burn_rate_1h             DOUBLE PRECISION NOT NULL,
    burn_rate_6h             DOUBLE PRECISION NOT NULL,
    burn_rate_24h            DOUBLE PRECISION NOT NULL,
    recorded_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_slo_snap ON slo_snapshots (slo_id, recorded_at DESC);
