// Package app holds everything the frameworks share: config, database access,
// validation and the CPU workload. Each cmd/<framework> only wires HTTP routes,
// so the benchmark compares the HTTP layer and nothing else.
package app

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Post struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PostInput struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

// ErrNotFound is returned when a post id does not exist.
var ErrNotFound = errors.New("not found")

type App struct {
	DB      *pgxpool.Pool
	Runtime string
	Addr    string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// New connects to Postgres, runs the migration and seeds 100 posts once.
func New(runtime string) *App {
	ctx := context.Background()
	maxConns, _ := strconv.Atoi(env("DB_MAX_CONNS", "50"))

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&pool_max_conns=%d",
		env("DB_USERNAME", "bench"), env("DB_PASSWORD", "secret"),
		env("DB_HOST", "localhost"), env("DB_PORT", "5432"),
		env("DB_DATABASE", "go_bench"), maxConns)

	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 30; i++ {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
			pool.Close()
		}
		log.Printf("waiting for database: %v", err)
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	return &App{DB: pool, Runtime: env("APP_RUNTIME", runtime), Addr: ":" + env("PORT", "8000")}
}

// migrate mirrors the Laravel posts migration. An advisory lock stops the
// containers, which all start at once, from seeding twice.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(42)`); err != nil {
		return err
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock(42)`)

	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS posts (
		id         BIGSERIAL PRIMARY KEY,
		title      VARCHAR(255) NOT NULL,
		body       TEXT NOT NULL,
		created_at TIMESTAMP(0) NULL,
		updated_at TIMESTAMP(0) NULL
	)`)
	if err != nil {
		return err
	}

	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM posts`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	rows := make([][]any, 100)
	now := time.Now().UTC()
	for i := range rows {
		rows[i] = []any{sentence(), paragraphs(3), now, now}
	}
	_, err = conn.CopyFrom(ctx, pgx.Identifier{"posts"},
		[]string{"title", "body", "created_at", "updated_at"}, pgx.CopyFromRows(rows))
	if err == nil {
		log.Printf("seeded 100 posts")
	}
	return err
}

const postCols = `id, title, body, created_at, updated_at`

func scanPost(row pgx.Row) (Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.Title, &p.Body, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// LatestPosts matches Laravel's Post::latest()->limit(20)->get().
func (a *App) LatestPosts(ctx context.Context) ([]Post, error) {
	rows, err := a.DB.Query(ctx, `SELECT `+postCols+` FROM posts ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := make([]Post, 0, 20)
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (a *App) FindPost(ctx context.Context, id int64) (Post, error) {
	p, err := scanPost(a.DB.QueryRow(ctx, `SELECT `+postCols+` FROM posts WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (a *App) CreatePost(ctx context.Context, title, body string) (Post, error) {
	now := time.Now().UTC().Truncate(time.Second)
	return scanPost(a.DB.QueryRow(ctx,
		`INSERT INTO posts (title, body, created_at, updated_at) VALUES ($1, $2, $3, $3) RETURNING `+postCols,
		title, body, now))
}

// Validate applies Laravel's rules: title required|string|max:255, body required|string.
// It returns nil when valid, otherwise a 422 body in Laravel's shape.
func Validate(in PostInput) map[string]any {
	errs := map[string][]string{}
	if in.Title == nil || strings.TrimSpace(*in.Title) == "" {
		errs["title"] = []string{"The title field is required."}
	} else if utf8.RuneCountInString(*in.Title) > 255 {
		errs["title"] = []string{"The title field must not be greater than 255 characters."}
	}
	if in.Body == nil || strings.TrimSpace(*in.Body) == "" {
		errs["body"] = []string{"The body field is required."}
	}
	if len(errs) == 0 {
		return nil
	}
	return map[string]any{"message": "The given data was invalid.", "errors": errs}
}

// Response bodies, shared so every framework sends the same JSON.

func (a *App) Health() map[string]any {
	return map[string]any{
		"status":    "ok",
		"runtime":   a.Runtime,
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

func ListBody(posts []Post) map[string]any { return map[string]any{"data": posts, "count": len(posts)} }
func PostBody(p Post) map[string]any       { return map[string]any{"data": p} }

var NotFoundBody = map[string]any{"message": "Not found."}
var ServerErrorBody = map[string]any{"message": "Server Error"}
var InvalidJSONBody = map[string]any{"message": "Invalid JSON body."}

// Heavy is the same CPU work as the Laravel /api/heavy route:
// recursive fibonacci(30) plus 100 md5 hashes and a JSON round trip.
func Heavy() map[string]any {
	result := fib(30)

	type nested struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type item struct {
		Index  int    `json:"index"`
		Value  string `json:"value"`
		Nested nested `json:"nested"`
	}
	data := make([]item, 100)
	for i := range data {
		sum := md5.Sum([]byte(strconv.Itoa(i)))
		data[i] = item{Index: i, Value: hex.EncodeToString(sum[:]), Nested: nested{A: i * 2, B: i * 3}}
	}
	encoded, _ := json.Marshal(data)
	var decoded []map[string]any
	_ = json.Unmarshal(encoded, &decoded)

	return map[string]any{"fibonacci_30": result, "iterations": 100, "status": "computed"}
}

func fib(n int) int {
	if n <= 1 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

// Seed text, similar in size to Faker's sentence() and paragraphs(3).

var words = strings.Fields(`lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod
tempor incididunt ut labore et dolore magna aliqua enim ad minim veniam quis nostrud exercitation
ullamco laboris nisi aliquip ex ea commodo consequat duis aute irure in reprehenderit voluptate
velit esse cillum fugiat nulla pariatur excepteur sint occaecat cupidatat non proident sunt culpa
qui officia deserunt mollit anim id est laborum`)

func sentence() string {
	n := 4 + rand.IntN(6)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = words[rand.IntN(len(words))]
	}
	s := strings.Join(parts, " ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func paragraphs(n int) string {
	ps := make([]string, n)
	for i := range ps {
		ss := make([]string, 3+rand.IntN(4))
		for j := range ss {
			ss[j] = sentence()
		}
		ps[i] = strings.Join(ss, " ")
	}
	return strings.Join(ps, "\n\n")
}
