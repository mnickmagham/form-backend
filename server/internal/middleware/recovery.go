package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
)

type recoveryConfig struct {
	level slog.Level
	out   io.Writer
}

func loadRecoveryConfig() *recoveryConfig {
	c := &recoveryConfig{
		level: slog.LevelError,
		out:   os.Stderr,
	}

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		c.level = slog.LevelDebug
	case "info":
		c.level = slog.LevelInfo
	case "warn", "warning":
		c.level = slog.LevelWarn
	case "error":
		c.level = slog.LevelError
	}

	switch strings.ToLower(os.Getenv("LOG_OUTPUT")) {
	case "stdout":
		c.out = os.Stdout
	case "stderr", "":
		c.out = os.Stderr
	default:
		if f, err := os.OpenFile(os.Getenv("LOG_OUTPUT"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			c.out = f
		}
	}

	return c
}

// Recovery catches panics from the wrapped handler (and anything inside it:
// logger, rate limiter, handlers, store), logs the stack trace with the
// correlation ID if one was set, and returns a 500 JSON error envelope.
// It must never panic itself, so a bad handler can't take down the process.
func Recovery(next http.Handler) http.Handler {
	c := loadRecoveryConfig()
	log := slog.New(slog.NewJSONHandler(c.out, &slog.HandlerOptions{Level: c.level}))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()

				id := "unknown"
				if v := r.Context().Value(requestIDKey{}); v != nil {
					if s, ok := v.(string); ok && s != "" {
						id = s
					}
				}

				log.Log(r.Context(), slog.LevelError, "panic recovered",
					slog.String("request_id", id),
					slog.Any("panic", rec),
					slog.String("stack", string(stack)),
				)

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-ID", id)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			}
		}()

		next.ServeHTTP(w, r)
	})
}
