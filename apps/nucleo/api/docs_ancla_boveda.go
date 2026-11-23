// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
)

// AnclarEnBoveda pone «vault:<id>» como ancla del documento `id` si la que tiene
// sigue siendo `antes` ("" = ninguna): `PUT /documents/{id}/vault-anchor`
// (services/docs, ancla_boveda.go). Es lo único que llega al servicio de Docs
// cuando el fichero del documento se escribe cifrado en el Drive de la bóveda
// (ADR 0086): el ancla, nunca el contenido.
//
// Las reglas las aplica el servicio: solo el dueño, solo desde sin ancla o desde
// otra de la bóveda (un «drive:» no se sustituye) y comparando con `antes`. Un 409
// `vault_anchor_changed` es que otra ventana ancló entretanto; `vault_anchor_taken`,
// que otro documento ya está anclado a ese fichero. Con los datos privados (Matrix)
// no hay servicio al que anclar: ErrPrivateDataOnly.
func (d *Docs) AnclarEnBoveda(ctx context.Context, id, ancla, antes string) error {
	if d == nil || d.private != nil {
		return ErrPrivateDataOnly
	}
	return d.sinCuerpo(ctx, http.MethodPut, "/documents/"+url.PathEscape(id)+"/vault-anchor",
		map[string]string{"source_ref": ancla, "expect": antes})
}
