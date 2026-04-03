package repository

import (
	"context"
	"errors"
	"fmt"

	db "github.com/go-grpc-sqlc/voice/gen/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrVoiceNotFound is returned when a voice lookup yields no result.
var ErrVoiceNotFound = errors.New("voice not found")

// ListVoicesParams bundles parameters for listing a user's voices.
type ListVoicesParams struct {
	UserID string
	Query  string // empty = no search filter
}

// VoiceRepo wraps the sqlc Querier to provide a clean repository interface.
type VoiceRepo struct {
	q db.Querier
}

// NewVoiceRepo creates a VoiceRepo backed by a sqlc Querier.
func NewVoiceRepo(q db.Querier) *VoiceRepo {
	return &VoiceRepo{q: q}
}

// ListVoices returns all custom voices for a user, optionally filtered by query.
func (r *VoiceRepo) ListVoices(ctx context.Context, params ListVoicesParams) ([]db.ListCustomVoicesRow, error) {
	if params.Query != "" {
		searchRows, err := r.q.ListCustomVoicesSearch(ctx, db.ListCustomVoicesSearchParams{
			UserID: params.UserID,
			Column2: pgtype.Text{
				String: params.Query,
				Valid:  true,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("repository: list voices search: %w", err)
		}

		// Map search results to the expected row type
		rows := make([]db.ListCustomVoicesRow, len(searchRows))
		for i, v := range searchRows {
			rows[i] = db.ListCustomVoicesRow{
				ID:          v.ID,
				Name:        v.Name,
				Description: v.Description,
				Category:    v.Category,
				Language:    v.Language,
				Variant:     v.Variant,
			}
		}
		return rows, nil
	}

	rows, err := r.q.ListCustomVoices(ctx, params.UserID)
	if err != nil {
		return nil, fmt.Errorf("repository: list voices: %w", err)
	}
	return rows, nil
}

// GetVoiceByIDAndUser fetches a voice owned by the specified user, or ErrVoiceNotFound.
func (r *VoiceRepo) GetVoiceByIDAndUser(ctx context.Context, id, userID string) (db.Voice, error) {
	voice, err := r.q.GetVoiceByIDAndUser(ctx, db.GetVoiceByIDAndUserParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Voice{}, ErrVoiceNotFound
		}
		return db.Voice{}, fmt.Errorf("repository: get voice: %w", err)
	}
	return voice, nil
}

// DeleteVoice removes a voice record owned by the given user.
func (r *VoiceRepo) DeleteVoice(ctx context.Context, id, userID string) error {
	if err := r.q.DeleteVoice(ctx, db.DeleteVoiceParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("repository: delete voice: %w", err)
	}
	return nil
}
