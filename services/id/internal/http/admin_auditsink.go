// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Destino SIEM de la auditoría (paridad con el export en streaming de Okta/Entra). El
// administrador de la organización apunta a su Splunk HEC / Sentinel / Elastic y Sytue
// le entrega los eventos según ocurren, en vez de obligarle a descargar un CSV.
//
//   - GET    /api/v1/admin/audit/sink — configuración actual y estado de la entrega.
//   - PUT    /api/v1/admin/audit/sink — configurarlo o actualizarlo.
//   - DELETE /api/v1/admin/audit/sink — retirarlo.
//
// Los dos secretos (la cabecera de autorización del SIEM y el de firma) se guardan
// SELLADOS y NUNCA se devuelven: la respuesta solo dice si están puestos. Un panel de
// administración que reenseña credenciales es un panel que las filtra por captura de
// pantalla, por caché del navegador y por el primer XSS que aparezca.
package http

import (
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// auditSinkResponse es la vista del sink de cara al admin: sin secretos, con estado.
type auditSinkResponse struct {
	URL string `json:"url"`
	// HasAuthHeader/HasSigningSecret dicen si están configurados, no cuáles son.
	HasAuthHeader    bool   `json:"has_auth_header"`
	HasSigningSecret bool   `json:"has_signing_secret"`
	Enabled          bool   `json:"enabled"`
	LastSentID       int64  `json:"last_sent_id"`
	LastError        string `json:"last_error,omitempty"`
	LastSentAt       string `json:"last_sent_at,omitempty"`
}

type auditSinkRequest struct {
	URL string `json:"url"`
	// AuthHeader vacío CONSERVA el que hubiera: así se puede cambiar la URL sin tener
	// que volver a escribir el token del SIEM (que además ya no se puede leer).
	AuthHeader    string `json:"auth_header"`
	SigningSecret string `json:"signing_secret"`
	Enabled       *bool  `json:"enabled"`
}

func (s *Server) handleAdminGetAuditSink(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sink, err := s.store.GetAuditSink(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, toAuditSinkResponse(sink))
}

func (s *Server) handleAdminSetAuditSink(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req auditSinkRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	endpoint := strings.TrimSpace(req.URL)
	if err := validSinkURL(endpoint); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Conservar los secretos que no se reenvían: el admin no puede releerlos, así que
	// exigirlos en cada edición obligaría a rotarlos para cambiar una coma de la URL.
	sink := store.AuditSink{OrgID: org.ID, URL: endpoint, Enabled: true}
	if prev, err := s.store.GetAuditSink(r.Context(), org.ID); err == nil {
		sink.AuthHeader, sink.SigningSecret = prev.AuthHeader, prev.SigningSecret
		sink.Enabled = prev.Enabled
	}
	if v := strings.TrimSpace(req.AuthHeader); v != "" {
		sink.AuthHeader = v
	}
	if v := strings.TrimSpace(req.SigningSecret); v != "" {
		sink.SigningSecret = v
	}
	if req.Enabled != nil {
		sink.Enabled = *req.Enabled
	}

	if err := s.store.SaveAuditSink(r.Context(), sink); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// La URL sí se audita (es configuración, no credencial); los secretos jamás.
	c, _ := claimsFrom(r.Context())
	e := auditEntry(c.Subject, org.ID, "admin.audit_sink.set", endpoint, r)
	e.Detail["enabled"] = sink.Enabled
	_ = s.store.Audit(r.Context(), nil, e)

	saved, err := s.store.GetAuditSink(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, toAuditSinkResponse(saved))
}

func (s *Server) handleAdminDeleteAuditSink(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ok, err := s.store.DeleteAuditSink(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("No SIEM destination is configured."))
		return
	}
	c, _ := claimsFrom(r.Context())
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "admin.audit_sink.delete", "", r))
	w.WriteHeader(http.StatusNoContent)
}

// validSinkURL exige http(s) absoluto con host. La defensa REAL contra SSRF es el
// cliente de la plataforma (resuelve y bloquea rangos privados en cada envío, también si
// el DNS cambia después); esto solo rechaza pronto lo que ni siquiera es una URL.
func validSinkURL(raw string) error {
	if raw == "" {
		return httpx.ErrBadRequest.WithMessage("La URL del destino es obligatoria.")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return httpx.ErrBadRequest.WithMessage("The URL must be an absolute http(s) URL.")
	}
	return nil
}

func toAuditSinkResponse(a store.AuditSink) auditSinkResponse {
	out := auditSinkResponse{
		URL:              a.URL,
		HasAuthHeader:    a.AuthHeader != "",
		HasSigningSecret: a.SigningSecret != "",
		Enabled:          a.Enabled,
		LastSentID:       a.LastSentID,
		LastError:        a.LastError,
	}
	if a.LastSentAt != nil {
		out.LastSentAt = a.LastSentAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}
