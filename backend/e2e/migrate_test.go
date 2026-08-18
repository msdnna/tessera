//go:build e2e

// Migration rollback. Every other test in the repo only ever migrates *up*, so
// the 50-odd .down.sql files have never been executed by anything — their first
// run would be during an actual rollback of an actual release. This exercises
// them against the real cmd/migrate binary (the one that ships in the image),
// on a database of its own so a `down` can't pull the schema out from under a
// parallel `make test-backend`.
package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// rollbackSteps is how far back the test rolls. Deep enough to cross several
// releases' worth of migrations, shallow enough to stay fast.
const rollbackSteps = 5

func TestMigrationsRollBackAndReapply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	name := "tessera_e2e_" + runID + "_mig"
	if err := createDatabase(ctx, name); err != nil {
		t.Fatalf("create migration database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(name) })
	dsn := withDBName(adminDBURL, name)

	// up from nothing
	if out, err := runMigrate(dsn); err != nil {
		t.Fatalf("migrate up on a fresh database: %v\n%s", err, out)
	}
	want := countUpMigrations(t)
	if got := schemaVersion(t, dsn); got != want {
		t.Fatalf("schema version after up = %d, want %d (one per .up.sql)", got, want)
	}
	before := schemaSnapshot(ctx, t, dsn)

	// down N — the half nothing has ever run
	if out, err := runMigrate(dsn, "-down", strconv.Itoa(rollbackSteps)); err != nil {
		t.Fatalf("migrate down %d: %v\n%s", rollbackSteps, err, out)
	}
	if got := schemaVersion(t, dsn); got != want-rollbackSteps {
		t.Fatalf("schema version after down %d = %d, want %d", rollbackSteps, got, want-rollbackSteps)
	}

	// up again — the state an operator lands in after rolling a release back and
	// then forward
	if out, err := runMigrate(dsn); err != nil {
		t.Fatalf("migrate up after rollback: %v\n%s", err, out)
	}
	if got := schemaVersion(t, dsn); got != want {
		t.Fatalf("schema version after re-up = %d, want %d", got, want)
	}

	after := schemaSnapshot(ctx, t, dsn)
	if before != after {
		t.Fatalf("schema differs after up→down→up — a .down.sql does not undo its .up.sql:\n%s",
			firstDiff(before, after))
	}
}

// TestMigrateVersionFlag covers the operator-facing half of the CLI: the flag
// people actually run inside the container to find out where a box stands.
func TestMigrateVersionFlag(t *testing.T) {
	out, err := runMigrate(serverDBURL, "-version")
	if err != nil {
		t.Fatalf("migrate -version: %v\n%s", err, out)
	}
	want := countUpMigrations(t)
	if !strings.Contains(out, fmt.Sprintf("schema version: %d", want)) {
		t.Errorf("output does not report version %d:\n%s", want, out)
	}
	if !strings.Contains(out, "dirty=false") {
		t.Errorf("schema reported dirty (or the flag changed shape):\n%s", out)
	}
}

// countUpMigrations returns the highest migration number on disk, which is the
// version a fully migrated database must report.
func countUpMigrations(t *testing.T) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(backendDir, "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no .up.sql migrations found (err=%v)", err)
	}
	numRE := regexp.MustCompile(`^(\d+)_`)
	highest := 0
	for _, f := range files {
		m := numRE.FindStringSubmatch(filepath.Base(f))
		if m == nil {
			t.Fatalf("migration %s does not start with a number", filepath.Base(f))
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(f), err)
		}
		if n > highest {
			highest = n
		}
	}
	// A gap would make "highest" and "how many ran" disagree, and the version
	// assertions below would chase a phantom.
	if highest != len(files) {
		t.Fatalf("migration numbering has a gap or a duplicate: highest=%d, files=%d", highest, len(files))
	}
	return highest
}

func schemaVersion(t *testing.T, dsn string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var v int
	var dirty bool
	if err := conn.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if dirty {
		t.Fatalf("schema_migrations is dirty at version %d — a migration failed halfway", v)
	}
	return v
}

// schemaSnapshot renders the shape of the public schema as text: every column
// with its type/nullability/default, plus every index definition. Comparing two
// snapshots is what turns "the down migration ran" into "the down migration
// actually undid the up".
func schemaSnapshot(ctx context.Context, t *testing.T, dsn string) string {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var sb strings.Builder
	cols, err := conn.Query(ctx, `
		SELECT table_name, column_name, data_type, is_nullable, coalesce(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name <> 'schema_migrations'
		ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	for cols.Next() {
		var table, col, typ, nullable, def string
		if err := cols.Scan(&table, &col, &typ, &nullable, &def); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		fmt.Fprintf(&sb, "column %s.%s %s nullable=%s default=%s\n", table, col, typ, nullable, def)
	}
	if err := cols.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}

	idx, err := conn.Query(ctx, `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
		ORDER BY indexdef`)
	if err != nil {
		t.Fatalf("read indexes: %v", err)
	}
	for idx.Next() {
		var def string
		if err := idx.Scan(&def); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		fmt.Fprintf(&sb, "index %s\n", def)
	}
	if err := idx.Err(); err != nil {
		t.Fatalf("iterate indexes: %v", err)
	}
	return sb.String()
}

// firstDiff reports the lines that differ between two snapshots, so a failure
// names the offending table instead of dumping the whole schema twice.
func firstDiff(before, after string) string {
	inAfter := map[string]bool{}
	for _, l := range strings.Split(after, "\n") {
		inAfter[l] = true
	}
	inBefore := map[string]bool{}
	for _, l := range strings.Split(before, "\n") {
		inBefore[l] = true
	}
	var sb strings.Builder
	for _, l := range strings.Split(before, "\n") {
		if l != "" && !inAfter[l] {
			fmt.Fprintf(&sb, "  lost:  %s\n", l)
		}
	}
	for _, l := range strings.Split(after, "\n") {
		if l != "" && !inBefore[l] {
			fmt.Fprintf(&sb, "  extra: %s\n", l)
		}
	}
	return sb.String()
}
