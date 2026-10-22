// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Administración de APPS de terceros (identidad SALIENTE) desde Sytue Admin: SP SAML
// registrados por la org (Sytue ID como IdP) y emisión de tokens de acceso inicial del
// registro dinámico OIDC (RFC 7591). CRUD acotado a la organización ACTIVA del
// solicitante (requireRole ya exige owner/admin). Aislamiento por org fail-closed: un
// SP de otra org ⇒ 404. Sigue el estilo de admin_sso.go.
package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// dcrTokenTTL es la vida de un token de acceso inicial del DCR (RFC 7591): corta, el
// admin lo emite y la app se registra poco después.
const dcrTokenTTL = 24 * time.Hour

// samlSPWire es la forma JSON de un SP SAML para la Console. Incluye la config del IdP
// (EntityID, metadata, SSO/SLO, initiate URL) que el admin registra en el SP de terceros.
func (s *Server) samlSPWire(orgID string, p domain.SAMLServiceProvider) map[string]any {
	return map[string]any{
		"id":                  p.ID,
		"name":                p.Name,
		"entity_id":           p.EntityID,
		"acs_url":             p.ACSURL,
		"cert_pem":            p.CertPEM,
		"name_id_format":      p.NameIDFormat,
		"attribute_mapping":   p.AttrMapping,
		"idp_initiated":       p.IDPInitiated,
		"relay_state_default": p.RelayStateDefault,
		"enabled":             p.Enabled,
		"configured":          p.Configured(),
		// Config del IdP (Sytue ID) para dársela al SP de terceros:
		"idp_entity_id":    s.idpEntityID(orgID),
		"idp_metadata_url": s.idpEntityID(orgID),
		"idp_sso_url":      s.idpSSOURL(),
		"idp_slo_url":      s.idpSLOURL(),
		"initiate_url":     s.idpInitiateURL(p.ID),
		"created_at":       p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":       p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

type samlSPRequest struct {
	Name              *string           `json:"name"`
	EntityID          *string           `json:"entity_id"`
	ACSURL            *string           `json:"acs_url"`
	CertPEM           *string           `json:"cert_pem"`
	NameIDFormat      *string           `json:"name_id_format"`
	AttributeMapping  map[string]string `json:"attribute_mapping"`
	IDPInitiated      *bool             `json:"idp_initiated"`
	RelayStateDefault *string           `json:"relay_state_default"`
	Enabled           *bool             `json:"enabled"`
}

// handleAdminListSAMLSPs — GET /api/v1/admin/apps/saml-sps — SP SAML de la org activa.
func (s *Server) handleAdminListSAMLSPs(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListSAMLServiceProviders(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, p := range list {
		items = append(items, s.samlSPWire(org.ID, p))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handleAdminCreateSAMLSP — POST /api/v1/admin/apps/saml-sps.
func (s *Server) handleAdminCreateSAMLSP(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	var req samlSPRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sp := domain.SAMLServiceProvider{
		OrgID:             org.ID,
		Name:              strings.TrimSpace(strDeref(req.Name)),
		EntityID:          strings.TrimSpace(strDeref(req.EntityID)),
		ACSURL:            strings.TrimSpace(strDeref(req.ACSURL)),
		CertPEM:           strings.TrimSpace(strDeref(req.CertPEM)),
		NameIDFormat:      strings.TrimSpace(strDeref(req.NameIDFormat)),
		AttrMapping:       normalizeMapping(req.AttributeMapping),
		IDPInitiated:      req.IDPInitiated != nil && *req.IDPInitiated,
		RelayStateDefault: strings.TrimSpace(strDeref(req.RelayStateDefault)),
		CreatedBy:         c.Subject,
		Enabled:           req.Enabled != nil && *req.Enabled,
	}
	if err := validateSAMLSP(sp); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := s.store.CreateSAMLServiceProvider(r.Context(), nil, sp)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err)) // ErrConflict (entity_id repetido) ⇒ 409
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.saml_sp.created", created.EntityID, r))
	httpx.JSON(w, r, http.StatusCreated, s.samlSPWire(org.ID, created))
}

// handleAdminGetSAMLSP — GET /api/v1/admin/apps/saml-sps/{id}. Cross-org ⇒ 404.
func (s *Server) handleAdminGetSAMLSP(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sp, err := s.loadOrgSAMLSP(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, s.samlSPWire(org.ID, sp))
}

// handleAdminUpdateSAMLSP — PATCH /api/v1/admin/apps/saml-sps/{id}.
func (s *Server) handleAdminUpdateSAMLSP(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	sp, err := s.loadOrgSAMLSP(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req samlSPRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Name != nil {
		sp.Name = strings.TrimSpace(*req.Name)
	}
	if req.EntityID != nil {
		sp.EntityID = strings.TrimSpace(*req.EntityID)
	}
	if req.ACSURL != nil {
		sp.ACSURL = strings.TrimSpace(*req.ACSURL)
	}
	if req.CertPEM != nil {
		sp.CertPEM = strings.TrimSpace(*req.CertPEM)
	}
	if req.NameIDFormat != nil {
		sp.NameIDFormat = strings.TrimSpace(*req.NameIDFormat)
	}
	if req.AttributeMapping != nil {
		sp.AttrMapping = normalizeMapping(req.AttributeMapping)
	}
	if req.IDPInitiated != nil {
		sp.IDPInitiated = *req.IDPInitiated
	}
	if req.RelayStateDefault != nil {
		sp.RelayStateDefault = strings.TrimSpace(*req.RelayStateDefault)
	}
	if req.Enabled != nil {
		sp.Enabled = *req.Enabled
	}
	if err := validateSAMLSP(sp); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := s.store.UpdateSAMLServiceProvider(r.Context(), nil, sp)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.saml_sp.updated", updated.EntityID, r))
	httpx.JSON(w, r, http.StatusOK, s.samlSPWire(org.ID, updated))
}

// handleAdminDeleteSAMLSP — DELETE /api/v1/admin/apps/saml-sps/{id}. Cross-org ⇒ 404.
func (s *Server) handleAdminDeleteSAMLSP(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, perr := uuid.Parse(id); perr != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ok, err := s.store.DeleteSAMLServiceProvider(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.saml_sp.deleted", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// handleAdminCreateDCRToken — POST /api/v1/admin/apps/dcr-tokens — emite un token de
// acceso inicial del registro dinámico (RFC 7591), acotado a la org activa. Se devuelve
// UNA vez (solo se persiste su hash).
func (s *Server) handleAdminCreateDCRToken(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	tok, err := domain.GenerateToken(32)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	exp := time.Now().Add(dcrTokenTTL)
	if err := s.store.InsertDCRToken(r.Context(), nil, domain.HashToken(tok), org.ID, c.Subject, exp); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "apps.dcr_token.issued", "", r))
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"initial_access_token":  tok,
		"registration_endpoint": strings.TrimRight(s.cfg.Issuer, "/") + "/oauth2/register",
		"expires_at":            exp.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// validateSAMLSP valida un SP SAML antes de guardarlo/activarlo: name/entity_id
// presentes, ACS https (o http localhost en dev). Devuelve un 400 sobrio si no.
func validateSAMLSP(p domain.SAMLServiceProvider) error {
	if p.Name == "" || p.EntityID == "" {
		return httpx.ErrBadRequest.WithMessage("name and entity_id are required.")
	}
	if p.ACSURL == "" || !validRegistrationRedirect(p.ACSURL) {
		return httpx.ErrBadRequest.WithMessage("acs_url must be a valid https URL (Assertion Consumer Service).")
	}
	return nil
}

// loadOrgSAMLSP carga el SP {id} acotado a la org (cross-org/inexistente ⇒ 404).
func (s *Server) loadOrgSAMLSP(r *http.Request, orgID string) (domain.SAMLServiceProvider, error) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		return domain.SAMLServiceProvider{}, httpx.ErrNotFound
	}
	p, err := s.store.GetSAMLServiceProvider(r.Context(), orgID, id)
	if err != nil {
		return domain.SAMLServiceProvider{}, mapDomainErr(err)
	}
	return p, nil
}
