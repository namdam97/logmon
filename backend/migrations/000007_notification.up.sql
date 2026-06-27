-- 000007_notification: notification BC (GĐ3). Kênh thông báo đa loại + lịch sử
-- gửi. config_encrypted lưu CẢ blob config (chứa secret) đã mã hóa AES-256-GCM
-- ("<keyID>:<base64>") — không lưu plaintext secret (doc_v2/09, hội đồng GĐ3).
-- workspace_id dùng workspace mặc định (multi-tenancy đầy đủ ở 3.6).

CREATE TABLE IF NOT EXISTS notification_channels (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     UUID         NOT NULL,
    name             VARCHAR(100) NOT NULL,
    channel_type     VARCHAR(20)  NOT NULL CHECK (channel_type IN ('slack', 'email', 'pagerduty', 'teams', 'webhook')),
    config_encrypted TEXT         NOT NULL,
    events           TEXT[]       NOT NULL,
    enabled          BOOLEAN      NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);

CREATE INDEX IF NOT EXISTS idx_notif_ch_ws_enabled ON notification_channels (workspace_id, enabled);

CREATE TABLE IF NOT EXISTS notification_history (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id  UUID        NOT NULL,
    channel_id    UUID        NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
    event_type    VARCHAR(50) NOT NULL,
    event_ref     TEXT        NOT NULL,
    status        VARCHAR(20) NOT NULL CHECK (status IN ('sent', 'failed', 'retrying')),
    response_code INT,
    error_message TEXT,
    sent_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notif_hist ON notification_history (workspace_id, sent_at DESC);
