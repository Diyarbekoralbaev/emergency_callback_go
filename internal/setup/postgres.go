package setup

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// identRe restricts DB identifiers we splice into SQL — role/db names come
// from operator input, so refuse anything beyond the boring charset instead
// of trying to escape it.
var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func ValidIdent(s string) bool { return identRe.MatchString(s) }

// PGAdmin provisions the role + database. Exactly one of the two transports
// is used: peer (sudo -u postgres psql, local socket) or a superuser pgx
// connection (local/remote TCP with password).
type PGAdmin struct {
	Peer     bool
	SuperURL string // pgx URL for TCP mode, e.g. postgres://postgres:pw@host:port/postgres
}

func (a *PGAdmin) query(ctx context.Context, sql string) (string, error) {
	if a.Peer {
		return runPsql(ctx, "-tAc", sql)
	}
	conn, err := pgx.Connect(ctx, a.SuperURL)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return "", err
		}
		for _, v := range vals {
			out = append(out, fmt.Sprint(v))
		}
	}
	return strings.Join(out, "\n"), rows.Err()
}

func (a *PGAdmin) exec(ctx context.Context, sql string) error {
	_, err := a.query(ctx, sql)
	return err
}

// execDB runs sql against a specific database (needed for schema grants).
func (a *PGAdmin) execDB(ctx context.Context, dbName, sql string) error {
	if a.Peer {
		_, err := runPsql(ctx, "-d", dbName, "-tAc", sql)
		return err
	}
	url, err := replaceDBInURL(a.SuperURL, dbName)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, sql)
	return err
}

func replaceDBInURL(superURL, dbName string) (string, error) {
	cfg, err := pgx.ParseConfig(superURL)
	if err != nil {
		return "", err
	}
	cfg.Database = dbName
	// Rebuild minimally: pgx configs aren't round-trippable; do a string swap
	// on the path segment instead.
	i := strings.LastIndex(superURL, "/")
	if i < 0 {
		return "", fmt.Errorf("unparseable url")
	}
	rest := superURL[i+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return superURL[:i+1] + dbName + rest[q:], nil
	}
	return superURL[:i+1] + dbName, nil
}

// RoleExists checks for an existing role.
func (a *PGAdmin) RoleExists(ctx context.Context, role string) (bool, error) {
	out, err := a.query(ctx, fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname='%s'", role))
	return strings.Contains(out, "1"), err
}

// DBExists checks for an existing database.
func (a *PGAdmin) DBExists(ctx context.Context, name string) (bool, error) {
	out, err := a.query(ctx, fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname='%s'", name))
	return strings.Contains(out, "1"), err
}

// EnsureRole creates the role or (with syncPassword) resets its password.
func (a *PGAdmin) EnsureRole(ctx context.Context, role, password string, exists, syncPassword bool) error {
	esc := strings.ReplaceAll(password, "'", "''")
	if exists {
		if !syncPassword {
			return nil
		}
		return a.exec(ctx, fmt.Sprintf(`ALTER ROLE "%s" LOGIN PASSWORD '%s'`, role, esc))
	}
	return a.exec(ctx, fmt.Sprintf(`CREATE ROLE "%s" LOGIN PASSWORD '%s'`, role, esc))
}

// EnsureDB creates the database owned by role and fixes ownership + the
// PG15+ public-schema privilege either way.
func (a *PGAdmin) EnsureDB(ctx context.Context, name, owner string, exists bool) error {
	if !exists {
		if err := a.exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s" OWNER "%s"`, name, owner)); err != nil {
			return err
		}
	}
	// Idempotent ownership + grants (PG15+ safe); failures on ALTER are
	// tolerable when the role already owns everything.
	_ = a.exec(ctx, fmt.Sprintf(`ALTER DATABASE "%s" OWNER TO "%s"`, name, owner))
	_ = a.exec(ctx, fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s"`, name, owner))
	if err := a.execDB(ctx, name, fmt.Sprintf(`GRANT ALL ON SCHEMA public TO "%s"`, owner)); err != nil {
		return err
	}
	_ = a.execDB(ctx, name, fmt.Sprintf(`ALTER SCHEMA public OWNER TO "%s"`, owner))
	return nil
}

// AdminUserExists reports whether the app users table already has this user.
// Returns false (no error) when the table doesn't exist yet.
func AdminUserExists(ctx context.Context, databaseURL, username string) (bool, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return false, err
	}
	defer conn.Close(ctx)
	var n int
	err = conn.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE username=$1", username).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "42P01") { // undefined_table
			return false, nil
		}
		return false, err
	}
	return n > 0, nil
}
