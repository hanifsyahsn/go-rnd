package testhelper

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// SeededUser is one users row, as read back by tests.
type SeededUser struct {
	ID           int64
	Name         string
	Email        string
	PasswordHash string
	Role         string
	IsActive     bool
}

// GetUser reads a user row.
func GetUser(t *testing.T, db *sql.DB, id int64) SeededUser {
	t.Helper()

	var u SeededUser
	err := db.QueryRow(`
		SELECT id, name, email, password_hash, role, is_active
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive)
	require.NoError(t, err, "get user %d", id)
	return u
}

// SeededRefreshToken is one refresh_tokens row, as read back by tests.
type SeededRefreshToken struct {
	Jti        uuid.UUID
	UserID     int64
	FamilyID   uuid.UUID
	ExpiresAt  time.Time
	RevokedAt  sql.NullTime
	ReplacedBy uuid.NullUUID
	// SessionExpiresAt is the login's cap (untrusted device); NULL = none.
	SessionExpiresAt sql.NullTime
}

// GetRefreshToken reads a refresh_tokens row.
func GetRefreshToken(t *testing.T, db *sql.DB, jti uuid.UUID) SeededRefreshToken {
	t.Helper()

	var r SeededRefreshToken
	err := db.QueryRow(`
		SELECT jti, user_id, family_id, expires_at, revoked_at, replaced_by, session_expires_at
		FROM refresh_tokens WHERE jti = $1
	`, jti).Scan(&r.Jti, &r.UserID, &r.FamilyID, &r.ExpiresAt, &r.RevokedAt, &r.ReplacedBy, &r.SessionExpiresAt)
	require.NoError(t, err, "get refresh token %s", jti)
	return r
}
