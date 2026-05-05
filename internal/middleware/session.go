package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/sijirama/beacon/internal/auth"
)

const SessionCookie = "beacon_session"

type SessionDeps struct {
	Store      *auth.Store
	SessionTTL time.Duration
}

// RequireSession redirects to /login if not authenticated.
func RequireSession(d SessionDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(SessionCookie)
		if err != nil || sid == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		email, err := d.Store.GetSession(c.Request.Context(), sid, d.SessionTTL)
		if err != nil {
			c.SetCookie(SessionCookie, "", -1, "/", "", false, true)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Set("user_email", email)
		c.Next()
	}
}

// LoadSession attaches the user_email to the context if present, but does not require it.
func LoadSession(d SessionDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(SessionCookie)
		if err != nil || sid == "" {
			c.Next()
			return
		}
		email, err := d.Store.GetSession(c.Request.Context(), sid, d.SessionTTL)
		if err == nil {
			c.Set("user_email", email)
		}
		c.Next()
	}
}
