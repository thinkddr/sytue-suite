// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"cmp"
	"context"
	"html"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Hashtag es un `#hashtag` con las notas que lo llevan.
type Hashtag struct {
	// Tag va sin el `#` y tal como se escribió en la nota más reciente que lo usa:
	// «#Reunión» y «#reunion» son el mismo (se comparan plegados), y se enseña el
	// último que alguien tecleó.
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Topes de la lista: cuántas notas se leen y cuántos hashtags se devuelven.
const (
	maxNotasConHashtag = 5000
	maxHashtags        = 200
)

// textoConHashtags es el texto del que se leen los hashtags de una nota: su título
// y su contenido ya sin etiquetas HTML (lo quita la consulta), con las entidades
// resueltas. La lista y la búsqueda por `#palabra` pasan las dos por aquí.
func textoConHashtags(titulo, contenido string) string {
	return titulo + "\n" + html.UnescapeString(contenido)
}

// Hashtags lista los hashtags de las notas que `sub` ve, con cuántas notas lleva
// cada uno, del más usado al menos. Mismas verjas que SearchNotes —índice
// incluido: pulsar un hashtag ES una búsqueda, y contar una nota que luego no
// sale sería delatarla— y sin lo archivado.
//
// ponytail: se leen del texto en cada petición, sin columna ni índice; el filtro
// `~ '#…'` recorre las notas visibles y el extractor corre en Go sobre las
// maxNotasConHashtag más recientes. Si un tenant llega a ese volumen o la lista
// tarda, el paso es una columna `hashtags text[]` con GIN mantenida al guardar.
func (s *Store) Hashtags(ctx context.Context, sub string) ([]Hashtag, error) {
	t := tenancy.MustFrom(ctx)
	porClave := map[string]*Hashtag{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT n.title, left(regexp_replace(n.content, '<[^>]*>', ' ', 'g'), 20000)
			  FROM notes n
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant = $1 AND n.deleted_at IS NULL
			   AND NOT nb.encrypted AND nb.archived_at IS NULL
			   AND COALESCE(n.indexable, nb.indexable)
			   AND (nb.owner_sub = $2 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $2)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $2))
			   AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $2)
			   -- Criba gruesa: un «#» que no abra una entidad (&#39;) ni un número.
			   -- Quién es hashtag de verdad lo decide domain.Hashtags.
			   AND (n.title || ' ' || n.content) ~ '#[^0-9[:space:]<&;#]'
			 ORDER BY n.updated_at DESC LIMIT $3`, t.Slug, sub, maxNotasConHashtag)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var titulo, contenido string
			if err := rows.Scan(&titulo, &contenido); err != nil {
				return err
			}
			// Una nota cuenta una vez por hashtag, lo repita lo que lo repita.
			vistos := map[string]bool{}
			for _, tag := range domain.Hashtags(textoConHashtags(titulo, contenido)) {
				clave := buscar.Plegar(tag)
				if vistos[clave] {
					continue
				}
				vistos[clave] = true
				if h := porClave[clave]; h != nil {
					h.Count++
				} else {
					porClave[clave] = &Hashtag{Tag: tag, Count: 1}
				}
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	out := make([]Hashtag, 0, len(porClave))
	for _, h := range porClave {
		out = append(out, *h)
	}
	slices.SortFunc(out, func(a, b Hashtag) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(buscar.Plegar(a.Tag), buscar.Plegar(b.Tag)))
	})
	return out[:min(len(out), maxHashtags)], nil
}

// llevaHashtag dice si el texto de una nota lleva el hashtag `tag`, plegado.
func llevaHashtag(titulo, contenido, tag string) bool {
	clave := buscar.Plegar(tag)
	return slices.ContainsFunc(domain.Hashtags(textoConHashtags(titulo, contenido)),
		func(t string) bool { return buscar.Plegar(t) == clave })
}
