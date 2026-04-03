-- +goose Up
-- +goose StatementBegin
CREATE TYPE voice_category AS ENUM ('GENERAL', 'NARRATION', 'CHARACTER');
CREATE TYPE voice_variant AS ENUM ('MALE', 'FEMALE', 'NEUTRAL');

CREATE TABLE voices (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  name          TEXT NOT NULL,
  description   TEXT,
  category      voice_category NOT NULL DEFAULT 'GENERAL',
  language      TEXT NOT NULL DEFAULT 'en-US',
  variant       voice_variant NOT NULL,
  s3_object_key TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_voices_user_id ON voices(user_id);
CREATE INDEX idx_voices_variant ON voices(variant);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE voices;
DROP TYPE voice_category;
DROP TYPE voice_variant;
-- +goose StatementEnd
