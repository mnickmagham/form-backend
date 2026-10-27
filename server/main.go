package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"form-backend/server/internal/handler"
	"form-backend/server/internal/middleware"
	"form-backend/server/internal/store"
	httpSwagger "github.com/swaggo/http-swagger"
)

func main() {
	db, err := store.Open()
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	studentStore := store.NewStudentStore(db)
	h := handler.NewStudentHandler(studentStore)

	mux := http.NewServeMux()
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)
	mux.Handle("GET /test-ui/", http.StripPrefix("/test-ui/", http.FileServer(http.Dir("ui"))))
	mux.HandleFunc("GET /healthz", healthHandler(db))
	mux.HandleFunc("POST /students", h.Register)
	mux.HandleFunc("GET /students", h.Search)
	mux.HandleFunc("GET /students/{id}", h.Get)
	mux.HandleFunc("PUT /students/{id}", h.Update)
	mux.HandleFunc("DELETE /students/{id}", h.Delete)
	mux.Handle("GET /", frontendHandler())

	rlPerMinute := 60
	if v := os.Getenv("RATE_LIMIT_PER_MINUTE"); v != "" {
		fmt.Sscanf(v, "%d", &rlPerMinute)
	}
	trustProxy := strings.EqualFold(os.Getenv("RATE_LIMIT_TRUST_PROXY"), "true")

	wrapped := middleware.Recovery(middleware.Logger(middleware.RateLimit(mux, rlPerMinute, trustProxy)))

	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, wrapped); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// frontendHandler serves the static frontend from FRONTEND_DIR (the repo's
// frontend/ directory when running locally). The old single-file test UI is
// kept at /test-ui/ and is used as a fallback when the frontend is missing.
func frontendHandler() http.Handler {
	dir := envOr("FRONTEND_DIR", "../frontend")
	if info, err := os.Stat(filepath.Join(dir, "index.html")); err != nil || info.IsDir() {
		log.Printf("frontend not found at %q (%v); falling back to ui", dir, err)
		dir = "ui"
	}
	return http.FileServer(http.Dir(dir))
}

// healthHandler reports whether the HTTP server and its database dependency
// are healthy. Docker Compose and uptime monitors use this endpoint.
func healthHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		status := http.StatusOK
		body := map[string]string{"status": "ok"}
		if err := db.PingContext(ctx); err != nil {
			status = http.StatusServiceUnavailable
			body["status"] = "unhealthy"
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
