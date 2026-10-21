// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
)

// «Pedir acceso de edición» desde la franja de solo lectura: el contrato común de
// platform/pkg/peticiones, igual en todos los servicios, sobre la ruta del recurso
// (`/documents/{id}/edit-requests`, `/calendars/{id}/edit-requests`…).

// Peticion es quien ha pedido editar algo.
type Peticion struct {
	Sub   string `json:"sub"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// Quien es el nombre con el que se enseña: el nombre, el correo o, si no hay otra cosa, el sub.
func (p Peticion) Quien() string {
	for _, s := range []string{p.Name, p.Email} {
		if s != "" {
			return s
		}
	}
	return p.Sub
}

// Peticiones habla con la ruta de peticiones de UN recurso.
type Peticiones struct {
	c    *Client
	ruta string
}

// NuevasPeticiones es el cliente de la ruta `ruta` (sin la barra final) del servicio de `c`.
func NuevasPeticiones(c *Client, ruta string) Peticiones { return Peticiones{c: c, ruta: ruta} }

func (p Peticiones) sin(ctx context.Context, metodo, ruta string) error {
	resp, err := p.c.Do(ctx, Request{Method: metodo, Path: ruta})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Pedir avisa a quien puede conceder. Pedirlo otra vez no vuelve a avisar.
func (p Peticiones) Pedir(ctx context.Context) error { return p.sin(ctx, http.MethodPost, p.ruta) }

// Pendientes son las que esperan respuesta. Solo quien puede conceder.
func (p Peticiones) Pendientes(ctx context.Context) ([]Peticion, error) {
	out, err := JSON[struct {
		Requests []Peticion `json:"requests"`
	}](ctx, p.c, Request{Path: p.ruta})
	return out.Requests, err
}

// Resolver concede o rechaza la petición de `sub`.
func (p Peticiones) Resolver(ctx context.Context, sub string, concede bool) error {
	r := p.ruta + "/" + url.PathEscape(sub)
	if concede {
		return p.sin(ctx, http.MethodPost, r+"/grant")
	}
	return p.sin(ctx, http.MethodDelete, r)
}
