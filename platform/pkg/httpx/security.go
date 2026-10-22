// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// logFrom devuelve el logger del contexto de la petición.
func logFrom(r *http.Request) *slog.Logger { return log.From(r.Context()) }

// SecurityHeadersConfig configura las cabeceras de seguridad por defecto que
// aplica todo servicio Sytue One (deny-by-default, defensa en profundidad).
//
// Los valores por defecto (DefaultSecurityHeaders) son estrictos y adecuados
// para superficies de API autenticadas. Las superficies públicas (www, landing)
// ajustan CSP/Referrer según necesidad, siempre partiendo de esta base.
type SecurityHeadersConfig struct {
	// ContentSecurityPolicy es la CSP completa. Vacía => no se emite (útil para
	// APIs JSON puras que ya envían nosniff; para superficies HTML usar una CSP
	// estricta como DefaultCSP).
	ContentSecurityPolicy string
	// HSTS activa Strict-Transport-Security. Solo debe activarse tras TLS 1.3.
	HSTS bool
	// HSTSMaxAge es el max-age de HSTS (por defecto 2 años).
	HSTSMaxAge time.Duration
	// HSTSIncludeSubdomains añade includeSubDomains.
	HSTSIncludeSubdomains bool
	// HSTSPreload añade preload (solo con includeSubDomains y max-age >= 1 año).
	HSTSPreload bool
	// FrameOptions controla X-Frame-Options (por defecto DENY). Cadena vacía la omite.
	FrameOptions string
	// ReferrerPolicy controla Referrer-Policy (por defecto no-referrer).
	ReferrerPolicy string
	// PermissionsPolicy controla Permissions-Policy (por defecto bloquea todo).
	PermissionsPolicy string
	// CrossOriginOpenerPolicy (por defecto same-origin).
	CrossOriginOpenerPolicy string
	// CrossOriginResourcePolicy (por defecto same-origin).
	CrossOriginResourcePolicy string
	// CrossOriginEmbedderPolicy (por defecto require-corp). Vacía la omite.
	CrossOriginEmbedderPolicy string
}

// DefaultCSP es una CSP estricta apta para superficies HTML de la suite: sin
// inline scripts, sin objetos, sin ancestros (equivalente a X-Frame-Options DENY).
const DefaultCSP = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; " +
	"frame-ancestors 'none'; upgrade-insecure-requests"

// DefaultSecurityHeaders devuelve la configuración endurecida por defecto.
// deny-by-default: sin permisos de navegador, sin embebido cross-origin, HSTS on.
func DefaultSecurityHeaders() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		HSTS:                      true,
		HSTSMaxAge:                2 * 365 * 24 * time.Hour,
		HSTSIncludeSubdomains:     true,
		HSTSPreload:               true,
		FrameOptions:              "DENY",
		ReferrerPolicy:            "no-referrer",
		PermissionsPolicy:         DefaultPermissionsPolicy,
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		CrossOriginEmbedderPolicy: "require-corp",
	}
}

// DefaultPermissionsPolicy deniega por defecto las APIs sensibles del navegador.
const DefaultPermissionsPolicy = "accelerometer=(), autoplay=(), camera=(), " +
	"clipboard-read=(), clipboard-write=(self), display-capture=(), " +
	"encrypted-media=(), fullscreen=(self), geolocation=(), gyroscope=(), " +
	"magnetometer=(), microphone=(), midi=(), payment=(), usb=()"

// SecurityHeaders es un middleware que fija cabeceras de seguridad en cada
// respuesta antes de invocar al siguiente handler.
//
//nolint:gocognit // tabla de cabeceras con variantes por config; partirla dispersaría la política de seguridad.
func SecurityHeaders(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	// Precalcular el valor de HSTS una sola vez.
	var hstsValue string
	if cfg.HSTS {
		maxAge := cfg.HSTSMaxAge
		if maxAge <= 0 {
			maxAge = 2 * 365 * 24 * time.Hour
		}
		var b strings.Builder
		b.WriteString("max-age=")
		b.WriteString(strconv.FormatInt(int64(maxAge.Seconds()), 10))
		if cfg.HSTSIncludeSubdomains {
			b.WriteString("; includeSubDomains")
		}
		if cfg.HSTSPreload && cfg.HSTSIncludeSubdomains {
			b.WriteString("; preload")
		}
		hstsValue = b.String()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			if cfg.ContentSecurityPolicy != "" {
				h.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
			}
			if cfg.FrameOptions != "" {
				h.Set("X-Frame-Options", cfg.FrameOptions)
			}
			if cfg.ReferrerPolicy != "" {
				h.Set("Referrer-Policy", cfg.ReferrerPolicy)
			}
			if cfg.PermissionsPolicy != "" {
				h.Set("Permissions-Policy", cfg.PermissionsPolicy)
			}
			if cfg.CrossOriginOpenerPolicy != "" {
				h.Set("Cross-Origin-Opener-Policy", cfg.CrossOriginOpenerPolicy)
			}
			if cfg.CrossOriginResourcePolicy != "" {
				h.Set("Cross-Origin-Resource-Policy", cfg.CrossOriginResourcePolicy)
			}
			if cfg.CrossOriginEmbedderPolicy != "" {
				h.Set("Cross-Origin-Embedder-Policy", cfg.CrossOriginEmbedderPolicy)
			}
			// Enviar HSTS solo sobre conexiones cifradas (evita cabecera engañosa en dev http).
			if hstsValue != "" && isTLS(r) {
				h.Set("Strict-Transport-Security", hstsValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isTLS indica si la petición llegó por TLS, directamente o vía proxy de confianza.
func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// randomToken genera un token aleatorio url-safe de n bytes de entropía.
func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
