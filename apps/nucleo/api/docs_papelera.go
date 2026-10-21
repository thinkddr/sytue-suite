// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
)

// La papelera de Sytue Docs (services/docs/internal/http/papelera.go): lo borrado de
// quien pregunta, restaurar, eliminar para siempre y vaciar. El servicio hace lo mismo
// con el fichero de cada documento en la papelera de Sytue Drive.

// Papelera devuelve los documentos de quien pregunta en la papelera, el último tirado
// primero (hasta 200). Con `kind`, solo de ese tipo: Calcus ve sus hojas.
func (d *Docs) Papelera(ctx context.Context, kind DocKind) ([]Document, error) {
	q := url.Values{}
	if kind != "" {
		q.Set("kind", string(kind))
	}
	out, err := JSON[struct {
		Documents []Document `json:"documents"`
	}](ctx, d.c, Request{Path: "/trash", Query: q})
	return out.Documents, err
}

// Restaurar saca el documento de la papelera. Un 409 con código `file_name_taken` es
// que en su carpeta de Drive ya hay otro fichero con ese nombre; con
// `already_restored`, que ya hay otro documento vivo abierto desde el mismo fichero.
func (d *Docs) Restaurar(ctx context.Context, id string) error {
	return d.sinCuerpo(ctx, http.MethodPost, "/trash/"+url.PathEscape(id)+"/restore", nil)
}

// Purgar lo elimina para siempre. Un 423 es una retención legal.
func (d *Docs) Purgar(ctx context.Context, id string) error {
	return d.sinCuerpo(ctx, http.MethodDelete, "/trash/"+url.PathEscape(id), nil)
}

// Vaciar elimina para siempre UNA página de la papelera (del tipo `kind`, si lo hay)
// a partir de `despues`. Devuelve cuántos no se pudieron eliminar y desde dónde
// seguir: "" es que no queda nada.
func (d *Docs) Vaciar(ctx context.Context, kind DocKind, despues string) (fallidos int, siguiente string, err error) {
	q := url.Values{}
	if kind != "" {
		q.Set("kind", string(kind))
	}
	if despues != "" {
		q.Set("after", despues)
	}
	out, err := JSON[struct {
		Failed int    `json:"failed"`
		Next   string `json:"next"`
	}](ctx, d.c, Request{Method: http.MethodDelete, Path: "/trash", Query: q})
	return out.Failed, out.Next, err
}
