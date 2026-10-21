// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// «Presentar con Cast» (ADR 0061, ampliación): quien presenta desde Sytue Diapos avisa de
// cada diapositiva con `PUT /lives/{id}/slide` y quien mira la lee con
// `GET /videos/{id}/slides`. Solo viaja la POSICIÓN —número, total y título—: ni el
// contenido ni las notas del ponente salen de Diapos.

// CastPosicion es la diapositiva en la que va quien presenta (`index` desde 0).
type CastPosicion struct {
	Index int    `json:"index"`
	Total int    `json:"total"`
	Title string `json:"title"`
	// Ref es de dónde sale la presentación (`docs:<id>`); vacía, sin origen.
	Ref string `json:"ref,omitempty"`
}

// CastMarca es un cambio de diapositiva con su instante, en ms desde el comienzo.
type CastMarca struct {
	AtMS  int64  `json:"at_ms"`
	Index int    `json:"index"`
	Title string `json:"title"`
}

// CastPresentacion es lo que Cast sabe de la presentación de un directo.
type CastPresentacion struct {
	Ref       string      `json:"ref"`
	Index     int         `json:"index"`
	Total     int         `json:"total"`
	Title     string      `json:"title"`
	StartedAt time.Time   `json:"started_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Marks     []CastMarca `json:"marks"`
}

// AvanzarDiapositiva avisa de la diapositiva en la que va el directo `id` (solo su dueño).
func (c *Cast) AvanzarDiapositiva(ctx context.Context, id string, p CastPosicion) (CastPresentacion, error) {
	datos, err := json.Marshal(p)
	if err != nil {
		return CastPresentacion{}, err
	}
	return JSON[CastPresentacion](ctx, c.c, Request{
		Method: http.MethodPut, Path: "/lives/" + url.PathEscape(id) + "/slide", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Diapositivas es la presentación del vídeo `id`; nil si no lleva ninguna.
func (c *Cast) Diapositivas(ctx context.Context, id string) (*CastPresentacion, error) {
	r, err := JSON[struct {
		Presentation *CastPresentacion `json:"presentation"`
	}](ctx, c.c, Request{Path: "/videos/" + url.PathEscape(id) + "/slides"})
	return r.Presentation, err
}
