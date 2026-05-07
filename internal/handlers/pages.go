package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/sijirama/beacon/internal/models"
	"github.com/sijirama/beacon/internal/queue"
	"github.com/sijirama/beacon/internal/stream"
	"github.com/sijirama/beacon/internal/web"
)

const pageSize = 10

type PagesHandler struct {
	db        *gorm.DB
	renderer  *web.Renderer
	inspector *asynq.Inspector
	hub       *stream.Hub
}

func NewPages(g *gorm.DB, r *web.Renderer, ins *asynq.Inspector, hub *stream.Hub) *PagesHandler {
	return &PagesHandler{db: g, renderer: r, inspector: ins, hub: hub}
}

// metaKeyRe restricts metadata.<KEY>=value parsing to safe characters so the
// key can be interpolated into a JSON path without sanitization concerns.
var metaKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)

type metaFilter struct {
	Key   string
	Value string
}

// parseSearch pulls metadata.X=Y tokens out of a free-text search string.
// What's left is plain text that still drives a LIKE search across the
// title/message/source/event columns.
func parseSearch(q string) (text string, metas []metaFilter) {
	rest := make([]string, 0)
	for _, tok := range strings.Fields(q) {
		if strings.HasPrefix(tok, "metadata.") {
			kv := strings.TrimPrefix(tok, "metadata.")
			if i := strings.IndexByte(kv, '='); i > 0 {
				key := kv[:i]
				val := kv[i+1:]
				if metaKeyRe.MatchString(key) {
					metas = append(metas, metaFilter{Key: key, Value: val})
					continue
				}
			}
		}
		rest = append(rest, tok)
	}
	return strings.Join(rest, " "), metas
}

type eventsFilters struct {
	Query   string
	Level   string
	TokenID string
	text    string
	metas   []metaFilter
	tokenID *uint
}

func parseEventsFilters(c *gin.Context) eventsFilters {
	raw := strings.TrimSpace(c.Query("q"))
	text, metas := parseSearch(raw)

	f := eventsFilters{
		Query:   raw,
		Level:   strings.TrimSpace(c.Query("level")),
		TokenID: strings.TrimSpace(c.Query("token_id")),
		text:    text,
		metas:   metas,
	}
	switch f.Level {
	case string(models.LevelInfo), string(models.LevelWarn), string(models.LevelDanger):
		// keep
	default:
		f.Level = ""
	}
	if f.TokenID != "" {
		if id, err := strconv.ParseUint(f.TokenID, 10, 64); err == nil {
			v := uint(id)
			f.tokenID = &v
		}
	}
	return f
}

func (f eventsFilters) apply(q *gorm.DB) *gorm.DB {
	if f.tokenID != nil {
		q = q.Where("token_id = ?", *f.tokenID)
	}
	if f.Level != "" {
		q = q.Where("level = ?", f.Level)
	}
	if f.text != "" {
		like := "%" + f.text + "%"
		q = q.Where(
			"title LIKE ? OR message LIKE ? OR source LIKE ? OR event LIKE ?",
			like, like, like, like,
		)
	}
	for _, m := range f.metas {
		q = q.Where("json_extract(metadata, ?) = ?", "$."+m.Key, m.Value)
	}
	return q
}

func (h *PagesHandler) Events(c *gin.Context) {
	f := parseEventsFilters(c)
	page := parsePage(c.Query("page"))

	base := f.apply(h.db.Model(&models.Event{}))

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		c.String(http.StatusInternalServerError, "count failed")
		return
	}

	totalPages := int((total + pageSize - 1) / pageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize

	var events []models.Event
	if err := base.Session(&gorm.Session{}).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&events).Error; err != nil {
		c.String(http.StatusInternalServerError, "query failed")
		return
	}

	h.renderer.Render(c, http.StatusOK, "events", gin.H{
		"Title":       "Events",
		"Events":      events,
		"Query":       f.Query,
		"Level":       f.Level,
		"TokenID":     f.TokenID,
		"Page":        page,
		"PageSize":    pageSize,
		"Total":       total,
		"TotalPages":  totalPages,
		"HasPrev":     page > 1,
		"HasNext":     int64(page*pageSize) < total,
		"PrevURL":     buildEventsURL(f.Query, f.Level, f.TokenID, page-1),
		"NextURL":     buildEventsURL(f.Query, f.Level, f.TokenID, page+1),
		"ClearURL":    buildEventsURL("", "", f.TokenID, 1),
		"StreamURL":   buildStreamURL(f.Query, f.Level, f.TokenID),
		"ShowingFrom": showingFrom(total, page, pageSize),
		"ShowingTo":   showingTo(total, page, pageSize),
	})
}

// Stream is a Server-Sent Events endpoint that pushes new events to the
// browser as they're created. It applies the same filters as the events
// page so live tail and the visible list stay consistent.
func (h *PagesHandler) Stream(c *gin.Context) {
	if h.hub == nil {
		c.String(http.StatusServiceUnavailable, "stream unavailable")
		return
	}
	f := parseEventsFilters(c)

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")

	// Reverse proxies (Cloudflare, nginx, Caddy) typically buffer the first
	// ~1–2 KB of a response before forwarding. With SSE that means the
	// browser sits on "connecting…" until enough body bytes have been
	// written, even though the server has produced headers + a small
	// comment. Pad the preamble out so the first chunk reliably gets
	// pushed through. The padding is an SSE comment line (starts with ":")
	// and is ignored by EventSource.
	preamble := "retry: 3000\n\n: " + strings.Repeat(" ", 2048) + "\n\n: connected\n\n"
	if _, err := fmt.Fprint(c.Writer, preamble); err != nil {
		return
	}
	c.Writer.Flush()

	log.Printf("stream: client connected (level=%q tokenID=%q metas=%d text=%q)",
		f.Level, f.TokenID, len(f.metas), f.text)

	ch, cancel := h.hub.Subscribe()
	defer cancel()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !matchesEvent(f, ev) {
				continue
			}
			row, err := h.renderer.RenderPartial("event-row", ev)
			if err != nil {
				continue
			}
			row = strings.ReplaceAll(strings.ReplaceAll(row, "\r", " "), "\n", " ")
			payload, err := json.Marshal(map[string]any{
				"id":   ev.ID,
				"html": row,
			})
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: event\ndata: %s\n\n", payload); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func matchesEvent(f eventsFilters, e stream.EventMsg) bool {
	if f.tokenID != nil {
		if e.TokenID == nil || *e.TokenID != *f.tokenID {
			return false
		}
	}
	if f.Level != "" && e.Level != f.Level {
		return false
	}
	if f.text != "" {
		needle := strings.ToLower(f.text)
		if !strings.Contains(strings.ToLower(e.Title), needle) &&
			!strings.Contains(strings.ToLower(e.Message), needle) &&
			!strings.Contains(strings.ToLower(e.Source), needle) &&
			!strings.Contains(strings.ToLower(e.Event), needle) {
			return false
		}
	}
	for _, m := range f.metas {
		if e.Metadata == nil {
			return false
		}
		v, ok := lookupPath(e.Metadata, m.Key)
		if !ok {
			return false
		}
		if !valueMatches(v, m.Value) {
			return false
		}
	}
	return true
}

func lookupPath(m map[string]any, dotted string) (any, bool) {
	keys := strings.Split(dotted, ".")
	var cur any = m
	for _, k := range keys {
		mp, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := mp[k]
		if !exists {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func valueMatches(actual any, want string) bool {
	switch v := actual.(type) {
	case string:
		return v == want
	case bool:
		return strconv.FormatBool(v) == want
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64) == want
	case json.Number:
		return v.String() == want
	case nil:
		return want == "null"
	default:
		return fmt.Sprintf("%v", v) == want
	}
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

	page := parsePage(c.Query("page"))
	base := h.db.Model(&models.Delivery{})

	var total int64
	base.Session(&gorm.Session{}).Count(&total)

	totalPages := int((total + pageSize - 1) / pageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize

	var deliveries []models.Delivery
	base.Session(&gorm.Session{}).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&deliveries)

	h.renderer.Render(c, http.StatusOK, "queue", gin.H{
		"Title":       "Queue",
		"Stats":       stats,
		"Tasks":       deliveries,
		"Page":        page,
		"PageSize":    pageSize,
		"Total":       total,
		"TotalPages":  totalPages,
		"HasPrev":     page > 1,
		"HasNext":     int64(page*pageSize) < total,
		"PrevURL":     buildPagedURL("/queue", page-1),
		"NextURL":     buildPagedURL("/queue", page+1),
		"ShowingFrom": showingFrom(total, page, pageSize),
		"ShowingTo":   showingTo(total, page, pageSize),
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
	page := parsePage(c.Query("page"))
	base := h.db.Model(&models.Token{})

	var total int64
	base.Session(&gorm.Session{}).Count(&total)

	totalPages := int((total + pageSize - 1) / pageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize

	var tokens []models.Token
	base.Session(&gorm.Session{}).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&tokens)

	counts := make(map[uint]int64, len(tokens))
	if len(tokens) > 0 {
		ids := make([]uint, 0, len(tokens))
		for _, t := range tokens {
			ids = append(ids, t.ID)
		}
		type row struct {
			TokenID uint  `gorm:"column:token_id"`
			N       int64 `gorm:"column:n"`
		}
		var rows []row
		h.db.Model(&models.Event{}).
			Select("token_id, COUNT(*) AS n").
			Where("token_id IN ?", ids).
			Group("token_id").
			Scan(&rows)
		for _, r := range rows {
			counts[r.TokenID] = r.N
		}
	}

	out := make([]tokenRow, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, tokenRow{
			ID:         t.ID,
			Name:       t.Name,
			Token:      t.Token,
			CreatedAt:  t.CreatedAt,
			EventCount: counts[t.ID],
		})
	}

	h.renderer.Render(c, http.StatusOK, "tokens", gin.H{
		"Title":       "API Tokens",
		"Tokens":      out,
		"Page":        page,
		"PageSize":    pageSize,
		"Total":       total,
		"TotalPages":  totalPages,
		"HasPrev":     page > 1,
		"HasNext":     int64(page*pageSize) < total,
		"PrevURL":     buildPagedURL("/tokens", page-1),
		"NextURL":     buildPagedURL("/tokens", page+1),
		"ShowingFrom": showingFrom(total, page, pageSize),
		"ShowingTo":   showingTo(total, page, pageSize),
	})
}

func parsePage(s string) int {
	page, err := strconv.Atoi(s)
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func eventsQueryValues(query, level, tokenID string, page int) url.Values {
	v := url.Values{}
	if query != "" {
		v.Set("q", query)
	}
	if level != "" {
		v.Set("level", level)
	}
	if tokenID != "" {
		v.Set("token_id", tokenID)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	return v
}

func buildEventsURL(query, level, tokenID string, page int) string {
	if page < 1 {
		page = 1
	}
	encoded := eventsQueryValues(query, level, tokenID, page).Encode()
	if encoded == "" {
		return "/"
	}
	return "/?" + encoded
}

func buildStreamURL(query, level, tokenID string) string {
	encoded := eventsQueryValues(query, level, tokenID, 0).Encode()
	if encoded == "" {
		return "/events/stream"
	}
	return "/events/stream?" + encoded
}

func buildPagedURL(path string, page int) string {
	if page <= 1 {
		return path
	}
	return path + "?page=" + strconv.Itoa(page)
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
