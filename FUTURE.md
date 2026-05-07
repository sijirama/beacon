# Future features

Ideas worth picking up later. Loosely ordered by how much they change what
Beacon *is* vs. just polishing what's already here.

## Routing & channels

- **Multi-channel notifications** — Slack, Discord, Telegram, ntfy.sh,
  generic webhook-forward, web push. Today every event becomes an email.
- **Per-token destinations** — let each token (or each level) pick its
  own channel(s), e.g. `info` → digest, `danger` → email + Slack.

## Reliability & ergonomics

- **Heartbeat / dead-man's switch monitors** — register an expected
  cadence ("this cron should ping every 1h"); fire a `danger` event
  automatically when the ping doesn't arrive. Healthchecks.io-style; a
  natural fit for something called "beacon".
- **Idempotency keys on `/emit`** — accept an `Idempotency-Key` header
  so a retrying client doesn't create duplicate events.
- **Snooze / mute rules** — silence events by source / level / regex
  during deploys, with an auto-expire.
- **Acknowledgements + escalation** — danger events get an "ack" link in
  the email; if not ack'd within N minutes, re-fire on a second channel.
- **Retention policy** — auto-prune events older than N days; keep
  per-source counts so dashboards still work.

## Surfaces

- **Token scopes** — emit-only vs read-only, so a dashboard token can be
  shared without granting the ability to forge events.
- **CLI `beacon send`** — `beacon send -l danger "backup failed"` reading
  the token from env. Trivial wrapper, big ergonomic win for shell.
- **Public per-source status pages** — read-only timeline of one source,
  shareable URL.

## Already shipped

- Live tail via SSE on `/`.
- Search by metadata (`metadata.host=db-01` syntax).
- Dedup of repeated notifications within a 60s window
  (events still stored — only the notification is suppressed).
