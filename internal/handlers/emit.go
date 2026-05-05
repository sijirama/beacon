package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/sijirama/beacon/internal/models"
	"github.com/sijirama/beacon/internal/queue"
)

type EmitHandler struct {
	db       *gorm.DB
	qClient  *asynq.Client
	retries  int
}

func NewEmit(g *gorm.DB, qClient *asynq.Client, retries int) *EmitHandler {
	return &EmitHandler{db: g, qClient: qClient, retries: retries}
}

type emitRequest struct {
	Source   string         `json:"source"`
	Event    string         `json:"event"`
	Title    string         `json:"title"`
	Message  string         `json:"message"`
	Level    string         `json:"level"`
	Channel  string         `json:"channel"`
	Metadata map[string]any `json:"metadata"`
}

func (h *EmitHandler) Post(c *gin.Context) {
	var req emitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel == "" {
		channel = "email"
	}
	if channel != "email" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only 'email' channel is supported in v1"})
		return
	}

	var tokenID *uint
	if v, ok := c.Get("token"); ok {
		if t, ok := v.(*models.Token); ok && t != nil {
			id := t.ID
			tokenID = &id
		}
	}

	ev := models.Event{
		TokenID:  tokenID,
		Source:   req.Source,
		Event:    req.Event,
		Title:    req.Title,
		Message:  req.Message,
		Level:    models.NormalizeLevel(strings.ToLower(req.Level)),
		Channel:  channel,
		IP:       c.ClientIP(),
		Metadata: models.Metadata(req.Metadata),
	}
	if err := h.db.Create(&ev).Error; err != nil {
		log.Printf("emit: insert event: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save event"})
		return
	}

	if err := queue.EnqueueEmail(h.qClient, ev.ID, h.retries); err != nil {
		log.Printf("emit: enqueue: %v", err)
		// We've already stored the event — return ok with a warning; the delivery row
		// will reflect the failure when the worker fails to pick it up.
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"event_id": ev.ID,
	})
}
