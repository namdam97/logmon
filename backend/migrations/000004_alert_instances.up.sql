-- 000004_alert_instances: instances firing/resolved nhận từ Alertmanager webhook
-- (alerting BC GĐ2.3). Khớp doc_v2/08 §3. Như alert_rules, GĐ2 KHÔNG đặt FK
-- (workspace_id/rule_id/acknowledged_by là UUID trần) — FK + RBAC thêm ở GĐ3.
-- rule_id để NULL ở GĐ2 (link rule ↔ instance làm ở GĐ3); labels JSONB giữ đủ
-- alertname/service/severity cho hiển thị + history.
CREATE TABLE IF NOT EXISTS alert_instances (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         UUID,                                   -- nullable: link ở GĐ3 (giữ history khi rule xóa)
    workspace_id    UUID         NOT NULL,
    fingerprint     VARCHAR(64)  NOT NULL,                  -- từ Alertmanager — khóa dedup
    status          VARCHAR(20)  NOT NULL DEFAULT 'firing'
                    CHECK (status IN ('firing', 'acknowledged', 'resolved')),
    fired_at        TIMESTAMPTZ  NOT NULL,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID,                                   -- ack ở GĐ2.4
    resolved_at     TIMESTAMPTZ,
    value           DOUBLE PRECISION,
    labels          JSONB        NOT NULL DEFAULT '{}',
    UNIQUE (fingerprint, fired_at)                          -- idempotency cho webhook lặp
);

CREATE INDEX IF NOT EXISTS idx_instances_ws_status ON alert_instances (workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_instances_rule ON alert_instances (rule_id, fired_at DESC);
