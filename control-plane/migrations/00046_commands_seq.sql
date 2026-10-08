-- +goose Up
-- Actual entry order in the command queue (#28).
--
-- created_at doesn't work for ordering: it's now(), the START of the transaction, so commands
-- queued in the same transaction tie. Claim ordered by it and still returned the
-- rows via the RETURNING of an UPDATE, with no guaranteed order — the Agent, which executes serially
-- in the received order, applied add → pause → resume → remove as resume, remove, add,
-- pause. seq is assigned to each INSERT, in the order the command enters.
--
-- Rows that already exist receive seq in the order they are on disk; they are history
-- (DONE/FAILED) or pending items from seconds ago, and the order between them no longer matters
-- as soon as they are delivered.
ALTER TABLE commands
    ADD COLUMN seq BIGINT GENERATED ALWAYS AS IDENTITY;

-- The Claim filters by Node and status and orders by seq.
CREATE INDEX idx_commands_node_status_seq ON commands (node_id, status, seq);
DROP INDEX IF EXISTS idx_commands_node_status;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_commands_node_status ON commands (node_id, status);
DROP INDEX IF EXISTS idx_commands_node_status_seq;
ALTER TABLE commands DROP COLUMN IF EXISTS seq;
