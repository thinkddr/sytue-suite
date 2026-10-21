// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// «Compartir con una persona»: el contrato común de los servicios que lo tienen
// (Photos y Notepads), igual en todos, sobre la ruta del recurso
// (`/albums/{id}/people`, `/notebooks/{id}/people`, `/notes/{id}/people`):
//
//	GET    <ruta>          con quién está compartido (su dueño)
//	PUT    <ruta>/<sub>    {permission: read|write}
//	DELETE <ruta>/<sub>
//
// La persona va por su `sub`: la app la saca del correo con el Directorio, igual que
// Drive, y el servicio lo vuelve a comprobar en Sytue ID antes de guardarlo.

// Niveles con que se comparte con una persona (los permisos de Calendar son otros).
const (
	NivelVer    = "read"
	NivelEditar = "write"
)

// PersonaCompartida es alguien con quien está compartido un recurso.
type PersonaCompartida struct {
	Sub     string `json:"subject"`
	Permiso string `json:"permission"`
}

// Personas habla con la ruta de personas de UN recurso.
type Personas struct {
	c    *Client
	ruta string
}

// NuevasPersonas es el cliente de la ruta `ruta` (sin la barra final) del servicio de `c`.
func NuevasPersonas(c *Client, ruta string) Personas { return Personas{c: c, ruta: ruta} }

// Lista son las personas con quien está compartido. Solo su dueño; a los demás, 404.
func (p Personas) Lista(ctx context.Context) ([]PersonaCompartida, error) {
	out, err := JSON[struct {
		People []PersonaCompartida `json:"people"`
	}](ctx, p.c, Request{Path: p.ruta})
	return out.People, err
}

// Compartir lo da a `sub` con `permiso` (o le cambia el nivel). `nueva` es que no lo
// tenía: el servicio solo avisa entonces. Un 404 con código `unknown_person` es que esa
// persona no es de la organización.
func (p Personas) Compartir(ctx context.Context, sub, permiso string) (nueva bool, err error) {
	cuerpo, err := json.Marshal(map[string]string{"permission": permiso})
	if err != nil {
		return false, err
	}
	r, err := JSON[struct {
		Created bool `json:"created"`
	}](ctx, p.c, Request{
		Method: http.MethodPut, Path: p.ruta + "/" + url.PathEscape(sub), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	return r.Created, err
}

// Quitar se lo retira a `sub`.
func (p Personas) Quitar(ctx context.Context, sub string) error {
	resp, err := p.c.Do(ctx, Request{Method: http.MethodDelete, Path: p.ruta + "/" + url.PathEscape(sub)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
