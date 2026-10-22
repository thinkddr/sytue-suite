// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Cambio de organización activa (multi-org real).
//
// La organización activa se fijaba al abrir la sesión y era la membresía MÁS
// ANTIGUA —`mems[0]` en login.go y en webauthn_login.go—, sin ninguna forma de
// cambiarla. Quien pertenece a dos organizaciones solo podía administrar la
// vieja: si en ella es `member`, las dieciséis páginas de la consola le
// devolvían 403 para siempre aunque fuera `owner` de la otra. Y crear una
// organización nueva no servía de nada, porque al reentrar el token seguía
// apuntando a la antigua.
package http

import (
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

type setActiveOrgRequest struct {
	OrgID string `json:"org_id"`
}

// handleSetActiveOrg — PUT /api/v1/me/active-organization — cambia a otra de MIS
// organizaciones.
//
// Las tres verjas son las mismas que aplica el login al elegirla por su cuenta
// (`createSession`), y por el mismo motivo: ser miembro, que la organización no
// esté suspendida y que tenga derecho de uso. Sin ellas, este endpoint sería la
// forma de entrar por la puerta de atrás en una organización cortada por impago
// o por incidente, que es justo lo que el estado `suspended` existe para
// impedir.
//
// El efecto NO es inmediato sobre los tokens ya emitidos —el claim `tenant` va
// firmado dentro de cada access token— y no puede serlo: revocar el token vivo
// del navegador que hace esta llamada lo dejaría a medias. Lo que sí se hace es
// cortar la vía por la que el tenant viejo sobreviviría solo: los refresh
// tokens, que llevan la organización congelada y refrescarían con ella durante
// días. Revocados, cada aplicación vuelve a pedir autorización cuando le caduque
// el access token (minutos) o al recargar, y la cookie de sesión sigue viva, así
// que eso no es un login nuevo sino una redirección invisible.
func (s *Server) handleSetActiveOrg(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req setActiveOrgRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Validar el UUID aquí y no en el store: el `$1::uuid` de PostgreSQL
	// convertiría una cadena cualquiera en un error de sintaxis SQL, o sea un 500
	// por un dato que el cliente escribió mal.
	if _, err := uuid.Parse(req.OrgID); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid organization id."))
		return
	}

	mems, err := s.store.ListOrgMembershipsForUser(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	var destino *domain.Organization
	for _, m := range mems {
		if m.OrgID == req.OrgID {
			destino = &domain.Organization{ID: m.OrgID, Slug: m.Slug, Name: m.Name, Status: m.Status}
			break
		}
	}
	// Mismo 403 para «no eres miembro» y para «no existe»: distinguirlos
	// convertiría este endpoint en un detector de organizaciones ajenas.
	if destino == nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("You are not a member of that organization."))
		return
	}
	if destino.Status != domain.StatusActive {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("That organization is suspended."))
		return
	}
	if !s.orgEntitled(r.Context(), destino.ID) {
		httpx.Fail(w, r, httpx.ErrNoActiveOrg)
		return
	}

	// El contexto se captura FUERA y se propaga dentro de la transacción: pedir
	// `r.Context()` otra vez desde la closure crea un contexto hermano del que
	// gobierna la transacción, así que cancelar la petición no la cancela a ella.
	ctx := r.Context()
	cambiadas := 0
	err = s.store.InTx(ctx, func(tx pg.Querier) error {
		n, err := s.store.SetSessionsOrg(ctx, tx, c.Subject, destino.ID)
		if err != nil {
			return err
		}
		cambiadas = n
		return s.store.RevokeRefreshTokensForUser(ctx, tx, c.Subject)
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// Se audita en la organización de DESTINO: quien revise el registro de una
	// organización tiene que ver quién entró en ella, y el registro de la de
	// origen no es donde nadie lo va a buscar.
	_ = s.store.Audit(r.Context(), nil,
		auditEntry(c.Subject, destino.ID, "auth.org.switched", destino.Slug, r))

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"org_id":           destino.ID,
		"slug":             destino.Slug,
		"name":             destino.Name,
		"sessions_updated": cambiadas,
		"reauthorize":      true,
		"reauthorize_hint": "the current access token keeps the previous tenant until it is reissued",
	})
}
