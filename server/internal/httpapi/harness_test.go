package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"nexus/internal/auth"
	"nexus/internal/checker"
	"nexus/internal/db"
	"nexus/internal/host"
	"nexus/internal/httpapi"
)

// adminURL is the Postgres connection used to create/drop ephemeral test
// databases. Overridable with NEXUS_TEST_DATABASE_URL.
func adminURL() string {
	if v := os.Getenv("NEXUS_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgresql://postgres:postgres@10.37.41.217:5432/postgres"
}

const testPassword = "correct-horse-battery-staple"

var dbCounter int64

// harness bundles the running test server plus its dependencies.
type harness struct {
	t       *testing.T
	server  *httpapi.Server
	ts      *httptest.Server
	host    *host.FakeHost
	prober  *checker.FakeProber
	fetcher *checker.FakeFetcher
	cookie  *http.Cookie
	dbName  string
	adminDB *pgxpool.Pool
	pool    *pgxpool.Pool
}

// newHarness spins up an ephemeral Postgres database, runs migrations, and
// starts the real chi router via httptest. The returned harness is already
// authenticated unless noLogin is true.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessCfg(t, harnessConfig{})
}

// newHarnessSeeded is newHarness with a migration-survival hook: the database
// is first migrated only to schema version fromVersion, seed runs against it
// (to insert rows under the old schema), and then the remaining migrations run
// before the server starts.
func newHarnessSeeded(t *testing.T, fromVersion uint, seed func(ctx context.Context, pool *pgxpool.Pool)) *harness {
	t.Helper()
	return newHarnessCfg(t, harnessConfig{fromVersion: fromVersion, seed: seed})
}

// newHarnessAuth is newHarness with extra Authenticator options (clock,
// cookie, proxy, limit policy) for the login-hardening tests.
func newHarnessAuth(t *testing.T, opts ...auth.Option) *harness {
	t.Helper()
	return newHarnessCfg(t, harnessConfig{authOpts: opts})
}

type harnessConfig struct {
	fromVersion uint
	seed        func(ctx context.Context, pool *pgxpool.Pool)
	authOpts    []auth.Option
}

func newHarnessCfg(t *testing.T, cfg harnessConfig) *harness {
	t.Helper()
	ctx := context.Background()
	seed, fromVersion := cfg.seed, cfg.fromVersion

	admin, err := pgxpool.New(ctx, adminURL())
	if err != nil {
		t.Skipf("cannot reach test postgres: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("cannot ping test postgres: %v", err)
	}

	n := atomic.AddInt64(&dbCounter, 1)
	dbName := fmt.Sprintf("nexus_test_%d_%d", os.Getpid(), n)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		admin.Close()
		t.Fatalf("create test db: %v", err)
	}

	testURL := replaceDBName(adminURL(), dbName)

	if seed != nil {
		if err := db.MigrateTo(testURL, fromVersion); err != nil {
			dropDB(ctx, admin, dbName)
			admin.Close()
			t.Fatalf("migrate to %d: %v", fromVersion, err)
		}
		seedPool, err := db.Connect(ctx, testURL)
		if err != nil {
			dropDB(ctx, admin, dbName)
			admin.Close()
			t.Fatalf("connect for seed: %v", err)
		}
		seed(ctx, seedPool)
		seedPool.Close()
	}

	if err := db.Migrate(testURL); err != nil {
		dropDB(ctx, admin, dbName)
		admin.Close()
		t.Fatalf("migrate: %v", err)
	}

	pool, err := db.Connect(ctx, testURL)
	if err != nil {
		dropDB(ctx, admin, dbName)
		admin.Close()
		t.Fatalf("connect test db: %v", err)
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.DefaultCost)
	authn := auth.New(string(hash), []byte("test-signing-key"), cfg.authOpts...)
	fakeHost := host.NewFakeHost()
	fakeProber := checker.NewFakeProber()
	fakeFetcher := checker.NewFakeFetcher()
	srv := httpapi.NewServer(pool, authn, fakeHost, fakeProber, fakeFetcher)

	ts := httptest.NewServer(srv.Router(nil))

	h := &harness{
		t:       t,
		server:  srv,
		ts:      ts,
		host:    fakeHost,
		prober:  fakeProber,
		fetcher: fakeFetcher,
		dbName:  dbName,
		adminDB: admin,
		pool:    pool,
	}
	t.Cleanup(func() {
		// Let create-time background work finish before the DB is dropped.
		srv.WaitBackgroundForTest()
		ts.Close()
		pool.Close()
		dropDB(context.Background(), admin, dbName)
		admin.Close()
	})

	h.login()
	return h
}

func dropDB(ctx context.Context, admin *pgxpool.Pool, name string) {
	// Terminate connections then drop.
	_, _ = admin.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", name)
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name)
}

// replaceDBName swaps the final path segment of a postgres URL with name.
func replaceDBName(url, name string) string {
	// url like postgresql://user:pass@host:port/dbname?params
	// Find the "/" after host[:port].
	schemeEnd := 0
	for i := 0; i+2 < len(url); i++ {
		if url[i] == ':' && url[i+1] == '/' && url[i+2] == '/' {
			schemeEnd = i + 3
			break
		}
	}
	rest := url[schemeEnd:]
	slash := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			slash = i
			break
		}
	}
	base := url
	query := ""
	if slash >= 0 {
		base = url[:schemeEnd+slash]
		after := rest[slash+1:]
		if q := indexByte(after, '?'); q >= 0 {
			query = after[q:]
		}
	}
	return base + "/" + name + query
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// login authenticates and stores the session cookie on the harness.
func (h *harness) login() {
	h.t.Helper()
	body, _ := json.Marshal(map[string]string{"password": testPassword})
	resp, err := http.Post(h.ts.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		h.t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			h.cookie = c
		}
	}
	if h.cookie == nil {
		h.t.Fatal("login did not set session cookie")
	}
}

// do performs an authenticated request and returns the response.
func (h *harness) do(method, path string, body interface{}) *http.Response {
	h.t.Helper()
	return h.doWithCookie(method, path, body, h.cookie)
}

func (h *harness) doWithCookie(method, path string, body interface{}, cookie *http.Cookie) *http.Response {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.ts.URL+path, rdr)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	return resp
}

// decode reads a JSON body into v and closes the response.
func decode(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// decodeItems reads a paged list envelope ({"items": [...], ...}) into v and
// closes the response.
func decodeItems(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	var env struct {
		Items json.RawMessage `json:"items"`
	}
	decode(t, resp, &env)
	if err := json.Unmarshal(env.Items, v); err != nil {
		t.Fatalf("decode items: %v", err)
	}
}

func mustStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, want, string(b))
	}
}

// path builds "prefix<id>" for int64 ids in request URLs.
func path(prefix string, id int64) string {
	return fmt.Sprintf("%s%d", prefix, id)
}

// canned vitals used by servers tests.
func cannedVitals() host.Vitals {
	return host.Vitals{
		CPUPercent: 42.5,
		MemTotal:   16 << 30,
		MemUsed:    8 << 30,
		DiskTotal:  512 << 30,
		DiskUsed:   100 << 30,
		NetRx:      1000,
		NetTx:      2000,
		Load1:      1.5,
		Load5:      1.2,
		Load15:     0.9,
		UptimeSecs: 3600,
		TopProcs: []host.Process{
			{PID: 1, Name: "init", CPU: 0.1, MemRSS: 1024, MemPerc: 0.1},
		},
	}
}

var _ = time.Now
