-- +goose Up
-- Node BitTorrent endpoint: listening port (reported by the Agent) and last source IP
-- (seen by the Control Plane in the heartbeat). The operator uses IP:port to
-- configure port forwarding manually (SeedGrid is firewall-agnostic).
ALTER TABLE nodes
    ADD COLUMN bt_port INTEGER,
    ADD COLUMN last_ip TEXT;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN bt_port,
    DROP COLUMN last_ip;
