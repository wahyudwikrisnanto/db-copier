// Package pg holds the direct Postgres operations: connectivity checks,
// version detection and wiping the target database.
package pg

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// Endpoint is a resolved address (after any SSH tunnel) plus credentials.
type Endpoint struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
}

func (e Endpoint) URL() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(e.User, e.Password),
		Host:   e.Host + ":" + strconv.Itoa(e.Port),
		Path:   "/" + e.Database,
	}
	q := url.Values{}
	if e.SSLMode != "" {
		q.Set("sslmode", e.SSLMode)
	}
	q.Set("connect_timeout", "15")
	q.Set("application_name", "db-copier")
	u.RawQuery = q.Encode()
	return u.String()
}

func Connect(ctx context.Context, e Endpoint) (*pgx.Conn, error) {
	return pgx.Connect(ctx, e.URL())
}

// ServerInfo returns the server version string and major version number.
func ServerInfo(ctx context.Context, e Endpoint) (string, int, error) {
	conn, err := Connect(ctx, e)
	if err != nil {
		return "", 0, err
	}
	defer conn.Close(ctx)
	var version string
	var num int
	if err := conn.QueryRow(ctx, "SELECT version(), current_setting('server_version_num')::int").Scan(&version, &num); err != nil {
		return "", 0, err
	}
	return version, num / 10000, nil
}

// Erase drops every non-system schema and every extension (except plpgsql)
// in the target database, then recreates an empty public schema. Runs in one
// transaction so a failure leaves the target untouched.
func Erase(ctx context.Context, e Endpoint, log func(string)) error {
	conn, err := Connect(ctx, e)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	schemas, err := collect(ctx, tx, `
		SELECT nspname FROM pg_namespace
		WHERE nspname NOT IN ('pg_catalog', 'information_schema')
		  AND nspname NOT LIKE 'pg\_toast%'
		  AND nspname NOT LIKE 'pg\_temp\_%'
		ORDER BY nspname`)
	if err != nil {
		return err
	}
	for _, stmt := range EraseStatements(schemas, nil) {
		log(stmt)
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	// Extensions installed into system schemas survive the schema drop, and
	// event triggers are database-wide, so both are removed separately.
	exts, err := collect(ctx, tx, `SELECT extname FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY extname`)
	if err != nil {
		return err
	}
	evts, err := collect(ctx, tx, `SELECT evtname FROM pg_event_trigger ORDER BY evtname`)
	if err != nil {
		return err
	}
	stmts := EraseStatements(nil, exts)
	for _, ev := range evts {
		stmts = append(stmts, "DROP EVENT TRIGGER IF EXISTS "+pgx.Identifier{ev}.Sanitize()+" CASCADE")
	}
	for _, stmt := range stmts {
		log(stmt)
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	// Recreate public the way initdb does, so pg_dump sees it as the default
	// schema and the restored database matches the source exactly.
	var asDBOwner bool
	if err := tx.QueryRow(ctx, `SELECT current_setting('server_version_num')::int >= 140000
		AND pg_has_role(current_user, 'pg_database_owner', 'MEMBER')`).Scan(&asDBOwner); err != nil {
		return err
	}
	create := "CREATE SCHEMA public"
	if asDBOwner {
		create += " AUTHORIZATION pg_database_owner"
	}
	for _, stmt := range []string{
		create,
		"COMMENT ON SCHEMA public IS 'standard public schema'",
		"GRANT USAGE ON SCHEMA public TO PUBLIC",
	} {
		log(stmt)
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return tx.Commit(ctx)
}

// EraseStatements builds the DROP statements for the given schemas and extensions.
func EraseStatements(schemas, extensions []string) []string {
	var out []string
	for _, s := range schemas {
		out = append(out, "DROP SCHEMA IF EXISTS "+pgx.Identifier{s}.Sanitize()+" CASCADE")
	}
	for _, x := range extensions {
		out = append(out, "DROP EXTENSION IF EXISTS "+pgx.Identifier{x}.Sanitize()+" CASCADE")
	}
	return out
}

func collect(ctx context.Context, tx pgx.Tx, sql string) ([]string, error) {
	rows, err := tx.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Table is a user table whose data pg_dump will copy, with its on-disk size.
type Table struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Plan describes what a copy of the database will transfer, for progress
// reporting. PostData estimates the indexes, constraints, triggers and
// materialized views pg_restore creates after the data.
type Plan struct {
	Tables   []Table `json:"tables"`
	PostData int     `json:"postData"`
}

const userNamespace = `n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_%'`

// LoadPlan reads table sizes and post-data object counts from the source.
func LoadPlan(ctx context.Context, e Endpoint) (Plan, error) {
	conn, err := Connect(ctx, e)
	if err != nil {
		return Plan{}, err
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT n.nspname || '.' || c.relname, pg_table_size(c.oid)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r' AND `+userNamespace+`
		  AND NOT EXISTS (SELECT 1 FROM pg_depend d
		                  WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e')
		ORDER BY 2 DESC, 1`)
	if err != nil {
		return Plan{}, err
	}
	tables, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Table, error) {
		var t Table
		return t, r.Scan(&t.Name, &t.Bytes)
	})
	if err != nil {
		return Plan{}, err
	}

	var postData int
	err = conn.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM pg_index i
		     JOIN pg_class c ON c.oid = i.indexrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		    WHERE `+userNamespace+`
		      AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conindid = i.indexrelid AND k.contype IN ('p', 'u', 'x')))
		+ (SELECT count(*) FROM pg_constraint k JOIN pg_namespace n ON n.oid = k.connamespace
		    WHERE `+userNamespace+` AND k.contype IN ('p', 'u', 'f', 'x'))
		+ (SELECT count(*) FROM pg_trigger t
		     JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		    WHERE NOT t.tgisinternal AND `+userNamespace+`)
		+ (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		    WHERE c.relkind = 'm' AND `+userNamespace+`)`).Scan(&postData)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Tables: tables, PostData: postData}, nil
}
