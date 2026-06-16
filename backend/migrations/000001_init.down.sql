-- Rollback 000001_init. DROP TABLE cũng xoá index idx_users_email kèm theo.
DROP TABLE IF EXISTS users;
