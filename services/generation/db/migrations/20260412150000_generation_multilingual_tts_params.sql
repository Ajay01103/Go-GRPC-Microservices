-- +goose Up
-- +goose StatementBegin
ALTER TABLE generations
  ADD COLUMN language_id TEXT NOT NULL DEFAULT 'en',
  ADD COLUMN exaggeration FLOAT NOT NULL DEFAULT 0.5,
  ADD COLUMN cfg_weight FLOAT NOT NULL DEFAULT 0.5;

ALTER TABLE generations
  DROP COLUMN IF EXISTS top_p,
  DROP COLUMN IF EXISTS top_k,
  DROP COLUMN IF EXISTS repetition_penalty;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE generations
  ADD COLUMN top_p FLOAT NOT NULL DEFAULT 0.95,
  ADD COLUMN top_k INT NOT NULL DEFAULT 1000,
  ADD COLUMN repetition_penalty FLOAT NOT NULL DEFAULT 1.2;

ALTER TABLE generations
  DROP COLUMN IF EXISTS language_id,
  DROP COLUMN IF EXISTS exaggeration,
  DROP COLUMN IF EXISTS cfg_weight;
-- +goose StatementEnd
