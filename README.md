# DB Copier

Desktop app that clones a PostgreSQL database (optionally reached through an SSH tunnel) into another database, usually a local one. Every copy **erases the target first**, then copies everything: schemas, tables, data, indexes, sequences (with current values), constraints, views, materialized views, functions, triggers, types, extensions and comments.

Built with Wails v2 (Go) + React + Tailwind v4 + shadcn/ui.

## How it works

1. Opens SSH tunnels for connections that need them.
2. Checks that `pg_dump` is at least the source server's major version.
3. Erases the target in one transaction: drops every non-system schema, remaining extensions and event triggers, then recreates `public` like `initdb` does.
4. Streams `pg_dump -Fc` from the source straight into `pg_restore` on the target.

Connections are stored in `~/.config/db-copier/connections.json`; passwords go to the OS keyring (falls back to a `0600` `secrets.json` when no keyring service is running). SSH host keys are trusted on first use and pinned in `~/.config/db-copier/known_hosts`.

## Requirements

- Go 1.25+, Node 20+
- `pg_dump` / `pg_restore` (same major version as the source server or newer)
- Linux: `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev`
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Develop / build

```sh
wails dev -tags webkit2_41      # hot reload
wails build -tags webkit2_41    # binary in build/bin/
```

## Tests

```sh
go test ./internal/...
```

Integration tests (real copy through an SSH tunnel, plus cancel) need three containers:

```sh
docker network create dbcopy-test
docker run -d --rm --name dbcopy-src --network dbcopy-test -e POSTGRES_PASSWORD=srcpw -e POSTGRES_DB=app -p 127.0.0.1:5433:5432 postgres:17
docker run -d --rm --name dbcopy-dst -e POSTGRES_PASSWORD=dstpw -e POSTGRES_DB=app_local -p 127.0.0.1:5434:5432 postgres:17
docker run -d --rm --name dbcopy-ssh --network dbcopy-test -e PASSWORD_ACCESS=true -e USER_NAME=tunnel -e USER_PASSWORD=sshpw -p 127.0.0.1:2222:2222 lscr.io/linuxserver/openssh-server:latest
docker exec dbcopy-ssh sed -i 's/^AllowTcpForwarding no/AllowTcpForwarding yes/' /config/sshd/sshd_config && docker restart dbcopy-ssh
# seed dbcopy-src with some schema/data, then:
DB_COPIER_IT=1 go test -tags integration ./internal/copier -v
```
