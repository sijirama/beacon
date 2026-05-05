# Beacon

> A small self-hosted webhook notification thing.

Beacon lets you `POST /emit` from a script and get a notification when something happens. Right now that notification channel is email, but the shape is meant to grow.

## What it does

- `POST /emit` with a bearer token, create a notification event
- Manage tokens and browse events in a small web UI
- Sign in with magic links
- Queue delivery jobs through Redis with retries
- Store everything in SQLite

## Quick start

```bash
git clone <this repo> beacon
cd beacon
cp .env.example .env
# fill in RESEND_API_KEY, ROOT_EMAILS, EMAIL_FROM, BASE_URL
docker compose up -d
```

Visit `BASE_URL`, sign in via magic link, create a token.

## Public endpoints

- `GET /health` → lightweight liveness check, returns `{"status":"ok"}`
- `GET /heartbeat` → deeper heartbeat, checks app, SQLite, Redis, and queue connectivity
- `GET /api-spec` → plain-text API spec for humans, scripts, and LLMs
- `GET /openapi.json` → OpenAPI 3.1 JSON document for tool integrations

## Send an event

Minimal — just a title:

```bash
curl -X POST https://beacon.sijibomi.com/emit \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"title":"hello from beacon"}'
```

Full payload — every field:

```bash
curl -X POST https://beacon.sijibomi.com/emit \
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
```

Danger level — red badge, "something is on fire" emails:

```bash
curl -X POST https://beacon.sijibomi.com/emit \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Backup failed",
    "message": "rsync exited with code 23",
    "level": "danger",
    "source": "nightly-backup",
    "metadata": {"host":"db-01","exit_code":23}
  }'
```

Response:

```json
{"status":"ok","event_id":42}
```

You get an email. That's it.

## Configuration

All config is via environment variables — see [`.env.example`](.env.example) for the full list.

## Stack

Go · Gin · GORM · SQLite · Redis · asynq · Resend
