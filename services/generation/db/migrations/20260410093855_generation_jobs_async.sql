-- +goose Up
-- +goose StatementBegin
ALTER TABLE generations
  ADD COLUMN job_id TEXT,
  ADD COLUMN status TEXT NOT NULL DEFAULT 'completed',
  ADD COLUMN audio_url TEXT,
  ADD COLUMN error_message TEXT,
  ADD COLUMN queued_at TIMESTAMPTZ,
  ADD COLUMN started_at TIMESTAMPTZ,
  ADD COLUMN completed_at TIMESTAMPTZ;

UPDATE generations
SET
  job_id = id,
  queued_at = COALESCE(created_at, NOW()),
  completed_at = CASE
    WHEN s3_object_key IS NOT NULL THEN COALESCE(updated_at, NOW())
    ELSE NULL
  END,
  status = CASE
    WHEN s3_object_key IS NOT NULL THEN 'completed'
    ELSE 'queued'
  END;

ALTER TABLE generations
  ALTER COLUMN job_id SET NOT NULL;

CREATE UNIQUE INDEX idx_generations_job_id ON generations(job_id);
CREATE INDEX idx_generations_status ON generations(status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_generations_status;
DROP INDEX IF EXISTS idx_generations_job_id;

ALTER TABLE generations
  DROP COLUMN IF EXISTS completed_at,
  DROP COLUMN IF EXISTS started_at,
  DROP COLUMN IF EXISTS queued_at,
  DROP COLUMN IF EXISTS error_message,
  DROP COLUMN IF EXISTS audio_url,
  DROP COLUMN IF EXISTS status,
  DROP COLUMN IF EXISTS job_id;
-- +goose StatementEnd
