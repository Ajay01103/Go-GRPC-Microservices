-- +goose Up
-- +goose StatementBegin
CREATE TABLE generations (
  id                 TEXT PRIMARY KEY,
  user_id            TEXT NOT NULL,
  voice_id           TEXT,           -- soft ref, no FK
  voice_name         TEXT NOT NULL,  -- snapshotted at creation time
  text               TEXT NOT NULL,
  r2_object_key      TEXT,
  temperature        FLOAT NOT NULL,
  top_p              FLOAT NOT NULL,
  top_k              INT NOT NULL,
  repetition_penalty FLOAT NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_generations_user_id ON generations(user_id);
CREATE INDEX idx_generations_voice_id ON generations(voice_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE generations;
-- +goose StatementEnd
