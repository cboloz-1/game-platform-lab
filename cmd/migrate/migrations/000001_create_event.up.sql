CREATE TABLE IF NOT EXISTS events (
    event_id TEXT PRIMARY KEY,
    player_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    client_timestamp BIGINT NOT NULL CHECK (client_timestamp > 0),
    server_timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS events_player_id_idx ON events (player_id);