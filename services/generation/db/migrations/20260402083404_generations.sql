-- +goose Up
-- +goose StatementBegin
CREATE TABLE generations (
  id                 TEXT PRIMARY KEY,
  user_id            TEXT NOT NULL,
  voice_id           TEXT,
  voice_name         TEXT NOT NULL,
  voice_key          TEXT NOT NULL,
  text               TEXT NOT NULL,
  s3_object_key      TEXT,
  temperature        FLOAT NOT NULL,
  language_id        TEXT NOT NULL DEFAULT 'en',
  exaggeration       FLOAT NOT NULL DEFAULT 0.5,
  cfg_weight         FLOAT NOT NULL DEFAULT 0.5,
  job_id             TEXT NOT NULL UNIQUE,
  status             TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
  error_message      TEXT,
  queued_at          TIMESTAMPTZ,
  started_at         TIMESTAMPTZ,
  completed_at       TIMESTAMPTZ,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_generations_user_id ON generations(user_id);
CREATE INDEX idx_generations_voice_id ON generations(voice_id);
CREATE UNIQUE INDEX idx_generations_job_id ON generations(job_id);
CREATE INDEX idx_generations_status ON generations(status);
CREATE INDEX idx_generations_user_created_at_desc ON generations(user_id, created_at DESC);
CREATE INDEX idx_generations_user_status ON generations(user_id, status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE generations;
-- +goose StatementEnd
