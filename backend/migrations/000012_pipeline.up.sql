-- 000012_pipeline: logpipeline management GĐ3.7 — pipeline_configs + dlq_entries.
-- workspace_id UUID (khớp BC khác); updated_by TEXT (khớp users.id TEXT).

CREATE TABLE IF NOT EXISTS pipeline_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL UNIQUE,
    mode            VARCHAR(1)  NOT NULL DEFAULT 'A' CHECK (mode IN ('A','B')),
    ilm_hot_days    INT NOT NULL DEFAULT 7,
    ilm_warm_days   INT NOT NULL DEFAULT 30,
    ilm_delete_days INT NOT NULL DEFAULT 90,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      TEXT REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS dlq_entries (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id    UUID NOT NULL,
    raw_message     TEXT NOT NULL,
    error_reason    TEXT NOT NULL,
    source_service  VARCHAR(100),
    kafka_meta      JSONB,                                   -- topic/partition/offset (Mode B)
    retry_count     INT NOT NULL DEFAULT 0,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending|retried|discarded
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    retried_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dlq_ws_status ON dlq_entries (workspace_id, status);
