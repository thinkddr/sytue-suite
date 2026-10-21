// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Directorio es la puerta de Sytue ID para pasar de un correo a una persona de la PROPIA
// organización y de vuelta (`POST /api/v1/directory:resolve`). Es lo que hace falta para
// compartir con alguien: los permisos se guardan contra el `sub`, no contra una dirección,
// y para pintar con quién está compartido algo hace falta el camino de vuelta.
//
// El servicio contesta 404 igual a «no existe» que a «es de otra organización», y es POST
// para que el correo no acabe en ninguna URL.
type Directorio struct{ c *Client }

// NewDirectorio crea el cliente sobre la raíz de Sytue ID (ServiceURL(apex, "id")).
func NewDirectorio(servicio string, s Session, h *http.Client) *Directorio {
	return &Directorio{c: New(servicio+"/api/v1", s, h)}
}

// Persona es alguien de la organización: su `sub`, su correo y, si lo hay, su nombre.
type Persona struct {
	ID     string `json:"user_id"`
	Email  string `json:"email"`
	Nombre string `json:"name,omitempty"`
}

// PorCorreo resuelve una dirección. Un 404 es que nadie de la organización la usa.
func (d *Directorio) PorCorreo(ctx context.Context, email string) (Persona, error) {
	return PostJSON[Persona](ctx, d.c, "/directory:resolve", map[string]string{"email": email})
}

// PorID es el camino de vuelta: de un `sub` ya guardado a su correo y su nombre.
func (d *Directorio) PorID(ctx context.Context, id string) (Persona, error) {
	return PostJSON[Persona](ctx, d.c, "/directory:resolve", map[string]string{"user_id": id})
}

// Miembro es una persona de la agenda de la organización (`GET /directory`), con TODAS
// las direcciones en las que recibe dentro de Sytue —su buzón y sus alias— y no solo la
// principal: a quien se invita desde fuera, `email` es el de la invitación.
type Miembro struct {
	ID           string   `json:"user_id"`
	Email        string   `json:"email"`
	Emails       []string `json:"emails,omitempty"`
	Nombre       string   `json:"name,omitempty"`
	Departamento string   `json:"department,omitempty"`
	Puesto       string   `json:"title,omitempty"`
}

// Agenda es la primera página de la agenda de la organización, de hasta `limite`
// personas (el servicio admite 500).
func (d *Directorio) Agenda(ctx context.Context, limite int) ([]Miembro, error) {
	r, err := JSON[struct {
		Items []Miembro `json:"items"`
	}](ctx, d.c, Request{Path: "/directory", Query: url.Values{"limit": {strconv.Itoa(limite)}}})
	return r.Items, err
}
