-- 000011_workspaces: multi-tenancy GĐ3.6 — workspaces, members (RBAC), audit log.
-- workspace_id ở các BC là UUID; users.id là TEXT → member.user_id TEXT (FK users).

CREATE TABLE IF NOT EXISTS workspaces (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    slug        VARCHAR(100) NOT NULL UNIQUE,   -- namespace ES data stream
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS workspace_members (
    workspace_id  UUID        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    user_id       TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role          VARCHAR(20) NOT NULL DEFAULT 'viewer',  -- viewer|editor|admin|platform_admin
    joined_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_wm_user ON workspace_members (user_id);

-- audit immutable (không UPDATE/DELETE; partition theo tháng khi lớn — GĐ4).
CREATE TABLE IF NOT EXISTS audit_logs (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id    UUID,
    user_id         TEXT,
    action          VARCHAR(50)  NOT NULL,       -- 'member.update', 'workspace.create'...
    resource_type   VARCHAR(50)  NOT NULL,
    resource_id     VARCHAR(100) NOT NULL,
    details         JSONB,
    ip_address      INET,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_ws_time ON audit_logs (workspace_id, created_at DESC);

-- Seed workspace mặc định (đã được các BC GĐ2 dùng làm _defaultWorkspaceID) để
-- dữ liệu hiện hữu vẫn hợp lệ và có thể gán thành viên.
INSERT INTO workspaces (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default', 'default')
ON CONFLICT (id) DO NOTHING;
