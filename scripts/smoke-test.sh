#!/usr/bin/env bash
# Smoke-Test gegen den laufenden Stack: Erreichbarkeit und Auth-Verhalten.
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
    printf '  [ok]   %-52s %s\n' "$name" "$actual"
    PASSED=$((PASSED + 1))
    return
  fi

  printf '  [FAIL] %-52s erwartet=%s ist=%s\n' "$name" "$expected" "$actual"
  FAILED=$((FAILED + 1))
}

skip() {
  printf '  [skip] %-52s %s\n' "$1" "$2"
  SKIPPED=$((SKIPPED + 1))
}

status_code() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"
}

echo "Smoke-Test gegen Backend ${BACKEND} (AUTH_MODE=${AUTH_MODE})"

# --- Oeffentliche Endpunkte ------------------------------------------------
check "GET /healthz"                     200 "$(status_code "${BACKEND}/healthz")"
check "GET /api/v1/status"               200 "$(status_code "${BACKEND}/api/v1/status")"

# --- Auth-Negativfaelle ---------------------------------------------------
check "GET /api/v1/data (ohne Auth)"     401 "$(status_code "${BACKEND}/api/v1/data")"
check "GET /does-not-exist (ohne Auth)"  401 "$(status_code "${BACKEND}/does-not-exist")"

if [[ "$AUTH_MODE" == "jwt" ]]; then
  skip "GET /api/v1/data (X-User-Email)"  "JWT-Modus ignoriert Header - erwartet 401"
  check "GET /api/v1/data (X-User-Email)" 401 \
    "$(status_code -H "X-User-Email: ${DEMO_USER_EMAIL}" "${BACKEND}/api/v1/data")"

  if [[ -n "$SMOKE_BEARER_TOKEN" ]]; then
    AUTH_HEADER=(-H "Authorization: Bearer ${SMOKE_BEARER_TOKEN}")
    check "GET /api/v1/data (JWT)"        200 "$(status_code "${AUTH_HEADER[@]}" "${BACKEND}/api/v1/data")"
    check "POST /api/v1/process (JWT)"    200 \
      "$(status_code -X POST -H "Content-Type: application/json" "${AUTH_HEADER[@]}" \
            --data '{"values":[10,20,30,40]}' "${BACKEND}/api/v1/process")"
    check "GET /does-not-exist (JWT)"     404 "$(status_code "${AUTH_HEADER[@]}" "${BACKEND}/does-not-exist")"
  else
    skip "Authentifizierte Pruefungen" "SMOKE_BEARER_TOKEN nicht gesetzt (echtes OIDC-Token noetig)"
  fi
else
  check "GET /api/v1/data (X-User-Email)" 200 \
    "$(status_code -H "X-User-Email: ${DEMO_USER_EMAIL}" "${BACKEND}/api/v1/data")"
  check "GET /api/v1/data (Bearer)"     200 \
    "$(status_code -H "Authorization: Bearer ${BEARER_TOKEN}" "${BACKEND}/api/v1/data")"
  check "GET /api/v1/data (fremde Domain)" 401 \
    "$(status_code -H "X-User-Email: attacker@evil.example" "${BACKEND}/api/v1/data")"
  check "POST /api/v1/process"          200 \
    "$(status_code -X POST -H "Content-Type: application/json" -H "X-User-Email: ${DEMO_USER_EMAIL}" \
          --data '{"values":[10,20,30,40]}' "${BACKEND}/api/v1/process")"
  check "POST /api/v1/process (leer)"   400 \
    "$(status_code -X POST -H "Content-Type: application/json" -H "X-User-Email: ${DEMO_USER_EMAIL}" \
          --data '{"values":[]}' "${BACKEND}/api/v1/process")"
  check "GET /does-not-exist (mit Auth)" 404 \
    "$(status_code -H "X-User-Email: ${DEMO_USER_EMAIL}" "${BACKEND}/does-not-exist")"
fi

# --- Worker und Frontend --------------------------------------------------
check "GET worker /healthz"              200 "$(status_code "${WORKER}/healthz")"
check "GET worker /docs"                 200 "$(status_code "${WORKER}/docs")"
check "GET frontend /"                   200 "$(status_code "${FRONTEND}/")"
check "GET frontend /healthz"            200 "$(status_code "${FRONTEND}/healthz")"
check "GET frontend /api/v1/status"      200 "$(status_code "${FRONTEND}/api/v1/status")"

echo
echo "Ergebnis: ${PASSED} ok, ${SKIPPED} uebersprungen, ${FAILED} fehlgeschlagen"

if [[ "$FAILED" -gt 0 ]]; then
  exit 1
fi
