-- +goose Up
-- +goose StatementBegin
ALTER TABLE voices
  ADD COLUMN IF NOT EXISTS owner_type TEXT,
  ADD COLUMN IF NOT EXISTS owner_id TEXT;

UPDATE voices
SET owner_type = CASE
      WHEN user_id = 'SYSTEM' THEN 'SYSTEM'
      ELSE 'USER'
    END,
    owner_id = CASE
      WHEN user_id = 'SYSTEM' THEN NULL
      ELSE user_id
    END
WHERE owner_type IS NULL;

ALTER TABLE voices
  ALTER COLUMN owner_type SET NOT NULL;

ALTER TABLE voices
  DROP CONSTRAINT IF EXISTS chk_voices_owner_model,
  ADD CONSTRAINT chk_voices_owner_model CHECK (
    (owner_type = 'SYSTEM' AND owner_id IS NULL)
    OR
    (owner_type = 'USER' AND owner_id IS NOT NULL)
  );

CREATE INDEX IF NOT EXISTS idx_voices_owner_type_owner_id
  ON voices(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_voices_owner_created_at_desc
  ON voices(owner_type, owner_id, created_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_voices_owner_created_at_desc;
DROP INDEX IF EXISTS idx_voices_owner_type_owner_id;

ALTER TABLE voices
  DROP CONSTRAINT IF EXISTS chk_voices_owner_model;

ALTER TABLE voices
  DROP COLUMN IF EXISTS owner_id,
  DROP COLUMN IF EXISTS owner_type;
-- +goose StatementEnd
