#!/usr/bin/env bash
# Smoke-Test gegen den laufenden Stack. Prueft Erreichbarkeit und Auth-Verhalten.
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

BACKEND="http://127.0.0.1:${BACKEND_PORT}"
WORKER="http://127.0.0.1:${WORKER_PORT}"
FRONTEND="http://127.0.0.1:${FRONTEND_PORT}"

DEMO_USER_EMAIL="${VITE_DEMO_USER_EMAIL:-demo.user@example.com}"
BEARER_TOKEN="${AUTH_BEARER_TOKEN:-dev-bearer-token-change-me}"

PASSED=0
FAILED=0

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

status_code() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"
}

echo "Smoke-Test gegen Backend ${BACKEND}"

check "GET /healthz"                200 "$(status_code "${BACKEND}/healthz")"
check "GET /api/v1/status"          200 "$(status_code "${BACKEND}/api/v1/status")"
check "GET /api/v1/data (ohne Auth)" 401 "$(status_code "${BACKEND}/api/v1/data")"
check "GET /api/v1/data (X-User-Email)" 200 \
  "$(status_code -H "X-User-Email: ${DEMO_USER_EMAIL}" "${BACKEND}/api/v1/data")"
check "GET /api/v1/data (Bearer)"    200 \
  "$(status_code -H "Authorization: Bearer ${BEARER_TOKEN}" "${BACKEND}/api/v1/data")"
check "GET /api/v1/data (fremde Domain)" 401 \
  "$(status_code -H "X-User-Email: attacker@evil.example" "${BACKEND}/api/v1/data")"
check "POST /api/v1/process"        200 \
  "$(status_code -X POST -H "Content-Type: application/json" -H "X-User-Email: ${DEMO_USER_EMAIL}" \
        --data '{"values":[10,20,30,40]}' "${BACKEND}/api/v1/process")"
check "POST /api/v1/process (leer)" 400 \
  "$(status_code -X POST -H "Content-Type: application/json" -H "X-User-Email: ${DEMO_USER_EMAIL}" \
        --data '{"values":[]}' "${BACKEND}/api/v1/process")"
check "GET /does-not-exist (ohne Auth)" 401 "$(status_code "${BACKEND}/does-not-exist")"
check "GET /does-not-exist (mit Auth)"  404 \
  "$(status_code -H "X-User-Email: ${DEMO_USER_EMAIL}" "${BACKEND}/does-not-exist")"
check "GET worker /healthz"         200 "$(status_code "${WORKER}/healthz")"
check "GET worker /docs"            200 "$(status_code "${WORKER}/docs")"
check "GET frontend /"              200 "$(status_code "${FRONTEND}/")"
check "GET frontend /healthz"       200 "$(status_code "${FRONTEND}/healthz")"
check "GET frontend /api/v1/status" 200 "$(status_code "${FRONTEND}/api/v1/status")"

echo
echo "Ergebnis: ${PASSED} ok, ${FAILED} fehlgeschlagen"

if [[ "$FAILED" -gt 0 ]]; then
  exit 1
fi
