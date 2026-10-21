// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/http/pprof"
	"runtime"
)

// errNoGuard se devuelve si se intenta montar pprof sin autorización.
var errNoGuard = errors.New("telemetry: RegisterPprof requiere un Guard de autorización (deny-by-default)")

// PprofConfig configura el endpoint de profiling protegido.
type PprofConfig struct {
	// PathPrefix bajo el que se montan los handlers (por defecto /debug/pprof).
	PathPrefix string
	// Guard es un middleware de autorización OBLIGATORIO (p. ej. RequireRole
	// admin del paquete authn, o una allowlist de red). deny-by-default: sin
	// Guard, RegisterPprof NO monta nada y devuelve error.
	Guard func(http.Handler) http.Handler
}

// Muxer es la superficie mínima para registrar rutas (compatible con chi y ServeMux).
type Muxer interface {
	Handle(pattern string, handler http.Handler)
}

// RegisterPprof monta los handlers net/http/pprof detrás del Guard de
// autorización. NUNCA deben exponerse sin protección: revelan memoria, goroutines
// y pueden inducir carga. Devuelve error si falta el Guard (deny-by-default).
func RegisterPprof(mux Muxer, cfg PprofConfig) error {
	if cfg.Guard == nil {
		return errNoGuard
	}
	prefix := cfg.PathPrefix
	if prefix == "" {
		prefix = "/debug/pprof"
	}

	index := http.HandlerFunc(pprof.Index)
	handlers := map[string]http.Handler{
		prefix + "/":        cfg.Guard(index),
		prefix + "/cmdline": cfg.Guard(http.HandlerFunc(pprof.Cmdline)),
		prefix + "/profile": cfg.Guard(http.HandlerFunc(pprof.Profile)),
		prefix + "/symbol":  cfg.Guard(http.HandlerFunc(pprof.Symbol)),
		prefix + "/trace":   cfg.Guard(http.HandlerFunc(pprof.Trace)),
	}
	for pattern, h := range handlers {
		mux.Handle(pattern, h)
	}
	return nil
}

// StaticTokenGuard es un Guard sencillo basado en un token de cabecera comparado
// en tiempo constante (para entornos donde no aplica el authn OIDC, p. ej. una
// red de administración). Prefiérase el guard de authn cuando exista sesión.
func StaticTokenGuard(header, token string) func(http.Handler) http.Handler {
	if header == "" {
		header = "X-Debug-Token"
	}
	want := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get(header))
			if len(want) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetBlockAndMutexProfiling activa el sampling de bloqueos y contención para que
// los perfiles de pprof sean útiles (desactivado por defecto en Go). Rates altos
// tienen coste: usar valores conservadores en producción.
func SetBlockAndMutexProfiling(blockRate, mutexFraction int) {
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
	}
	if mutexFraction > 0 {
		runtime.SetMutexProfileFraction(mutexFraction)
	}
}
