package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/sijirama/beacon/internal/models"
	"github.com/sijirama/beacon/internal/queue"
	"github.com/sijirama/beacon/internal/stream"
)

// dedupWindow collapses notifications for events that share source + title +
// level inside this rolling window. The event is still stored — only the
// notification is suppressed — so the audit trail stays complete.
const dedupWindow = 60 * time.Second

type EmitHandler struct {
	db      *gorm.DB
	qClient *asynq.Client
	hub     *stream.Hub
	retries int
}

func NewEmit(g *gorm.DB, qClient *asynq.Client, hub *stream.Hub, retries int) *EmitHandler {
	return &EmitHandler{db: g, qClient: qClient, hub: hub, retries: retries}
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

	deduped, priorID := h.checkDedup(ev)
	if !deduped {
		if err := queue.EnqueueEmail(h.qClient, ev.ID, h.retries); err != nil {
			log.Printf("emit: enqueue: %v", err)
		}
	} else {
		log.Printf("emit: dedup suppressed notification event=%d (matched prior=%d)", ev.ID, priorID)
	}

	if h.hub != nil {
		h.hub.Publish(stream.EventMsg{
			ID:        ev.ID,
			TokenID:   ev.TokenID,
			Source:    ev.Source,
			Event:     ev.Event,
			Title:     ev.Title,
			Message:   ev.Message,
			Level:     string(ev.Level),
			Metadata:  map[string]any(ev.Metadata),
			CreatedAt: ev.CreatedAt,
		})
	}

	resp := gin.H{
		"status":   "ok",
		"event_id": ev.ID,
	}
	if deduped {
		resp["deduped"] = true
		resp["deduped_with"] = priorID
	}
	c.JSON(http.StatusOK, resp)
}

// checkDedup returns true (and the matching prior event id) if there is a
// recent event with the same source + title + level within dedupWindow.
// Uses Find rather than First so the empty case isn't logged as a warning.
func (h *EmitHandler) checkDedup(ev models.Event) (bool, uint) {
	cutoff := time.Now().UTC().Add(-dedupWindow)
	var priors []models.Event
	err := h.db.
		Where("id <> ? AND source = ? AND title = ? AND level = ? AND created_at >= ?",
			ev.ID, ev.Source, ev.Title, ev.Level, cutoff).
		Order("created_at DESC").
		Limit(1).
		Find(&priors).Error
	if err != nil {
		log.Printf("emit: dedup lookup: %v", err)
		return false, 0
	}
	if len(priors) == 0 {
		return false, 0
	}
	return true, priors[0].ID
}
