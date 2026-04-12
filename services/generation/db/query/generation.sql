-- name: GetGenerationByIDAndUser :one
SELECT *
FROM generations
WHERE id = $1 AND user_id = $2
LIMIT 1;

-- name: GetGenerationByJobIDAndUser :one
SELECT *
FROM generations
WHERE job_id = $1 AND user_id = $2
LIMIT 1;

-- name: ListGenerationsByUser :many
SELECT *
FROM generations
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: CreateGenerationJob :one
INSERT INTO generations (
	id,
	job_id,
	user_id,
	voice_id,
	voice_name,
	text,
	temperature,
	language_id,
	exaggeration,
	cfg_weight,
	status,
	queued_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
RETURNING *;

-- name: MarkGenerationProcessing :exec
UPDATE generations
SET
	status = 'processing',
	started_at = NOW(),
	updated_at = NOW(),
	error_message = NULL
WHERE id = $1 AND status = 'queued';

-- name: MarkGenerationCompleted :exec
UPDATE generations
SET
	status = 'completed',
	audio_url = $2,
	s3_object_key = $3,
	completed_at = NOW(),
	updated_at = NOW(),
	error_message = NULL
WHERE id = $1;

-- name: MarkGenerationFailed :exec
UPDATE generations
SET
	status = 'failed',
	error_message = $2,
	completed_at = NOW(),
	updated_at = NOW()
WHERE id = $1;
