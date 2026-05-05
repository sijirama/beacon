# Beacon

> A self-hosted webhook → email notification service for your scripts, ML runs, and personal devops.

## Why

I wanted a dead-simple way to get pinged when long things finish — ML training jobs, cron scripts, deploys, backups. Just email me when something happens. No SaaS, no shared dashboards, no extra accounts.

So I built Beacon. Point any script at it with a bearer token and it'll email you. You can self-host it too.

## What it does

- `POST /emit` with a bearer token → email lands in your inbox
- Web UI to manage API tokens, browse events, watch the email queue
- Magic-link login (passwordless, allowlisted by email)
- Redis-backed email queue with retries
- SQLite for persistence, single Docker container to deploy

## Quick start

```bash
git clone <this repo> beacon
cd beacon
cp .env.example .env
# fill in RESEND_API_KEY, ROOT_EMAILS, EMAIL_FROM, BASE_URL
docker compose up -d
```

Visit `BASE_URL`, sign in via magic link, create a token.

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
