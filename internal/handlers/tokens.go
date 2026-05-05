package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/sijirama/beacon/internal/models"
)

type TokensHandler struct {
	db *gorm.DB
}

func NewTokens(g *gorm.DB) *TokensHandler {
	return &TokensHandler{db: g}
}

func (h *TokensHandler) Create(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		c.Redirect(http.StatusFound, "/tokens")
		return
	}
	tok := models.Token{
		Name:  name,
		Token: generateToken(),
	}
	if err := h.db.Create(&tok).Error; err != nil {
		c.String(http.StatusInternalServerError, "could not create token: %v", err)
		return
	}
	c.Redirect(http.StatusFound, "/tokens")
}

func (h *TokensHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.Redirect(http.StatusFound, "/tokens")
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Event{}).Where("token_id = ?", uint(id)).Update("token_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Token{}, uint(id)).Error
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "could not delete token: %v", err)
		return
	}
	c.Redirect(http.StatusFound, "/tokens")
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "bcn_" + base64.RawURLEncoding.EncodeToString(b)
}
