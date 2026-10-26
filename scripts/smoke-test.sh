#!/usr/bin/env bash
# Smoke-Test gegen den laufenden Stack: Erreichbarkeit, Auth- und RBAC-Verhalten.
# Beruecksichtigt AUTH_MODE (jwt | dev_header | mock).
set -euo pipefail

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

FRONTEND_PORT="${FRONTEND_PORT:-3000}"
BACKEND_PORT="${BACKEND_PORT:-8080}"
WORKER_PORT="${WORKER_PORT:-8000}"
AUTH_MODE="${AUTH_MODE:-dev_header}"

BACKEND="http://127.0.0.1:${BACKEND_PORT}"
WORKER="http://127.0.0.1:${WORKER_PORT}"
FRONTEND="http://127.0.0.1:${FRONTEND_PORT}"

DEMO_USER_EMAIL="${VITE_DEMO_USER_EMAIL:-demo.user@example.com}"

# Eigene Client-IPs fuer Negativ-Checks: das Rate-Limit (401/403-Budget) wuerde
# sonst die positiven Checks derselben IP blockieren.
# Pro Lauf eigene IPs (Dokumentationsbereich 198.51.100.0/24), damit
# wiederholte Smoke-Laeufe nicht am eigenen Rate-Limit haengen bleiben.
RUN_SUFFIX=$(( RANDOM % 100 + 1 ))
AUTH_NEG_IP="${AUTH_NEG_IP:-198.51.100.${RUN_SUFFIX}}"
RBAC_NEG_IP="${RBAC_NEG_IP:-198.51.100.$(( RUN_SUFFIX + 100 ))}"
AUTH_NEG=(-H "X-Real-IP: ${AUTH_NEG_IP}")
RBAC_NEG=(-H "X-Real-IP: ${RBAC_NEG_IP}")
BEARER_TOKEN="${AUTH_BEARER_TOKEN:-dev-bearer-token-change-me}"
SMOKE_BEARER_TOKEN="${SMOKE_BEARER_TOKEN:-}"

PASSED=0
FAILED=0
SKIPPED=0

check() {
  local name="$1"
  local expected="$2"
  local actual="$3"

  if [[ "$actual" == "$expected" ]]; then
    printf '  [ok]   %-56s %s\n' "$name" "$actual"
    PASSED=$((PASSED + 1))
    return
  fi

  printf '  [FAIL] %-56s erwartet=%s ist=%s\n' "$name" "$expected" "$actual"
  FAILED=$((FAILED + 1))
}

skip() {
  printf '  [skip] %-56s %s\n' "$1" "$2"
  SKIPPED=$((SKIPPED + 1))
}

status_code() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"
}

check_body() {
  local name="$1"
  local needle="$2"
  local body="$3"

  if [[ "$body" == *"$needle"* ]]; then
    printf '  [ok]   %-56s %s
' "$name" "gefunden"
    PASSED=$((PASSED + 1))
    return
  fi

  printf '  [FAIL] %-56s %s fehlt im Body
' "$name" "$needle"
  FAILED=$((FAILED + 1))
}

echo "Smoke-Test gegen Backend ${BACKEND} (AUTH_MODE=${AUTH_MODE})"

# --- Oeffentliche Endpunkte ------------------------------------------------
check "GET /livez (Liveness)"                200 "$(status_code "${BACKEND}/livez")"
check "GET /readyz (Readiness)"              200 "$(status_code "${BACKEND}/readyz")"
check "GET /healthz (entfernt)"              401 "$(status_code "${AUTH_NEG[@]}" "${BACKEND}/healthz")"
check "GET /api/v1/status"                   200 "$(status_code "${BACKEND}/api/v1/status")"

# --- Auth-Negativfaelle ---------------------------------------------------
check "GET /api/v1/data (ohne Auth)"         401 "$(status_code "${AUTH_NEG[@]}" "${BACKEND}/api/v1/data")"
check "GET /api/v1/protected/operator (ohne Auth)" 401 "$(status_code "${AUTH_NEG[@]}" "${BACKEND}/api/v1/protected/operator")"
check "GET /does-not-exist (ohne Auth)"      401 "$(status_code "${AUTH_NEG[@]}" "${BACKEND}/does-not-exist")"

if [[ "$AUTH_MODE" == "jwt" ]]; then
  check "GET /api/v1/data (X-User-Email ignoriert)" 401 \
    "$(status_code "${AUTH_NEG[@]}" -H "X-User-Email: ${DEMO_USER_EMAIL}" "${BACKEND}/api/v1/data")"
  check "audit-logs ohne Token"            401 "$(status_code "${AUTH_NEG[@]}" "${BACKEND}/api/v1/protected/admin/audit-logs")"

  if [[ -n "$SMOKE_BEARER_TOKEN" ]]; then
    AUTH_HEADER=(-H "Authorization: Bearer ${SMOKE_BEARER_TOKEN}")
    check "GET /api/v1/me (JWT)"              200 "$(status_code "${AUTH_HEADER[@]}" "${BACKEND}/api/v1/me")"
    check "GET /api/v1/data (JWT)"           200 "$(status_code "${AUTH_HEADER[@]}" "${BACKEND}/api/v1/data")"
    check "POST /api/v1/process (JWT)"       200 \
      "$(status_code -X POST -H "Content-Type: application/json" "${AUTH_HEADER[@]}" \
            --data '{"values":[10,20,30,40]}' "${BACKEND}/api/v1/process")"
    check "GET /does-not-exist (JWT)"        404 "$(status_code "${AUTH_HEADER[@]}" "${BACKEND}/does-not-exist")"
    skip "RBAC-Rollenmatrix"                 "Rollen stammen im JWT-Modus aus den Claims (Token je Rolle noetig)"
  else
    skip "Authentifizierte Pruefungen"       "SMOKE_BEARER_TOKEN nicht gesetzt (echtes OIDC-Token noetig)"
  fi
else
  USER=(-H "X-User-Email: ${DEMO_USER_EMAIL}")
  OPERATOR=(-H "X-User-Email: ${DEMO_USER_EMAIL}" -H "X-User-Roles: Operator")
  ADMIN=(-H "X-User-Email: ${DEMO_USER_EMAIL}" -H "X-User-Roles: Admin")
  MULTI=(-H "X-User-Email: ${DEMO_USER_EMAIL}" -H "X-User-Roles: Viewer,Operator")

  check "GET /api/v1/me (Default-Rolle)"   200 "$(status_code "${USER[@]}" "${BACKEND}/api/v1/me")"
  check "GET /api/v1/data (X-User-Email)"  200 "$(status_code "${USER[@]}" "${BACKEND}/api/v1/data")"
  check "GET /api/v1/data (Bearer)"        200 "$(status_code -H "Authorization: Bearer ${BEARER_TOKEN}" "${BACKEND}/api/v1/data")"
  check "GET /api/v1/data (fremde Domain)" 401 \
    "$(status_code "${AUTH_NEG[@]}" -H "X-User-Email: attacker@evil.example" "${BACKEND}/api/v1/data")"
  check "POST /api/v1/process"             200 \
    "$(status_code -X POST -H "Content-Type: application/json" "${USER[@]}" \
          --data '{"values":[10,20,30,40]}' "${BACKEND}/api/v1/process")"
  check "POST /api/v1/process (leer)"      400 \
    "$(status_code -X POST -H "Content-Type: application/json" "${USER[@]}" \
          --data '{"values":[]}' "${BACKEND}/api/v1/process")"
  check "GET /does-not-exist (mit Auth)"   404 "$(status_code "${USER[@]}" "${BACKEND}/does-not-exist")"

  # --- RBAC-Matrix --------------------------------------------------------
  check "operator (Rolle User)"            403 "$(status_code "${RBAC_NEG[@]}" "${USER[@]}" "${BACKEND}/api/v1/protected/operator")"
  check "operator (Rolle Operator)"        200 "$(status_code "${OPERATOR[@]}" "${BACKEND}/api/v1/protected/operator")"
  check "operator (Rolle Admin)"           200 "$(status_code "${ADMIN[@]}" "${BACKEND}/api/v1/protected/operator")"
  check "operator (mehrere Rollen)"        200 "$(status_code "${MULTI[@]}" "${BACKEND}/api/v1/protected/operator")"
  check "admin/reset (Rolle User)"         403 "$(status_code "${RBAC_NEG[@]}" -X POST "${USER[@]}" "${BACKEND}/api/v1/protected/admin/reset")"
  check "admin/reset (Rolle Operator)"     403 "$(status_code "${RBAC_NEG[@]}" -X POST "${OPERATOR[@]}" "${BACKEND}/api/v1/protected/admin/reset")"
  check "admin/reset (Rolle Admin)"        200 "$(status_code -X POST "${ADMIN[@]}" "${BACKEND}/api/v1/protected/admin/reset")"

  # --- Audit-Log und Seed -------------------------------------------------
  curl -s -o /dev/null -X POST "${RBAC_NEG[@]}" "${OPERATOR[@]}" "${BACKEND}/api/v1/protected/admin/reset"
  check "audit-logs (Rolle Operator)"      403 "$(status_code "${RBAC_NEG[@]}" "${OPERATOR[@]}" "${BACKEND}/api/v1/protected/admin/audit-logs")"
  check "audit-logs (Rolle Admin)"         200 "$(status_code "${ADMIN[@]}" "${BACKEND}/api/v1/protected/admin/audit-logs?limit=20")"
  check_body "audit-logs enthaelt rbac.deny" '"action":"rbac.deny"' \
    "$(curl -s "${ADMIN[@]}" "${BACKEND}/api/v1/protected/admin/audit-logs?limit=20")"
  check_body "audit-logs enthaelt admin.reset" '"action":"admin.reset"' \
    "$(curl -s "${ADMIN[@]}" "${BACKEND}/api/v1/protected/admin/audit-logs?limit=20")"
  check_body "Dev-Seed vorhanden" "Onboarding-Automatisierung" \
    "$(curl -s "${USER[@]}" "${BACKEND}/api/v1/data")"
fi

# --- Rate-Limit (isolierte Test-IP, blockiert echte Checks nicht) --------
RATE_IP="203.0.113.7"
for _ in $(seq 1 12); do
  LAST_RATE_CODE="$(status_code -H "X-Real-IP: ${RATE_IP}" "${BACKEND}/api/v1/data")"
done
check "Rate-Limit nach 12 Fehlversuchen"      429 "${LAST_RATE_CODE}"

# --- Worker und Frontend --------------------------------------------------
check "GET worker /healthz"                200 "$(status_code "${WORKER}/healthz")"
check "GET worker /docs"                   200 "$(status_code "${WORKER}/docs")"
check "GET frontend /"                     200 "$(status_code "${FRONTEND}/")"
check "GET frontend /healthz"              200 "$(status_code "${FRONTEND}/healthz")"
check "GET frontend /api/v1/status"        200 "$(status_code "${FRONTEND}/api/v1/status")"

echo
echo "Ergebnis: ${PASSED} ok, ${SKIPPED} uebersprungen, ${FAILED} fehlgeschlagen"

if [[ "$FAILED" -gt 0 ]]; then
  exit 1
fi
