package middleware

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// client holds one token bucket per client key.
type client struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

// rateLimiter is a per-client token bucket. Safe for concurrent use.
type rateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*client
	rate       float64 // tokens per second
	burst      float64 // max tokens (full bucket)
	trustProxy bool
	skip       map[string]bool
}

// newRateLimiter builds a limiter that refills `reqPerMinute` tokens per
// minute, bursting up to one full minute's worth.
func newRateLimiter(reqPerMinute int, trustProxy bool) *rateLimiter {
	rate := float64(reqPerMinute) / 60.0
	if rate <= 0 {
		rate = 1
	}
	rl := &rateLimiter{
		clients:    make(map[string]*client),
		rate:       rate,
		burst:      rate * 60,
		trustProxy: trustProxy,
		skip:       make(map[string]bool),
	}
	for _, p := range strings.Split(os.Getenv("RATE_LIMIT_SKIP_PATHS"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			rl.skip[p] = true
		}
	}
	return rl
}

// cleanup evicts clients idle for more than 5 minutes so memory stays flat.
func (rl *rateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		now := time.Now()
		for k, c := range rl.clients {
			if now.Sub(c.lastSeen) > 5*time.Minute {
				delete(rl.clients, k)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks whether the request from key should be permitted.
func (rl *rateLimiter) allow(key string) (allowed bool, remaining int, reset time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	c, ok := rl.clients[key]
	if !ok {
		c = &client{tokens: rl.burst, last: now}
		rl.clients[key] = c
	}

	// Refill tokens based on elapsed time.
	c.tokens += now.Sub(c.last).Seconds() * rl.rate
	if c.tokens > rl.burst {
		c.tokens = rl.burst
	}
	c.last = now
	c.lastSeen = now

	reset = now.Add(time.Duration(float64(time.Second) / rl.rate))
	if c.tokens >= 1 {
		c.tokens--
		allowed = true
		remaining = int(c.tokens)
	} else {
		allowed = false
		remaining = 0
	}
	return allowed, remaining, reset
}

// rateLimitKey returns the client key for rate limiting.
func rateLimitKey(r *http.Request, trustProxy bool) string {
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx > 0 {
		ip = ip[:idx]
	}
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if parts := strings.Split(xff, ","); len(parts) > 0 {
				ip = strings.TrimSpace(parts[0])
			}
		}
	}
	return ip
}

// RateLimit wraps next with a per-client token bucket. Rejected requests get
// 429 with Retry-After and X-RateLimit-* headers plus a JSON error body.
// Never blocks the calling handler; check-and-consume is O(1).
func RateLimit(next http.Handler, reqPerMinute int, trustProxy bool) http.Handler {
	rl := newRateLimiter(reqPerMinute, trustProxy)
	go rl.cleanup()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rl.skip[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		key := rateLimitKey(r, trustProxy)
		allowed, remaining, reset := rl.allow(key)

		w.Header().Set("X-RateLimit-Limit", itoa(int(rl.burst)))
		w.Header().Set("X-RateLimit-Remaining", itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", itoa(int(reset.Unix())))

		if !allowed {
			w.Header().Set("Retry-After", itoa(int(time.Until(reset).Seconds()+1)))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// itoa is a tiny integer-to-string helper to avoid importing strconv for one call.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
