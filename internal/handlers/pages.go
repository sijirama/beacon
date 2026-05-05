package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	const pageSize = 25

	var events []models.Event
	queryText := strings.TrimSpace(c.Query("q"))
	level := strings.TrimSpace(c.Query("level"))
	tokenIDStr := strings.TrimSpace(c.Query("token_id"))
	page := parsePage(c.Query("page"))

	q := h.db.Model(&models.Event{})
	if tokenIDStr := c.Query("token_id"); tokenIDStr != "" {
		if id, err := strconv.ParseUint(tokenIDStr, 10, 64); err == nil {
			q = q.Where("token_id = ?", uint(id))
		}
	}
	switch level {
	case "", string(models.LevelInfo), string(models.LevelWarn), string(models.LevelDanger):
		if level != "" {
			q = q.Where("level = ?", level)
		}
	default:
		level = ""
	}
	if queryText != "" {
		like := "%" + queryText + "%"
		q = q.Where(
			"title LIKE ? OR message LIKE ? OR source LIKE ? OR event LIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	q.Count(&total)

	totalPages := int((total + pageSize - 1) / pageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * pageSize
	q.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&events)

	h.renderer.Render(c, http.StatusOK, "events", gin.H{
		"Title":       "Events",
		"Events":      events,
		"Query":       queryText,
		"Level":       level,
		"TokenID":     tokenIDStr,
		"Page":        page,
		"PageSize":    pageSize,
		"Total":       total,
		"TotalPages":  totalPages,
		"HasPrev":     page > 1,
		"HasNext":     int64(page*pageSize) < total,
		"PrevURL":     buildEventsURL(queryText, level, tokenIDStr, page-1),
		"NextURL":     buildEventsURL(queryText, level, tokenIDStr, page+1),
		"ClearURL":    buildEventsURL("", "", tokenIDStr, 1),
		"ShowingFrom": showingFrom(total, page, pageSize),
		"ShowingTo":   showingTo(total, page, pageSize),
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

func parsePage(s string) int {
	page, err := strconv.Atoi(s)
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func buildEventsURL(queryText, level, tokenID string, page int) string {
	if page < 1 {
		page = 1
	}
	values := url.Values{}
	if queryText != "" {
		values.Set("q", queryText)
	}
	if level != "" {
		values.Set("level", level)
	}
	if tokenID != "" {
		values.Set("token_id", tokenID)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	encoded := values.Encode()
	if encoded == "" {
		return "/"
	}
	return "/?" + encoded
}

func showingFrom(total int64, page, pageSize int) int64 {
	if total == 0 {
		return 0
	}
	return int64((page-1)*pageSize) + 1
}

func showingTo(total int64, page, pageSize int) int64 {
	end := int64(page * pageSize)
	if end > total {
		return total
	}
	return end
}
