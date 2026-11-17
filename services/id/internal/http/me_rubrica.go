// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Rúbrica guardada: la firma dibujada a mano que la persona pidió recordar al
// firmar en CertiPDF («Recordar mi firma»). Vive en la cuenta, cifrada, y se
// olvida desde CertiPDF o desde Mi cuenta.
//
//	GET    /api/v1/me/rubrica   {trazos, updated_at} o 404 si no hay ninguna
//	PUT    /api/v1/me/rubrica   {trazos}: la guarda o la sustituye
//	DELETE /api/v1/me/rubrica   la olvida (no-op si no había)
//
// Se guardan los TRAZOS y no una imagen: son lo que el lienzo de CertiPDF vuelve a
// pintar y lo que el motor estampa como vectores, sin perder nitidez.

// Límites del lienzo de CertiPDF (480×148) y de lo que razonablemente es una firma.
// Acotan el tamaño de lo que se cifra y guarda: una firma no tiene 64 trazos ni
// miles de puntos por trazo, y sin tope el endpoint sería un almacén gratuito.
const (
	rubricaAncho          = 480
	rubricaAlto           = 148
	rubricaMaxTrazos      = 64
	rubricaMaxPuntos      = 4000
	rubricaMaxPuntosTrazo = 2000
)

type rubricaBody struct {
	Trazos [][][2]float64 `json:"trazos"`
}

var errRubricaInvalida = errors.New("rúbrica inválida")

// validarRubrica comprueba que los trazos caben en el lienzo y en los límites, y
// devuelve su JSON canónico (el que se cifra). Una rúbrica sin ningún trazo de dos
// puntos no es una firma: no hay nada que recordar.
func validarRubrica(trazos [][][2]float64) (string, error) {
	if len(trazos) == 0 || len(trazos) > rubricaMaxTrazos {
		return "", errRubricaInvalida
	}
	total, conTrazo := 0, false
	for _, t := range trazos {
		if len(t) == 0 || len(t) > rubricaMaxPuntosTrazo {
			return "", errRubricaInvalida
		}
		total += len(t)
		if len(t) > 1 {
			conTrazo = true
		}
		for _, p := range t {
			x, y := p[0], p[1]
			if math.IsNaN(x) || math.IsNaN(y) || x < 0 || y < 0 || x > rubricaAncho || y > rubricaAlto {
				return "", errRubricaInvalida
			}
		}
	}
	if total > rubricaMaxPuntos || !conTrazo {
		return "", errRubricaInvalida
	}
	b, err := json.Marshal(trazos)
	if err != nil {
		return "", errRubricaInvalida
	}
	return string(b), nil
}

func (s *Server) handleMeRubricaGet(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	claro, updatedAt, err := s.store.GetRubrica(r.Context(), c.Subject)
	if errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	var trazos [][][2]float64
	if json.Unmarshal([]byte(claro), &trazos) != nil {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	// Una firma no se queda en ninguna caché intermedia.
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"trazos": trazos, "updated_at": updatedAt})
}

func (s *Server) handleMeRubricaPut(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req rubricaBody
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	norm, err := validarRubrica(req.Trazos)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid signature."))
		return
	}
	if err := s.store.SetRubrica(r.Context(), c.Subject, norm); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.signature.saved", "", r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMeRubricaDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if err := s.store.ClearRubrica(r.Context(), c.Subject); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.signature.removed", "", r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}
