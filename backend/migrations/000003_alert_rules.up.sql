-- 000003_alert_rules: alerting BC (GĐ2). workspace_id dùng workspace mặc định
-- ở GĐ2; FK + RBAC thêm ở GĐ3. alert_instances tách sang migration 2.3.
CREATE TABLE IF NOT EXISTS alert_rules (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID         NOT NULL,
    name            VARCHAR(100) NOT NULL,
    expression      TEXT         NOT NULL,                -- PromQL (validate bằng parser ở app)
    for_duration    INTERVAL     NOT NULL,
    severity        VARCHAR(20)  NOT NULL CHECK (severity IN ('critical', 'warning', 'info')),
    service         VARCHAR(100) NOT NULL,
    labels          JSONB        NOT NULL DEFAULT '{}',
    annotations     JSONB        NOT NULL DEFAULT '{}',   -- bắt buộc có summary + runbook_url (validate ở app/domain)
    enabled         BOOLEAN      NOT NULL DEFAULT true,
    sync_status     VARCHAR(20)  NOT NULL DEFAULT 'pending', -- pending|synced|error
    sync_error      TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX IF NOT EXISTS idx_rules_ws_svc ON alert_rules (workspace_id, service);
