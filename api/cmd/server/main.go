package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type server struct {
	db *pgxpool.Pool
}

func main() {
	port := env("PORT", "8080")
	databaseURL := os.Getenv("DATABASE_URL")
	var pool *pgxpool.Pool
	if databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		pool, err = pgxpool.New(ctx, databaseURL)
		if err != nil {
			log.Fatalf("database configuration failed: %v", err)
		}
		if err = pool.Ping(ctx); err != nil {
			log.Fatalf("database connection failed: %v", err)
		}
		if err = runMigrations(pool); err != nil {
			log.Fatalf("database migrations failed: %v", err)
		}
		defer pool.Close()
	}

	s := &server{db: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/status", s.status)
	mux.HandleFunc("GET /api/v1/geocode", s.geocode)
	mux.HandleFunc("POST /api/v1/routes", s.directions)
	mux.HandleFunc("GET /api/v1/safety-reports", s.listSafetyReports)
	mux.Handle("POST /api/v1/safety-reports", s.authenticate(http.HandlerFunc(s.submitSafetyReport)))
	mux.Handle("GET /api/v1/admin/safety-reports", s.authenticate(http.HandlerFunc(s.listPendingSafetyReports)))
	mux.Handle("PATCH /api/v1/admin/safety-reports/{id}", s.authenticate(http.HandlerFunc(s.moderateSafetyReport)))
	mux.Handle("DELETE /api/v1/admin/safety-reports/{id}", s.authenticate(http.HandlerFunc(s.deleteSafetyReport)))
	mux.Handle("POST /api/v1/admin/safety-reports/import", s.authenticate(http.HandlerFunc(s.importSafetyFeeds)))
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/password-reset/request", s.requestPasswordReset)
	mux.HandleFunc("POST /api/v1/auth/password-reset/confirm", s.confirmPasswordReset)
	mux.Handle("GET /api/v1/auth/me", s.authenticate(http.HandlerFunc(s.me)))
	mux.Handle("POST /api/v1/auth/logout", s.authenticate(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/contacts", s.authenticate(http.HandlerFunc(s.listContacts)))
	mux.Handle("GET /api/v1/places", s.authenticate(http.HandlerFunc(s.listSavedPlaces)))
	mux.Handle("POST /api/v1/places", s.authenticate(http.HandlerFunc(s.createSavedPlace)))
	mux.Handle("DELETE /api/v1/places/{id}", s.authenticate(http.HandlerFunc(s.deleteSavedPlace)))
	mux.Handle("GET /api/v1/routes/history", s.authenticate(http.HandlerFunc(s.listRouteHistory)))
	mux.Handle("POST /api/v1/routes/history", s.authenticate(http.HandlerFunc(s.createRouteHistory)))
	mux.Handle("POST /api/v1/contacts", s.authenticate(http.HandlerFunc(s.createContact)))
	mux.Handle("DELETE /api/v1/contacts/{id}", s.authenticate(http.HandlerFunc(s.deleteContact)))
	mux.Handle("POST /api/v1/sos", s.authenticate(http.HandlerFunc(s.createSOS)))
	mux.Handle("GET /api/v1/sos", s.authenticate(http.HandlerFunc(s.listSOS)))

	handler := securityHeaders(cors(mux))
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("SafeRoute API listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "saferoute-api"})
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	dbStatus := "not_configured"
	if s.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"service": "saferoute-api", "database": "unavailable"})
			return
		}
		dbStatus = "connected"
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "saferoute-api", "database": dbStatus})
}

func cors(next http.Handler) http.Handler {
	allowed := strings.Split(env("CORS_ORIGINS", "http://localhost:5173"), ",")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, candidate := range allowed {
			if strings.TrimSpace(candidate) == origin && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		log.Printf("response encode error: %v", err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func runMigrations(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil { return fmt.Errorf("read embedded migrations: %w", err) }
	sort.Slice(entries, func(i,j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") { continue }
		var applied bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, entry.Name()).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if applied { continue }
		sqlBytes, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil { return fmt.Errorf("read migration %s: %w", entry.Name(), err) }
		tx, err := pool.Begin(ctx)
		if err != nil { return fmt.Errorf("begin migration %s: %w", entry.Name(), err) }
		for _, statement := range strings.Split(string(sqlBytes), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" { continue }
			if _, err = tx.Exec(ctx, statement); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, entry.Name()); err != nil { _ = tx.Rollback(ctx); return fmt.Errorf("record migration %s: %w", entry.Name(), err) }
		if err = tx.Commit(ctx); err != nil { return fmt.Errorf("commit migration %s: %w", entry.Name(), err) }
		log.Printf("applied database migration %s", entry.Name())
	}
	return nil
}
