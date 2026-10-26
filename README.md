# Anchor Blueprint

Produktionsnahes Monorepo-Template für ein internes Unternehmens-Tool:
React-Frontend, Go-Backend mit OIDC/JWT-Authentifizierung, Python-Worker und
PostgreSQL 16 — orchestriert über Docker Compose, abgesichert durch eine
GitHub-Actions-CI.

| Schicht | Technik | Image |
|---|---|---|
| Frontend | React 19, TypeScript 5.9, Vite 8, Tailwind 4 | `node:24-alpine` → `nginx:alpine` |
| Backend | Go 1.27, Stdlib-`ServeMux`, pgx v5, `golang-jwt/v5` | `golang:1.27-alpine` → `alpine:3.22` |
| Worker | Python 3.11, FastAPI, Uvicorn, Pydantic 2 | `python:3.11-slim` |
| Datenbank | PostgreSQL 16 | `postgres:16-alpine` |

---

## 1. Quickstart

```bash
cp .env.example .env
docker compose up -d
```

| Dienst | URL | Zweck |
|---|---|---|
| Frontend | http://localhost:3000 | Dashboard (Nginx + React) |
| Backend | http://localhost:8080/healthz | Health-Check (Go) |
| Backend | http://localhost:8080/api/v1/status | Systemmetriken inkl. `auth_mode` |
| Worker | http://localhost:8000/docs | OpenAPI-Doku (FastAPI) |
| Datenbank | `localhost:5432` | PostgreSQL 16 |

### Makefile

```bash
make up        # .env anlegen (falls fehlend), Stack im Hintergrund starten
make down      # Stack stoppen (Volumes bleiben)
make test      # Go-, Frontend- und Worker-Tests lokal
make smoke     # HTTP-Smoke-Test gegen den laufenden Stack
make lint      # go vet + golangci-lint + ruff
make logs      # Logs folgen
make ps        # Container-Status
make rebuild   # Images ohne Cache neu bauen und starten
make clean     # Stack + Volume entfernen (Datenverlust)
```

> `make test` erwartet lokal: Go 1.27, Node ≥ 22.12 und Python 3.11.
> Das Worker-Venv (`.venv`) legt das Target selbst an.

---

## 2. Architektur

```
                Browser (JWT oder Dev-Header)
                   │  :3000
            ┌──────▼───────┐
            │   frontend   │  Nginx: statische Assets + /api-Proxy
            └──────┬───────┘
                   │  /api/* → backend:8080
            ┌──────▼───────┐   Bearer-JWT   ┌──────────────┐
            │   backend    │──── prüft ────▶│  OIDC-JWKS   │ (iss/aud, RS256)
            │   (Go)       │                └──────────────┘
            └──┬────────┬──┘
               │        │ POST /process/calc
               │        └───────────────▶ ┌──────────────┐
               │                          │    worker    │ (FastAPI)
               │                          └──────────────┘
               │ pgx
        ┌──────▼───────┐
        │      db      │  PostgreSQL 16 + Named Volume
        └──────────────┘
```

- **Ein Netzwerk** (`anchor-blueprint-net`) für interne Namensauflösung.
- **Ein Volume** (`anchor-postgres-data`) — überlebt `down`, verschwindet bei `down -v`.
- **Startreihenfolge** über Healthchecks: `backend` nach `db`, `frontend` nach `backend`.
- **Host-Binding** Default `127.0.0.1`; LAN-Zugriff bewusst über `BIND_ADDRESS=0.0.0.0`.
- **Request-Pfad im Backend**: `RequestID → Logging → Recovery → CORS → Auth → Router`.

---

## 3. Ordnerstruktur

```
anchor-blueprint/
├── docker-compose.yml           # Orchestrierung, Healthchecks, Netz/Volume
├── .env.example                 # alle Variablen mit Fallbacks
├── Makefile                     # up/down/test/smoke/lint/…
├── .github/workflows/ci.yml     # CI: Backend, Frontend, Worker, Security
├── scripts/smoke-test.sh        # HTTP-Smoke-Test (auth-mode-aware)
├── db/
│   ├── init/001_schema.sql      # Bootstrap-Schema + Seed (nur erster Start)
│   └── README.md
├── backend/
│   ├── cmd/server/main.go       # Config, Verifier-Auswahl, Graceful Shutdown
│   ├── internal/api/            # Router, Handler, Fehler-Envelope
│   ├── internal/auth/           # JWT/JWKS-Verifier, Dev-Header, Middleware
│   ├── internal/httpx/          # Context-Helfer (Request-ID, Identity)
│   ├── internal/middleware/     # RequestID, Logging, Recovery, CORS
│   ├── internal/store/          # pgxpool-Repository
│   ├── internal/worker/         # HTTP-Client zum Python-Worker
│   ├── .golangci.yml            # Lint-Konfiguration (CI nutzt dieselbe)
│   └── Dockerfile               # Multi-Stage, non-root (UID 10001)
├── frontend/
│   ├── src/components/          # Layout-Shell, StatusBadge, Tabelle, Job-Panel
│   ├── src/services/api.ts      # typisierter Fetch-Client (Bearer oder Header)
│   ├── src/App.tsx              # Dashboard, Status-Polling, Job-Trigger
│   ├── nginx.conf               # SPA-Fallback, /api-Proxy, CSP, Security-Header
│   └── Dockerfile               # node:24-alpine → nginx:alpine
└── worker/
    ├── app/main.py              # Endpunkte, Lifespan, Fehlerformat
    ├── app/processing.py        # reine Berechnungsfunktionen
    ├── app/models.py            # Pydantic-Schemas (extra=forbid, Limits)
    ├── app/settings.py          # Env-Konfiguration
    ├── tests/                   # pytest: Processing + API
    ├── pyproject.toml           # ruff- und pytest-Konfiguration
    └── Dockerfile               # python:3.11-slim, non-root (UID 10001)
```

---

## 4. Authentifizierung

`AUTH_MODE` schaltet das Verfahren um. Standard im Code ist **`jwt`**.

### `AUTH_MODE=jwt` (Produktion)

RS256-JWT-Validierung gegen ein OIDC-JWKS:

| Variable | Pflicht | Bedeutung |
|---|---|---|
| `OIDC_ISSUER` | ja | erwarteter `iss`-Claim, z. B. Entra-ID-Tenant-URL |
| `OIDC_AUDIENCE` | ja | erwarteter `aud`-Claim (API-App-ID) |
| `OIDC_JWKS_URL` | ja | JWKS-Endpunkt der Schlüssel |
| `OIDC_JWKS_CACHE_TTL` | nein (15m) | Cache-Dauer der Schlüssel im Speicher |
| `OIDC_HTTP_TIMEOUT` | nein (5s) | Timeout beim JWKS-Abruf |
| `AUTH_CLOCK_LEEWAY` | nein (30s) | Toleranz für Clock-Skew bei `exp`/`nbf` |

Geprüft wird: Signatur (nur `RS256`, `kid`-Lookup), `exp` (Pflicht-Claim),
`iat`, `iss`, `aud`, `sub` (Pflicht). Unbekannte `kid` lösen genau einen
JWKS-Refresh aus (Key-Rotation); nicht abrufbares JWKS antwortet mit `503`
statt `401`.

Aus dem Token werden `sub`, `email` (Fallback `preferred_username`), `roles`
und `groups` gelesen und als `httpx.Identity` in den Request-Context gelegt;
das Access-Log führt `user` und `auth_method`.

### `AUTH_MODE=dev_header` / `mock` (nur lokal)

- `X-User-Email: name@firma.example` mit Domain-Allowlist
  (`AUTH_ALLOWED_EMAIL_DOMAINS`) **oder**
- statischer `AUTH_BEARER_TOKEN`.
- Der Server loggt beim Start eine Warnung: der Header ist **nicht
  fälschungssicher**. Im `jwt`-Modus wird `X-User-Email` vollständig ignoriert.

### Grundregeln

- **Deny-by-Default**: öffentlich sind ausschließlich `/healthz` und
  `/api/v1/status`. Alles andere — auch unbekannte Pfade — liefert ohne
  gültige Identität `401` (keine Pfad-Enumeration).
- `OPTIONS` (CORS-Preflight) passiert die Auth-Middleware, wird aber von der
  CORS-Middleware beantwortet.
- CORS nur für explizite Origins, CSP und Security-Header in `nginx.conf`,
  non-root-Container, `ReadHeaderTimeout`, Request-Body-Limit 1 MiB.

### JWT lokal testen

Die Go-Tests erzeugen ein RSA-Schlüsselpaar, signieren Test-Tokens und
validieren sie gegen einen gemockten JWKS-Server (`httptest`) — ohne Cloud:

```bash
cd backend && go test -race ./internal/auth -v
```

Abgedeckt: gültiges Token, abgelaufenes Token, falscher Issuer, falsche
Audience, manipulierte Signatur, unbekannte `kid`, Algorithmus-Verwechslung
(HS256), fehlendes `sub`, JWKS-Cache, Key-Rotation, JWKS-Ausfall,
Deny-by-Default und Header-Sperre im JWT-Modus.

---

## 5. API-Verträge

### Backend (Go)

| Methode | Pfad | Auth | Beschreibung |
|---|---|---|---|
| GET | `/healthz` | – | Ping auf PostgreSQL; `503` wenn DB nicht erreichbar |
| GET | `/api/v1/status` | – | Uptime, Go-Version, Goroutines, Speicher, DB-Latenz, `auth_mode` |
| GET | `/api/v1/data` | ✅ | Beispiel-Datensätze aus `example_entries` |
| POST | `/api/v1/process` | ✅ | `{"values":[1,2,3]}` → Worker `/process/calc` |

Fehlerformat (einheitlich):

```json
{"error":{"code":"unauthorized","message":"missing or invalid credentials","request_id":"8bf53ee17b29b36e"}}
```

### Worker (Python)

| Methode | Pfad | Beschreibung |
|---|---|---|
| GET | `/healthz` | Liveness |
| GET | `/docs` | interaktive OpenAPI-Doku |
| POST | `/process/calc` | Aggregat: count, sum, mean, median, min, max, stddev, p90 |

Grenzen: max. 10 000 Werte (`MAX_VALUES`), nur endliche Zahlen,
`extra="forbid"`.

---

## 6. Konfiguration

Alle Variablen stehen in `.env.example` und haben in `docker-compose.yml`
Fallbacks. Pflichtwerte werden beim Start validiert — fehlt etwas, bricht der
Prozess ab, statt unsicher zu starten.

| Variable | Default | Wirkung |
|---|---|---|
| `BIND_ADDRESS` | `127.0.0.1` | Host-Bind für alle Ports |
| `FRONTEND_PORT` / `BACKEND_PORT` / `WORKER_PORT` / `DB_PORT` | `3000` / `8080` / `8000` / `5432` | Host-Ports |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | `anchor` / `anchor` / `change-me-in-development` | DB-Zugang |
| `AUTH_MODE` | `dev_header` in Compose, `jwt` im Code | Auth-Verfahren |
| `AUTH_ALLOWED_EMAIL_DOMAINS` | `example.com` | erlaubte Header-Domains (Dev) |
| `AUTH_BEARER_TOKEN` | `dev-bearer-token-change-me` | statischer Token (Dev) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000,…` | erlaubte Browser-Origins |
| `WORKER_TIMEOUT` | `10s` | Timeout Backend → Worker |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful-Shutdown-Fenster |
| `VITE_API_BASE_URL` | leer | leer = relative Pfade über Nginx-Proxy |
| `VITE_DEMO_USER_EMAIL` | `demo.user@example.com` | Header-Identität im Dev-Modus |
| `VITE_BEARER_TOKEN` | leer | optionaler Bearer für Dev-Aufrufe |
| `WORKER_MAX_VALUES` | `10000` | Obergrenze Werte pro Job |

Echte Secrets gehören in einen Secret-Store (Docker Secrets, Vault, Cloud
Secret Manager) und werden als Umgebungsvariablen injiziert.

---

## 7. CI-Pipeline (`.github/workflows/ci.yml`)

Läuft bei Push und Pull Request auf `main`, bricht laufende Läufe derselben
Ref ab (`concurrency`) und hat nur `contents: read`.

| Job | Schritte |
|---|---|
| `backend` | `go vet ./...` · golangci-lint 2.15 (inkl. staticcheck) · `go test -v -race ./...` |
| `frontend` | `npm ci` · `npx tsc --noEmit` · `npm run build` |
| `worker` | `ruff check .` · `ruff format --check .` · `pytest` |
| `security` | Trivy `fs`-Scan, `HIGH`/`CRITICAL`, `--ignore-unfixed`, Exit 1 bei Fund |

Lokal dieselben Prüfungen: `make lint` und `make test`. Der Trivy-Scan lässt
sich ohne CI reproduzieren:

```bash
docker run --rm -v "$PWD":/repo aquasec/trivy:latest fs \
  --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed /repo
```

---

## 8. Neuen Endpunkt hinzufügen

### Backend (Go)

1. Route in `backend/internal/api/router.go` registrieren:
   ```go
   mux.HandleFunc("GET /api/v1/reports", h.Reports)
   ```
2. Handler in `handlers.go`: Guard-Clauses zuerst, dann Fachlogik, Fehler über
   `respondError`.
3. Datenzugriff als Methode in `internal/store/store.go` (kein SQL im Handler).
4. Externe Aufrufe kapseln (Muster `internal/worker/client.go`).
5. Auth: nichts zu tun — Deny-by-Default greift. Soll der Pfad öffentlich sein,
   ihn in `main.go` zu `PublicPaths` hinzufügen (bewusste Entscheidung).
6. `docker compose up -d --build backend`.

### Worker (Python)

1. Pydantic-Modell in `worker/app/models.py`.
2. Reine Funktion in `worker/app/processing.py`.
3. Route in `worker/app/main.py`.
4. Backend-Aufruf in `backend/internal/worker/client.go` ergänzen.
5. Test in `worker/tests/` ergänzen — `make test` führt ihn aus.

### Frontend (React)

1. Typen in `frontend/src/services/types.ts`.
2. Aufruf in `frontend/src/services/api.ts` (Fehler über `ApiError`).
3. UI in `src/components/` und Einhängen in `App.tsx`.
4. Lokal ohne Container: `npm install && npm run dev` (Proxy `/api` → `:8080`).

---

## 9. Datenbank & Migrationen

- `db/init/*.sql` läuft **nur beim ersten Start** eines leeren Volumes.
  Schemaänderungen brauchen ein Migrationswerkzeug (`golang-migrate`,
  `goose`, `atlas`).
- Volume zurücksetzen (löscht alle Daten): `docker compose down -v`.
- Direkter Zugriff:
  ```bash
  docker compose exec db psql -U anchor -d anchor -c '\dt'
  ```

---

## 10. Graceful Shutdown

`docker compose stop backend` sendet `SIGTERM`: keine neuen Requests, laufende
Requests bis `SHUTDOWN_TIMEOUT`, dann Pool schließen:

```json
{"level":"INFO","msg":"shutdown signal received"}
{"level":"INFO","msg":"shutdown complete"}
```

---

## 11. Troubleshooting

| Symptom | Ursache / Lösung |
|---|---|
| `Bind for 0.0.0.0:8080 failed: port is already allocated` | Host-Port belegt → `BACKEND_PORT` (bzw. 3000/8000/5432) in `.env` ändern. |
| Backend `unhealthy`, Log `hostname resolving error … lookup db` | Container wurde nach einem fehlgeschlagenen Start halb ans Netz gehängt → `docker compose down && docker compose up -d`. |
| `401` auf `/api/v1/data` bei `AUTH_MODE=jwt` | `Authorization: Bearer <JWT>` senden; `X-User-Email` wird bewusst ignoriert. |
| `401` im Dev-Modus | Domain nicht in `AUTH_ALLOWED_EMAIL_DOMAINS` oder Token falsch. |
| `503` mit `auth_backend_unavailable` | JWKS nicht erreichbar (`OIDC_JWKS_URL`) — Netzwerk/Issuer prüfen. |
| Frontend zeigt „Backend offline“ | Backend nicht gesund (`make ps`) oder `/api`-Proxy falsch. |
| Änderungen an `db/init/*.sql` wirken nicht | Init läuft nur bei leerem Volume → `docker compose down -v`. |
| `502` auf `/api/v1/process` | Worker nicht erreichbar → `docker compose logs worker`. |

---

## 12. Nächste Ausbaustufen

1. Migrationen etablieren und `db/init` als reines Bootstrap behandeln.
2. Rollen-/Scope-Autorisierung auf Basis der `roles`/`groups`-Claims (RBAC).
3. Observability: `/metrics` (Prometheus), Trace-IDs, zentrale Log-Sammlung.
4. Trivy zusätzlich auf die gebauten Images (`image`-Scan) und auf `main`-Merge.
5. Deployment-Pipeline (Staging → Production) mit signierten Images.
