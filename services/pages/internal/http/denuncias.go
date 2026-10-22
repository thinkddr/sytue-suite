// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// montarDenuncias registra las denuncias de contenido, POST /_pages/reports (§6.4).
//
// Sin cubo que rechace (WEB-05): quien más interés tiene en que no lleguen las denuncias
// de una web es quien la publica, y un cubo por sitio —el de antes era de cada réplica,
// por Host y sin reto— se vacía a propósito para callarlas. Lo que frena la inundación
// es el reto ALTCHA, obligatorio y de un solo uso, y que el buzón de abuso reciba un
// aviso por sitio y hora; las demás denuncias quedan en abuse_reports.
func montarDenuncias(g grupos, h *Handlers) {
	g.publico.Post("/reports", h.denunciarAbuso)
}

type denunciaPeticion struct {
	Reason  string `json:"reason"`
	Details string `json:"details"`
	Email   string `json:"email"`
	Path    string `json:"path"`
	Altcha  string `json:"altcha"`
}

func validarOrigin(origin, host string) bool {
	return origin != "" && (strings.EqualFold(origin, "https://"+host) || strings.EqualFold(origin, "http://"+host))
}

func (h *Handlers) resolverSiteID(ctx context.Context, host string) (uuid.UUID, error) {
	if h.Lector != nil && h.Lector.Listo() {
		sIDStr, siteMapa, ok := h.Lector.ObtenerPorHost(host)
		if ok && siteMapa.Status != "suspended" && siteMapa.Status != "unpublished" {
			return uuid.Parse(sIDStr)
		}
	}

	if h.Store != nil {
		_, siteID, err := h.Store.ResolverHostPublico(ctx, host, h.Cfg.SitesDomain)
		if err == nil {
			return siteID, nil
		}
	}

	return uuid.Nil, errors.New("site_unavailable")
}

func validarMotivo(reason string) bool {
	switch domain.AbuseReason(reason) {
	case domain.AbuseReasonPhishing, domain.AbuseReasonMalware, domain.AbuseReasonIllegal, domain.AbuseReasonCopyright, domain.AbuseReasonOther:
		return true
	default:
		return false
	}
}

func (h *Handlers) denunciarAbuso(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if !validarOrigin(r.Header.Get("Origin"), host) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("origin_mismatch"))
		return
	}

	siteID, err := h.resolverSiteID(r.Context(), host)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusServiceUnavailable, "site_unavailable", "el sitio no está disponible"))
		return
	}

	var req denunciaPeticion
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}

	// El reto es obligatorio: antes se comprobaba solo si venía, así que omitirlo lo
	// saltaba (WEB-05). Es el de GET /_pages/altcha, el mismo del checkout y de los
	// formularios, que es el que pide el cliente exportado (submitReport).
	if !h.retoValido(r.Context(), req.Altcha) {
		httpx.Fail(w, r, httpx.NewError(http.StatusUnprocessableEntity, "altcha_invalid", "el reto ALTCHA no es válido o ha expirado"))
		return
	}

	if !validarMotivo(req.Reason) {
		httpx.Fail(w, r, httpx.NewError(http.StatusUnprocessableEntity, "invalid_reason", "motivo de denuncia no válido"))
		return
	}

	if len(req.Path) > 500 {
		req.Path = req.Path[:500]
	}
	if len(req.Details) > 4000 {
		req.Details = req.Details[:4000]
	}
	if len(req.Email) > 254 {
		req.Email = req.Email[:254]
	}

	rep := &domain.AbuseReport{
		ID:            uuid.New(),
		SiteID:        siteID,
		Host:          host,
		Path:          req.Path,
		Reason:        domain.AbuseReason(req.Reason),
		Details:       req.Details,
		ReporterEmail: req.Email,
		Status:        domain.AbuseReportStatusOpen,
		CreatedAt:     time.Now().UTC(),
	}

	enLaHora := 1
	if h.Store != nil {
		n, err := h.Store.CrearDenunciaAbuso(r.Context(), rep)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		enLaHora = n
	}

	// Un aviso por sitio y hora: el de la primera denuncia (WEB-05).
	if enLaHora == 1 {
		h.enviarAvisoCorreo(r.Context(), host, rep)
	}
	w.WriteHeader(http.StatusNoContent)
}

// retoValido comprueba un ALTCHA de un solo uso. Sin verificador no pasa nada: cerrado.
func (h *Handlers) retoValido(ctx context.Context, payload string) bool {
	if h.Altcha == nil {
		return false
	}
	ok, err := h.Altcha.Verify(ctx, payload)
	return err == nil && ok
}

func (h *Handlers) enviarAvisoCorreo(ctx context.Context, host string, rep *domain.AbuseReport) {
	if h.Cfg.Correo.Activo() && h.Cfg.AbusoCorreo != "" {
		asunto := fmt.Sprintf("[Sytue Web] Denuncia de abuso: %s", host)
		cuerpo := fmt.Sprintf("Denuncia de abuso recibida:\n\nHost: %s\nRuta: %s\nMotivo: %s\nDetalles:\n%s\nInformante: %s\nFecha: %s\n\n"+
			"Es el único aviso de este sitio en la próxima hora: si llegan más denuncias suyas, están en abuse_reports.\n",
			host, rep.Path, rep.Reason, rep.Details, rep.ReporterEmail, rep.CreatedAt.Format(time.RFC3339))
		_ = correo.Enviar(ctx, h.Cfg.Correo, h.Cfg.AbusoCorreo, asunto, cuerpo)
	}
}
