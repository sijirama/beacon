package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/sijirama/beacon/internal/models"
	"github.com/sijirama/beacon/internal/queue"
	"github.com/sijirama/beacon/internal/web"
)

type PagesHandler struct {
	db        *gorm.DB
	renderer  *web.Renderer
	inspector *asynq.Inspector
}

func NewPages(g *gorm.DB, r *web.Renderer, ins *asynq.Inspector) *PagesHandler {
	return &PagesHandler{db: g, renderer: r, inspector: ins}
}

func (h *PagesHandler) Events(c *gin.Context) {
	var events []models.Event
	q := h.db.Order("created_at DESC").Limit(100)
	if tokenIDStr := c.Query("token_id"); tokenIDStr != "" {
		if id, err := strconv.ParseUint(tokenIDStr, 10, 64); err == nil {
			q = q.Where("token_id = ?", uint(id))
		}
	}
	q.Find(&events)
	h.renderer.Render(c, http.StatusOK, "events", gin.H{
		"Title":  "Events",
		"Events": events,
	})
}

func (h *PagesHandler) EventDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "bad event id")
		return
	}
	var ev models.Event
	if err := h.db.Preload("Token").First(&ev, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(http.StatusNotFound, "event not found")
			return
		}
		c.String(http.StatusInternalServerError, "lookup failed")
		return
	}
	var deliveries []models.Delivery
	h.db.Where("event_id = ?", ev.ID).Order("created_at DESC").Find(&deliveries)
	h.renderer.Render(c, http.StatusOK, "event_detail", gin.H{
		"Title":      "Event #" + idStr,
		"Event":      ev,
		"Deliveries": deliveries,
	})
}

func (h *PagesHandler) Queue(c *gin.Context) {
	stats := map[string]int{"pending": 0, "active": 0, "retry": 0, "archived": 0}
	if h.inspector != nil {
		if info, err := h.inspector.GetQueueInfo(queue.QueueDefault); err == nil {
			stats["pending"] = info.Pending
			stats["active"] = info.Active
			stats["retry"] = info.Retry
			stats["archived"] = info.Archived
		}
	}
	var deliveries []models.Delivery
	h.db.Order("created_at DESC").Limit(50).Find(&deliveries)
	h.renderer.Render(c, http.StatusOK, "queue", gin.H{
		"Title": "Queue",
		"Stats": stats,
		"Tasks": deliveries,
	})
}

type tokenRow struct {
	ID         uint
	Name       string
	Token      string
	CreatedAt  time.Time
	EventCount int64
}

func (h *PagesHandler) Tokens(c *gin.Context) {
	var tokens []models.Token
	h.db.Order("created_at DESC").Find(&tokens)

	out := make([]tokenRow, 0, len(tokens))
	for _, t := range tokens {
		var n int64
		h.db.Model(&models.Event{}).Where("token_id = ?", t.ID).Count(&n)
		out = append(out, tokenRow{
			ID:         t.ID,
			Name:       t.Name,
			Token:      t.Token,
			CreatedAt:  t.CreatedAt,
			EventCount: n,
		})
	}
	h.renderer.Render(c, http.StatusOK, "tokens", gin.H{
		"Title":  "API Tokens",
		"Tokens": out,
	})
}
