// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// EL SELLO DE TIEMPO DE UNA FIRMA HECHA EN EL DISPOSITIVO (ADR 0086, «Firmar en el
// dispositivo»).
//
// Quien tiene su Drive en la bóveda firma el PDF en el navegador: ni el documento ni
// el certificado llegan aquí. Lo único que puede necesitar del servidor es el sello
// RFC 3161, porque la autoridad del despliegue (su URL, su usuario, sus anclas) solo
// la conoce este servicio. Así que este endpoint recibe SOLO el resumen SHA-256 del
// valor de la firma —el messageImprint que se sella, el mismo que manda el firmador
// del servidor (`huellaDeFirma`)— y devuelve el token tal cual lo emitió la autoridad,
// después de comprobar que sella ESA huella, con el nonce de esta petición y, si hay
// anclas, que lo firma la autoridad configurada (`PedirSello`).
//
// Con un resumen de la firma no se puede saber nada del documento: es el hash de un
// número firmado, no del PDF. Por eso no se acepta otra cosa que 32 bytes.

// limiteSello acota cuántos sellos pide una persona: cada uno es una llamada a una
// autoridad externa (y, si es de pago, dinero). Uno por segundo con ráfaga de diez
// sobra para firmar varios PDF seguidos.
//
// ponytail: por réplica, como execLimiter; a pg.RateLimiter si hiciera falta compartido.
var limiteSello = httpx.NewTokenBucket(1, 10)

var errSelloNoConfigurado = httpx.NewError(http.StatusNotFound, "pdf_timestamp_unconfigured",
	"This deployment has no time-stamping authority. Signatures are made without a time stamp.")

// selloDisponibleJSON es lo que la interfaz necesita saber ANTES de firmar: si habrá
// sello y de quién, para decirlo en vez de descubrirlo después.
type selloDisponibleJSON struct {
	Disponible bool   `json:"disponible"`
	Autoridad  string `json:"autoridad"`
}

type pedirSelloJSON struct {
	// Resumen es el SHA-256 del valor de la firma (base64). Nada más.
	Resumen []byte `json:"resumen"`
}

type selloJSON struct {
	// Token es el TimeStampToken (DER, base64) con los bytes de la autoridad.
	Token []byte `json:"token"`
}

// pdfSelloDisponible atiende GET /api/v1/pdf/timestamp.
func (h *Handler) pdfSelloDisponible(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, selloDisponibleJSON{
		Disponible: h.sello.URL != "",
		Autoridad:  hostDe(h.sello.URL),
	})
}

// pdfSellar atiende POST /api/v1/pdf/timestamp {"resumen": "<base64 de 32 bytes>"}.
func (h *Handler) pdfSellar(w http.ResponseWriter, r *http.Request) {
	quien, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if h.sello.URL == "" {
		httpx.Fail(w, r, errSelloNoConfigurado)
		return
	}
	if res, _ := limiteSello.Allow(r.Context(), "sello:"+quien.Tenant+":"+quien.Sub); !res.Allowed {
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again in a moment."))
		return
	}
	var in pedirSelloJSON
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(in.Resumen) != sha256.Size {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`"resumen" must be the SHA-256 of the signature value, in base64.`))
		return
	}
	token, err := domain.PedirSello(r.Context(), in.Resumen, h.sello)
	if err != nil {
		log.From(r.Context()).Warn("sello de una firma del dispositivo: la autoridad no lo dio", "error", err)
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "pdf_timestamp_unavailable",
			"The time-stamping authority did not answer. Try again, or sign without a time stamp."))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, selloJSON{Token: token})
}
