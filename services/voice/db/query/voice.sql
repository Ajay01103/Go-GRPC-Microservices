-- name: ListCustomVoices :many
SELECT id, name, description, category, language, variant
FROM voices
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListCustomVoicesSearch :many
SELECT id, name, description, category, language, variant
FROM voices
WHERE user_id = $1
  AND (
    name        ILIKE '%' || $2 || '%'
    OR description ILIKE '%' || $2 || '%'
  )
ORDER BY created_at DESC;

-- name: GetVoice :one
SELECT id, user_id, name, description, category, language, variant, s3_object_key, created_at, updated_at
FROM voices
WHERE id = $1
LIMIT 1;

-- name: GetVoiceByIDAndUser :one
SELECT id, user_id, name, description, category, language, variant, s3_object_key, created_at, updated_at
FROM voices
WHERE id = $1
  AND user_id = $2
LIMIT 1;

-- name: CreateVoice :one
INSERT INTO voices (id, user_id, name, description, category, language, variant, s3_object_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, user_id, name, description, category, language, variant, s3_object_key, created_at, updated_at;

-- name: DeleteVoice :exec
DELETE FROM voices
WHERE id = $1
  AND user_id = $2;

-- name: GetSystemVoiceByName :one
SELECT * FROM voices
WHERE name = $1 AND user_id = 'SYSTEM'
LIMIT 1;

-- name: UpdateSystemVoiceS3Key :exec
UPDATE voices
SET s3_object_key = $2,
    description = $3,
    category = $4,
    language = $5,
    updated_at = NOW()
WHERE id = $1;

-- name: DeleteSystemVoiceByID :exec
DELETE FROM voices
WHERE id = $1 AND user_id = 'SYSTEM';
