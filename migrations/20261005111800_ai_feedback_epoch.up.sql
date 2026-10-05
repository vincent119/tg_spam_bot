ALTER TABLE ai_detection_events
    ADD COLUMN IF NOT EXISTS feedback_epoch BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_ai_feedback_cache
    ON ai_detection_events (
        chat_id, feedback_epoch, content_fingerprint, provider, model,
        prompt_version, rule_version, created_at DESC
    ) WHERE status = 'completed';

COMMENT ON COLUMN ai_detection_events.feedback_epoch IS '判定時群組人工標記版本';
