package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/sijirama/beacon/internal/auth"
)

type SystemHandler struct {
	baseURL string
	db      *sql.DB
	store   *auth.Store
	queue   *asynq.Client
}

func NewSystem(baseURL string, db *sql.DB, store *auth.Store, queue *asynq.Client) *SystemHandler {
	return &SystemHandler{
		baseURL: strings.TrimRight(baseURL, "/"),
		db:      db,
		store:   store,
		queue:   queue,
	}
}

func (h *SystemHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *SystemHandler) Heartbeat(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	status := gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"checks": gin.H{
			"app":   "ok",
			"db":    "ok",
			"redis": "ok",
			"queue": "ok",
		},
	}

	if err := h.db.PingContext(ctx); err != nil {
		status["status"] = "degraded"
		status["checks"].(gin.H)["db"] = err.Error()
	}
	if err := h.store.Ping(ctx); err != nil {
		status["status"] = "degraded"
		status["checks"].(gin.H)["redis"] = err.Error()
	}
	if err := h.queue.Ping(); err != nil {
		status["status"] = "degraded"
		status["checks"].(gin.H)["queue"] = err.Error()
	}

	code := http.StatusOK
	if status["status"] != "ok" {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, status)
}

func (h *SystemHandler) APISpec(c *gin.Context) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, apiSpecDocument(h.baseURL))
}

func (h *SystemHandler) OpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, openAPIDocument(h.baseURL))
}

func apiSpecDocument(baseURL string) string {
	return fmt.Sprintf(`Beacon API Specification

Base URL
- %s

Purpose
- Beacon accepts authenticated webhook-style event submissions and sends email notifications.
- This service currently supports one programmatic write endpoint: POST /emit
- The web UI endpoints are browser-oriented and use cookie sessions; they are not the primary integration surface for scripts or LLM agents.

Authentication
- API requests to POST /emit must include an Authorization header:
  Authorization: Bearer <token>
- Tokens are created in the Beacon web UI under /tokens
- If the token is missing, malformed, or invalid, the request fails with HTTP 401

Content Type
- Send JSON with header:
  Content-Type: application/json

Endpoint: POST /emit
- Path: /emit
- Auth required: yes (Bearer token)
- Purpose: create an event record and enqueue an email delivery job

Request Body
- title: string, required
  Human-readable event title. Empty or whitespace-only values are rejected.
- message: string, optional
  Longer free-form event detail shown in the UI and email.
- source: string, optional
  System or subsystem name, for example "nightly-backup" or "ml-training".
- event: string, optional
  Event type name, for example "completed", "failed", or "started".
- level: string, optional
  Allowed semantic values:
  - "info"
  - "warn" or "warning" -> normalized to "warn"
  - "danger", "critical", or "error" -> normalized to "danger"
  Any other value is treated as "info".
- channel: string, optional
  Only "email" is supported in v1.
  If omitted, Beacon defaults it to "email".
  Any non-"email" value is rejected with HTTP 400.
- metadata: object, optional
  Arbitrary JSON object with extra fields. Nested values are allowed if they are valid JSON.

Successful Response
- HTTP 200
- JSON body:
  {
    "status": "ok",
    "event_id": 42
  }

Error Responses
- HTTP 400
  - invalid JSON
  - missing or blank title
  - unsupported channel
- HTTP 401
  - missing Authorization header
  - invalid Authorization header
  - invalid token
- HTTP 500
  - internal persistence or authentication lookup failure

Delivery Semantics
- A successful POST /emit means the event was stored.
- Beacon then enqueues an email job in Redis for asynchronous delivery.
- Email sending may still fail after the HTTP 200 response if the queue or provider later encounters an error.
- Delivery attempts and errors are visible in the web UI on the event detail page.

Minimal Example
curl -X POST %s/emit \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"title":"hello from beacon"}'

Full Example
curl -X POST %s/emit \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "ml-training",
    "event": "completed",
    "title": "Training run finished",
    "message": "Resnet50 finished after 4h12m. Final val_acc = 0.927.",
    "level": "info",
    "channel": "email",
    "metadata": {
      "run_id": "rn_8a3f9c",
      "duration": "4h12m",
      "val_acc": 0.927,
      "checkpoint": "s3://models/rn_8a3f9c/best.pt"
    }
  }'

Failure Example
curl -X POST %s/emit \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Backup failed",
    "message": "rsync exited with code 23",
    "level": "danger",
    "source": "nightly-backup",
    "metadata": {"host":"db-01","exit_code":23}
  }'

Read-Only Public Endpoints
- GET /health
  Lightweight liveness endpoint. Returns HTTP 200 with {"status":"ok"} if the HTTP app is up.
- GET /heartbeat
  Deeper readiness-style endpoint. Checks app, SQLite, Redis, and queue connectivity.
  Returns HTTP 200 when all checks pass.
  Returns HTTP 503 when any dependency check fails.
- GET /api-spec
  Returns this plain-text specification.
- GET /openapi.json
  Returns the OpenAPI 3.1 JSON document for this service.

Heartbeat Response Shape
{
  "status": "ok",
  "timestamp": "2026-05-05T01:23:45Z",
  "checks": {
    "app": "ok",
    "db": "ok",
    "redis": "ok",
    "queue": "ok"
  }
}

Notes For LLM Agents
- Use POST /emit for all programmatic notifications.
- Always send a non-empty title.
- Always send the Bearer token header.
- Prefer level values "info", "warn", or "danger".
- Do not send channel values other than "email".
- Treat HTTP 200 as "event accepted and recorded", not necessarily "email already sent".
`, baseURL, baseURL, baseURL, baseURL)
}

func openAPIDocument(baseURL string) gin.H {
	return gin.H{
		"openapi": "3.1.0",
		"info": gin.H{
			"title":       "Beacon API",
			"version":     "1.0.0",
			"description": "Beacon accepts authenticated webhook-style event submissions and sends email notifications.",
		},
		"servers": []gin.H{
			{"url": baseURL},
		},
		"paths": gin.H{
			"/emit": gin.H{
				"post": gin.H{
					"summary":     "Create an event and enqueue email delivery",
					"description": "Stores an event, then enqueues an asynchronous email notification job.",
					"security":    []gin.H{{"bearerAuth": []string{}}},
					"requestBody": gin.H{
						"required": true,
						"content": gin.H{
							"application/json": gin.H{
								"schema": gin.H{
									"type":     "object",
									"required": []string{"title"},
									"properties": gin.H{
										"title": gin.H{
											"type":        "string",
											"description": "Human-readable event title. Must not be blank.",
										},
										"message": gin.H{
											"type":        "string",
											"description": "Longer free-form event detail shown in the UI and email.",
										},
										"source": gin.H{
											"type":        "string",
											"description": "System or subsystem name, e.g. nightly-backup or ml-training.",
										},
										"event": gin.H{
											"type":        "string",
											"description": "Event type name, e.g. completed, failed, started.",
										},
										"level": gin.H{
											"type":        "string",
											"description": "Semantic level. Prefer info, warn, or danger. warning maps to warn; critical and error map to danger.",
											"enum":        []string{"info", "warn", "warning", "danger", "critical", "error"},
										},
										"channel": gin.H{
											"type":        "string",
											"description": "Notification channel. Only email is supported in v1.",
											"enum":        []string{"email"},
											"default":     "email",
										},
										"metadata": gin.H{
											"type":                 "object",
											"description":          "Arbitrary JSON object with extra fields.",
											"additionalProperties": true,
										},
									},
								},
								"examples": gin.H{
									"minimal": gin.H{
										"value": gin.H{
											"title": "hello from beacon",
										},
									},
									"full": gin.H{
										"value": gin.H{
											"source":  "ml-training",
											"event":   "completed",
											"title":   "Training run finished",
											"message": "Resnet50 finished after 4h12m. Final val_acc = 0.927.",
											"level":   "info",
											"channel": "email",
											"metadata": gin.H{
												"run_id":     "rn_8a3f9c",
												"duration":   "4h12m",
												"val_acc":    0.927,
												"checkpoint": "s3://models/rn_8a3f9c/best.pt",
											},
										},
									},
								},
							},
						},
					},
					"responses": gin.H{
						"200": gin.H{
							"description": "Event accepted and recorded",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"type": "object",
										"properties": gin.H{
											"status":   gin.H{"type": "string", "example": "ok"},
											"event_id": gin.H{"type": "integer", "example": 42},
										},
									},
								},
							},
						},
						"400": gin.H{
							"description": "Invalid JSON, blank title, or unsupported channel",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"$ref": "#/components/schemas/ErrorResponse",
									},
								},
							},
						},
						"401": gin.H{
							"description": "Missing, malformed, or invalid bearer token",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"$ref": "#/components/schemas/ErrorResponse",
									},
								},
							},
						},
						"500": gin.H{
							"description": "Internal persistence or auth lookup failure",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"$ref": "#/components/schemas/ErrorResponse",
									},
								},
							},
						},
					},
				},
			},
			"/health": gin.H{
				"get": gin.H{
					"summary":     "Lightweight liveness check",
					"description": "Returns ok when the HTTP app is up.",
					"responses": gin.H{
						"200": gin.H{
							"description": "Service is up",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"type": "object",
										"properties": gin.H{
											"status": gin.H{"type": "string", "example": "ok"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/heartbeat": gin.H{
				"get": gin.H{
					"summary":     "Dependency-aware heartbeat",
					"description": "Checks app, SQLite, Redis, and queue connectivity.",
					"responses": gin.H{
						"200": gin.H{
							"description": "All dependency checks passed",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"$ref": "#/components/schemas/HeartbeatResponse",
									},
								},
							},
						},
						"503": gin.H{
							"description": "One or more dependency checks failed",
							"content": gin.H{
								"application/json": gin.H{
									"schema": gin.H{
										"$ref": "#/components/schemas/HeartbeatResponse",
									},
								},
							},
						},
					},
				},
			},
			"/api-spec": gin.H{
				"get": gin.H{
					"summary":     "Plain-text API specification",
					"description": "Returns a compact plain-text spec suitable for pasting into an LLM.",
					"responses": gin.H{
						"200": gin.H{
							"description": "Plain-text API specification",
							"content": gin.H{
								"text/plain": gin.H{
									"schema": gin.H{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
			"/openapi.json": gin.H{
				"get": gin.H{
					"summary":     "OpenAPI document",
					"description": "Returns the OpenAPI 3.1 JSON document for this service.",
					"responses": gin.H{
						"200": gin.H{
							"description": "OpenAPI JSON document",
						},
					},
				},
			},
		},
		"components": gin.H{
			"securitySchemes": gin.H{
				"bearerAuth": gin.H{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "API token",
				},
			},
			"schemas": gin.H{
				"ErrorResponse": gin.H{
					"type": "object",
					"properties": gin.H{
						"error": gin.H{"type": "string"},
					},
					"required": []string{"error"},
				},
				"HeartbeatResponse": gin.H{
					"type": "object",
					"properties": gin.H{
						"status":    gin.H{"type": "string", "example": "ok"},
						"timestamp": gin.H{"type": "string", "format": "date-time"},
						"checks": gin.H{
							"type": "object",
							"properties": gin.H{
								"app":   gin.H{"type": "string", "example": "ok"},
								"db":    gin.H{"type": "string", "example": "ok"},
								"redis": gin.H{"type": "string", "example": "ok"},
								"queue": gin.H{"type": "string", "example": "ok"},
							},
							"required": []string{"app", "db", "redis", "queue"},
						},
					},
					"required": []string{"status", "timestamp", "checks"},
				},
			},
		},
	}
}
