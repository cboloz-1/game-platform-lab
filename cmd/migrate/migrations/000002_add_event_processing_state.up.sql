ALTER TABLE events
    ADD COLUMN status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN last_error TEXT,
    ADD COLUMN next_attempt_at TIMESTAMPTZ;