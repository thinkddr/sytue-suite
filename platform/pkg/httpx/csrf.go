// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"crypto/subtle"
	"net/http"
	"time"
)

// Cabeceras y cookie canónicas del esquema CSRF double-submit de Sytue One.
const (
	// HeaderCSRFToken es la cabecera que el cliente debe reflejar en peticiones mutadoras.
	HeaderCSRFToken = "X-CSRF-Token" //nolint:gosec // G101: es el NOMBRE de una cabecera, no una credencial.
	// DefaultCSRFCookieName es el nombre de la cookie del token (no HttpOnly: el
	// front debe leerla para reflejarla en la cabecera — el secreto está en que
	// un origen ajeno no puede leer la cookie ni forzar la cabecera custom).
	DefaultCSRFCookieName = "sytue_csrf"
)

// CSRFConfig configura la protección CSRF double-submit.
type CSRFConfig struct {
	// CookieName (por defecto sytue_csrf).
	CookieName string
	// CookieDomain para compartir el token en `.sytue.eu`.
	CookieDomain string
	// CookiePath (por defecto "/").
	CookiePath string
	// TokenLength en bytes de entropía (por defecto 32).
	TokenLength int
	// MaxAge de la cookie de token (por defecto 12h).
	MaxAge time.Duration
}

func (c CSRFConfig) withDefaults() CSRFConfig {
	if c.CookieName == "" {
		c.CookieName = DefaultCSRFCookieName
	}
	if c.CookiePath == "" {
		c.CookiePath = "/"
	}
	if c.TokenLength <= 0 {
		c.TokenLength = 32
	}
	if c.MaxAge <= 0 {
		c.MaxAge = 12 * time.Hour
	}
	return c
}

// safeMethod indica si el método HTTP es de solo lectura (no requiere token CSRF).
func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// CSRF implementa protección double-submit: en métodos mutadores exige que la
// cabecera X-CSRF-Token coincida (comparación en tiempo constante) con la cookie
// de token. En métodos seguros, si falta la cookie, emite una nueva (para que el
// front la lea y la refleje). No requiere estado en servidor.
func CSRF(cfg CSRFConfig) func(http.Handler) http.Handler {
	cfg = cfg.withDefaults()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, _ := r.Cookie(cfg.CookieName)

			if safeMethod(r.Method) {
				// Sembrar el token si aún no existe, para que el front lo tenga disponible.
				if cookie == nil || cookie.Value == "" {
					if tok, err := randomToken(cfg.TokenLength); err == nil {
						issueCSRFCookie(w, cfg, tok)
					}
				}
				next.ServeHTTP(w, r)
				return
			}

			// Método mutador: validar double-submit.
			if cookie == nil || cookie.Value == "" {
				Fail(w, r, ErrForbidden.WithMessage("Falta el token CSRF."))
				return
			}
			header := r.Header.Get(HeaderCSRFToken)
			if header == "" {
				Fail(w, r, ErrForbidden.WithMessage("Falta la cabecera CSRF."))
				return
			}
			if subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
				Fail(w, r, ErrForbidden.WithMessage("Token CSRF inválido."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func issueCSRFCookie(w http.ResponseWriter, cfg CSRFConfig, token string) {
	// La cookie CSRF NO es HttpOnly: el front debe leerla para el double-submit.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: la cookie CSRF NO es HttpOnly a propósito (double-submit: el front debe leerla); Secure y SameSite=Strict sí.
		Name:     cfg.CookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		MaxAge:   int(cfg.MaxAge.Seconds()),
		Expires:  time.Now().Add(cfg.MaxAge),
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Add("Vary", "Cookie")
}

// IssueCSRFToken genera y siembra un token CSRF explícitamente (p. ej. tras
// login desde el BFF). Devuelve el token emitido.
func IssueCSRFToken(w http.ResponseWriter, cfg CSRFConfig) (string, error) {
	cfg = cfg.withDefaults()
	tok, err := randomToken(cfg.TokenLength)
	if err != nil {
		return "", err
	}
	issueCSRFCookie(w, cfg, tok)
	return tok, nil
}
