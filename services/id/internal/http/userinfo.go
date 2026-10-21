// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"slices"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

// handleUserinfo implementa el userinfo endpoint (OIDC Core §5.3). Valida el
// access token (Bearer) y devuelve los claims del usuario según los scopes.
func (s *Server) handleUserinfo(w http.ResponseWriter, r *http.Request) {
	raw, ok := bearer(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Missing access token."))
		return
	}
	set, err := s.keys.PublicJWKS()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	tok, err := oidc.Verify(raw, s.cfg.Issuer, s.cfg.ResourceAudience, set)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Invalid token."))
		return
	}

	user, err := s.store.GetUserByID(r.Context(), tok.Subject())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Sujeto desconocido."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	// La misma `picture` que el id_token (oidc.PictureURL). Best-effort, como allí:
	// si la consulta falla se omite y el cliente pinta las iniciales.
	picture := ""
	if tiene, ver, err := s.store.HasAvatar(r.Context(), user.ID); err == nil && tiene {
		picture = oidc.PictureURL(s.cfg.Issuer, versionAvatar(ver))
	}
	out := userinfoClaims(user, claimStrings(tok, "scope"), picture)
	if v, ok := tok.Get("tenant"); ok {
		out["tenant"] = v
	}
	if v, ok := tok.Get("roles"); ok {
		out["roles"] = v
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// userinfoClaims construye los claims del userinfo (OIDC Core §5.3.2/§5.4) para los
// scopes CONCEDIDOS. Un claim que el modelo de usuario no tiene se OMITE (no se
// devuelve vacío): una librería cliente que mapea `name` a la ficha del usuario
// preferirá no verlo a pintar una cadena vacía. `picture` es la URL ya compuesta, o
// "" si la persona no tiene foto.
func userinfoClaims(u domain.User, scopes []string, picture string) map[string]any {
	out := map[string]any{"sub": u.ID}
	set := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	if slices.Contains(scopes, "email") {
		out["email"] = u.Email
		out["email_verified"] = u.EmailVerified
	}
	if slices.Contains(scopes, "profile") {
		set("name", u.Name)
		set("given_name", u.GivenName)
		set("family_name", u.FamilyName)
		set("locale", u.Locale)
		// preferred_username: el identificador con el que se inicia sesión en Sytue ES
		// el email (no hay campo username en el modelo). Muchas apps lo usan como login
		// local; devolver el email es el dato real, no uno inventado.
		set("preferred_username", u.Email)
		set("picture", picture)
	}
	return out
}
