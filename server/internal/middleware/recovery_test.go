package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRecoveryCatchesPanic proves a panicking handler returns 500 JSON
// instead of crashing the process, and echoes the request ID.
func TestRecoveryCatchesPanic(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest("GET", "/students/1", nil)
	ctx := context.WithValue(req.Context(), requestIDKey{}, "trace-panic")
	req = req.WithContext(ctx)
	req.Header.Set("X-Request-ID", "trace-panic")
	rec := httptest.NewRecorder()

	Recovery(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") != "trace-panic" {
		t.Fatalf("expected echoed request id, got %q", rec.Header().Get("X-Request-ID"))
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json, got %q", rec.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("500 body not valid JSON: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Fatalf("unexpected body: %v", body)
	}
}

// TestRecoveryLetsHealthyHandlerPass proves recovery doesn't interfere
// with normal requests.
func TestRecoveryLetsHealthyHandlerPass(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	req := httptest.NewRequest("GET", "/students/1", nil)
	rec := httptest.NewRecorder()

	Recovery(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
