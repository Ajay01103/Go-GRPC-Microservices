-- +goose Up
-- +goose StatementBegin
CREATE TYPE voice_category AS ENUM ('GENERAL', 'NARRATION', 'CHARACTER');
CREATE TYPE voice_variant AS ENUM ('MALE', 'FEMALE', 'NEUTRAL');

CREATE TABLE voices (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL,
  owner_type    TEXT NOT NULL,
  owner_id      TEXT,
  name          TEXT NOT NULL,
  description   TEXT,
  category      voice_category NOT NULL DEFAULT 'GENERAL',
  language      TEXT NOT NULL DEFAULT 'en-US',
  variant       voice_variant NOT NULL,
  s3_object_key TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_voices_owner_model CHECK (
    owner_type IN ('SYSTEM', 'USER')
    AND (
      (owner_type = 'SYSTEM' AND owner_id IS NULL)
      OR
      (owner_type = 'USER' AND owner_id IS NOT NULL)
    )
  )
);

CREATE INDEX idx_voices_user_id ON voices(user_id);
CREATE INDEX idx_voices_variant ON voices(variant);
CREATE INDEX idx_voices_owner_type_owner_id ON voices(owner_type, owner_id);
CREATE INDEX idx_voices_owner_created_at_desc ON voices(owner_type, owner_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS voices;
DROP TYPE voice_category;
DROP TYPE voice_variant;
-- +goose StatementEnd
