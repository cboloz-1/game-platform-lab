ALTER TABLE events
    DROP COLUMN status,
    DROP COLUMN attempt_count,
    DROP COLUMN last_error,
    DROP COLUMN next_attempt_at;