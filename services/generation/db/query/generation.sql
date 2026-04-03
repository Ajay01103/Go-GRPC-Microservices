-- name: GetGeneration :one
SELECT * FROM generations WHERE id = $1 LIMIT 1;
