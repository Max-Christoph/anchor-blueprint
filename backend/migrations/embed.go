// Package migrations haelt die versionierten SQL-Migrationen (golang-migrate).
// Die Dateien werden per embed.FS ins Binary kompiliert; der Lauf erfolgt beim
// Start des Backends (DB_AUTO_MIGRATE).
package migrations

import "embed"

// FS enthaelt alle .sql-Migrationen (Format: <version>_<name>.{up,down}.sql).
//
//go:embed *.sql
var FS embed.FS
