// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"cmp"
	"slices"
	"strings"

	"golang.org/x/text/collate"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// OrdenAlbum es en qué orden se ven las tarjetas de «Álbumes». El servicio los da por
// última modificación, que es «recientes» y el de siempre; los otros dos son de lectura
// y no tocan nada en el servidor. Es el `ordenAlbumes.ts` de la web, con los mismos
// casos en testdata/orden-albumes.json.
type OrdenAlbum string

// Los tres órdenes, en el orden del selector.
const (
	OrdenAlbumReciente OrdenAlbum = "recent"
	OrdenAlbumTitulo   OrdenAlbum = "title"
	OrdenAlbumCuenta   OrdenAlbum = "count"
)

// OrdenesAlbum son las opciones de «Ordenar por».
var OrdenesAlbum = []OrdenAlbum{OrdenAlbumReciente, OrdenAlbumTitulo, OrdenAlbumCuenta}

// ClavePrefOrdenAlbumes es la preferencia que lo recuerda: la clave de `localStorage`
// de la web.
const ClavePrefOrdenAlbumes = "sytue.photos.ordenAlbumes"

// EsOrdenAlbum dice si s es uno de los tres órdenes.
func EsOrdenAlbum(s string) bool { return slices.Contains(OrdenesAlbum, OrdenAlbum(s)) }

// OrdenarAlbumes devuelve una copia ordenada. Cada orden desempata con el siguiente
// criterio que tiene sentido y, al final, con el id: dos álbumes iguales no pueden
// bailar entre recargas. El título se compara con el idioma de quien mira, sin mirar
// mayúsculas ni tildes y con los números como números («Viaje 2» antes que «Viaje
// 10»), que es el `Intl.Collator(…, {sensitivity: 'base', numeric: true})` de la web.
func OrdenarAlbumes(lista []api.Album, o OrdenAlbum, l i18n.Locale) []api.Album {
	col := collate.New(l.Tag(), collate.Loose, collate.Numeric)
	porTitulo := func(a, b api.Album) int {
		return col.CompareString(strings.TrimSpace(a.Title), strings.TrimSpace(b.Title))
	}
	porReciente := func(a, b api.Album) int { return b.UpdatedAt.Compare(a.UpdatedAt) }
	var f func(a, b api.Album) int
	switch o {
	case OrdenAlbumTitulo:
		f = func(a, b api.Album) int {
			return cmp.Or(porTitulo(a, b), porReciente(a, b), strings.Compare(a.ID, b.ID))
		}
	case OrdenAlbumCuenta:
		f = func(a, b api.Album) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), porTitulo(a, b), strings.Compare(a.ID, b.ID))
		}
	default:
		// El del servicio: `updated_at DESC, id DESC`.
		f = func(a, b api.Album) int { return cmp.Or(porReciente(a, b), strings.Compare(b.ID, a.ID)) }
	}
	out := slices.Clone(lista)
	slices.SortStableFunc(out, f)
	return out
}
