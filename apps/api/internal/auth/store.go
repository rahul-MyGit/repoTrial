package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnauthenticated = errors.New("session not found")

type User struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type Identity struct {
	Subject     string
	Email       string
	DisplayName string
	AvatarURL   *string
}

type Repository interface {
	CreateSession(context.Context, Identity, [32]byte, [32]byte, time.Time) (User, error)
	GetSession(context.Context, [32]byte) (User, [32]byte, error)
	RevokeSession(context.Context, [32]byte) error
}

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateSession(
	ctx context.Context,
	identity Identity,
	tokenHash [32]byte,
	csrfHash [32]byte,
	expiresAt time.Time,
) (User, error) {
	var user User
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `INSERT INTO users (google_subject, email, display_name, avatar_url)
VALUES ($1, $2, $3, $4)
ON CONFLICT (google_subject) DO UPDATE SET
 email = EXCLUDED.email, display_name = EXCLUDED.display_name,
 avatar_url = EXCLUDED.avatar_url, updated_at = now()
RETURNING id::text, display_name, avatar_url`,
		identity.Subject,
		identity.Email,
		identity.DisplayName,
		identity.AvatarURL,
	).Scan(&user.ID, &user.DisplayName, &user.AvatarURL)
	if err != nil {
		return User{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, csrf_token_hash, expires_at)
VALUES ($1::uuid, $2, $3, $4)`, user.ID, tokenHash[:], csrfHash[:], expiresAt)
	if err != nil {
		return User{}, err
	}
	return user, tx.Commit(ctx)
}

func (r *PostgresRepository) GetSession(ctx context.Context, tokenHash [32]byte) (User, [32]byte, error) {
	var user User
	var csrfBytes []byte
	err := r.pool.QueryRow(ctx, `SELECT u.id::text, u.display_name, u.avatar_url, s.csrf_token_hash
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()`, tokenHash[:]).Scan(
		&user.ID,
		&user.DisplayName,
		&user.AvatarURL,
		&csrfBytes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, [32]byte{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, [32]byte{}, err
	}
	var csrfHash [32]byte
	if len(csrfBytes) != sha256.Size {
		return User{}, [32]byte{}, errors.New("invalid stored CSRF hash")
	}
	copy(csrfHash[:], csrfBytes)
	return user, csrfHash, nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, tokenHash [32]byte) error {
	tag, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, tokenHash[:])
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthenticated
	}
	return nil
}
