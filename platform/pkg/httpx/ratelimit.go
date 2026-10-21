// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimitResult es el veredicto de un limitador para una clave dada.
type RateLimitResult struct {
	// Allowed indica si la petición puede proceder.
	Allowed bool
	// Remaining son los tokens restantes tras esta petición (informativo).
	Remaining int
	// RetryAfter sugiere el tiempo hasta recuperar un token (para Retry-After).
	RetryAfter time.Duration
	// Limit es la capacidad del bucket (para X-RateLimit-Limit).
	Limit int
}

// RateLimiter decide si una clave puede consumir un token. Implementaciones:
// TokenBucket (memoria) para un proceso, o una versión sobre Postgres para
// límites compartidos entre réplicas (ver pkg/pg.RateLimiter).
type RateLimiter interface {
	Allow(ctx context.Context, key string) (RateLimitResult, error)
}

// RateLimitConfig configura el middleware de rate-limiting.
type RateLimitConfig struct {
	// Limiter es el backend (obligatorio).
	Limiter RateLimiter
	// KeyFunc deriva la clave de límite de la petición (por defecto IP del cliente).
	KeyFunc func(*http.Request) string
	// FailOpen: si el backend falla, ¿se deja pasar (true) o se rechaza (false)?
	// Por seguridad (anti-fuerza-bruta) el defecto es false (fail-closed).
	FailOpen bool
}

// RateLimit es un middleware token-bucket. Añade cabeceras X-RateLimit-* y
// Retry-After, y responde 429 cuando se supera el límite.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	keyFn := cfg.KeyFunc
	if keyFn == nil {
		keyFn = clientIP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.Limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			res, err := cfg.Limiter.Allow(r.Context(), keyFn(r))
			if err != nil {
				log := logFrom(r)
				log.Warn("rate limiter error", "error", err)
				if !cfg.FailOpen {
					Fail(w, r, ErrUnavailable.WithMessage("Limitador no disponible."))
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if res.Limit > 0 {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
				w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max0(res.Remaining)))
			}
			if !res.Allowed {
				if res.RetryAfter > 0 {
					secs := int(res.RetryAfter.Seconds())
					if secs < 1 {
						secs = 1
					}
					w.Header().Set("Retry-After", strconv.Itoa(secs))
				}
				Fail(w, r, ErrTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// TokenBucket es un limitador token-bucket en memoria, apto para un único
// proceso o como capa L1 delante de un limitador compartido. Recarga a Rate
// tokens por segundo hasta Burst.
type TokenBucket struct {
	rate    float64 // tokens por segundo
	burst   float64
	mu      sync.Mutex
	buckets map[string]*bucketState
	now     func() time.Time
	lastGC  time.Time
	gcEvery time.Duration
	idleTTL time.Duration
}

type bucketState struct {
	tokens float64
	last   time.Time
}

// NewTokenBucket crea un limitador que permite `perSecond` peticiones sostenidas
// con ráfagas de hasta `burst`.
func NewTokenBucket(perSecond float64, burst int) *TokenBucket {
	if perSecond <= 0 {
		perSecond = 1
	}
	if burst <= 0 {
		burst = 1
	}
	return &TokenBucket{
		rate:    perSecond,
		burst:   float64(burst),
		buckets: make(map[string]*bucketState),
		now:     time.Now,
		gcEvery: time.Minute,
		idleTTL: 10 * time.Minute,
	}
}

// Allow implementa RateLimiter.
func (tb *TokenBucket) Allow(_ context.Context, key string) (RateLimitResult, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := tb.now()
	tb.gc(now)

	b, ok := tb.buckets[key]
	if !ok {
		b = &bucketState{tokens: tb.burst, last: now}
		tb.buckets[key] = b
	}
	// Recargar según el tiempo transcurrido.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = minf(tb.burst, b.tokens+elapsed*tb.rate)
		b.last = now
	}

	res := RateLimitResult{Limit: int(tb.burst)}
	if b.tokens >= 1 {
		b.tokens--
		res.Allowed = true
		res.Remaining = int(b.tokens)
		return res, nil
	}
	// Sin tokens: calcular espera hasta el próximo token.
	needed := 1 - b.tokens
	res.RetryAfter = time.Duration(needed/tb.rate*float64(time.Second)) + time.Millisecond
	res.Remaining = 0
	return res, nil
}

// gc purga buckets ociosos para acotar memoria (llamado bajo lock).
func (tb *TokenBucket) gc(now time.Time) {
	if now.Sub(tb.lastGC) < tb.gcEvery {
		return
	}
	tb.lastGC = now
	for k, b := range tb.buckets {
		if now.Sub(b.last) > tb.idleTTL {
			delete(tb.buckets, k)
		}
	}
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
