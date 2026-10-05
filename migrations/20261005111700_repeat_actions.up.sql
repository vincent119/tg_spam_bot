CREATE TABLE IF NOT EXISTS repeat_action_executions (
    event_id VARCHAR(128) PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    content_fingerprint TEXT NOT NULL,
    window_millis BIGINT NOT NULL,
    count BIGINT NOT NULL,
    truncated BOOLEAN NOT NULL,
    action VARCHAR(16) NOT NULL,
    mode VARCHAR(16) NOT NULL,
    policy_version VARCHAR(32) NOT NULL,
    steps JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_repeat_execution_scope
    ON repeat_action_executions (chat_id, user_id);

CREATE TABLE IF NOT EXISTS repeat_action_steps (
    action_key VARCHAR(200) PRIMARY KEY,
    first_event_id VARCHAR(128) NOT NULL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    kind VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL,
    retryable BOOLEAN NOT NULL,
    error_code VARCHAR(64) NOT NULL,
    error_text VARCHAR(500) NOT NULL,
    attempt_count BIGINT NOT NULL,
    had_unknown_outcome BOOLEAN NOT NULL,
    lease_token VARCHAR(32) NOT NULL,
    claimed_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_repeat_delete_target
    ON repeat_action_steps (chat_id, message_id)
    WHERE kind = 'delete';

COMMENT ON TABLE repeat_action_executions IS '精確重複處置的不可變事件快照';
COMMENT ON TABLE repeat_action_steps IS '精確重複處置的逐項 Telegram 動作';
