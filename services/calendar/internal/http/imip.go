// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
)

// RutaRespuestaIMIP es por donde Sytue Mail entrega el REPLY de una invitación que
// contestaron por correo. Es interna —bajo /internal, que el Ingress no publica— y va
// con el secreto de servicio a servicio de la suite.
const RutaRespuestaIMIP = "/internal/imip/reply"

// Las dos cabeceras que Mail pone: la organización del buzón que lo recibió y la
// dirección que FIRMA el correo (su From, ya autenticado por DKIM/DMARC en Mail).
const (
	CabeceraTenantIMIP    = "X-Sytue-Tenant"
	CabeceraRemitenteIMIP = "X-Sytue-Remitente"
)

// maxRespuestaIMIP acota el cuerpo: un REPLY es un VEVENT con un asistente.
const maxRespuestaIMIP = 256 << 10

// respuestaIMIP recibe el REPLY (el iCalendar tal cual en el cuerpo) y lo aplica. El
// secreto lo pone `httpx.SecretoInterno` al montarla.
func (h *Handlers) respuestaIMIP() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := tenancy.New(strings.TrimSpace(r.Header.Get(CabeceraTenantIMIP)))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Missing or invalid tenant."))
			return
		}
		cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxRespuestaIMIP+1))
		if err != nil || len(cuerpo) > maxRespuestaIMIP {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unreadable or too large body."))
			return
		}
		err = h.svc.RecibirRespuesta(tenancy.Into(r.Context(), t), r.Header.Get(CabeceraRemitenteIMIP), string(cuerpo))
		switch {
		case err == nil:
			httpx.NoContent(w)
		case errors.Is(err, service.ErrRespuestaAjena):
			httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The REPLY is not from its sender."))
		case errors.Is(err, domain.ErrNotFound):
			// Nada pendiente que contestar (ya contestada, borrada, o de otro): Mail
			// no tiene que reintentar.
			httpx.NoContent(w)
		default:
			fail(w, r, err)
		}
	}
}
