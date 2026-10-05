ALTER TABLE detection_events
    ADD COLUMN IF NOT EXISTS username TEXT,
    ADD COLUMN IF NOT EXISTS first_name TEXT,
    ADD COLUMN IF NOT EXISTS message_sent_at TIMESTAMPTZ;

COMMENT ON COLUMN detection_events.username IS 'Telegram 發送當時 username 快照';
COMMENT ON COLUMN detection_events.first_name IS 'Telegram 發送當時 first_name 快照';
COMMENT ON COLUMN detection_events.message_sent_at IS 'Telegram 原始訊息發送 UTC 時間';
