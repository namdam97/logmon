-- 000002_outbox: transactional outbox (ADR-016). Event ghi cùng TX với
-- state change; relay quét pending bằng FOR UPDATE SKIP LOCKED.
CREATE TABLE IF NOT EXISTS outbox_events (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    aggregate_type  VARCHAR(50)  NOT NULL,
    aggregate_id    UUID         NOT NULL,
    event_type      VARCHAR(50)  NOT NULL,
    payload         JSONB        NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending', -- pending|published|failed
    retry_count     INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ
);

-- Partial index: relay chỉ quét status='pending'.
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_events (id) WHERE status = 'pending';
