package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/sijirama/beacon/internal/auth"
	"github.com/sijirama/beacon/internal/config"
	"github.com/sijirama/beacon/internal/middleware"
	"github.com/sijirama/beacon/internal/services/email"
	"github.com/sijirama/beacon/internal/web"
)

type AuthHandler struct {
	cfg      config.Config
	store    *auth.Store
	sender   *email.Sender
	renderer *web.Renderer
}

func NewAuth(cfg config.Config, store *auth.Store, sender *email.Sender, r *web.Renderer) *AuthHandler {
	return &AuthHandler{cfg: cfg, store: store, sender: sender, renderer: r}
}

func (h *AuthHandler) GetLogin(c *gin.Context) {
	if _, ok := c.Get("user_email"); ok {
		c.Redirect(http.StatusFound, "/")
		return
	}
	h.renderer.Render(c, http.StatusOK, "login", gin.H{
		"Title": "Sign in",
	})
}

func (h *AuthHandler) PostLogin(c *gin.Context) {
	emailAddr := strings.TrimSpace(strings.ToLower(c.PostForm("email")))
	if emailAddr == "" {
		h.renderer.Render(c, http.StatusBadRequest, "login", gin.H{
			"Title": "Sign in",
			"Error": "Please enter an email.",
		})
		return
	}

	// Always show "check your email" — never leak whether the address is allowlisted.
	if h.cfg.IsRootEmail(emailAddr) {
		token, err := h.store.CreateMagicLink(c.Request.Context(), emailAddr, h.cfg.MagicLinkTTL)
		if err != nil {
			log.Printf("auth: create magic link: %v", err)
		} else {
			link := fmt.Sprintf("%s/auth/verify?token=%s", strings.TrimRight(h.cfg.BaseURL, "/"), token)
			html := magicLinkHTML(link, h.cfg.MagicLinkTTL)
			if _, err := h.sender.Send(c.Request.Context(), emailAddr, "Sign in to Beacon", html); err != nil {
				log.Printf("auth: send magic link to %s: %v", emailAddr, err)
			}
		}
	} else {
		log.Printf("auth: rejected login attempt for %s (not in ROOT_EMAILS)", emailAddr)
	}

	h.renderer.Render(c, http.StatusOK, "login_sent", gin.H{
		"Title": "Check your email",
		"Email": emailAddr,
	})
}

func (h *AuthHandler) GetVerify(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		h.renderer.Render(c, http.StatusBadRequest, "login", gin.H{
			"Title": "Sign in",
			"Error": "Invalid or expired link. Try again.",
		})
		return
	}
	emailAddr, err := h.store.ConsumeMagicLink(c.Request.Context(), token)
	if err != nil {
		h.renderer.Render(c, http.StatusBadRequest, "login", gin.H{
			"Title": "Sign in",
			"Error": "Invalid or expired link. Try again.",
		})
		return
	}
	sid, err := h.store.CreateSession(c.Request.Context(), emailAddr, h.cfg.SessionTTL)
	if err != nil {
		log.Printf("auth: create session: %v", err)
		c.String(http.StatusInternalServerError, "could not create session")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		middleware.SessionCookie,
		sid,
		int(h.cfg.SessionTTL/time.Second),
		"/",
		"",
		h.cfg.CookieSecure,
		true, // HttpOnly
	)
	c.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) PostLogout(c *gin.Context) {
	if sid, err := c.Cookie(middleware.SessionCookie); err == nil && sid != "" {
		_ = h.store.DeleteSession(c.Request.Context(), sid)
	}
	c.SetCookie(middleware.SessionCookie, "", -1, "/", "", h.cfg.CookieSecure, true)
	c.Redirect(http.StatusFound, "/login")
}

func magicLinkHTML(link string, ttl time.Duration) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f7f7f7;margin:0;padding:24px;color:#1a1a1a;">
  <div style="max-width:520px;margin:0 auto;background:#fff;border-radius:8px;padding:32px;">
    <h1 style="font-size:20px;margin:0 0 16px;">Sign in to Beacon</h1>
    <p style="font-size:15px;line-height:1.6;margin:0 0 24px;">Click the button below to sign in. This link expires in %d minutes and can only be used once.</p>
    <p style="margin:0 0 24px;">
      <a href="%s" style="display:inline-block;background:#0a0a0a;color:#fff;padding:10px 16px;border-radius:6px;text-decoration:none;font-weight:500;">Sign in</a>
    </p>
    <p style="font-size:12px;color:#999;margin:0;word-break:break-all;">Or paste this link: %s</p>
    <p style="font-size:12px;color:#999;margin-top:16px;">If you didn't request this, you can safely ignore it.</p>
  </div>
</body></html>`, int(ttl.Minutes()), link, link)
}
