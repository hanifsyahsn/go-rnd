package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authlogin "github.com/hanifsyahsn/go-rnd/internal/features/auth/login"
	authloginsqlc "github.com/hanifsyahsn/go-rnd/internal/features/auth/login/sqlc"
	authlogout "github.com/hanifsyahsn/go-rnd/internal/features/auth/logout"
	authrefresh "github.com/hanifsyahsn/go-rnd/internal/features/auth/refresh"
	authregister "github.com/hanifsyahsn/go-rnd/internal/features/auth/register"
	authregistersqlc "github.com/hanifsyahsn/go-rnd/internal/features/auth/register/sqlc"
	authshared "github.com/hanifsyahsn/go-rnd/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go-rnd/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go-rnd/internal/features/me"
	mesqlc "github.com/hanifsyahsn/go-rnd/internal/features/me/sqlc"
	"github.com/hanifsyahsn/go-rnd/internal/platform/jwttoken"
	middlewaresqlc "github.com/hanifsyahsn/go-rnd/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go-rnd/internal/shared/testhelper"
	"github.com/hanifsyahsn/go-rnd/internal/shared/validation"
	"github.com/stretchr/testify/require"
)

// newIntegrationRouter wires every slice exactly like cmd/api/main.go, but on
// the test container's database and a throwaway ES256 key.
func newIntegrationRouter(t *testing.T, db *sql.DB) *gin.Engine {
	t.Helper()

	require.NoError(t, validation.RegisterGinValidators())

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tokenMaker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "test-client", "test-issuer")

	const accessTTL, refreshTTL, untrustedTTL = 15 * time.Minute, 24 * time.Hour, 24 * time.Hour
	cookies := authshared.CookieConfig{}
	authSharedQuerier := authsharedsqlc.New(db)

	handlers := Handlers{
		Register: authregister.NewHandler(authregister.NewService(authregistersqlc.New(db))),
		Login: authlogin.NewHandler(
			authlogin.NewService(authloginsqlc.New(db), authSharedQuerier, tokenMaker, accessTTL, refreshTTL, untrustedTTL),
			cookies,
		),
		Refresh: authrefresh.NewHandler(
			authrefresh.NewService(authrefresh.NewStore(db), tokenMaker, accessTTL, refreshTTL),
			cookies,
		),
		Logout: authlogout.NewHandler(authlogout.NewService(authSharedQuerier, tokenMaker), cookies),

		Me: me.NewHandler(me.NewService(mesqlc.New(db))),
	}

	return NewRouter("test-version", tokenMaker, middlewaresqlc.New(db), handlers)
}

// apiClient sends JSON:API requests to the router in-process.
type apiClient struct {
	t      *testing.T
	router *gin.Engine
}

type apiResponse struct {
	Code   int
	Header http.Header
	Body   map[string]any
}

// do sends one request; body (when set) goes as application/vnd.api+json and
// token (when set) as a bearer access token.
func (c apiClient) do(method, path, body, token string) apiResponse {
	c.t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Accept", "application/vnd.api+json")
	if body != "" {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)

	res := apiResponse{Code: rec.Code, Header: rec.Header()}
	if rec.Body.Len() > 0 {
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &res.Body), "body: %s", rec.Body.String())
	}
	return res
}

// login signs in with the seeded default password and returns the access token.
func (c apiClient) login(email string) string {
	c.t.Helper()
	access, _ := c.loginTokens(email)
	return access
}

// loginTokens signs in with the seeded default password and returns the
// access and refresh tokens from meta.
func (c apiClient) loginTokens(email string) (access, refresh string) {
	c.t.Helper()

	res := c.do(http.MethodPost, "/api/auth/login",
		`{"data":{"type":"sessions","attributes":{"email":"`+email+`","password":"`+testhelper.DefaultPassword+`"}}}`, "")
	require.Equal(c.t, http.StatusOK, res.Code, "login: %v", res.Body)
	return metaString(c.t, res, "access_token"), metaString(c.t, res, "refresh_token")
}

// metaString reads meta[name] as a string.
func metaString(t *testing.T, res apiResponse, name string) string {
	t.Helper()
	return res.Body["meta"].(map[string]any)[name].(string)
}

// dataAttr reads data.attributes[name] of a single-resource document.
func dataAttr(t *testing.T, res apiResponse, name string) any {
	t.Helper()
	return res.Body["data"].(map[string]any)["attributes"].(map[string]any)[name]
}

// sessionDoc is a sessions document carrying a refresh token (refresh / logout
// without the cookie).
func sessionDoc(refreshToken string) string {
	return `{"data":{"type":"sessions","attributes":{"refresh_token":"` + refreshToken + `"}}}`
}

// TestRouter_EndToEnd_I drives the real router, middleware, handlers,
// services and sqlc against a migrated Postgres container: every layer at
// once, over HTTP.
func TestRouter_EndToEnd_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	router := newIntegrationRouter(t, testDB)

	testCases := []struct {
		name string
		run  func(t *testing.T, c apiClient)
	}{
		{
			name: "register, login, me, then the session lifecycle: refresh → logout",
			run: func(t *testing.T, c apiClient) {
				res := c.do(http.MethodPost, "/api/auth/register",
					`{"data":{"type":"users","attributes":{"name":"Jane","email":"jane@example.com","password":"`+testhelper.DefaultPassword+`"}}}`, "")
				require.Equal(t, http.StatusCreated, res.Code, "register: %v", res.Body)
				require.Equal(t, "user", dataAttr(t, res, "role"))

				access, refresh := c.loginTokens("jane@example.com")

				res = c.do(http.MethodGet, "/api/me", "", access)
				require.Equal(t, http.StatusOK, res.Code)
				require.Equal(t, "jane@example.com", dataAttr(t, res, "email"))

				res = c.do(http.MethodPost, "/api/auth/refresh", sessionDoc(refresh), "")
				require.Equal(t, http.StatusOK, res.Code, "refresh: %v", res.Body)
				rotated := metaString(t, res, "refresh_token")
				require.NotEqual(t, refresh, rotated, "refresh rotates the refresh token")
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", metaString(t, res, "access_token")).Code)

				res = c.do(http.MethodPost, "/api/auth/logout", sessionDoc(rotated), "")
				require.Equal(t, http.StatusNoContent, res.Code)

				res = c.do(http.MethodPost, "/api/auth/refresh", sessionDoc(rotated), "")
				require.Equal(t, http.StatusUnauthorized, res.Code, "logout revokes the family")
			},
		},
		{
			name: "a user deactivated after login is rejected on the next request",
			run: func(t *testing.T, c apiClient) {
				id := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "gone@example.com"})
				token := c.login("gone@example.com")
				require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/api/me", "", token).Code)

				_, err := testDB.Exec(`UPDATE users SET is_active = false WHERE id = $1`, id)
				require.NoError(t, err)

				require.Equal(t, http.StatusUnauthorized, c.do(http.MethodGet, "/api/me", "", token).Code)
			},
		},
		{
			name: "no token is a 401",
			run: func(t *testing.T, c apiClient) {
				res := c.do(http.MethodGet, "/api/me", "", "")
				require.Equal(t, http.StatusUnauthorized, res.Code)
				require.NotEmpty(t, res.Body["errors"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			tc.run(t, apiClient{t: t, router: router})
		})
	}
}
