// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenBucketBurstAndRefill(t *testing.T) {
	now := time.Now()
	tb := NewTokenBucket(1, 2) // 1/s, burst 2
	tb.now = func() time.Time { return now }

	// Dos peticiones permitidas (burst), la tercera no.
	for i := 0; i < 2; i++ {
		res, _ := tb.Allow(context.Background(), "k")
		if !res.Allowed {
			t.Fatalf("petición %d debería permitirse", i)
		}
	}
	res, _ := tb.Allow(context.Background(), "k")
	if res.Allowed {
		t.Fatal("tercera petición debe rechazarse (sin tokens)")
	}
	if res.RetryAfter <= 0 {
		t.Error("debe sugerir Retry-After")
	}

	// Tras 1s se recupera 1 token.
	now = now.Add(time.Second)
	res, _ = tb.Allow(context.Background(), "k")
	if !res.Allowed {
		t.Fatal("tras recarga debe permitirse")
	}
}

func TestTokenBucketPerKey(t *testing.T) {
	tb := NewTokenBucket(1, 1)
	if r, _ := tb.Allow(context.Background(), "a"); !r.Allowed {
		t.Fatal("clave a")
	}
	if r, _ := tb.Allow(context.Background(), "b"); !r.Allowed {
		t.Fatal("clave b independiente")
	}
}

type errLimiter struct{}

func (errLimiter) Allow(context.Context, string) (RateLimitResult, error) {
	return RateLimitResult{}, errors.New("backend caído")
}

func TestRateLimitMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RateLimit(RateLimitConfig{Limiter: NewTokenBucket(1, 1)})(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("primera petición status=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda petición debe ser 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("falta Retry-After en 429")
	}
}

func TestRateLimitFailClosed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RateLimit(RateLimitConfig{Limiter: errLimiter{}})(next) // FailOpen=false por defecto
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("fail-closed debe dar 503, got %d", rec.Code)
	}
}
