-- 000014_usage: GĐ4.5 — hạn mức tài nguyên per workspace (quota enforcement).
-- Usage thực tế đọc runtime từ Prometheus/ES (không lưu bảng).

CREATE TABLE IF NOT EXISTS workspace_quotas (
    workspace_id                UUID PRIMARY KEY,
    max_ingestion_bytes_per_day BIGINT      NOT NULL,
    max_storage_bytes           BIGINT      NOT NULL,
    retention_days              INT         NOT NULL,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);
