// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// La presencia es lo CUARTO transversal de la suite, y el único que NO se guarda en la
// máquina: vive en el Hub porque la ven los demás. Si me pongo en «ocupado», eso lo tiene
// que saber quien me escribe, no solo mis trece ventanas. Una copia local sería una
// segunda verdad que se queda vieja en cuanto alguien la cambia desde la web o desde el
// teléfono (ADR 0050).
//
// Los estados son los del servicio (services/hub/internal/domain): `online`, `away`,
// `busy` y `offline`. Ojo, que el cliente TypeScript de la web dice `dnd` y el servicio
// rechaza ese valor en `ValidState`; aquí se usan los del servicio.

// EstadoPresencia es cómo aparece una persona ante las demás.
type EstadoPresencia string

const (
	EnLinea      EstadoPresencia = "online"
	Ausente      EstadoPresencia = "away"
	NoMolestar   EstadoPresencia = "busy"
	Desconectado EstadoPresencia = "offline"
)

// Presencia es lo que el Hub sabe de alguien.
type Presencia struct {
	Subject  string          `json:"subject"`
	Estado   EstadoPresencia `json:"state"`
	Mensaje  string          `json:"status,omitempty"`
	UltimaEn time.Time       `json:"last_seen"`
}

// MaxPresencias es el tope del Hub por consulta (`QueryPresence`: 200); de ahí para
// arriba contesta 400 y no se sabe la de nadie.
const MaxPresencias = 200

// Presencias pregunta por la de estas personas (Hub: GET /presence?subjects=…), de
// MaxPresencias en MaxPresencias: los miembros de un canal grande pasan del tope. Sin
// nadie en la lista, el Hub contesta con la de quien pregunta.
func (h *Hub) Presencias(ctx context.Context, subjects ...string) ([]Presencia, error) {
	if len(subjects) == 0 {
		return h.presencias(ctx, nil)
	}
	var todas []Presencia
	for tanda := range slices.Chunk(subjects, MaxPresencias) {
		ps, err := h.presencias(ctx, tanda)
		if err != nil {
			return todas, err
		}
		todas = append(todas, ps...)
	}
	return todas, nil
}

// presencias es UNA consulta al Hub.
func (h *Hub) presencias(ctx context.Context, subjects []string) ([]Presencia, error) {
	q := url.Values{}
	if len(subjects) > 0 {
		q.Set("subjects", strings.Join(subjects, ","))
	}
	out, err := JSON[struct {
		Presence []Presencia `json:"presence"`
	}](ctx, h.c, Request{Path: "/presence", Query: q})
	return out.Presence, err
}

// PonerPresencia publica la propia y devuelve cada cuánto hay que repetirlo para no
// caducar (Hub: PUT /presence). El Hub tiene la presencia en una clave con TTL, así que
// quien deja de latir pasa a `offline` solo.
func (h *Hub) PonerPresencia(ctx context.Context, e EstadoPresencia, mensaje string) (time.Duration, error) {
	cuerpo, err := json.Marshal(struct {
		State  EstadoPresencia `json:"state"`
		Status string          `json:"status,omitempty"`
	}{e, mensaje})
	if err != nil {
		return 0, err
	}
	out, err := JSON[struct {
		Cada int `json:"heartbeat_seconds"`
	}](ctx, h.c, Request{
		Method: http.MethodPut, Path: "/presence", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return 0, err
	}
	if out.Cada <= 0 {
		return 0, errors.New("api: el Hub no ha dicho cada cuánto latir")
	}
	return time.Duration(out.Cada) * time.Second, nil
}

// Desconectar marca la presencia propia como desconectada AHORA, sin esperar al TTL
// (Hub: DELETE /presence). Es lo que se manda al cerrar la última ventana.
func (h *Hub) Desconectar(ctx context.Context) error {
	resp, err := h.c.Do(ctx, Request{Method: http.MethodDelete, Path: "/presence"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
