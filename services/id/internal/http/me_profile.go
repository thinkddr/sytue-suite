// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// updateProfileRequest — PUNTEROS, no cadenas. Es un PATCH: hay que distinguir
// «no viene» de «viene vacío». Con `string` a secas, un cliente que solo manda el
// nombre borraría el puesto y el departamento de quien los tuviera puestos, y con
// punteros el campo ausente conserva lo que hay y `""` lo vacía a propósito.
type updateProfileRequest struct {
	Name       *string `json:"name"`
	GivenName  *string `json:"given_name"`
	FamilyName *string `json:"family_name"`
	Title      *string `json:"title"`
	Department *string `json:"department"`
	Location   *string `json:"location"`
	Locale     *string `json:"locale"`
}

// valor resuelve un campo parcheable: el nuevo si viene, el actual si no.
func valor(nuevo *string, actual string) string {
	if nuevo == nil {
		return actual
	}
	return *nuevo
}

// handleMeUpdate — PATCH /api/v1/me — edición de perfil self-service: los MISMOS
// siete campos que el formulario de /account (nombre visible, nombre de pila,
// apellidos, puesto, departamento, ubicación e idioma). Aceptaba solo `name`, así
// que la pantalla nueva no podía ofrecer lo que la vieja sí, y quien entrara por
// ahí perdía seis campos de su ficha.
//
// El email verificado, el rol y el tenant quedan FUERA y no por olvido: cambiar el
// email es otro flujo (con verificación) y rol/tenant los gobierna la
// administración de la organización. `SetProfile` no menciona esas columnas en su
// SQL, así que este camino no puede tocarlas aunque el cuerpo traiga campos extra.
//
// ponytail: lectura y escritura sin transacción. Dos PATCH a la vez sobre el
// MISMO usuario podrían pisarse un campo; es su propia ficha desde su propia
// sesión, así que la carrera exige dos pestañas guardando a la vez y lo que se
// pierde es un campo de texto. Si algún día esto lo escribe también SCIM, va en
// una tx.
func (s *Server) handleMeUpdate(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req updateProfileRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// EL NOMBRE SE VALIDA ANTES DE LEER NADA. Es lo que se enseña de una persona en
	// toda la suite, y una ficha sin él deja filas en blanco por todas partes. Va
	// primero para rechazar sin tocar el store, que es lo que comprueba
	// `TestMeUpdateRejectsBlankName` —y lo que lo hace barato frente a un cliente
	// que insista—. Si `name` no viene, no hay nada que validar: el actual se
	// conserva y ya cumplía.
	if req.Name != nil && domain.SanitizeDisplayName(*req.Name) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The name cannot be empty."))
		return
	}
	// Se parte del perfil ACTUAL para que los campos ausentes sobrevivan:
	// `SetProfile` escribe las siete columnas de una vez.
	actual, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	edit := domain.ProfileEdit{
		Name:       valor(req.Name, actual.Name),
		GivenName:  valor(req.GivenName, actual.GivenName),
		FamilyName: valor(req.FamilyName, actual.FamilyName),
		Title:      valor(req.Title, actual.Title),
		Department: valor(req.Department, actual.Department),
		Location:   valor(req.Location, actual.Location),
		Locale:     valor(req.Locale, actual.Locale),
	}
	user, err := s.store.SetProfile(r.Context(), nil, c.Subject, edit)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.profile.updated", "", r))
	out := meProfileWire(user)
	setAvatarFields(r.Context(), s, user.ID, out)
	httpx.JSON(w, r, http.StatusOK, out)
}

// setAvatarFields añade has_avatar/avatar_version a la forma JSON del perfil.
// Aparte de meProfileWire porque necesita el store (una consulta barata: solo
// updated_at, no el data URI) y esa función no lo tiene a mano.
func setAvatarFields(ctx context.Context, s *Server, userID string, out map[string]any) {
	has, updatedAt, err := s.store.HasAvatar(ctx, userID)
	if err != nil {
		return // el perfil no se rompe por esto; el cliente ve has_avatar=false
	}
	out["has_avatar"] = has
	if has {
		out["avatar_version"] = versionAvatar(updatedAt)
	}
}

// meProfileWire es la forma JSON del perfil propio. La comparten `PATCH /me` y
// `GET /me` para que la pantalla no tenga que releer tras guardar ni reconciliar
// dos formas distintas del mismo usuario.
func meProfileWire(u domain.User) map[string]any {
	return map[string]any{
		"id":             u.ID,
		"email":          u.Email,
		"username":       u.Username,
		"email_verified": u.EmailVerified,
		"phone":          u.Phone,
		"recovery_email": u.RecoveryEmail,
		"name":           u.Name,
		"given_name":     u.GivenName,
		"family_name":    u.FamilyName,
		"title":          u.Title,
		"department":     u.Department,
		"location":       u.Location,
		"locale":         u.Locale,
		"mfa_enabled":    u.MFAEnabled,
	}
}
