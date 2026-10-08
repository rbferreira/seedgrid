-- +goose Up
-- The Audit action as code + parameters (#57, ADR-041 §3): the phrase is assembled by those who
-- read it, in their language. `action` (the Portuguese text) remains stored — it's the fallback for
-- those consuming the API without a catalog, and what applies to records prior to this column.
ALTER TABLE audit_log ADD COLUMN action_code TEXT;
ALTER TABLE audit_log ADD COLUMN action_params JSONB;

-- +goose Down
ALTER TABLE audit_log DROP COLUMN action_params;
ALTER TABLE audit_log DROP COLUMN action_code;
