# AGENTS.md — Navigationskarte für Coding-Assistenten

Anchor Blueprint: internes Standort-Tool. Go-Backend (OIDC/JWT, RBAC, Audit),
React-Frontend, FastAPI-Worker, PostgreSQL 16 mit DDL-Migrationen.
**Eingefroren bei v1.0.0-rc1** — keine neuen Features ohne explizite Aufforderung.

## Commands (Verifikation)

| Zweck | Befehl |
|---|---|
| Tests (Go `-race`, `tsc`, `pytest`) | `make test` |
| Lint (go vet, golangci-lint inkl. gosec, ruff) | `make lint` |
| Integration (laufender Stack: Auth, RBAC, Audit) | `make smoke` |

Vor `make smoke`: `make up` (baut und startet den Stack; Ports stehen in `.env`).

## Wo was liegt

- `backend/cmd/server` — Start: Config, Migrationen, Seed, Graceful Shutdown
- `backend/internal/api` — Router und Handler; `internal/auth` — JWT/JWKS, RBAC-Middleware
- `backend/internal/store` — SQL (pgxpool); `internal/migrate` + `backend/migrations` — Schema
- `backend/internal/audit` — Audit-Trail; `internal/httpx` — Request-ID und Identity
- `frontend/src/{components,services}` — UI; `worker/app` — FastAPI-Service
- `docker-compose.yml`, `scripts/smoke-test.sh`, `scripts/rename-project.sh`

## Strict Guardrails (Hard Invariants)

- **Auth/RBAC niemals entfernen oder abschwächen** — Middleware, `RequireRole`/`RequireGroup`,
  Deny-by-Default und `PublicPaths` nicht lockern, um fehlschlagende Tests zu „lösen".
  Ein roter Auth-Test ist ein Code-Problem: Code fixen, nicht die Prüfung.
- **Keine neuen Dependencies** ohne explizite Aufforderung (`go.mod`, `package.json`,
  `requirements.txt`).
- **SQL ausschließlich parametrisiert** (`$1`, `$2`); niemals Query-Strings per
  `fmt.Sprintf` zusammensetzen.
- **Schema-Änderungen ausschließlich als nummerierte Up/Down-Migration** in
  `backend/migrations/NNNNNN_name.{up,down}.sql`; angewendete Migrationen nicht editieren.
- **Fehler niemals mit `_ = err` ignorieren** — zurückgeben oder bewusst strukturiert loggen.
- **Audit-Schreibpfade nicht erweitern**: `audit_logs` nur für `rbac.deny` und `admin.reset`;
  Lesezugriffe gehören ins App-Log (`slog`), nicht in die Tabelle.
- **401/403 nicht umgehen** — Rate-Limit und Audit hängen an diesen Statuscodes.

## Konventionen

- Kommentare und Fehlermeldungen auf Deutsch, Identifikatoren englisch.
- Handler: Guard-Clauses zuerst, Fehler über `respondError`, Identität via
  `httpx.IdentityFrom(r.Context())`.
- Neue Routen immer mit Rollenprüfung: `mux.Handle("GET /api/v1/…", auth.RequireRole("Admin")(…))`.
- `make lint` und `make test` müssen vor jedem Commit grün sein.

## Verknüpfung

- Kopie dieser Datei für GitHub Copilot: `.github/copilot-instructions.md` (synchron halten).
