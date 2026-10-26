package migrations_test

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Max-Christoph/anchor-blueprint/backend/migrations"
)

var migrationName = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.(up|down)\.sql$`)

// TestEmbeddedMigrations verifiziert Namensschema, lueckenlose Nummerierung,
// Up/Down-Paare und nicht-leere Inhalte.
func TestEmbeddedMigrations(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	type pair struct {
		up   string
		down string
	}
	versions := make(map[int]*pair)

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		matches := migrationName.FindStringSubmatch(name)
		if matches == nil {
			t.Fatalf("Dateiname entspricht nicht dem Schema <version>_<name>.{up,down}.sql: %s", name)
		}
		version, convErr := strconv.Atoi(matches[1])
		if convErr != nil {
			t.Fatalf("Version nicht lesbar: %s", name)
		}

		content, readErr := migrations.FS.ReadFile(name)
		if readErr != nil {
			t.Fatalf("ReadFile %s: %v", name, readErr)
		}
		if strings.TrimSpace(string(content)) == "" {
			t.Fatalf("Migration %s ist leer", name)
		}

		current, ok := versions[version]
		if !ok {
			current = &pair{}
			versions[version] = current
		}
		if matches[3] == "up" {
			current.up = name
		} else {
			current.down = name
		}
	}

	if len(versions) == 0 {
		t.Fatal("keine Migrationen eingebettet")
	}

	for version := 1; version <= len(versions); version++ {
		if _, ok := versions[version]; !ok {
			t.Fatalf("Nummerierung nicht lueckenlos: Version %d fehlt (vorhanden: %d)", version, len(versions))
		}
	}

	for version, pair := range versions {
		if pair.up == "" {
			t.Errorf("Version %d hat keine up-Migration", version)
		}
		if pair.down == "" {
			t.Errorf("Version %d hat keine down-Migration", version)
		}
	}
}
