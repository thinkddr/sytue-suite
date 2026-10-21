// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"net/http"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// bearerToken extrae el token Bearer de la cabecera Authorization. Como
// excepción, en peticiones SSE (Accept: text/event-stream) y en handshakes
// WebSocket acepta el token en ?access_token= porque ni EventSource ni la API
// WebSocket del navegador admiten cabeceras. Se acota a esos dos casos para
// minimizar la exposición del token en URLs/logs; el TTL corto lo mitiga.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h != "" {
		const prefix = "Bearer "
		if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):]), true
		}
		return "", false
	}
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") || isWebSocketUpgrade(r) {
		if t := strings.TrimSpace(r.URL.Query().Get("access_token")); t != "" {
			return t, true
		}
	}
	return "", false
}

// isWebSocketUpgrade detecta un handshake WebSocket (Connection: Upgrade +
// Upgrade: websocket); en él el navegador no puede fijar la cabecera Authorization.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// RequireAuth es un middleware que exige un JWT válido de Sytue ID. Inyecta los
// claims y, si el token trae tenant válido, también el tenant en el contexto.
func (v *Verifier) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Falta el token Bearer."))
			return
		}
		claims, err := v.Verify(r.Context(), raw)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Token inválido o expirado.").Wrap(err))
			return
		}

		ctx := Into(r.Context(), claims)
		if t, ok := claims.TenantOrEmpty(); ok {
			ctx = tenancy.Into(ctx, t)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireGoogleAPIs protege las rutas /compat/google de un producto. Acepta los
// tokens de siempre y, además, los que Sytue ID emite a una app de terceros con
// scopes de Google (audiencia GoogleAPIsAudience). A esos últimos les exige el scope
// de la operación: uno de `read` o de `write` para GET y HEAD, y uno de `write` para
// lo demás. Un token de la suite pasa como con RequireAuth: sus permisos son los de
// la persona, no los de una app.
func (v *Verifier) RequireGoogleAPIs(read, write []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Falta el token Bearer."))
				return
			}
			claims, err := v.verify(r.Context(), raw, GoogleAPIsAudience)
			if err != nil {
				httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Token inválido o expirado.").Wrap(err))
				return
			}
			propias := v.audiences()
			if len(propias) > 0 && !anyAudienceMatches(claims.Audience, propias) {
				permitidos := write
				if r.Method == http.MethodGet || r.Method == http.MethodHead {
					permitidos = append(slices.Clone(read), write...)
				}
				if !slices.ContainsFunc(permitidos, claims.HasScope) {
					httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Request had insufficient authentication scopes."))
					return
				}
			}
			ctx := Into(r.Context(), claims)
			if t, ok := claims.TenantOrEmpty(); ok {
				ctx = tenancy.Into(ctx, t)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole exige que el usuario autenticado tenga al menos uno de los roles.
// Debe componerse DESPUÉS de RequireAuth.
func (v *Verifier) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			if !claims.HasAnyRole(roles...) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Rol insuficiente para esta acción."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireTenant exige que el usuario autenticado tenga un tenant válido activo.
// Debe componerse DESPUÉS de RequireAuth. Garantiza que las rutas multi-tenant
// nunca se ejecuten sin tenant en el contexto (necesario para RLS).
// RequireOrgKind deja pasar solo a las organizaciones de la clase indicada. Es la
// verja de los productos que una cuenta personal no tiene —Sytue Admin, SyERP—,
// y NO puede sustituirse por RequireRole: la persona es `owner` de su propia
// organización personal, así que por rol entraría en todo.
//
// Un token SIN el claim cuenta como "org": es anterior a la distinción, y lo que
// había antes eran organizaciones. Así un despliegue a medias no deja a nadie
// fuera de lo que ya usaba.
func (v *Verifier) RequireOrgKind(kinds ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			kind := claims.OrgKind
			if kind == "" {
				kind = "org"
			}
			for _, want := range kinds {
				if kind == want {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Este producto no está disponible para cuentas personales."))
		})
	}
}

func (v *Verifier) RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := FromContext(r.Context())
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		// Código propio (`no_active_organization`) y no un `forbidden` a secas:
		// la causa habitual es que Sytue ID emitió el token en modo bootstrap
		// —sesión SIN organización— porque la suscripción de esa organización no
		// da derecho de uso todavía. Con el código genérico, las diez apps lo
		// mostraban como un fallo de carga y el usuario no tenía forma de saber
		// que le faltaba la suscripción.
		if _, ok := claims.TenantOrEmpty(); !ok {
			httpx.Fail(w, r, httpx.ErrNoActiveOrg)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope exige que el token incluya el scope dado. Tras RequireAuth.
func (v *Verifier) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			if !claims.HasScope(scope) {
				httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Scope insuficiente."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Token es el token con el que ha llegado la petición (el mismo que validó
// RequireAuth), para hablar en nombre de quien llama con otro servicio de la suite.
// Vacío si no hay.
func Token(r *http.Request) string {
	t, _ := bearerToken(r)
	return t
}
