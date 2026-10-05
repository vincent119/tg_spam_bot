ALTER TABLE detection_events
    DROP COLUMN IF EXISTS message_sent_at,
    DROP COLUMN IF EXISTS first_name,
    DROP COLUMN IF EXISTS username;
