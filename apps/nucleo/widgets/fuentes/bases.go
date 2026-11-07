// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Bases lee la lista de bases. Peticiones: 1.
func Bases(_ Comunes, b *api.Bases) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		lista, err := b.ListBases(ctx)
		if err != nil {
			return widgets.Detail{}, err
		}
		items := make([]widgets.Item, 0, min(len(lista), widgets.MaxItems))
		for _, base := range lista {
			if nombre := strings.TrimSpace(base.Name); nombre != "" {
				items = append(items, widgets.Item{
					Title: nombre, Subtitle: base.Description,
					Badge: strings.ToUpper(base.Role), StartAt: unix(base.UpdatedAt),
				})
			}
			if llenos(items) {
				break
			}
		}
		return widgets.Detail{
			Counters: []widgets.Counter{{Kind: "documents", Value: int64(len(lista))}},
			Items:    widgets.SanitizeItems(items),
		}, nil
	}
}
