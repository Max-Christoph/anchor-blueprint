#!/usr/bin/env bash
# Benennt das Template fuer ein neues Standort-Projekt um:
# Compose-Projekt, Netz/Volume, Go-Modulpfad, UI-Titel, Paketnamen, Doku.
#
# Verwendung: ./scripts/rename-project.sh <slug> [Anzeigename]
#   ./scripts/rename-project.sh werk-nord "Werk Nord"
set -euo pipefail

usage() {
  cat <<'EOF'
Verwendung: ./scripts/rename-project.sh <slug> [Anzeigename]

  <slug>          kebab-case, z. B. werk-nord (mind. 3 Zeichen, a-z0-9-)
  [Anzeigename]   Titel im UI, Standard: Slug in Title-Case (z. B. "Werk Nord")

Beispiel:
  ./scripts/rename-project.sh werk-nord "Werk Nord"
EOF
}

slug="${1:-}"
if [[ -z "$slug" ]]; then
  usage
  exit 1
fi
if [[ ! "$slug" =~ ^[a-z][a-z0-9-]{2,62}$ ]]; then
  echo "Fehler: Slug muss ^[a-z][a-z0-9-]{2,62}$ entsprechen (erhalten: '${slug}')." >&2
  exit 1
fi

display="${2:-$(printf '%s' "$slug" | tr '-' ' ' | awk '{for (i = 1; i <= NF; i++) $i = toupper(substr($i, 1, 1)) substr($i, 2)} 1')}"
# Zeichen entfernen, die sed/HTML brechen wuerden.
display="${display//[\/&\\]/}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

if [[ -n "$(git status --porcelain 2>/dev/null || true)" ]]; then
  echo "Warnung: Working Tree ist nicht sauber - Umbenennung erfolgt trotzdem." >&2
fi

echo "Umbenennung in '${slug}' (Anzeigename: '${display}')"
echo "Repository: ${root}"

mapfile -t files < <(
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git ls-files
  else
    find . -type f -not -path './.git/*' -not -path './node_modules/*' -not -path './.venv/*' \
      -not -path './worker/.venv/*' -not -path './frontend/dist/*'
  fi
)

changed=0
for file in "${files[@]}"; do
  [[ -f "$file" ]] || continue
  # Binaerdateien ueberspringen
  if ! grep -Iq . "$file" 2>/dev/null; then
    continue
  fi
  if ! grep -qE 'anchor-blueprint|Anchor Blueprint|anchor-postgres-data' "$file"; then
    continue
  fi
  sed -i \
    -e "s/anchor-blueprint/${slug}/g" \
    -e "s/Anchor Blueprint/${display}/g" \
    -e "s/anchor-postgres-data/${slug}-postgres-data/g" \
    "$file"
  changed=$((changed + 1))
done

echo "Angepasste Dateien: ${changed}"
echo
echo "Naechste Schritte:"
echo "  1. cd backend && gofmt -w ./cmd ./internal && go build ./... && go test ./..."
echo "  2. cd frontend && npm ci && npm run build"
echo "  3. DB-Name/User (POSTGRES_DB/POSTGRES_USER) in .env anpassen, falls gewuenscht"
echo "  4. git add -A && git commit -m \"chore: rename template to ${slug}\""
echo
echo "Hinweis: Der Go-Modulpfad ist jetzt github.com/<org>/${slug}/backend -"
echo "         bei abweichender Organisation den Pfad in backend/go.mod + Imports anpassen."
