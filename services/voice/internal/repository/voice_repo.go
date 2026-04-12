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

const systemUserID = "SYSTEM"

// ListScope controls which logical voice set is returned.
type ListScope string

const (
	ListScopeCustom ListScope = "custom"
	ListScopeSystem ListScope = "system"
	ListScopeAll    ListScope = "all"
)

// ListVoicesParams bundles parameters for listing a user's voices.
type ListVoicesParams struct {
	UserID string
	Scope  ListScope
	Query  string // empty = no search filter
}

// CreateVoiceParams bundles parameters for creating a custom voice.
type CreateVoiceParams struct {
	ID          string
	UserID      string
	Name        string
	Description string
	Category    db.VoiceCategory
	Language    string
	Variant     db.VoiceVariant
	S3ObjectKey string
}

// UpdateVoiceParams bundles parameters for updating custom voice metadata.
type UpdateVoiceParams struct {
	ID          string
	UserID      string
	Name        string
	Description string
	Category    db.VoiceCategory
	Language    string
}

// Repository defines the data access contract for voices.
type Repository interface {
	ListVoices(ctx context.Context, params ListVoicesParams) ([]db.ListCustomVoicesRow, error)
	GetVoiceByID(ctx context.Context, id string) (db.Voice, error)
	GetVoiceByIDAndUser(ctx context.Context, id, userID string) (db.Voice, error)
	CreateVoice(ctx context.Context, params CreateVoiceParams) (db.Voice, error)
	UpdateVoice(ctx context.Context, params UpdateVoiceParams) (db.Voice, error)
	DeleteVoice(ctx context.Context, id, userID string) error
}

// VoiceRepo wraps the sqlc Querier to provide a clean repository interface.
type VoiceRepo struct {
	q db.Querier
}

// NewVoiceRepo creates a VoiceRepo backed by a sqlc Querier.
func NewVoiceRepo(q db.Querier) *VoiceRepo {
	return &VoiceRepo{q: q}
}

// ListVoices returns voices for the requested scope, optionally filtered by query.
func (r *VoiceRepo) ListVoices(ctx context.Context, params ListVoicesParams) ([]db.ListCustomVoicesRow, error) {
	scope := params.Scope
	if scope == "" {
		scope = ListScopeAll
	}

	switch scope {
	case ListScopeCustom:
		return r.listVoicesForUser(ctx, params.UserID, params.Query)
	case ListScopeSystem:
		return r.listVoicesForUser(ctx, systemUserID, params.Query)
	case ListScopeAll:
		if params.UserID == systemUserID {
			return r.listVoicesForUser(ctx, systemUserID, params.Query)
		}

		customRows, err := r.listVoicesForUser(ctx, params.UserID, params.Query)
		if err != nil {
			return nil, err
		}

		systemRows, err := r.listVoicesForUser(ctx, systemUserID, params.Query)
		if err != nil {
			return nil, err
		}

		rows := make([]db.ListCustomVoicesRow, 0, len(customRows)+len(systemRows))
		rows = append(rows, customRows...)
		rows = append(rows, systemRows...)
		return rows, nil
	default:
		return nil, fmt.Errorf("repository: unsupported list scope %q", scope)
	}
}

func (r *VoiceRepo) listVoicesForUser(ctx context.Context, userID, query string) ([]db.ListCustomVoicesRow, error) {
	if query != "" {
		searchRows, err := r.q.ListCustomVoicesSearch(ctx, db.ListCustomVoicesSearchParams{
			UserID: userID,
			Column2: pgtype.Text{
				String: query,
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

	rows, err := r.q.ListCustomVoices(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("repository: list voices: %w", err)
	}
	return rows, nil
}

// GetVoiceByID fetches a voice by id, or ErrVoiceNotFound if not present.
func (r *VoiceRepo) GetVoiceByID(ctx context.Context, id string) (db.Voice, error) {
	voice, err := r.q.GetVoice(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Voice{}, ErrVoiceNotFound
		}
		return db.Voice{}, fmt.Errorf("repository: get voice by id: %w", err)
	}
	return voice, nil
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

// CreateVoice creates a new custom voice record.
func (r *VoiceRepo) CreateVoice(ctx context.Context, params CreateVoiceParams) (db.Voice, error) {
	voice, err := r.q.CreateVoice(ctx, db.CreateVoiceParams{
		ID:     params.ID,
		UserID: params.UserID,
		Name:   params.Name,
		Description: pgtype.Text{
			String: params.Description,
			Valid:  params.Description != "",
		},
		Category: params.Category,
		Language: params.Language,
		Variant:  params.Variant,
		S3ObjectKey: pgtype.Text{
			String: params.S3ObjectKey,
			Valid:  params.S3ObjectKey != "",
		},
	})
	if err != nil {
		return db.Voice{}, fmt.Errorf("repository: create voice: %w", err)
	}

	return voice, nil
}

// UpdateVoice updates an existing custom voice metadata record.
func (r *VoiceRepo) UpdateVoice(ctx context.Context, params UpdateVoiceParams) (db.Voice, error) {
	voice, err := r.q.UpdateVoice(ctx, db.UpdateVoiceParams{
		ID:     params.ID,
		UserID: params.UserID,
		Name:   params.Name,
		Description: pgtype.Text{
			String: params.Description,
			Valid:  params.Description != "",
		},
		Category: params.Category,
		Language: params.Language,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Voice{}, ErrVoiceNotFound
		}
		return db.Voice{}, fmt.Errorf("repository: update voice: %w", err)
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
