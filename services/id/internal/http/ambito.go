// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

// Dónde se DECLARA el ámbito de RLS de una petición (migración 0127 y
// store/ambito.go). Solo en grupos de rutas que por contrato tocan UNA organización
// —la activa del token— o UNA persona —la del token—: ahí la base pasa a negar lo de
// fuera aunque al SQL de un método le falte el predicado.
//
// No se pone en el grupo de owner/admin porque ese grupo sí cruza organizaciones a
// propósito (`/orgs/{slug}/members`, el listado de las organizaciones que gestiona
// quien llama), ni en los flujos del IdP, que deciden la organización y no la
// conocen de antemano.

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

type claveOrgActiva struct{}

// confinarAOrgActiva resuelve la organización activa del token y confina a ella toda
// consulta de la petición. La organización resuelta queda en el contexto: activeOrg la
// reutiliza en vez de volver a leerla.
func (s *Server) confinarAOrgActiva(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org, err := s.activeOrg(r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), claveOrgActiva{}, org)
		ctx = store.ConAmbitoDeOrganizacion(ctx, org.ID, org.Slug)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// confinarAPersona confina toda consulta de la petición a las filas de la persona del
// token. Es para las rutas de seguridad de la propia cuenta (sesiones, dispositivos,
// contraseñas de aplicación, segundo factor, bóveda), donde nada legítimo es de otra
// persona. Un sujeto que no es un uuid no es una persona: se rechaza aquí, antes de
// que la base lo rechace con un 500.
func (s *Server) confinarAPersona(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := claimsFrom(r.Context())
		if !ok {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		if _, err := uuid.Parse(c.Subject); err != nil {
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This token does not belong to a person."))
			return
		}
		next.ServeHTTP(w, r.WithContext(store.ConAmbitoDePersona(r.Context(), c.Subject)))
	})
}

// orgActivaDelContexto devuelve la organización que confinarAOrgActiva ya resolvió
// para el tenant del token, si la hay.
func orgActivaDelContexto(ctx context.Context, tenant string) (domain.Organization, bool) {
	org, ok := ctx.Value(claveOrgActiva{}).(domain.Organization)
	return org, ok && tenant != "" && org.Slug == tenant
}
