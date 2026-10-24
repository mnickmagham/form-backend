package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRateLimitPerIP_Demo proves requests are limited per source IP:
// IP A exhausts its bucket and gets 429, while IP B (different bucket)
// still gets 200 on the same limiter instance.
func TestRateLimitPerIP_Demo(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	// 2 requests per minute, burst 2
	limited := RateLimit(next, 2, false)

	reqFrom := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/students", nil)
		req.RemoteAddr = ip + ":54321"
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec
	}

	// IP A: first two pass (burst), third is rejected
	a1 := reqFrom("1.2.3.4")
	a2 := reqFrom("1.2.3.4")
	a3 := reqFrom("1.2.3.4")

	// IP B: separate bucket, still passes
	b1 := reqFrom("5.6.7.8")

	fmt.Println("=== Per-IP rate limit demo (2 req/min, burst 2) ===")
	fmt.Printf("IP 1.2.3.4 req1: status=%d remaining=%s limit=%s\n",
		a1.Code, a1.Header().Get("X-RateLimit-Remaining"), a1.Header().Get("X-RateLimit-Limit"))
	fmt.Printf("IP 1.2.3.4 req2: status=%d remaining=%s limit=%s\n",
		a2.Code, a2.Header().Get("X-RateLimit-Remaining"), a2.Header().Get("X-RateLimit-Limit"))
	fmt.Printf("IP 1.2.3.4 req3: status=%d retry-after=%s\n",
		a3.Code, a3.Header().Get("Retry-After"))
	fmt.Printf("IP 5.6.7.8 req1: status=%d remaining=%s limit=%s\n",
		b1.Code, b1.Header().Get("X-RateLimit-Remaining"), b1.Header().Get("X-RateLimit-Limit"))

	if a1.Code != http.StatusOK {
		t.Fatalf("IP A req1 expected 200, got %d", a1.Code)
	}
	if a2.Code != http.StatusOK {
		t.Fatalf("IP A req2 expected 200, got %d", a2.Code)
	}
	if a3.Code != http.StatusTooManyRequests {
		t.Fatalf("IP A req3 expected 429, got %d", a3.Code)
	}
	if b1.Code != http.StatusOK {
		t.Fatalf("IP B req1 expected 200 (separate bucket), got %d", b1.Code)
	}
	if a3.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}
