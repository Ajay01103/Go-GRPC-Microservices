package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/go-grpc-sqlc/auth/gen/sqlc"
)

// ErrUserNotFound is returned when a user lookup yields no result.
var ErrUserNotFound = errors.New("user not found")

// ErrEmailTaken is returned when registering with an already-used email.
var ErrEmailTaken = errors.New("email already in use")

// ErrNameTaken is returned when registering with an already-used name.
var ErrNameTaken = errors.New("name already in use")

// UserRepo wraps the sqlc Querier to provide a clean repository interface.
type UserRepo struct {
	q db.Querier
}

// NewUserRepo creates a UserRepo backed by a sqlc Querier.
func NewUserRepo(q db.Querier) *UserRepo {
	return &UserRepo{q: q}
}

// CreateUser inserts a new user record. Detects unique-constraint violations.
func (r *UserRepo) CreateUser(ctx context.Context, email, name, hashedPassword string) (db.User, error) {
	user, err := r.q.CreateUser(ctx, db.CreateUserParams{
		ID:       uuid.New().String(),
		Email:    email,
		Name:     name,
		Password: hashedPassword,
	})
	if err != nil {
		// pgx returns the constraint name inside the error message for unique violations
		errMsg := err.Error()
		if containsAny(errMsg, "users_email_key", "unique", "email") {
			return db.User{}, ErrEmailTaken
		}
		if containsAny(errMsg, "users_name_key", "unique", "name") {
			return db.User{}, ErrNameTaken
		}
		return db.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// GetByEmail fetches a user by email.
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (db.User, error) {
	user, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, ErrUserNotFound
		}
		return db.User{}, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

// GetByID fetches a user by UUID.
func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (db.User, error) {
	user, err := r.q.GetUserByID(ctx, id.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, ErrUserNotFound
		}
		return db.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}

// containsAny is a simple multi-string containment check.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
