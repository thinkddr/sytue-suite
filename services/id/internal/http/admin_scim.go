// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Estado REAL del aprovisionamiento SCIM de una organización.
//
// La consola tenía un KPI «Provisioning: OK» y un distintivo «SCIM activo»
// escritos en el JSX, sin consultar nada. En producción SCIM está apagado
// (`/scim/v2/ServiceProviderConfig` responde 503), así que la pantalla anunciaba
// que el aprovisionamiento —y sobre todo el DESaprovisionamiento— del IdP
// corporativo funcionaba mientras nadie revocaba ninguna cuenta al salir de la
// empresa. Los dos se retiraron, y esto es lo que faltaba para poder volver a
// decir algo: el estado medido, no el deseado.
package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// handleAdminSCIMStatus — GET /api/v1/admin/scim/status.
//
// `configured` compara el slug de ESTA organización con el que lleva el token
// SCIM del despliegue: el token es uno y está ligado a UNA organización
// (`SCIMOrgSlug`), así que para todas las demás SCIM no está encendido por más
// que el proceso lo tenga configurado. Decir «activo» mirando solo si hay token
// sería repetir el mismo error del distintivo que se quitó, con otra fuente.
func (s *Server) handleAdminSCIMStatus(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Configurado = esta organización tiene un token VIVO creado desde su
	// consola, o el token único del despliegue apunta a ella (compatibilidad).
	tokens, err := s.store.ListSCIMTokens(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	configurado := len(tokens) > 0 ||
		(s.cfg.SCIMBearerToken != "" && s.cfg.SCIMOrgSlug != "" && s.cfg.SCIMOrgSlug == org.Slug)

	st, err := s.store.SCIMStatusForOrg(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// El endpoint SCIM que el IdP corporativo tiene que apuntar. Se devuelve
	// SIEMPRE, también cuando no está configurado: es el dato que hace falta para
	// configurarlo, y tenerlo que reconstruir a mano desde la documentación es de
	// donde salen las conexiones con una barra de más.
	cuerpo := map[string]any{
		"configured":        configurado,
		"base_url":          s.cfg.Issuer + "/scim/v2",
		"users_provisioned": st.Provisioned,
		// Los tokens vivos, sin el secreto: la pantalla necesita poder decir
		// cuántos hay, de cuándo son y si alguno se ha usado alguna vez.
		"tokens": tokens,
	}
	// Una organización aprovisionada de la que hace meses que no llega nada es
	// exactamente igual de sospechosa que una sin configurar, y solo esta fecha
	// lo distingue.
	if st.LastEventAt != nil {
		cuerpo["last_event_at"] = st.LastEventAt.UTC().Format(time.RFC3339)
	}
	httpx.JSON(w, r, http.StatusOK, cuerpo)
}

// handleAdminCreateSCIMToken — POST /api/v1/admin/scim/tokens.
//
// Genera el token de aprovisionamiento de ESTA organización y lo devuelve UNA
// vez. Antes esto no existía: el token era una variable de entorno del
// despliegue, así que conectar Entra u Okta exigía que alguien de Sytue editara
// el chart y reiniciara el IdP. Un cliente no podía configurarlo, y por eso no
// había tampoco dónde explicarlo.
//
// El secreto no se guarda: solo su SHA-256. Perderlo no es un problema —se crea
// otro y se revoca el viejo—, pero poder releerlo sí lo sería: quedaría en la
// primera captura de pantalla y en el historial de cualquiera que abra la
// pantalla después.
func (s *Server) handleAdminCreateSCIMToken(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Label string `json:"label"`
	}
	// Sin cuerpo también vale: la etiqueta ayuda pero no puede ser un requisito
	// para encender el aprovisionamiento.
	_ = httpx.DecodeJSON(w, r, &in)

	secreto, err := domain.GenerateToken(32)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	c, _ := claimsFrom(r.Context())
	tok, err := s.store.CreateSCIMToken(r.Context(), nil, org.ID,
		domain.HashToken(secreto), strings.TrimSpace(in.Label), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil,
		auditEntry(c.Subject, org.ID, "scim.token.created", tok.ID, r))

	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"id":         tok.ID,
		"label":      tok.Label,
		"created_at": tok.CreatedAt.UTC().Format(time.RFC3339),
		// La ÚNICA vez que sale de aquí. La pantalla tiene que decirlo antes de
		// que quien lo crea cierre el diálogo.
		"token":    secreto,
		"base_url": s.cfg.Issuer + "/scim/v2",
	})
}

// handleAdminRevokeSCIMToken — DELETE /api/v1/admin/scim/tokens/{id}.
//
// Corta el aprovisionamiento en el acto y sin desplegar nada, que es justo lo
// que no se podía hacer cuando el token vivía en una variable de entorno.
func (s *Server) handleAdminRevokeSCIMToken(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	ok, err := s.store.RevokeSCIMToken(r.Context(), nil, org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		// Mismo 404 para «no existe» y «es de otra organización».
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	c, _ := claimsFrom(r.Context())
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "scim.token.revoked", id, r))
	w.WriteHeader(http.StatusNoContent)
}
