DROP INDEX IF EXISTS idx_ai_feedback_cache;

ALTER TABLE ai_detection_events
    DROP COLUMN IF EXISTS feedback_epoch;
