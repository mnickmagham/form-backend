package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitAllowsAndCountsDown(t *testing.T) {
	defer withEnv("RATE_LIMIT_SKIP_PATHS", "", t)()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	limited := RateLimit(next, 120, false) // 2/sec

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/students", nil)
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	// Fourth should still be OK (burst=120), just check headers exist.
	req := httptest.NewRequest("GET", "/students", nil)
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 within burst, got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatal("missing X-RateLimit-Limit header")
	}
	if rec.Header().Get("X-RateLimit-Remaining") == "" {
		t.Fatal("missing X-RateLimit-Remaining header")
	}
}

func TestRateLimitRejectsWith429(t *testing.T) {
	defer withEnv("RATE_LIMIT_SKIP_PATHS", "", t)()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limited := RateLimit(next, 1, false) // 1/min, burst 1

	// First passes.
	req := httptest.NewRequest("GET", "/students", nil)
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rec.Code)
	}

	// Second is rejected immediately (burst already spent).
	req = httptest.NewRequest("GET", "/students", nil)
	rec = httptest.NewRecorder()
	limited.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header on 429")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json content type, got %q", rec.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("429 body not valid JSON: %v", err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Fatalf("unexpected 429 body: %v", body)
	}
}

func TestRateLimitSkipsPath(t *testing.T) {
	defer withEnv("RATE_LIMIT_SKIP_PATHS", "/healthz", t)()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	limited := RateLimit(next, 1, false) // burst 1 — would reject the second call

	// Hit the skip path twice; both should pass because it's skipped.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/healthz", nil)
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("skipped path expected 200, got %d", rec.Code)
		}
	}
	if !called {
		t.Fatal("expected inner handler to run for skipped path")
	}
}

func TestRateLimitTrustProxy(t *testing.T) {
	defer withEnv("RATE_LIMIT_SKIP_PATHS", "", t)()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limited := RateLimit(next, 1, true) // trust proxy, burst 1

	// First request via proxy header passes.
	req := httptest.NewRequest("GET", "/students", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	rec := httptest.NewRecorder()
	limited.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy request 1 expected 200, got %d", rec.Code)
	}

	// Second request from same proxy IP is rejected.
	req = httptest.NewRequest("GET", "/students", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	rec = httptest.NewRecorder()
	limited.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("proxy request 2 expected 429, got %d", rec.Code)
	}
}

func TestRateLimitConcurrent(t *testing.T) {
	defer withEnv("RATE_LIMIT_SKIP_PATHS", "", t)()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limited := RateLimit(next, 600, false) // 10/sec, burst 60

	done := make(chan int, 50)
	for i := 0; i < 50; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/students", nil)
			rec := httptest.NewRecorder()
			limited.ServeHTTP(rec, req)
			done <- rec.Code
		}()
	}
	time.Sleep(300 * time.Millisecond)

	okCount := 0
	for i := 0; i < 50; i++ {
		if <-done == http.StatusOK {
			okCount++
		}
	}
	if okCount == 0 {
		t.Fatal("expected at least some requests to pass under concurrency")
	}
}
