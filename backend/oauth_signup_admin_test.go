// Privilege escalation through a discarded error in OAuth auto-registration.
//
// The very first account on a fresh instance is the global admin, and both
// registration paths decide that the same way: count the users, and if the
// count is zero, set is_admin. Register (handlers/auth.go) refuses with a 500
// when the count cannot be read. The GitLab auto-register path discarded that
// error instead — `count, _ := h.q.CountUsers(c)` — and Go's zero value for the
// count is 0, which reads as "first user".
//
// So the guard failed open: one transient failure on that single query (a pool
// acquire timeout under load, a connection reaped by a proxy, a failover) was
// enough to hand the account being auto-registered the global admin bit. That
// bit is the top of the instance — GET /admin/users hands out every account's
// address, PATCH /admin/users/:id/admin promotes anyone, and
// POST /admin/users/:id/reset-link mints a password reset link for any account,
// which is a takeover of every user on the server.
//
// The fault is injected where it actually happens in production: at the pool.
// countUsersUnreachable wraps the real pool and, once, runs the CountUsers
// statement on a context that is already cancelled — so pgx fails the acquire
// and returns a genuine error, exactly as it does when the pool cannot hand out
// a connection. Every other statement of the flow goes to the live pool
// untouched, which is the whole point: this is one query failing, not the
// database being down.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tessera/config"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// countUsersStmt is the body sqlc generates for CountUsers. Matching on it is
// what makes the injection surgical; the test asserts the arm actually fired,
// so a rename of the query turns this into a failure rather than a silent pass.
const countUsersStmt = "SELECT count(*) FROM users"

// countUsersUnreachable is a db.DBTX that forwards everything to the pool, but
// the first CountUsers goes out on a cancelled context and comes back as a real
// pgx error.
type countUsersUnreachable struct {
	pool  *pgxpool.Pool
	armed atomic.Bool
}

// ctxFor hands the statement either the caller's context or a dead one.
func (f *countUsersUnreachable) ctxFor(ctx context.Context, sql string) context.Context {
	if !strings.Contains(sql, countUsersStmt) || !f.armed.CompareAndSwap(true, false) {
		return ctx
	}
	dead, cancel := context.WithCancel(ctx)
	cancel()
	return dead
}

func (f *countUsersUnreachable) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return f.pool.Exec(f.ctxFor(ctx, sql), sql, args...)
}

func (f *countUsersUnreachable) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return f.pool.Query(f.ctxFor(ctx, sql), sql, args...)
}

func (f *countUsersUnreachable) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return f.pool.QueryRow(f.ctxFor(ctx, sql), sql, args...)
}

// TestOAuthSignupDoesNotFailOpenToAdmin drives a real "Continue with GitLab"
// sign-in for an address nobody holds, against an instance that already has
// plenty of users, while CountUsers is unreachable exactly once.
//
// The instance is not empty, so the honest answer is "not the first account".
// Whatever the flow does with the failure — refuse, or carry on — it must not
// come out the other side holding the admin bit.
func TestOAuthSignupDoesNotFailOpenToAdmin(t *testing.T) {
	// Not parallel: flips the instance-wide OAuth provider row.
	n := userSeq.Add(1)
	email := fmt.Sprintf("gl-count-%d@test.local", n)
	configureIDP(t, newFakeIDP(t, 930000+n, fmt.Sprintf("gl-count-%d", n), email).URL)

	// The shared harness signs up users of its own, and configureIDP just made
	// one more, so "count == 0" is false by any honest reading.
	var users int64
	if err := testPool.QueryRow(context.Background(), countUsersStmt).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users == 0 {
		t.Fatalf("на инстансе нет ни одного юзера — тест не отличит fail-open от правды")
	}

	broken := &countUsersUnreachable{pool: testPool}
	broken.armed.Store(true)
	cfg := &config.Config{
		JWTSecret:     "integration-test-secret-min32chars!!",
		UploadDir:     testUploadDir,
		EncryptionKey: "integration-test-encryption-key",
		PublicURL:     "http://tessera.test",
		CORSOrigin:    "*",
	}
	hub := realtime.NewHub()
	go hub.Run()
	r, _ := newRouter(cfg, db.New(broken), testPool, hub, mail.New(mail.Config{}))
	srv := httptest.NewServer(r)
	defer srv.Close()

	frag := gitlabSignInAt(t, srv.URL)

	if broken.armed.Load() {
		t.Fatalf("CountUsers за весь вход так и не позвали: запрос переименовали, "+
			"инъекция не сработала и тест ничего не проверил (%v)", frag)
	}

	var isAdmin bool
	err := testPool.QueryRow(context.Background(),
		`SELECT is_admin FROM users WHERE email = $1`, email).Scan(&isAdmin)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Provisioning refused, which is the sibling path's answer: no account,
		// no admin bit. The caller learns why from the redirect.
		if reason := frag.Get("oauth_error"); reason == "" {
			t.Fatalf("учётка не завелась, но клиенту не сказали почему: %v", frag)
		}
	case err != nil:
		t.Fatalf("read is_admin: %v", err)
	case isAdmin:
		t.Fatalf("эскалация: сорванный CountUsers сделал авто-заведённого через GitLab "+
			"пользователя %s глобальным админом на инстансе с %d учётками — "+
			"ему открыты адреса всех, назначение админов и reset-link на любую учётку",
			email, users)
	}
}
