// Command benchmark runs a comprehensive performance comparison between
// VoidDB and PostgreSQL across several workloads.
//
// Prerequisites:
//   - VoidDB server running on VOID_URL (default http://localhost:7700)
//   - PostgreSQL accessible at PG_DSN (default postgres://postgres:postgres@localhost:5432/bench?sslmode=disable)
//
// Run:
//
//	go run benchmark/main.go
//	go run benchmark/main.go -workers 8 -records 100000
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

// ── Configuration ─────────────────────────────────────────────────────────────

var (
	voidURL  = flag.String("void-url", envOr("VOID_URL", "http://localhost:7700"), "VoidDB server URL")
	pgDSN    = flag.String("pg-dsn", envOr("PG_DSN", "postgres://postgres:postgres@localhost:5432/bench?sslmode=disable"), "PostgreSQL DSN")
	records  = flag.Int("records", 50_000, "number of records to insert/query")
	workers  = flag.Int("workers", 4, "number of concurrent workers")
	token    string
)

// ── Result ────────────────────────────────────────────────────────────────────

// BenchResult holds timing results for a single benchmark.
type BenchResult struct {
	Name     string
	DB       string
	Ops      int64
	Duration time.Duration
	Errors   int64
}

func (r BenchResult) OpsPerSec() float64 {
	if r.Duration == 0 {
		return 0
	}
	return float64(r.Ops) / r.Duration.Seconds()
}

func (r BenchResult) AvgLatency() time.Duration {
	if r.Ops == 0 {
		return 0
	}
	return r.Duration / time.Duration(r.Ops)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║        VoidDB vs PostgreSQL  Benchmark Suite         ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Printf("  Records : %d\n", *records)
	fmt.Printf("  Workers : %d\n", *workers)
	fmt.Println()

	ctx := context.Background()

	// --- Authenticate with VoidDB ---
	var err error
	token, err = voidLogin(ctx, *voidURL, "admin", "admin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ VoidDB login failed: %v\n", err)
		fmt.Println("  Tip: ensure VoidDB is running (docker-compose up voiddb)")
	}

	// --- Connect to PostgreSQL ---
	pg, pgErr := sql.Open("postgres", *pgDSN)
	if pgErr == nil {
		pgErr = pg.PingContext(ctx)
	}
	if pgErr != nil {
		fmt.Fprintf(os.Stderr, "✗ PostgreSQL unreachable: %v\n", pgErr)
		fmt.Println("  Tip: ensure PostgreSQL is running (docker-compose up postgres)")
	}

	// --- Setup ---
	if token != "" {
		setupVoid(ctx, *voidURL, token)
	}
	if pg != nil && pgErr == nil {
		setupPostgres(ctx, pg)
	}

	var results []BenchResult

	// ── Benchmark 1: Sequential INSERT ───────────────────────────────────────
	fmt.Println("▶ Benchmark 1: Sequential INSERT")
	if token != "" {
		results = append(results, benchVoidInsert(ctx, *voidURL, *records, 1))
	}
	if pg != nil && pgErr == nil {
		results = append(results, benchPGInsert(ctx, pg, *records, 1))
	}
	printResults(results)
	results = nil

	// ── Benchmark 2: Concurrent INSERT ───────────────────────────────────────
	fmt.Printf("▶ Benchmark 2: Concurrent INSERT (%d workers)\n", *workers)
	if token != "" {
		results = append(results, benchVoidInsert(ctx, *voidURL, *records, *workers))
	}
	if pg != nil && pgErr == nil {
		results = append(results, benchPGInsert(ctx, pg, *records, *workers))
	}
	printResults(results)
	results = nil

	// ── Benchmark 3: Point GET by ID ──────────────────────────────────────────
	fmt.Println("▶ Benchmark 3: Point GET (lookup by primary key)")
	if token != "" {
		ids := seedVoid(ctx, *voidURL, min(*records, 10_000))
		results = append(results, benchVoidGet(ctx, *voidURL, ids, *workers))
	}
	if pg != nil && pgErr == nil {
		ids := seedPG(ctx, pg, min(*records, 10_000))
		results = append(results, benchPGGet(ctx, pg, ids, *workers))
	}
	printResults(results)
	results = nil

	// ── Benchmark 4: Range scan / filter query ────────────────────────────────
	fmt.Println("▶ Benchmark 4: Range SCAN (filter age >= 18, limit 100)")
	if token != "" {
		results = append(results, benchVoidScan(ctx, *voidURL, *workers))
	}
	if pg != nil && pgErr == nil {
		results = append(results, benchPGScan(ctx, pg, *workers))
	}
	printResults(results)

	fmt.Println("\n✓ Benchmark complete.")
}

// ── VoidDB helpers ────────────────────────────────────────────────────────────

func voidLogin(ctx context.Context, base, user, pass string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/v1/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var res struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return res.AccessToken, nil
}

func voidRequest(ctx context.Context, method, url, tok string, body interface{}, out interface{}) error {
	var bodyStr string
	if body != nil {
		b, _ := json.Marshal(body)
		bodyStr = string(b)
	}
	req, _ := http.NewRequestWithContext(ctx, method, url, strings.NewReader(bodyStr))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		json.NewDecoder(resp.Body).Decode(out)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func setupVoid(ctx context.Context, base, tok string) {
	voidRequest(ctx, "POST", base+"/v1/databases", tok, map[string]string{"name": "bench"}, nil)
	voidRequest(ctx, "POST", base+"/v1/databases/bench/collections", tok, map[string]string{"name": "users"}, nil)
}

func setupPostgres(ctx context.Context, db *sql.DB) {
	db.ExecContext(ctx, `DROP TABLE IF EXISTS bench_users`)
	db.ExecContext(ctx, `CREATE TABLE bench_users (
		id TEXT PRIMARY KEY,
		name TEXT,
		age INT,
		email TEXT,
		active BOOLEAN,
		created_at TIMESTAMP
	)`)
}

func randDoc(i int) map[string]interface{} {
	return map[string]interface{}{
		"name":       fmt.Sprintf("User%d", i),
		"age":        rand.Intn(80) + 10,
		"email":      fmt.Sprintf("user%d@example.com", i),
		"active":     rand.Intn(2) == 1,
		"created_at": time.Now().Unix(),
	}
}

func benchVoidInsert(ctx context.Context, base string, n, w int) BenchResult {
	var ops, errs atomic.Int64
	perWorker := n / w
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < w; i++ {
		wg.Add(1)
		offset := i * perWorker
		go func(off int) {
			defer wg.Done()
			for j := off; j < off+perWorker; j++ {
				err := voidRequest(ctx, "POST", base+"/v1/databases/bench/users", token, randDoc(j), nil)
				if err != nil {
					errs.Add(1)
				} else {
					ops.Add(1)
				}
			}
		}(offset)
	}
	wg.Wait()
	return BenchResult{Name: "Sequential INSERT", DB: "VoidDB", Ops: ops.Load(), Duration: time.Since(start), Errors: errs.Load()}
}

func seedVoid(ctx context.Context, base string, n int) []string {
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		var res struct {
			ID string `json:"_id"`
		}
		voidRequest(ctx, "POST", base+"/v1/databases/bench/users", token, randDoc(i), &res)
		if res.ID != "" {
			ids = append(ids, res.ID)
		}
	}
	return ids
}

func benchVoidGet(ctx context.Context, base string, ids []string, w int) BenchResult {
	var ops, errs atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	chunk := len(ids) / w
	for i := 0; i < w; i++ {
		wg.Add(1)
		slice := ids[i*chunk : (i+1)*chunk]
		go func(sl []string) {
			defer wg.Done()
			for _, id := range sl {
				err := voidRequest(ctx, "GET", base+"/v1/databases/bench/users/"+id, token, nil, nil)
				if err != nil {
					errs.Add(1)
				} else {
					ops.Add(1)
				}
			}
		}(slice)
	}
	wg.Wait()
	return BenchResult{Name: "Point GET", DB: "VoidDB", Ops: ops.Load(), Duration: time.Since(start), Errors: errs.Load()}
}

func benchVoidScan(ctx context.Context, base string, w int) BenchResult {
	query := map[string]interface{}{
		"where":    []map[string]interface{}{{"field": "age", "op": "gte", "value": 18}},
		"order_by": []map[string]interface{}{{"field": "name", "dir": "asc"}},
		"limit":    100,
	}
	var ops, errs atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < w; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				err := voidRequest(ctx, "POST", base+"/v1/databases/bench/users/query", token, query, nil)
				if err != nil {
					errs.Add(1)
				} else {
					ops.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	return BenchResult{Name: "Range SCAN", DB: "VoidDB", Ops: ops.Load(), Duration: time.Since(start), Errors: errs.Load()}
}

// ── PostgreSQL helpers ────────────────────────────────────────────────────────

func benchPGInsert(ctx context.Context, db *sql.DB, n, w int) BenchResult {
	var ops, errs atomic.Int64
	perWorker := n / w
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < w; i++ {
		wg.Add(1)
		offset := i * perWorker
		go func(off int) {
			defer wg.Done()
			for j := off; j < off+perWorker; j++ {
				d := randDoc(j)
				_, err := db.ExecContext(ctx,
					`INSERT INTO bench_users (id, name, age, email, active, created_at) VALUES (gen_random_uuid()::text, $1, $2, $3, $4, NOW())`,
					d["name"], d["age"], d["email"], d["active"])
				if err != nil {
					errs.Add(1)
				} else {
					ops.Add(1)
				}
			}
		}(offset)
	}
	wg.Wait()
	return BenchResult{Name: "Sequential INSERT", DB: "PostgreSQL", Ops: ops.Load(), Duration: time.Since(start), Errors: errs.Load()}
}

func seedPG(ctx context.Context, db *sql.DB, n int) []string {
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		d := randDoc(i)
		var id string
		db.QueryRowContext(ctx,
			`INSERT INTO bench_users (id, name, age, email, active, created_at) VALUES (gen_random_uuid()::text, $1, $2, $3, $4, NOW()) RETURNING id`,
			d["name"], d["age"], d["email"], d["active"]).Scan(&id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func benchPGGet(ctx context.Context, db *sql.DB, ids []string, w int) BenchResult {
	var ops, errs atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	chunk := len(ids) / w
	for i := 0; i < w; i++ {
		wg.Add(1)
		slice := ids[i*chunk : (i+1)*chunk]
		go func(sl []string) {
			defer wg.Done()
			for _, id := range sl {
				row := db.QueryRowContext(ctx, `SELECT id FROM bench_users WHERE id = $1`, id)
				var dummy string
				if err := row.Scan(&dummy); err != nil {
					errs.Add(1)
				} else {
					ops.Add(1)
				}
			}
		}(slice)
	}
	wg.Wait()
	return BenchResult{Name: "Point GET", DB: "PostgreSQL", Ops: ops.Load(), Duration: time.Since(start), Errors: errs.Load()}
}

func benchPGScan(ctx context.Context, db *sql.DB, w int) BenchResult {
	var ops, errs atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < w; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rows, err := db.QueryContext(ctx,
					`SELECT id, name, age FROM bench_users WHERE age >= 18 ORDER BY name LIMIT 100`)
				if err != nil {
					errs.Add(1)
					continue
				}
				for rows.Next() {
					var id, name string
					var age int
					rows.Scan(&id, &name, &age)
				}
				rows.Close()
				ops.Add(1)
			}
		}()
	}
	wg.Wait()
	return BenchResult{Name: "Range SCAN", DB: "PostgreSQL", Ops: ops.Load(), Duration: time.Since(start), Errors: errs.Load()}
}

// ── Output ────────────────────────────────────────────────────────────────────

func printResults(results []BenchResult) {
	fmt.Printf("\n  %-20s  %-12s  %-14s  %-16s  %s\n",
		"Database", "Ops", "Duration", "Ops/sec", "Avg Latency")
	fmt.Println("  " + strings.Repeat("─", 80))
	for _, r := range results {
		speedup := ""
		fmt.Printf("  %-20s  %-12d  %-14s  %-16.0f  %s %s\n",
			r.DB, r.Ops, r.Duration.Round(time.Millisecond),
			r.OpsPerSec(), r.AvgLatency().Round(time.Microsecond), speedup)
	}
	if len(results) == 2 && results[0].OpsPerSec() > 0 && results[1].OpsPerSec() > 0 {
		ratio := results[0].OpsPerSec() / results[1].OpsPerSec()
		winner := results[0].DB
		if ratio < 1 {
			ratio = 1 / ratio
			winner = results[1].DB
		}
		fmt.Printf("\n  → %s is %.1fx faster\n", winner, ratio)
	}
	fmt.Println()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
