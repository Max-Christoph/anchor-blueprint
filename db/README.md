# db/

- `init/` wird beim **ersten** Start eines leeren Postgres-Volumes nach
  `/docker-entrypoint-initdb.d` gemountet und alphabetisch ausgefuehrt.
- Schema-Aenderungen nach dem ersten Start: Migrationen einfuehren
  (`golang-migrate`, `goose`, `atlas`). Skripte in `init/` sind kein
  Migrationssystem.
- Volume zuruecksetzen (loescht alle Daten):
  `docker compose down -v`
