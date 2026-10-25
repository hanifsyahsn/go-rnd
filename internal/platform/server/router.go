package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	authlogin "github.com/hanifsyahsn/go-rnd/internal/features/auth/login"
	authlogout "github.com/hanifsyahsn/go-rnd/internal/features/auth/logout"
	authrefresh "github.com/hanifsyahsn/go-rnd/internal/features/auth/refresh"
	authregister "github.com/hanifsyahsn/go-rnd/internal/features/auth/register"
	"github.com/hanifsyahsn/go-rnd/internal/features/me"
	"github.com/hanifsyahsn/go-rnd/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go-rnd/internal/platform/middleware"
	"github.com/hanifsyahsn/go-rnd/internal/shared/apperror"
	"github.com/hanifsyahsn/go-rnd/internal/shared/jsonapi"
)

// Handlers is every feature slice's handler, wired in cmd/api/main.go. A new
// slice adds its field here and one Route call in NewRouter.
type Handlers struct {
	Register authregister.H
	Login    authlogin.H
	Refresh  authrefresh.H
	Logout   authlogout.H

	Me me.H
}

// NewRouter mounts two groups under /api, both behind JSON:API content
// negotiation (middleware.JSONAPIMediaType):
//
//	public   no auth (register / login / refresh / logout)
//	authed   AccessTokenMiddleware (any active user)
//
// An admin-only group is api.Group("/admin") with AccessTokenMiddleware +
// middleware.RequireRole(enum.RoleAdmin); add it when a route needs it.
//
// Every route (404 / 405 / panic included) runs RequestID → AccessLog →
// recovery first, so every response carries X-Request-ID and gets one access
// log record.
//
// There's also an unauthenticated GET /healthz liveness probe that reports the build
// version. Unknown routes (404), a wrong method (405) and a panic (500) answer
// with JSON:API errors too.
func NewRouter(version string, tokenMaker jwttoken.M, userStatus middleware.UserStatusReader, h Handlers) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.AccessLog(), gin.CustomRecovery(func(c *gin.Context, recovered any) {
		jsonapi.WriteError(c.Writer, apperror.CodeInternal, apperror.CodeInternal.SafeToThrow(),
			fmt.Errorf("panic: %v", recovered))
		c.Abort()
	}))
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		jsonapi.WriteError(c.Writer, apperror.CodeNotFound, "That resource doesn't exist.", nil)
	})
	r.NoMethod(func(c *gin.Context) {
		jsonapi.WriteError(c.Writer, apperror.CodeMethodNotAllowed,
			fmt.Sprintf("%s isn't allowed on this resource.", c.Request.Method), nil)
	})

	r.GET("/healthz", func(c *gin.Context) {
		jsonapi.WriteMeta(c.Writer, c.Request, http.StatusOK, gin.H{"status": "up", "version": version})
	})

	api := r.Group("/api", middleware.JSONAPIMediaType())

	public := api.Group("")
	{
		authregister.Route(public, h.Register)
		authlogin.Route(public, h.Login)
		authrefresh.Route(public, h.Refresh)
		authlogout.Route(public, h.Logout)
	}

	authed := api.Group("")
	authed.Use(middleware.AccessTokenMiddleware(tokenMaker, userStatus))
	{
		me.Route(authed, h.Me)
	}

	return r
}
