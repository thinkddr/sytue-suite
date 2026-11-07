// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// paginaDocs es la primera página de la portada de Docus, Calcus y Diapos.
const paginaDocs = 100

// Docs es la Fuente de Docus, Calcus o Diapos (según kind): los documentos de ese tipo
// más recientes, sin búsqueda ni destacados, también con un documento abierto.
// Peticiones: 1.
func Docs(_ Comunes, docs *api.Docs, kind api.DocKind) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		lista, cursor, err := docs.List(ctx, api.DocsFilter{Kind: kind, Limit: paginaDocs})
		if err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		if cursor == "" {
			d.Counters = []widgets.Counter{{Kind: "documents", Value: int64(len(lista))}}
		}
		items := make([]widgets.Item, 0, min(len(lista), widgets.MaxItems))
		for _, doc := range lista {
			titulo := strings.TrimSpace(doc.Title)
			if titulo == "" {
				continue
			}
			insignia := ""
			if doc.Starred {
				insignia = "★"
			}
			items = append(items, widgets.Item{Title: titulo, Badge: insignia, StartAt: unix(doc.UpdatedAt), Unread: doc.Starred})
			if llenos(items) {
				break
			}
		}
		d.Items = widgets.SanitizeItems(items)
		return d, nil
	}
}
