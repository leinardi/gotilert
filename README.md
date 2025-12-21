# Gotilert

**Gotify-compatible** gateway that forwards notifications to **Prometheus Alertmanager**.
Use it when a tool supports Gotify (but not Alertmanager) and you still want a single alerting pipeline.

---

## 💡 What is "Gotilert"?

Gotilert exposes a small subset of Gotify's app API (mainly `POST /message`), validates/authenticates the request using an **app token**, then turns
the message into a **firing Alertmanager alert** (via `/api/v2/alerts`).

It's intentionally small: no users, no Web UI, no message history, just a bridge.

---

## 🪶 Resource usage

Gotilert is designed to be lightweight. In typical Docker deployments it can sit around **~2.5 MiB RAM** when idle (exact usage depends on platform,
Go version, and container runtime settings).

---

## 📦 What Gotilert Does

- Implements **Gotify-ish** API:
    - `POST /message` (JSON + form)
    - Token auth via:
        - `X-Gotify-Key: <token>`
        - `?token=<token>`
        - `Authorization: Bearer <token>`
- Forwards to Alertmanager:
    - `POST /api/v2/alerts`
    - Optional **Basic Auth** or **Bearer token**
    - Optional `tlsConfig.insecureSkipVerify` (useful for homelab self-signed setups)
    - **Bounded retries** with short backoff for transient hiccups (timeouts, connection errors) and upstream `429/5xx`
- Mapping:
    - Gotify `priority` → Alert severity via `defaults.severityFromPriority` (required)
    - TTL controls `startsAt/endsAt` (config, required: `defaults.ttl > 0`)
    - Gotify well-known `extras` → Alertmanager **annotations**:
        - `client::display.contentType` → `gotify_content_type`
        - `client::notification.click.url` → `gotify_click_url`
        - `client::notification.bigImageUrl` → `gotify_big_image_url`
        - `android::action.onReceive.intentUrl` → `gotify_on_receive_intent_url`
- Routing flexibility:
    - Per-app token config: `appName`, labels, severity overrides
    - `alertname` can be overridden globally (defaults) and per-app
- Alert identity (Gotify-like behavior):
    - Alertmanager deduplicates alerts by their **labels**
    - Gotilert includes a unique `gotilert_id` **label** per incoming message so every `POST /message` becomes a distinct alert

---

## 🔌 Endpoints

- `GET /healthz` → `200 ok`
- `GET /readyz` → `200 ok` when Gotilert considers itself ready to forward
- `POST /message` → Gotify-ish JSON response (and forwards to Alertmanager)

---

## 🚀 Quick Start

### 1) Create a config file

A complete example is in: `examples/gotilert.yaml`.

### 2) Run locally (binary)

```bash
./gotilert --config.file=examples/gotilert.yaml --log-level=info --log-format=plain
````

### 3) Run with Docker (single host)

```bash
docker run --rm \
  -p 8008:8008 \
  -v "$(pwd)/examples/gotilert.yaml:/config/gotilert.yaml:ro" \
  ghcr.io/leinardi/gotilert:latest \
  --config.file=/config/gotilert.yaml --log-level=info --log-format=plain
```

### 4) Docker Compose example

See: [`deployments/docker/docker-compose.yaml`](deployments/docker/docker-compose.yaml)

---

## 📨 Send a notification (Gotify-compatible)

### JSON

```bash
curl -sS -X POST \
  -H 'X-Gotify-Key: TOKEN_FOR_TRUENAS' \
  -H 'Content-Type: application/json' \
  -d '{"message":"Disk warning","title":"truenas","priority":7}' \
  http://localhost:8008/message
```

### Form

```bash
curl -sS -X POST \
  -H 'Authorization: Bearer TOKEN_FOR_TRUENAS' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data 'message=Hello&title=test&priority=5' \
  http://localhost:8008/message
```

Validation rules:

- `message` is **required**
- `priority` defaults to `5` if missing and must be `>= 0`
- `title` is optional

---

## ⚙️ Configuration

Gotilert is configured via YAML and loaded with:

```bash
--config.file=/path/to/gotilert.yaml
```

### Example

See: [`examples/gotilert.yaml`](examples/gotilert.yaml)

### TTL (required)

`defaults.ttl` must be **> 0**. It controls:

- `startsAt = now()`
- `endsAt = now() + defaults.ttl`

Typical values:

- `15m` / `1h` for "notification-style" messages
- longer TTLs keep alerts firing longer (and can affect repeat notifications in Alertmanager)

### Mapping rules

Severity mapping precedence:

1. `apps.<token>.severityFromPriority` (if present)
2. `defaults.severityFromPriority` (always required)

Labels are merged in this order:

1. `defaults.labels`
2. `apps.<token>.labels`
3. computed labels (e.g., `alertname`, `app`, `severity`, …)

Alert name precedence:

1. `apps.<token>.alertname` (if set)
2. `defaults.alertname`
3. `GotilertNotification` (fallback)

---

## 🔔 Alertmanager routing tips (important)

Alertmanager notification delivery depends on `route.group_by` and timers.
If you want Gotilert to behave closer to Gotify (**one notification per message**),
group Gotilert alerts by `gotilert_id`.

Example:

```yaml
route:
  receiver: ops-default
  group_by: [ 'environment', 'alertname', 'instance' ]
  group_wait: 10s
  group_interval: 2m
  repeat_interval: 4h
  routes:
    - receiver: ops-default
      matchers:
        - source="gotilert"
      group_by: [ 'gotilert_id' ]
      group_wait: 0s
      group_interval: 10s
      repeat_interval: 24h
```

Tip: set `defaults.labels.environment` (e.g. `prod`) so alert grouping never mixes environments.

---

## ✅ Health & Readiness

- `/healthz` is a basic liveness endpoint.
- `/readyz` is intended to reflect "can forward" (lightweight readiness check).

---

## 🔐 Security Notes

- Treat app tokens as **secrets** (don't print them, don't commit them).
- Gotilert is best run on an **internal network** (it's an ingress point for alerts).
- If you expose it externally, put it behind a reverse proxy and apply:

    - TLS
    - IP allowlists / auth
    - rate limiting

---

## 🤝 Contributing

PRs and issues are welcome.

Suggested local checks:

- `make check`
