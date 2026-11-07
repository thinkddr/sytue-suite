// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Notes cuenta las notas de todos los cuadernos (NoteCount ya aplica los permisos) y
// lista las del cuaderno INICIAL (el primero fuera del Archivo), aunque la app tenga
// otro abierto. Un cuaderno cifrado no da filas: nunca se descifra. Peticiones: 2.
func Notes(_ Comunes, n *api.Notes) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		cuadernos, err := n.ListNotebooks(ctx)
		if err != nil {
			return widgets.Detail{}, err
		}
		var d widgets.Detail
		if total, valido := totalDeNotas(cuadernos); valido {
			d.Counters = []widgets.Counter{{Kind: "notes", Value: total}}
		}
		i := slices.IndexFunc(cuadernos, func(c api.Notebook) bool { return c.ArchivedAt == nil })
		if i < 0 || cuadernos[i].Encrypted {
			return d, nil
		}
		notas, err := n.ListNotes(ctx, cuadernos[i].ID)
		if err != nil {
			return widgets.Detail{}, err
		}
		d.Items = itemsDeNotas(cuadernos[i], notas)
		return d, nil
	}
}

func totalDeNotas(lista []api.Notebook) (int64, bool) {
	ids := make(map[string]struct{}, len(lista))
	var total int64
	for _, c := range lista {
		if c.ArchivedAt != nil {
			continue
		}
		if _, repetido := ids[c.ID]; repetido {
			continue
		}
		if c.NoteCount < 0 || int64(c.NoteCount) > 1_000_000_000_000-total {
			return 0, false
		}
		ids[c.ID] = struct{}{}
		total += int64(c.NoteCount)
	}
	return total, true
}

// itemsDeNotas: las fijadas delante y el resto de la más reciente a la más antigua.
func itemsDeNotas(c api.Notebook, lista []api.Note) []widgets.Item {
	if c.Encrypted {
		return nil
	}
	candidatas := make([]api.Note, 0, len(lista))
	for _, n := range lista {
		if n.ContentCipher == "" && n.CancelledAt == nil {
			candidatas = append(candidatas, n)
		}
	}
	slices.SortStableFunc(candidatas, func(a, b api.Note) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
	items := make([]widgets.Item, 0, min(len(candidatas), widgets.MaxItems))
	for _, n := range candidatas {
		titulo := strings.TrimSpace(n.Title)
		if titulo == "" && n.Kind == "task" {
			titulo = strings.TrimSpace(htmltext.Text(n.Content))
		}
		if titulo == "" {
			continue
		}
		insignia := ""
		switch {
		case n.Kind == "task" && n.Checked:
			insignia = "✓"
		case n.Kind == "task":
			insignia = "○"
		case n.Pinned:
			insignia = "★"
		}
		items = append(items, widgets.Item{
			Title: titulo, Subtitle: c.Title, Badge: insignia, StartAt: unix(n.UpdatedAt),
			Color: c.Color, Unread: n.Kind == "task" && !n.Checked,
		})
		if llenos(items) {
			break
		}
	}
	return widgets.SanitizeItems(items)
}
