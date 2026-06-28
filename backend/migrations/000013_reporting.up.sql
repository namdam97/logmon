-- 000013_reporting: GĐ4.3 — scheduled reports + async export jobs.
-- workspace_id UUID; user_id TEXT (khớp users.id); channel_id FK notification_channels.

CREATE TABLE IF NOT EXISTS report_schedules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID        NOT NULL,
    report_type     VARCHAR(50) NOT NULL,
    cron_expression VARCHAR(50) NOT NULL,
    timezone        VARCHAR(50) NOT NULL DEFAULT 'UTC',
    format          VARCHAR(10) NOT NULL DEFAULT 'pdf',
    recipients      TEXT[]      NOT NULL,
    channel_id      UUID REFERENCES notification_channels (id),
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    last_run_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_report_sched_ws ON report_schedules (workspace_id);
CREATE INDEX IF NOT EXISTS idx_report_sched_enabled ON report_schedules (enabled) WHERE enabled = true;

CREATE TABLE IF NOT EXISTS export_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID        NOT NULL,
    user_id         TEXT        NOT NULL REFERENCES users (id),
    export_type     VARCHAR(20) NOT NULL,       -- logs|metrics|report
    query_params    JSONB       NOT NULL,
    format          VARCHAR(10) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending|processing|completed|failed
    row_count       BIGINT,
    file_path       TEXT,                       -- S3 key (dev: local path)
    file_size_bytes BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_export_jobs_ws ON export_jobs (workspace_id, created_at DESC);
-- Partial index cho worker claim (status='pending', SKIP LOCKED).
CREATE INDEX IF NOT EXISTS idx_export_jobs_pending ON export_jobs (created_at) WHERE status = 'pending';
