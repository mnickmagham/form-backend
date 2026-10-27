package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type config struct {
	level slog.Level
	out   io.Writer
	skip  map[string]bool
}

// loadConfig reads logging knobs from the environment. Called on each
// Logger() construction (once at startup), so tests can flip env vars.
func loadConfig() *config {
	c := &config{
		level: slog.LevelInfo,
		out:   os.Stdout,
		skip:  make(map[string]bool),
	}

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		c.level = slog.LevelDebug
	case "warn", "warning":
		c.level = slog.LevelWarn
	case "error":
		c.level = slog.LevelError
	}

	switch strings.ToLower(os.Getenv("LOG_OUTPUT")) {
	case "stderr":
		c.out = os.Stderr
	case "stdout", "":
		c.out = os.Stdout
	default:
		if f, err := os.OpenFile(os.Getenv("LOG_OUTPUT"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			c.out = f
		}
	}

	for _, p := range strings.Split(os.Getenv("LOG_SKIP_PATHS"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			c.skip[p] = true
		}
	}

	return c
}

// responseWriter captures status code and bytes written so the log line can
// include them. http.ResponseWriter hides both otherwise.
type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
	done   bool
}

func (w *responseWriter) WriteHeader(code int) {
	if w.done {
		return
	}
	w.done = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.done {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *responseWriter) code() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

type requestIDKey struct{}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

// Logger logs each request as one structured JSON line, then echoes the
// correlation ID back as X-Request-ID so clients can trace their requests.
// Never logs request/response bodies (PII + perf). Safe: a logger failure
// never breaks the request.
func Logger(next http.Handler) http.Handler {
	c := loadConfig()
	log := slog.New(slog.NewJSONHandler(c.out, &slog.HandlerOptions{Level: c.level}))
	skip := c.skip

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skip[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newID()
		}
		// Headers must be set before the handler writes the response;
		// setting them afterwards is a no-op on a real server.
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		r = r.WithContext(ctx)

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		dur := time.Since(start)

		level := slog.LevelInfo
		switch {
		case rw.code() >= 500:
			level = slog.LevelError
		case rw.code() >= 400:
			level = slog.LevelWarn
		}

		log.Log(ctx, level, "request",
			slog.String("request_id", id),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("client_ip", clientIP(r)),
			slog.String("user_agent", r.UserAgent()),
			slog.Int("status", rw.code()),
			slog.Int("bytes", rw.bytes),
			slog.String("duration", dur.String()),
		)
	})
}

// clientIP returns the best-effort client (source) address.
func clientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx > 0 {
		ip = ip[:idx]
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if parts := strings.Split(xff, ","); len(parts) > 0 {
			ip = strings.TrimSpace(parts[0])
		}
	}
	if xrip := r.Header.Get("X-Real-Ip"); xrip != "" {
		ip = xrip
	}
	return ip
}
