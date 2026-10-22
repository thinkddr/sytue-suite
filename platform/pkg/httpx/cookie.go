// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"time"
)

// CookieOptions describe una cookie segura por defecto de Sytue One.
// Los defaults son los más protectores: HttpOnly, Secure y SameSite=Strict.
type CookieOptions struct {
	Name     string
	Value    string
	Path     string
	Domain   string
	MaxAge   time.Duration
	HTTPOnly *bool
	Secure   *bool
	SameSite http.SameSite
}

// SetSecureCookie escribe una cookie con los atributos de seguridad por defecto
// (HttpOnly, Secure, SameSite=Strict, Path=/). Los punteros permiten sobrescribir
// solo los atributos que un caso concreto necesita relajar (con justificación).
func SetSecureCookie(w http.ResponseWriter, opts CookieOptions) {
	path := opts.Path
	if path == "" {
		path = "/"
	}
	httpOnly := true
	if opts.HTTPOnly != nil {
		httpOnly = *opts.HTTPOnly
	}
	secure := true
	if opts.Secure != nil {
		secure = *opts.Secure
	}
	sameSite := opts.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteStrictMode
	}

	c := &http.Cookie{ //nolint:gosec // G124: Secure/HttpOnly/SameSite son secure-by-default; los punteros solo permiten relajarlos con justificación explícita del llamante.
		Name:     opts.Name,
		Value:    opts.Value,
		Path:     path,
		Domain:   opts.Domain,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: sameSite,
	}
	if opts.MaxAge > 0 {
		c.MaxAge = int(opts.MaxAge.Seconds())
		c.Expires = time.Now().Add(opts.MaxAge)
	}
	http.SetCookie(w, c)
}

// ClearCookie borra una cookie fijando MaxAge negativo, conservando los atributos
// de seguridad (necesario para que el navegador la elimine correctamente).
func ClearCookie(w http.ResponseWriter, name, path, domain string) {
	if path == "" {
		path = "/"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		Domain:   domain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}
