// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// # Lo que deciden quien recibe y su organización (ADR 0059)
//
// Una persona puede BLOQUEAR a quien le pide firmas desde otra organización: desde la
// petición recibida, y se quita desde sus ajustes. Y una organización decide qué hace
// con las que le llegan de fuera: abiertas (lo de siempre), cerradas, o solo de las
// organizaciones de confianza.
//
// Quien pide NO se entera de ninguna de las dos cosas: su petición se «envía» igual
// —misma respuesta, mismo enlace de Drive, la ve en su lista—, pero no llega: ni aviso,
// ni lista, ni se puede abrir (`silenciada`). Si se le dijera, sabría a quién le ha
// bloqueado alguien, o en qué organización está una persona que `people:resolve` no
// cuenta a propósito (ADR 0040).

var errOrgNoAdmite = httpx.ErrForbidden.WithCode("pdf_signature_org_closed").
	WithMessage("Your organisation does not accept signature requests from this organisation.")

// noLlega dice si una petición de `solicitante` a `destinatario` (de fuera) no llega:
// porque la persona lo ha bloqueado, o porque ninguna de sus organizaciones admite
// peticiones de la organización de quien pide. Su cuenta personal, si la tiene, las
// admite siempre: ahí decide ella, con sus bloqueos.
//
// SE HACEN SIEMPRE LOS TRES PASOS, y en este orden: si el bloqueo se mirara antes y
// cortara, una petición bloqueada no preguntaría a Sytue ID y se notaría —más rápida,
// y sin el 503 que da cuando Sytue ID no contesta—.
func (h *Handler) noLlega(ctx context.Context, destinatario, solicitante string) (bool, error) {
	orgs, personal, err := peticiones.OrganizacionesDe(ctx, h.idBaseURL, h.idSecreto, destinatario)
	if err != nil {
		// Sin saberlo, no se crea nada: dejarla llegar se saltaría la política.
		return false, httpx.NewError(http.StatusServiceUnavailable, "pdf_signature_requests_unavailable",
			"Signature requests to other organisations are not available right now. Try again later.").Wrap(err)
	}
	t, _ := tenancy.From(ctx)
	admitida, err := h.peticionesFirma.FirmaAdmitida(ctx, orgs, t.Slug)
	if err != nil {
		return false, err
	}
	bloqueado, err := h.peticionesFirma.FirmaBloqueada(ctx, destinatario, solicitante)
	return bloqueado || (!admitida && !personal), err
}

// admiteDesdeAqui: la organización del TOKEN de quien llama (no la de la petición, a
// la que ya pasó el middleware) admite peticiones de la organización del contexto. Lo
// piden «abrir» y «firmar», que dejan ficheros en la unidad de esa organización;
// «rechazar» y «bloquear» no, a propósito: decir que no, o que no más, vale siempre.
func (h *Handler) admiteDesdeAqui(w http.ResponseWriter, r *http.Request) bool {
	c, _ := authn.FromContext(r.Context())
	t, _ := tenancy.From(r.Context())
	if !esExterno(r.Context()) || c.Tenant == t.Slug {
		return true // no es de fuera: no hay política que mirar
	}
	si, err := h.peticionesFirma.FirmaAdmitida(r.Context(), []string{c.Tenant}, t.Slug)
	if err != nil {
		httpx.Fail(w, r, err)
		return false
	}
	if !si {
		httpx.Fail(w, r, errOrgNoAdmite)
	}
	return si
}

// bloquearDesdePeticion — POST /api/v1/pdf/signature-requests/{id}/block: quien la
// recibió desde otra organización bloquea a quien la pidió. La petición deja de
// llegarle, y las que vengan; quien pidió no recibe nada.
func (h *Handler) bloquearDesdePeticion(w http.ResponseWriter, r *http.Request) {
	subj, id, ok := h.peticionDeLaRuta(w, r)
	if !ok {
		return
	}
	// En cualquier estado —también tras rechazarla o firmarla—, pero solo de fuera:
	// dentro de la organización no se bloquea.
	p, err := h.peticionesFirma.PeticionDeFueraRecibida(r.Context(), id, subj.Sub)
	if err != nil {
		h.failPeticionFirma(w, r, err)
		return
	}
	c, _ := authn.FromContext(r.Context())
	err = h.peticionesFirma.BloquearFirma(r.Context(), subj.Sub, p.Solicitante, p.SolicitanteNombre, c.Tenant)
	switch {
	case errors.Is(err, store.ErrDemasiadosBloqueos):
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "pdf_signature_blocks_full",
			"You have blocked too many people. Unblock someone first."))
		return
	case errors.Is(err, store.ErrBloqueoSinSentido):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("You cannot block yourself."))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// desbloquearFirma — DELETE /api/v1/pdf/signature-blocks/{sub}: desde los ajustes de
// quien bloqueó. Lo que llegó silenciado mientras tanto no aparece.
func (h *Handler) desbloquearFirma(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	otro := chi.URLParam(r, "sub")
	if _, err := uuid.Parse(otro); err != nil || h.peticionesFirma == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err := h.peticionesFirma.DesbloquearFirma(r.Context(), subj.Sub, otro); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// politicaFirma — GET /api/v1/pdf/signature-policy (owner o admin).
func (h *Handler) politicaFirma(w http.ResponseWriter, r *http.Request) {
	if h.peticionesFirma == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	p, err := h.peticionesFirma.PoliticaDeFirma(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

// guardarPoliticaFirma — PUT /api/v1/pdf/signature-policy (owner o admin).
func (h *Handler) guardarPoliticaFirma(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok || h.peticionesFirma == nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var in store.PoliticaFirma
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	limpia, err := politicaValida(in, t.Slug)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := h.peticionesFirma.GuardarPoliticaDeFirma(r.Context(), limpia, subj.Sub)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

// politicaValida recorta y comprueba la política: un modo conocido y, de confianza,
// hasta MaxConfianza organizaciones con nombre de organización válido, sin repetir y
// sin la propia. Sin modo de confianza, la lista se vacía: no se guarda lo que no
// aplica.
func politicaValida(in store.PoliticaFirma, propia string) (store.PoliticaFirma, error) {
	out := store.PoliticaFirma{Modo: in.Modo, Confianza: []string{}}
	switch in.Modo {
	case store.PoliticaAbierta, store.PoliticaCerrada:
		return out, nil
	case store.PoliticaConfianza:
	default:
		return out, httpx.ErrBadRequest.WithMessage(`"mode" must be "abierta", "cerrada" or "confianza".`)
	}
	if len(in.Confianza) > store.MaxConfianza {
		return out, httpx.ErrBadRequest.WithMessage("Too many trusted organisations.")
	}
	for _, o := range in.Confianza {
		o = strings.ToLower(strings.TrimSpace(o))
		t, err := tenancy.New(o)
		if err != nil {
			return out, httpx.ErrBadRequest.WithCode("pdf_signature_policy_org").
				WithMessage("That is not an organisation name: " + o).WithDetails(map[string]string{"org": o})
		}
		if t.Slug != propia && !slices.Contains(out.Confianza, t.Slug) {
			out.Confianza = append(out.Confianza, t.Slug)
		}
	}
	return out, nil
}
