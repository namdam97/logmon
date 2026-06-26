-- 000005_refresh_tokens: refresh-token rotation cho identity BC (GĐ2.5b, ADR-023).
-- Chỉ lưu SHA-256(token) — KHÔNG lưu token thô. family_id gom các token cùng một
-- chuỗi rotation; phát hiện reuse (dùng lại token đã rotate) → revoke cả family.
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id   UUID         NOT NULL,
    token_hash  VARCHAR(64)  NOT NULL UNIQUE,    -- hex sha256
    used_at     TIMESTAMPTZ,                     -- NULL = chưa rotate
    expires_at  TIMESTAMPTZ  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_user_family ON refresh_tokens (user_id, family_id);
