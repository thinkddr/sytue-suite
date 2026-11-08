// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"cmp"
	"context"
	"strings"
	"unicode"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// Contacts lee las libretas y los contactos de la INICIAL (la predeterminada, o la
// primera), aunque la app tenga otra abierta. Peticiones: 2.
func Contacts(_ Comunes, cs *api.Contacts) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		libretas, err := cs.AddressBooks(ctx)
		if err != nil {
			return widgets.Detail{}, err
		}
		ids := make(map[string]struct{}, len(libretas))
		for _, l := range libretas {
			ids[l.ID] = struct{}{}
		}
		d := widgets.Detail{Counters: []widgets.Counter{{Kind: "address_books", Value: int64(len(ids))}}}
		inicial := libretaInicial(libretas)
		if inicial == nil {
			return d, nil
		}
		lista, err := cs.InBook(ctx, inicial.ID)
		if err != nil {
			return widgets.Detail{}, err
		}
		d.Items = itemsDeContactos(lista)
		return d, nil
	}
}

func libretaInicial(libretas []api.AddressBook) *api.AddressBook {
	for i := range libretas {
		if libretas[i].IsDefault {
			return &libretas[i]
		}
	}
	if len(libretas) > 0 {
		return &libretas[0]
	}
	return nil
}

func itemsDeContactos(lista []api.Contact) []widgets.Item {
	items := make([]widgets.Item, 0, min(len(lista), widgets.MaxItems))
	for _, c := range lista {
		nombre := strings.TrimSpace(c.DisplayName())
		if nombre == "" {
			continue
		}
		subtitulo := strings.TrimSpace(cmp.Or(c.Organization, c.Title))
		if subtitulo == "" && len(c.Emails) > 0 {
			subtitulo = strings.TrimSpace(c.Emails[0].Email)
		}
		items = append(items, widgets.Item{
			Title: nombre, Subtitle: subtitulo, Badge: monograma(nombre), StartAt: unix(c.UpdatedAt),
		})
		if llenos(items) {
			break
		}
	}
	return widgets.SanitizeItems(items)
}

// monograma son las iniciales de las dos primeras palabras.
func monograma(s string) string {
	var r []rune
	for _, p := range strings.Fields(s) {
		for _, ch := range p {
			if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
				r = append(r, unicode.ToUpper(ch))
				break
			}
		}
		if len(r) == 2 {
			break
		}
	}
	return string(r)
}
