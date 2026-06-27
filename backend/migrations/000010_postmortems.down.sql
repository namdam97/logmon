-- Rollback 000010_postmortems.
DROP INDEX IF EXISTS idx_incidents_resolved_postmortem;
DROP TABLE IF EXISTS action_items;
DROP TABLE IF EXISTS postmortems;
