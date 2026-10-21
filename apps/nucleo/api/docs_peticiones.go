// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/url"
)

// Peticiones es la ruta de «pedir acceso de edición» de un documento (peticiones.go).
func (d *Docs) Peticiones(id string) Peticiones {
	return NuevasPeticiones(d.c, "/documents/"+url.PathEscape(id)+"/edit-requests")
}

// EdicionPedida dice si quien pregunta ya pidió editar el documento y sigue pendiente.
func (d *Docs) EdicionPedida(ctx context.Context, id string) (bool, error) {
	out, err := JSON[struct {
		EditRequested bool `json:"edit_requested"`
	}](ctx, d.c, Request{Path: "/documents/" + url.PathEscape(id)})
	return out.EditRequested, err
}
