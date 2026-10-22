// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Contraseñas de aplicación por API JSON (bearer). Existían solo como formulario
// en `/account`, así que eran la ÚNICA parte de la cuenta que no se podía
// gestionar sin renderizar ese HTML —y es la credencial con la que entra un
// cliente de correo, o sea justo la que más falta hace configurar desde donde
// esté cada cual.
//
// El dominio no cambia: mismo generador, mismo hash y mismo store que el
// formulario. Aquí solo está la cara JSON.
package http

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

type createAppPasswordRequest struct {
	Label string `json:"label"`
}

// handleMeAppPasswords — GET /api/v1/me/app-passwords — las vivas del usuario del
// token. Solo metadatos: el secreto no se guarda, únicamente su hash.
func (s *Server) handleMeAppPasswords(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	list, err := s.store.ListAppPasswords(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"app_passwords": list})
}

// handleMeAppPasswordCreate — POST /api/v1/me/app-passwords — crea una y la
// devuelve EN CLARO, una sola vez.
//
// Es la única respuesta de esta API que contiene un secreto, y por eso no hay
// forma de volver a consultarlo: si se pierde, se revoca y se crea otra. Mismo
// trato que los códigos de recuperación del segundo factor.
func (s *Server) handleMeAppPasswordCreate(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createAppPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	etiqueta := strings.TrimSpace(req.Label)
	if len(etiqueta) > 60 {
		etiqueta = etiqueta[:60]
	}
	claro, err := generarAppPassword()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	fila, err := s.store.CreateAppPassword(r.Context(), c.Subject,
		domain.HashToken(NormalizeAppPassword(claro)), etiqueta)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "app_password.created", fila.ID, r))
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"id":         fila.ID,
		"label":      fila.Label,
		"created_at": fila.CreatedAt,
		// La única vez que viaja. El cliente la enseña y no vuelve a pedirla.
		"password": claro,
	})
}

// handleMeAppPasswordRevoke — DELETE /api/v1/me/app-passwords/{id}.
// La autorización va en el WHERE del store (user_id): un id ajeno no casa.
func (s *Server) handleMeAppPasswordRevoke(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if !uuidRe.MatchString(id) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	revocada, err := s.store.RevokeAppPassword(r.Context(), c.Subject, id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !revocada {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "app_password.revoked", id, r))
	w.WriteHeader(http.StatusNoContent)
}

// handleMeAppleProfile — POST /api/v1/me/apple-profile — el perfil de Apple de las
// apps nativas, en UN paso: crea una contraseña de aplicación para este equipo y la
// mete en las tres cuentas (correo, calendario y contactos), así que el sistema las
// instala sin pedir nada. La contraseña no viaja de otra forma ni se puede volver a
// leer: se revoca en Mi cuenta como cualquier otra, con su etiqueta.
func (s *Server) handleMeAppleProfile(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createAppPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	perfil, err := s.perfilConClave(r, c.Subject, c.Tenant, req.Label)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	servirPerfil(w, http.StatusCreated, perfil)
}

// PlazoEnlacePerfil es cuánto vale el enlace del iPhone: lo que tarda Safari en abrirse.
const PlazoEnlacePerfil = 10 * time.Minute

// handleMeAppleProfileLink — POST /api/v1/me/apple-profile/link — el enlace de un solo
// uso que la app del iPhone abre en Safari: iOS solo instala perfiles desde ahí, y
// Safari no tiene la sesión de la app. El perfil (y su contraseña) se crea al canjearlo.
func (s *Server) handleMeAppleProfileLink(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createAppPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	crudo := make([]byte, 32)
	if _, err := rand.Read(crudo); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	token := base64.RawURLEncoding.EncodeToString(crudo)
	caduca := time.Now().Add(PlazoEnlacePerfil)
	if err := s.store.CreateAppleProfileLink(r.Context(), domain.HashToken(token), c.Subject, c.Tenant, etiquetaPerfil(req.Label), caduca); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"url":        strings.TrimRight(s.cfg.Issuer, "/") + "/account/apple-profile/" + token,
		"expires_at": caduca.UTC(),
	})
}

// handleAppleProfileLink — GET /account/apple-profile/{token} — canjea el enlace: UNA
// vez y antes de que caduque. El token es la única credencial (Safari no trae sesión),
// por eso el canje es atómico y lo usado no se reabre.
func (s *Server) handleAppleProfileLink(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		httpFailHTML(w, r, http.StatusNotFound, "Este enlace no existe.")
		return
	}
	userID, tenant, etiqueta, err := s.store.ConsumeAppleProfileLink(r.Context(), domain.HashToken(token))
	if err != nil {
		httpFailHTML(w, r, http.StatusGone, "Este enlace ya se ha usado o ha caducado. Vuelve a pulsar el botón en la app.")
		return
	}
	perfil, err := s.perfilConClave(r, userID, tenant, etiqueta)
	if err != nil {
		s.log.Error("perfil .mobileconfig por enlace", "error", err)
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo generar el perfil de configuración.")
		return
	}
	servirPerfil(w, http.StatusOK, perfil)
}

// etiquetaPerfil es la de la contraseña de aplicación del perfil, acotada como las demás.
func etiquetaPerfil(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		v = "Perfil de Apple"
	}
	if len(v) > 60 {
		v = v[:60]
	}
	return v
}

// perfilConClave crea una contraseña de aplicación para `userID` y genera el perfil con
// ella dentro. Primero el perfil: si no se puede generar, no queda detrás una credencial
// viva que nadie va a usar.
func (s *Server) perfilConClave(r *http.Request, userID, tenant, etiqueta string) ([]byte, error) {
	user, err := s.store.GetUserByID(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	claro, err := generarAppPassword()
	if err != nil {
		return nil, err
	}
	orgID := ""
	if org, err := s.store.GetOrgBySlug(r.Context(), tenant); err == nil {
		orgID = org.ID
	}
	perfil, err := buildAppleProfile(s.cfg, user, s.anclaSMIMEDeLaOrg(r, orgID), claro)
	if err != nil {
		return nil, err
	}
	fila, err := s.store.CreateAppPassword(r.Context(), userID,
		domain.HashToken(NormalizeAppPassword(claro)), etiquetaPerfil(etiqueta))
	if err != nil {
		return nil, err
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(userID, tenant, "app_password.created", fila.ID, r))
	return perfil, nil
}

// servirPerfil manda el perfil como descarga, sin caché.
func servirPerfil(w http.ResponseWriter, estado int, perfil []byte) {
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="sytue.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(estado)
	_, _ = w.Write(perfil)
}
