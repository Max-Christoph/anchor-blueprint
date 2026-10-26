# Enterprise Tool — Monorepo-Template

Produktionsnahes Template für ein internes Unternehmens-Tool: React-Frontend,
Go-Backend, Python-Worker und PostgreSQL 16, orchestriert über Docker Compose.

Alle vier Komponenten laufen als eigenständige Container, sind über ein
internes Bridge-Netzwerk verbunden und werden über einen einzigen Befehl
gestartet.

---

## 1. Quickstart

```bash
cp .env.example .env
docker compose up --build
```

| Dienst     | URL                                   | Zweck                        |
|------------|---------------------------------------|------------------------------|
| Frontend   | http://localhost:3000                 | Dashboard (Nginx + React)    |
| Backend    | http://localhost:8080/healthz         | Health-Check (Go)            |
| Backend    | http://localhost:8080/api/v1/status   | Systemmetriken               |
| Worker     | http://localhost:8000/docs            | OpenAPI-Doku (FastAPI)       |
| Datenbank  | `localhost:5432`                      | PostgreSQL 16                |

Alternativ mit Makefile:

```bash
make up      # .env anlegen (falls fehlend), Stack bauen und starten
make logs    # Logs folgen
make test    # Smoke-Test gegen den laufenden Stack
make down    # Stack stoppen (Daten bleiben)
make clean   # Stack + Volume entfernen (Datenverlust)
```

### Health-Zustand prüfen

```bash
docker compose ps
curl -fsS http://localhost:8080/healthz
./scripts/smoke-test.sh
```

---

## 2. Architektur

```
                Browser
                   │  :3000
            ┌──────▼───────┐
            │   frontend   │  Nginx, statische Assets + /api-Proxy
            └──────┬───────┘
                   │  /api/*  →  backend:8080
            ┌──────▼───────┐        ┌──────────────┐
            │   backend    │───────▶│    worker    │  POST /process/calc
            │   (Go)       │  HTTP  │  (FastAPI)   │
            └──────┬───────┘        └──────────────┘
                   │  pgx
            ┌──────▼───────┐
            │      db      │  PostgreSQL 16 + Named Volume
            └──────────────┘
```

- **Ein Netzwerk** (`enterprise-tool-net`): interne Namensauflösung
  (`db`, `backend`, `worker`), keine externen Abhängigkeiten.
- **Ein Volume** (`enterprise-tool-postgres-data`): Daten überleben `down`,
  werden erst bei `down -v` gelöscht.
- **Startreihenfolge**: `backend` startet erst, wenn der DB-Healthcheck grün
  ist; `frontend` erst, wenn das Backend gesund ist (Nginx-Upstream löst
  `backend` beim Start auf).
- **Bind-Adresse**: standardmäßig `127.0.0.1` — nur lokal erreichbar. Für
  LAN-Zugriff `BIND_ADDRESS=0.0.0.0` in `.env` setzen.

---

## 3. Ordnerstruktur

```
enterprise-tool/
├── docker-compose.yml        # Root-Orchestrierung, Healthchecks, Netz/Volume
├── .env.example              # Alle Variablen mit Fallbacks
├── Makefile                  # up/down/logs/test/clean/fmt
├── scripts/smoke-test.sh     # HTTP-Smoke-Test inkl. Auth-Verhalten
├── db/
│   ├── init/001_schema.sql   # Schema + Seed, läuft nur beim ersten Start
│   └── README.md
├── backend/                  # Go 1.25, nur Stdlib + pgx
│   ├── cmd/server/main.go    # Config, Middleware-Kette, Graceful Shutdown
│   ├── internal/api/         # Router, Handler, Response-Envelope
│   ├── internal/middleware/  # RequestID, Logging, Recovery, CORS, Auth-Stub
│   ├── internal/store/       # pgxpool-Repository, Ping, Queries
│   ├── internal/worker/      # HTTP-Client zum Python-Worker
│   └── Dockerfile            # Multi-Stage: golang:alpine → alpine, non-root
├── frontend/                 # React 19 + TypeScript + Vite + Tailwind 4
│   ├── src/components/       # Layout-Shell, StatusBadge, Tabelle, Job-Panel
│   ├── src/services/api.ts   # typisierter Fetch-Client + ApiError
│   ├── src/App.tsx           # Dashboard, Status-Polling, Daten, Job-Trigger
│   ├── nginx.conf            # SPA-Fallback, /api-Proxy, Security-Header
│   └── Dockerfile            # Multi-Stage: node:alpine → nginx:alpine
└── worker/                   # Python 3.11 + FastAPI + Uvicorn
    ├── app/main.py           # Endpunkte, Lifespan, Fehlerformat
    ├── app/processing.py     # reine Berechnungsfunktionen
    ├── app/models.py         # Pydantic-Schemas mit Validierung
    ├── app/settings.py       # Env-Konfiguration
    └── Dockerfile            # python:3.11-slim, non-root (UID 10001)
```

---

## 4. API-Verträge

### Backend (Go)

| Methode | Pfad                | Auth | Beschreibung                                     |
|---------|---------------------|------|--------------------------------------------------|
| GET     | `/healthz`          | –    | Ping auf PostgreSQL; `503` wenn DB nicht erreichbar |
| GET     | `/api/v1/status`    | –    | Uptime, Go-Version, Goroutines, Speicher, DB-Latenz |
| GET     | `/api/v1/data`      | ✅   | Beispiel-Datensätze aus `example_entries`        |
| POST    | `/api/v1/process`   | ✅   | `{"values":[1,2,3]}` → Worker `/process/calc`    |

Fehlerformat (einheitlich):

```json
{"error":{"code":"unauthorized","message":"missing or invalid credentials","request_id":"8bf53ee17b29b36e"}}
```

### Worker (Python)

| Methode | Pfad            | Beschreibung                                    |
|---------|-----------------|-------------------------------------------------|
| GET     | `/healthz`      | Liveness des Workers                            |
| GET     | `/docs`         | Interaktive OpenAPI-Doku                        |
| POST    | `/process/calc` | Aggregat: count, sum, mean, median, min, max, stddev, p90 |

Grenzen: maximal 10 000 Werte (`MAX_VALUES`), nur endliche Zahlen,
`extra="forbid"` gegen unbekannte Felder.

---

## 5. Authentifizierung (Stub — vor Produktivbetrieb ersetzen)

Die Middleware in `backend/internal/middleware/auth.go` ist ein **Stub**:

- **Bearer-Token**: `Authorization: Bearer <AUTH_BEARER_TOKEN>`,
  Vergleich mit `subtle.ConstantTimeCompare`.
- **Header-Identität**: `X-User-Email: name@firma.example`, Domain muss in
  `AUTH_ALLOWED_EMAIL_DOMAINS` stehen.
- **Deny-by-Default**: öffentlich sind ausschließlich `/healthz` und
  `/api/v1/status`. Alles andere — auch unbekannte Pfade — liefert ohne
  gültige Identität `401` (keine Pfad-Enumeration).
- Erfolgreiche Authentifizierung landet als `Identity{Subject, Method}` im
  Context und im Access-Log (`user`, `auth_method`).

> **Wichtig:** `X-User-Email` ist nicht fälschungssicher. In Produktion
> ersetzt man den Stub durch OIDC/JWT-Validierung (JWKS, `iss`/`aud`/`exp`)
> oder mTLS am Ingress. Die Signatur der Middleware (`func(http.Handler) http.Handler`)
> bleibt dabei unverändert — nur `identify()` wird ausgetauscht.

Weitere Härtung im Template: CORS nur für explizite Origins, Security-Header
und CSP in `frontend/nginx.conf`, non-root-Container (Backend UID 10001,
Worker UID 10001), `ReadHeaderTimeout` gegen Slowloris, Request-Body-Limit
1 MiB im Backend.

---

## 6. Konfiguration

Alle Variablen stehen in `.env.example` und haben in `docker-compose.yml`
einen Fallback. Pflichtwerte ohne Fallback werden beim Start validiert
(`DATABASE_URL`, `AUTH_BEARER_TOKEN`, `AUTH_ALLOWED_EMAIL_DOMAINS`) — das
Backend bricht bei Fehlkonfiguration ab, statt unsicher zu starten.

| Variable | Default | Wirkung |
|----------|---------|---------|
| `BIND_ADDRESS` | `127.0.0.1` | Host-Bind für alle Ports |
| `FRONTEND_PORT` / `BACKEND_PORT` / `WORKER_PORT` / `DB_PORT` | `3000` / `8080` / `8000` / `5432` | Host-Ports |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | `enterprise` / `enterprise` / `change-me-in-development` | DB-Zugang |
| `AUTH_BEARER_TOKEN` | `dev-bearer-token-change-me` | Token für Service-Aufrufe |
| `AUTH_ALLOWED_EMAIL_DOMAINS` | `example.com` | Erlaubte Header-Domains (CSV) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000,...` | Erlaubte Browser-Origins |
| `WORKER_TIMEOUT` | `10s` | Timeout Backend → Worker |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful-Shutdown-Fenster |
| `VITE_API_BASE_URL` | leer | leer = relative Pfade über Nginx-Proxy |
| `VITE_DEMO_USER_EMAIL` | `demo.user@example.com` | Identität des Auth-Stubs im Browser |
| `WORKER_MAX_VALUES` | `10000` | Obergrenze Werte pro Job |

Echte Secrets gehören nicht in `.env`, sondern in einen Secret-Store
(Docker Secrets, Vault, Cloud Secret Manager) und werden als Umgebungs­variablen
injiziert.

---

## 7. Neuen Endpunkt hinzufügen

### Backend (Go)

1. Route registrieren in `backend/internal/api/router.go`:
   ```go
   mux.HandleFunc("GET /api/v1/reports", h.Reports)
   ```
2. Handler in `backend/internal/api/handlers.go` ergänzen — Guard-Clauses
   zuerst, dann Fachlogik, Fehler über `respondError`.
3. Datenzugriff als Methode in `backend/internal/store/store.go` ergänzen
   (kein SQL in Handlern).
4. Externe Aufrufe über einen Client in `internal/…` kapseln
   (Muster: `internal/worker/client.go`), niemals direkt im Handler.
5. Auth: nichts zu tun — Deny-by-Default greift automatisch. Soll der
   Endpunkt öffentlich sein, ihn in `main.go` zu `authCfg.PublicPaths`
   hinzufügen (bewusste Entscheidung).
6. Neustart: `docker compose up -d --build backend`.

### Worker (Python)

1. Pydantic-Modelle in `worker/app/models.py` ergänzen.
2. Reine Funktion in `worker/app/processing.py` implementieren (testbar,
   ohne Framework-Abhängigkeit).
3. Route in `worker/app/main.py` registrieren (`@app.post("/process/…")`).
4. Backend-Aufruf: Methode in `backend/internal/worker/client.go` ergänzen.

### Frontend (React)

1. Typen in `frontend/src/services/types.ts` (Spiegel des JSON-Vertrags).
2. Aufruf in `frontend/src/services/api.ts` ergänzen — alle Fehler laufen
   über `ApiError`/`describeError`.
3. UI in `frontend/src/components/` bauen und in `App.tsx` einhängen.
4. Lokale Entwicklung ohne Container: `npm install && npm run dev`
   (Dev-Proxy `/api` → `localhost:8080`).

---

## 8. Datenbank & Migrationen

- `db/init/*.sql` wird **nur beim ersten Start** eines leeren Volumes
  ausgeführt. Spätere Schemaänderungen brauchen ein Migrationswerkzeug
  (`golang-migrate`, `goose`, `atlas`).
- Volume zurücksetzen (löscht alle Daten): `docker compose down -v`.
- Direkter DB-Zugriff:
  ```bash
  docker compose exec db psql -U enterprise -d enterprise -c '\dt'
  ```

---

## 9. Graceful Shutdown

`docker compose stop backend` sendet `SIGTERM`; das Backend stoppt die
Annahme neuer Requests, wartet laufende Requests bis `SHUTDOWN_TIMEOUT` ab
und schließt danach den Connection-Pool. Im Log sichtbar als:

```json
{"level":"INFO","msg":"shutdown signal received"}
{"level":"INFO","msg":"shutdown complete"}
```

---

## 10. Troubleshooting

| Symptom | Ursache / Lösung |
|---------|------------------|
| `Bind for 0.0.0.0:8080 failed: port is already allocated` | Host-Port belegt. `BACKEND_PORT=18080` in `.env` setzen (gilt analog für 3000/8000/5432). |
| Backend startet nicht, `ping database after 6 attempts` | DB nicht bereit oder falsche Credentials. `docker compose logs db`, `POSTGRES_*` prüfen. |
| `401` auf `/api/v1/data` | Auth-Stub: `X-User-Email` mit erlaubter Domain oder `Authorization: Bearer <AUTH_BEARER_TOKEN>` senden. |
| Frontend zeigt „Backend offline“ | Backend-Container nicht gesund (`docker compose ps`) oder `/api`-Proxy falsch konfiguriert. |
| Änderungen an `db/init/*.sql` wirken nicht | Init-Skripte laufen nur bei leerem Volume: `docker compose down -v`. |
| `502` auf `/api/v1/process` | Worker nicht erreichbar — `docker compose logs worker`. |

---

## 11. Nächste Schritte für den Produktivbetrieb

1. Auth-Stub durch OIDC/JWT ersetzen (Issuer, JWKS, Audience, Rollen).
2. Secrets in einen Secret-Store auslagern.
3. TLS terminieren (Ingress/Reverse-Proxy) und `BIND_ADDRESS` intern halten.
4. Migrationen einführen und `db/init` als reines Bootstrap behandeln.
5. CI-Pipeline: `go vet` + `go test`, `npm run build` (inkl. Typecheck),
   `ruff`/`pytest`, Container-Scan (Trivy), Image-Signatur.
6. Observability: `/metrics` (Prometheus) ergänzen, Logs zentral sammeln.
