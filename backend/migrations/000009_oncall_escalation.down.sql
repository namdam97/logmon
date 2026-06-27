-- Rollback 000009_oncall_escalation.
DROP TABLE IF EXISTS incident_escalations;
DROP TABLE IF EXISTS escalation_policies;
DROP TABLE IF EXISTS oncall_overrides;
DROP TABLE IF EXISTS oncall_schedules;
