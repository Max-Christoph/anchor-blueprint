# Anchor Blueprint

Produktionsnahes Monorepo-Template für ein internes Unternehmens-Tool:
React-Frontend, Go-Backend mit OIDC/JWT-Authentifizierung, **RBAC** und
**persistentem Audit-Log**, Python-Worker, PostgreSQL 16 mit **DDL-Migrationen
(golang-migrate)** und separatem Seed-Command — orchestriert über Docker Compose,
abgesichert durch eine GitHub-Actions-CI.

| Schicht | Technik | Image |
|---|---|---|
| Frontend | React 19, TypeScript 5.9, Vite 8, Tailwind 4 | `node:24-alpine` → `nginx:alpine` |
| Backend | Go 1.27, `ServeMux`, pgx v5, `golang-jwt/v5`, `golang-migrate/v4` | `golang:1.27-alpine` → `alpine:3.22` |
| Worker | Python 3.11, FastAPI, Uvicorn, Pydantic 2 | `python:3.11-slim` |
| Datenbank | PostgreSQL 16 (Schema + Seed über Migrationen) | `postgres:16-alpine` |

---

## 1. Quickstart

```bash
cp .env.example .env
docker compose up -d
```

| Dienst | URL | Zweck |
|---|---|---|
| Frontend | http://localhost:3000 | Dashboard, RBAC-Demo, Daten, Worker-Job |
| Backend | http://localhost:8080/healthz | Health-Check (Go) |
| Backend | http://localhost:8080/api/v1/status | Systemmetriken inkl. `auth_mode` |
| Worker | http://localhost:8000/docs | OpenAPI-Doku (FastAPI) |
| Datenbank | `localhost:5432` | PostgreSQL 16 |

Beim Start führt das Backend **automatisch** alle ausstehenden Migrationen aus
(`DB_AUTO_MIGRATE=true`), bevor der HTTP-Server lauscht — die Datenbank darf
also leer sein. Migrationen enthalten **nur DDL**; Beispieldaten kommen aus dem
separaten Seed-Schritt (`DB_SEED_DEV_DATA=true` beim Start oder `make seed`).

### Makefile

```bash
make up        # .env anlegen (falls fehlend), Stack im Hintergrund starten
make down      # Stack stoppen (Volumes bleiben)
make test      # Go- (inkl. RBAC + Migrationen), Frontend- und Worker-Tests
make smoke     # HTTP-Smoke-Test inkl. RBAC-Matrix und Audit-Log
make seed      # Dev-Beispieldaten idempotent einspielen
make lint      # go vet + golangci-lint + ruff
make logs      # Logs folgen
make ps        # Container-Status
make rebuild   # Images ohne Cache neu bauen und starten
make clean     # Stack + Volume entfernen (Datenverlust)
```

> `make test` erwartet lokal Go 1.27, Node ≥ 22.12 und Python 3.11.
> Der Migrations-Integrationstest nutzt `TEST_DATABASE_URL` (Default: der
> Compose-Postgres auf `127.0.0.1:5432`) und legt sich dort eine temporäre
> Datenbank an; ist keine erreichbar, wird er übersprungen.

---

## 2. Architektur

```
                Browser (JWT oder Dev-Header + X-User-Roles)
                   │  :3000
            ┌──────▼───────┐
            │   frontend   │  Nginx: Assets + /api-Proxy
            └──────┬───────┘
                   │  /api/*
            ┌──────▼───────┐  Bearer-JWT   ┌──────────────┐
            │   backend    │──prüft───────▶│  OIDC-JWKS   │
            │   (Go)       │               └──────────────┘
            │              │──RBAC────────▶ roles / groups
            └──┬────────┬──┘
               │        └────POST /process/calc────▶ ┌──────────┐
               │                                     │  worker  │
               │ pgx                                 └──────────┘
        ┌──────▼───────┐   golang-migrate (embed.FS) beim Start
        │      db      │   PostgreSQL 16 + Named Volume
        └──────────────┘
```

- **Ein Netzwerk** (`anchor-blueprint-net`), **ein Volume** (`anchor-postgres-data`).
- **Startreihenfolge** über Healthchecks: `backend` nach `db`, `frontend` nach `backend`.
- **Host-Binding** Default `127.0.0.1`; LAN-Zugriff bewusst via `BIND_ADDRESS=0.0.0.0`.
- **Request-Pfad**: `RequestID → Logging → Recovery → CORS → Auth → Router (RBAC pro Route)`.

---

## 3. Ordnerstruktur

```
anchor-blueprint/
├── docker-compose.yml           # Orchestrierung, Healthchecks, Netz/Volume
├── .env.example                 # alle Variablen mit Fallbacks
├── Makefile                     # up/down/test/smoke/lint/…
├── .github/workflows/ci.yml     # CI: Backend (+Postgres-Service), Frontend, Worker, Security
├── scripts/smoke-test.sh        # HTTP-Smoke-Test inkl. RBAC-Matrix
├── backend/
│   ├── cmd/server/main.go       # Config, Verifier, Migrationen, Seed, Graceful Shutdown
│   ├── cmd/seed/main.go          # Standalone-Seed (Dev-Beispieldaten)
│   ├── migrations/              # DDL-Migrationen + embed.FS (keine Seed-Daten!)
│   │   ├── 000001_init_schema.{up,down}.sql
│   │   ├── 000002_create_audit_logs.{up,down}.sql
│   │   └── embed.go
│   ├── internal/api/            # Router, Handler (inkl. RBAC- und Audit-Endpunkte)
│   ├── internal/audit/          # Audit-Eintrag, Recorder-Interface, Context-Helfer
│   ├── internal/auth/           # JWT/JWKS, Dev-Header, RBAC-Middleware (schreibt Audit)
│   ├── internal/httpx/          # Context-Helfer (Request-ID, Identity + Rollen-Helper)
│   ├── internal/seed/           # idempotente Dev-Beispieldaten
│   ├── internal/middleware/     # RequestID, Logging, Recovery, CORS
│   ├── internal/migrate/        # golang-migrate-Runner (embed.FS → DB)
│   ├── internal/store/          # pgxpool-Repository
│   ├── internal/worker/         # HTTP-Client zum Python-Worker
│   ├── .golangci.yml            # Lint-Konfiguration (CI nutzt dieselbe)
│   └── Dockerfile               # Multi-Stage, non-root (UID 10001)
├── frontend/
│   ├── src/components/          # Layout, StatusBadge, DataTable, ProcessPanel,
│   │                            # RoleSwitcher (nur Dev), RoleInfo (JWT), Notice
│   ├── src/services/api.ts      # typisierter Client (Bearer/Dev-Header + X-User-Roles)
│   ├── src/App.tsx              # Dashboard, Rollenwahl, 403-Feedback
│   ├── nginx.conf               # SPA-Fallback, /api-Proxy, CSP, Security-Header
│   └── Dockerfile               # node:24-alpine → nginx:alpine
└── worker/
    ├── app/                     # FastAPI-App, Processing, Modelle, Settings
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
| `OIDC_ISSUER` | ja | erwarteter `iss`-Claim (z. B. Entra-ID-Tenant-URL) |
| `OIDC_AUDIENCE` | ja | erwarteter `aud`-Claim (API-App-ID) |
| `OIDC_JWKS_URL` | ja | JWKS-Endpunkt |
| `OIDC_JWKS_CACHE_TTL` | nein (15m) | Cache-Dauer der Schlüssel |
| `OIDC_HTTP_TIMEOUT` | nein (5s) | Timeout beim JWKS-Abruf |
| `AUTH_CLOCK_LEEWAY` | nein (30s) | Toleranz für Clock-Skew |

Geprüft: Signatur (nur `RS256`, `kid`-Lookup), `exp` (Pflicht), `iat`, `iss`,
`aud`, `sub`. Unbekannte `kid` lösen genau einen JWKS-Refresh aus;
nicht abrufbares JWKS → `503`.

### `AUTH_MODE=dev_header` / `mock` (nur lokal)

- `X-User-Email: name@firma.example` mit Domain-Allowlist
  (`AUTH_ALLOWED_EMAIL_DOMAINS`) **oder** statischer `AUTH_BEARER_TOKEN`.
- Rollen über `X-User-Roles: Admin,Operator`; ohne Header greift
  `AUTH_DEV_DEFAULT_ROLE` (Default `User`).
- Der Server loggt beim Start eine Warnung; im `jwt`-Modus werden
  `X-User-Email` und `X-User-Roles` vollständig ignoriert.

### Grundregeln

- **Deny-by-Default**: öffentlich sind nur `/healthz` und `/api/v1/status`;
  alles andere liefert ohne Identität `401` (auch unbekannte Pfade).
- CORS nur für explizite Origins, CSP + Security-Header in `nginx.conf`,
  non-root-Container, `ReadHeaderTimeout`, Body-Limit 1 MiB.

---

## 5. RBAC

Rollen und Gruppen landen beim Authentifizieren in `httpx.Identity`:

| Quelle | Rollen | Gruppen |
|---|---|---|
| JWT | Claim `AUTH_ROLES_CLAIM` (Default `roles`) | Claim `AUTH_GROUPS_CLAIM` (Default `groups`) |
| JWT + `OIDC_GROUP_ROLE_MAP` | Gruppen-IDs werden auf Rollen gemappt | unverändert |
| Dev-Header | `X-User-Roles` oder `AUTH_DEV_DEFAULT_ROLE` | – |

`OIDC_GROUP_ROLE_MAP` (Format `group-id=Admin;andere-id=Operator,Viewer`)
erlaubt es, Entra-Gruppenobjekt-IDs ohne Codeänderung auf App-Rollen zu mappen.

### Middleware

```go
mux.Handle("GET /api/v1/protected/operator",
    auth.RequireRole("Admin", "Operator")(http.HandlerFunc(h.OperatorAction)))
mux.Handle("POST /api/v1/protected/admin/reset",
    auth.RequireRole("Admin")(http.HandlerFunc(h.AdminReset)))
```

- `RequireRole(roles ...string)` — mindestens eine Rolle, sonst `403`.
- `RequireGroup(groups ...string)` — analog für Gruppen.
- Rollen-/Gruppenvergleich ignoriert Groß-/Kleinschreibung und Leerraum.
- Jede Ablehnung wird als `rbac rejected` mit `required`, `granted_roles`,
  `user` und `request_id` geloggt (Audit-Spur).

### Demo-Endpunkte

| Methode | Pfad | Erlaubt |
|---|---|---|
| GET | `/api/v1/me` | jede authentifizierte Identität |
| GET | `/api/v1/protected/operator` | `Admin` **oder** `Operator` |
| POST | `/api/v1/protected/admin/reset` | ausschließlich `Admin` (Dry-Run) |

Die Rollenmatrix ist im Smoke-Test, in `internal/auth/roles_test.go` und in
`internal/api/rbac_test.go` abgedeckt (401 ohne Credentials, 403 bei falscher
Rolle, 200 bei passender, mehrere Rollen, Groß-/Kleinschreibung).

### Audit-Log (Postgres)

Jede 403-Entscheidung der RBAC-Middleware und jede sensitive Aktion landet in
`audit_logs` (Migration `000002`):

| Spalte | Inhalt |
|---|---|
| `id`, `timestamp` | Primärschlüssel, Zeitpunkt (Default `now()`) |
| `request_id` | Request-ID aus dem Context (Korrelation mit Logs) |
| `user_id` | `sub` bzw. `X-User-Email` |
| `action` | `rbac.deny`, `admin.reset`, `audit.read` |
| `resource` | `METHOD /pfad` |
| `status_code` | 403 bzw. 200 |
| `details` (JSONB) | `required`, `granted_roles`, `granted_groups`, `auth_method`, … |

Der Schreibvorgang ist **direkt (synchron) mit 2 s Timeout**: bei so
seltenen, sicherheitsrelevanten Ereignissen ist Nachvollziehbarkeit wichtiger
als Latenz. Schreibfehler werden geloggt, kippen den Request aber nicht.
Lesen lässt sich das Log über `GET /api/v1/protected/admin/audit-logs?limit=50`
(nur `Admin`).

---

## 6. Migrationen, Seed und Schema-Lifecycle

### Migrationen (nur DDL)

- `backend/migrations/<version>_<name>.{up,down}.sql`, per `embed.FS` ins Binary
  kompiliert. **Inhalt ausschließlich DDL** (Tabellen, Indizes, Constraints) —
  keine Beispieldaten, keine fachlichen Inserts.
- Beim Start: DB-Ping (Retry) → `migrate.Up()` → HTTP-Server. Jede Migration läuft
  in einer Transaktion, Stand in `schema_migrations`; `dirty` ist ein Startfehler.
- `DB_AUTO_MIGRATE=false` deaktiviert den Lauf (externer Migrationsjob).
- `docker-compose.yml` mountet **kein** `init.sql` mehr.

### Seed (separat, idempotent)

Beispieldaten gehören nicht in die Versionierung, sondern in `internal/seed`:

```bash
make seed                                     # im laufenden Container
# oder beim Start:
DB_SEED_DEV_DATA=true docker compose up -d
# oder direkt:
cd backend && DATABASE_URL="postgres://…" go run ./cmd/seed
```

`seed.DevData()` schreibt `INSERT … ON CONFLICT (name) DO NOTHING` und ist damit
beliebig oft ausführbar. Der Code-Default ist **aus** (`DB_SEED_DEV_DATA=false`);
Compose setzt für lokale Entwicklung `true`.

### Rollback

```bash
cd backend && TEST_DATABASE_URL="postgres://…" go test -run TestDown ./internal/migrate
```

`migrate.Down()` rollt alle Migrationen zurück (Tests nutzen das, um „up →
Schema da, down → Schema weg, up → wieder da" zu belegen).

## 7. API-Verträge

### Backend (Go)

| Methode | Pfad | Auth | Rollen | Beschreibung |
|---|---|---|---|---|
| GET | `/healthz` | – | – | DB-Ping; `503` wenn nicht erreichbar |
| GET | `/api/v1/status` | – | – | Uptime, Go-Version, Speicher, DB-Latenz, `auth_mode` |
| GET | `/api/v1/me` | ✅ | – | eigene Identität (sub, email, roles, groups, auth_method) |
| GET | `/api/v1/data` | ✅ | – | Beispiel-Datensätze |
| POST | `/api/v1/process` | ✅ | – | `{"values":[1,2,3]}` → Worker |
| GET | `/api/v1/protected/operator` | ✅ | Admin, Operator | RBAC-Demo |
| POST | `/api/v1/protected/admin/reset` | ✅ | Admin | RBAC-Demo (Dry-Run, wird auditiert) |
| GET | `/api/v1/protected/admin/audit-logs` | ✅ | Admin | letzte Audit-Einträge (`?limit=`) |

Fehlerformat:

```json
{"error":{"code":"forbidden","message":"insufficient permissions","request_id":"8bf53ee17b29b36e"}}
```

### Worker (Python)

| Methode | Pfad | Beschreibung |
|---|---|---|
| GET | `/healthz` | Liveness |
| GET | `/docs` | OpenAPI-Doku |
| POST | `/process/calc` | Aggregat: count, sum, mean, median, min, max, stddev, p90 |

---

## 8. Frontend: Dev- und JWT-Modus getrennt

Das UI richtet sich nach `auth_mode` aus `GET /api/v1/status`:

| `auth_mode` | UI |
|---|---|
| `dev_header` / `mock` | `RoleSwitcher` sichtbar: Rollen umschalten, `GET /protected/operator` und `POST /protected/admin/reset` auslösen; 403 erscheint rot, 200 grün |
| `jwt` | **kein** Umschalter: Rollen nur lesend als Badges (`RoleInfo`), Hinweis „Rollen werden über OIDC-Token gesteuert" |

Technisch sendet der API-Client `X-User-Roles` ausschließlich im Dev-Modus
(`setRoleHeaderEnabled`), sodass im JWT-Modus kein irreführender Header
mitgeht. Rollen-Badges im Header stammen aus `/api/v1/me`.

## 9. Konfiguration

| Variable | Default | Wirkung |
|---|---|---|
| `BIND_ADDRESS` | `127.0.0.1` | Host-Bind für alle Ports |
| `FRONTEND_PORT` / `BACKEND_PORT` / `WORKER_PORT` / `DB_PORT` | `3000` / `8080` / `8000` / `5432` | Host-Ports |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | `anchor` / `anchor` / `change-me-in-development` | DB-Zugang |
| `DB_AUTO_MIGRATE` | `true` | DDL-Migrationen beim Start anwenden |
| `DB_SEED_DEV_DATA` | `compose: true`, Code: `false` | Dev-Beispieldaten beim Start einspielen |
| `AUTH_MODE` | `dev_header` in Compose, `jwt` im Code | Auth-Verfahren |
| `AUTH_ROLES_CLAIM` / `AUTH_GROUPS_CLAIM` | `roles` / `groups` | RBAC-Claims |
| `OIDC_GROUP_ROLE_MAP` | leer | Gruppen → Rollen (`id=Role;id2=RoleA,RoleB`) |
| `AUTH_DEV_DEFAULT_ROLE` | `User` | Rolle ohne `X-User-Roles` |
| `AUTH_ALLOWED_EMAIL_DOMAINS` | `example.com` | erlaubte Header-Domains (Dev) |
| `AUTH_BEARER_TOKEN` | `dev-bearer-token-change-me` | statischer Token (Dev) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000,…` | erlaubte Browser-Origins |
| `WORKER_TIMEOUT` / `SHUTDOWN_TIMEOUT` | `10s` / `10s` | Timeouts |
| `VITE_API_BASE_URL` | leer | leer = relative Pfade über Nginx |
| `VITE_DEMO_USER_EMAIL` | `demo.user@example.com` | Header-Identität (Dev) |
| `VITE_BEARER_TOKEN` | leer | optionaler Bearer (Dev) |

Echte Secrets gehören in einen Secret-Store, nicht in `.env`.

---

## 10. CI-Pipeline (`.github/workflows/ci.yml`)

| Job | Schritte |
|---|---|
| `backend` | `go vet` · golangci-lint 2.15 (inkl. staticcheck) · `go test -v -race ./...` gegen einen **Postgres-16-Service** (Migrations-Integrationstest) |
| `frontend` | `npm ci` · `npx tsc --noEmit` · `npm run build` |
| `worker` | `ruff check` · `ruff format --check` · `pytest` |
| `security` | Trivy `fs`-Scan, HIGH/CRITICAL, `--ignore-unfixed`, Exit 1 bei Fund |

Lokal dieselben Prüfungen: `make lint` und `make test`.

---

## 11. Neuen Endpunkt hinzufügen

### Backend (Go)

1. Route in `internal/api/router.go` registrieren — mit Rollen, wenn nötig:
   ```go
   mux.Handle("GET /api/v1/reports",
       auth.RequireRole("Admin", "Operator")(http.HandlerFunc(h.Reports)))
   ```
2. Handler in `handlers.go`: Guard-Clauses zuerst, Fachlogik danach, Fehler über
   `respondError`; Identität via `httpx.IdentityFrom(r.Context())`.
3. Datenzugriff als Methode in `internal/store/store.go`.
4. Schemaänderung? Neue Migration in `backend/migrations/` (up **und** down).
5. `docker compose up -d --build backend`.

### Worker (Python)

1. Pydantic-Modell in `worker/app/models.py`.
2. Reine Funktion in `worker/app/processing.py`.
3. Route in `worker/app/main.py`, Aufruf im Backend (`internal/worker/client.go`).
4. Test in `worker/tests/`.

### Frontend (React)

1. Typen in `src/services/types.ts`, Aufruf in `src/services/api.ts`.
2. UI in `src/components/`, Einhängen in `App.tsx`; 403 gezielt behandeln
   (`error instanceof ApiError && error.status === 403`).

---

## 12. Graceful Shutdown

`docker compose stop backend` → `SIGTERM`: keine neuen Requests, laufende
Requests bis `SHUTDOWN_TIMEOUT`, dann Pool schließen. Im Log:

```json
{"msg":"shutdown signal received"}
{"msg":"shutdown complete"}
```

---

## 13. Troubleshooting

| Symptom | Ursache / Lösung |
|---|---|
| `Bind … port is already allocated` | Host-Port belegt → `BACKEND_PORT`/`FRONTEND_PORT`/… in `.env` ändern. |
| Backend startet nicht, `database migrations: schema is dirty at version N` | abgebrochene Migration: Ursache prüfen, dann `dirty` in `schema_migrations` per Hand zurücksetzen oder DB neu aufbauen (`docker compose down -v`). |
| `401` auf `/api/v1/protected/*` | Credentials fehlen (JWT bzw. `X-User-Email`). |
| `403` auf `/api/v1/protected/*` | Rolle fehlt — im Dev-Modus `X-User-Roles` prüfen, im JWT-Modus Rolle/Gruppen-Mapping im IdP. |
| `503 auth_backend_unavailable` | JWKS nicht erreichbar (`OIDC_JWKS_URL`). |
| Änderungen an Migrationen wirken nicht | bereits angewendete Versionen laufen nicht erneut → neue Migration anlegen. |
| Datenbank leer, keine Beispieldaten | Seed separat ausführen (`make seed` bzw. `DB_SEED_DEV_DATA=true`) — Migrationen liefern nur DDL. |
| `audit_logs` bleibt leer | Audit wird nur bei 403 und bei `admin.reset`/`audit.read` geschrieben; Schreibfehler stehen als `audit write failed` im Backend-Log. |
| `502` auf `/api/v1/process` | Worker nicht erreichbar (`docker compose logs worker`). |

---

## 14. Nächste Ausbaustufen

1. Policy-Engine statt fester Rollen-Matrix (OPA/Casbin) für feinere Scopes.
2. Audit-Log persistieren (RBAC-Entscheidungen in eine Tabelle/Stream).
3. `/metrics` (Prometheus) und Trace-IDs.
4. Trivy zusätzlich als Image-Scan, Deployment-Pipeline mit signierten Images.
