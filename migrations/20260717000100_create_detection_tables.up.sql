CREATE TABLE processed_updates (
    update_id BIGINT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('processing', 'completed')),
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE detection_events (
    event_id TEXT PRIMARY KEY,
    update_id BIGINT NOT NULL UNIQUE,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    content_fingerprint TEXT NOT NULL,
    category_id TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT '',
    score INTEGER NOT NULL DEFAULT 0,
    threshold INTEGER NOT NULL DEFAULT 0,
    rule_version TEXT NOT NULL,
    mode TEXT NOT NULL,
    is_spam BOOLEAN NOT NULL,
    matches JSONB NOT NULL DEFAULT '[]',
    signals JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE violations (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE REFERENCES detection_events(event_id),
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    category_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX violations_active_idx ON violations (chat_id, user_id, occurred_at DESC);

CREATE TABLE enforcement_actions (
    action_key TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES detection_events(event_id),
    kind TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    retryable BOOLEAN NOT NULL DEFAULT false,
    error_code TEXT NOT NULL DEFAULT '',
    error_text TEXT NOT NULL DEFAULT '',
    ended_at TIMESTAMPTZ
);

CREATE TABLE trusted_members (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    reason TEXT NOT NULL DEFAULT 'trusted',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, user_id)
);
