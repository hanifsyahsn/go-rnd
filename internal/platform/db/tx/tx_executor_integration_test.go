package tx

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/hanifsyahsn/go-rnd/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

// TestTx_Exec_I runs Exec against a real Postgres: what fn writes is
// committed when it returns nil and rolled back, as a whole, when it fails.
func TestTx_Exec_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	identityFactory := func(tx *sql.Tx) *sql.Tx { return tx }
	errBoom := errors.New("boom")

	// insertToken writes one refresh_tokens row through the transaction.
	insertToken := func(ctx context.Context, tx *sql.Tx, userID int64) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO refresh_tokens (jti, user_id, family_id, expires_at)
			VALUES (gen_random_uuid(), $1, gen_random_uuid(), now() + interval '1 hour')
		`, userID)
		return err
	}

	testCases := []struct {
		name     string
		fn       func(ctx context.Context, userID int64) func(tx *sql.Tx) error
		wantErr  error
		wantRows int
	}{
		{
			name: "fn succeeds: both writes are committed",
			fn: func(ctx context.Context, userID int64) func(tx *sql.Tx) error {
				return func(tx *sql.Tx) error {
					if err := insertToken(ctx, tx, userID); err != nil {
						return err
					}
					return insertToken(ctx, tx, userID)
				}
			},
			wantRows: 2,
		},
		{
			name: "fn fails after a write: the write is rolled back",
			fn: func(ctx context.Context, userID int64) func(tx *sql.Tx) error {
				return func(tx *sql.Tx) error {
					if err := insertToken(ctx, tx, userID); err != nil {
						return err
					}
					return errBoom
				}
			},
			wantErr:  errBoom,
			wantRows: 0,
		},
		{
			name: "a failing statement (FK violation) rolls back the earlier write",
			fn: func(ctx context.Context, userID int64) func(tx *sql.Tx) error {
				return func(tx *sql.Tx) error {
					if err := insertToken(ctx, tx, userID); err != nil {
						return err
					}
					return insertToken(ctx, tx, userID+1000)
				}
			},
			wantRows: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			ctx := context.Background()
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})

			err := Exec(ctx, testDB, identityFactory, tc.fn(ctx, userID))
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantRows == 0:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}

			var count int
			require.NoError(t, testDB.QueryRow(`SELECT count(*) FROM refresh_tokens`).Scan(&count))
			require.Equal(t, tc.wantRows, count)
		})
	}
}
