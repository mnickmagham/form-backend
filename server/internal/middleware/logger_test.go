package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func withEnv(key, value string, t *testing.T) func() {
	prev, ok := os.LookupEnv(key)
	t.Helper()
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
	return func() {
		if ok {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	}
}

func TestLoggerPassThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	})

	req := httptest.NewRequest("POST", "/students", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	Logger(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("expected X-Request-ID response header")
	}
}

func TestLoggerEchoesRequestID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/students/1", nil)
	req.Header.Set("X-Request-ID", "fixed-trace-id")
	rec := httptest.NewRecorder()

	Logger(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "fixed-trace-id" {
		t.Fatalf("expected echoed request id, got %q", got)
	}
}

func TestLoggerSkipsPath(t *testing.T) {
	defer withEnv("LOG_SKIP_PATHS", "/swagger/,/healthz", t)()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/swagger/", nil)
	rec := httptest.NewRecorder()

	Logger(next).ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "" {
		t.Fatal("skipped path should not get X-Request-ID")
	}
}

func TestLoggerLogsJSONWithLevel(t *testing.T) {
	defer withEnv("LOG_OUTPUT", "", t)() // stdout
	defer withEnv("LOG_LEVEL", "debug", t)()

	logFile := os.TempDir() + "/form-backend-logger-test.log"
	defer os.Remove(logFile) // best effort; slog holds the handle open on Windows
	defer withEnv("LOG_OUTPUT", logFile, t)()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest("GET", "/students/999", nil)
	rec := httptest.NewRecorder()

	Logger(next).ServeHTTP(rec, req)

	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("expected log file to be created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected log file to be non-empty")
	}
}
