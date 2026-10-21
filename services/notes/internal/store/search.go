// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"html"
	"strings"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Hit es un resultado de búsqueda con lo justo para pintar una fila: el Hub lo
// traduce a su modelo canónico y la app de Notes lo usa tal cual.
type Hit struct {
	NoteID       string `json:"id"`
	NotebookID   string `json:"notebook_id"`
	NotebookName string `json:"notebook"`
	Title        string `json:"title"`
	Snippet      string `json:"snippet"`
	Kind         string `json:"kind"`
	UpdatedAt    string `json:"updated_at"`
	// Headings son los títulos (h1..h6) que hay DENTRO de la nota, en orden. Los
	// pide la herramienta de enlaces del editor: escribir el nombre de un título
	// de otra nota tiene que bastar para enlazarla, como en BasKet, y sin esto el
	// desplegable solo podría ofrecer notas enteras.
	Headings []string `json:"headings"`
}

// longitudFragmento es lo que mide el fragmento de un resultado, en letras: las 200
// que ya se enseñaban, ahora alrededor de lo buscado.
const longitudFragmento = 200

// EL ÍNDICE ES OPT-OUT POR NOTA Y POR CUADERNO
//
// Una nota entra en la búsqueda solo si su valor EFECTIVO de `indexable` es
// cierto: el suyo si lo tiene, y si no el de su cuaderno. Un cuaderno cifrado
// nunca entra (no hay texto que indexar) y una nota privada de otra persona
// tampoco, aunque esté en un cuaderno compartido.
//
// El filtro va en la CONSULTA y no en el que llama: es la diferencia entre «no se
// enseña» y «no se puede sacar».

// SearchNotes busca en las notas visibles e indexables para `sub`. Lo archivado
// solo sale si se pide (`archivados`): el Hub federa sin pedirlo y no lo ve.
func (s *Store) SearchNotes(ctx context.Context, sub, query string, limit int, archivados bool) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" || limit <= 0 {
		return nil, nil
	}
	if limit > 100 {
		limit = 100
	}
	// BUSCAR «#plan» ES PEDIR LAS NOTAS CON ESE HASHTAG, no las que dicen «plan»:
	// el índice no guarda el «#», así que por sí solo devolvería las dos cosas. El
	// índice sigue cribando (la palabra tiene que estar), `patron` aparta en SQL las
	// que no llevan «#plan» escrito, y quien decide es el mismo extractor que
	// cuenta los hashtags (llevaHashtag), fila a fila: «#plan» no es «#planes».
	tag, esHashtag := domain.HashtagDeConsulta(query)
	patron, tope := "", limit
	if esHashtag {
		// Un «_» de la etiqueta hace de comodín en el LIKE: deja pasar de más, que
		// es lo que se le pide a una criba.
		patron = "%#" + buscar.Plegar(tag) + "%"
		// ponytail: se piden más filas de las que se devuelven porque la criba deja
		// pasar prefijos («#planes»). Con más de 500 notas así, faltarían resultados.
		tope = 500
	}
	t := tenancy.MustFrom(ctx)
	var out []Hit
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT n.id::text, n.notebook_id::text, nb.title, n.title,
			       -- El texto entero (con tope) y no sus 200 primeras letras: el
			       -- fragmento se corta en Go alrededor de lo buscado (buscar.Fragmento).
			       left(regexp_replace(n.content, '<[^>]*>', ' ', 'g'), 20000),
			       n.kind, to_char(n.updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF:00'),
			       -- Los títulos de dentro, sin sus etiquetas internas y recortados: el
			       -- desplegable del editor los enseña tal cual.
			       (SELECT COALESCE(array_agg(left(regexp_replace(m[1], '<[^>]*>', '', 'g'), 120)), '{}')
			          FROM regexp_matches(n.content, '<h[1-6][^>]*>(.*?)</h[1-6]>', 'g') AS m)
			  FROM notes n
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant = $1 AND n.deleted_at IS NULL
			   AND NOT nb.encrypted
			   AND COALESCE(n.indexable, nb.indexable)
			   AND (nb.owner_sub = $2 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $2)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $2))
			   AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $2)
			   AND ($5 OR nb.archived_at IS NULL)
			   AND n.search @@ websearch_to_tsquery('simple', plegar($3))
			   AND ($6 = '' OR plegar(n.title || ' ' || n.content) LIKE $6)
			 ORDER BY ts_rank(n.search, websearch_to_tsquery('simple', plegar($3))) DESC, n.updated_at DESC
			 LIMIT $4`, t.Slug, sub, query, tope, archivados, patron)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() && len(out) < limit {
			var h Hit
			if err := rows.Scan(&h.NoteID, &h.NotebookID, &h.NotebookName, &h.Title,
				&h.Snippet, &h.Kind, &h.UpdatedAt, &h.Headings); err != nil {
				return err
			}
			if esHashtag && !llevaHashtag(h.Title, h.Snippet, tag) {
				continue
			}
			h.Snippet = buscar.Fragmento(html.UnescapeString(h.Snippet), query, longitudFragmento)
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}

// RecentNotes devuelve la actividad reciente de `sub` (búsqueda sin término). El
// Hub la usa para su carril de «reciente»; respeta las mismas verjas que la
// búsqueda salvo la de indexación, que ahí no aplica: son TUS notas, no un índice.
// Lo archivado, igual que en SearchNotes: solo si se pide.
func (s *Store) RecentNotes(ctx context.Context, sub string, limit int, archivados bool) ([]Hit, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	t := tenancy.MustFrom(ctx)
	var out []Hit
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT n.id::text, n.notebook_id::text, nb.title, n.title,
			       left(regexp_replace(n.content, '<[^>]*>', ' ', 'g'), 200),
			       n.kind, to_char(n.updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF:00')
			  FROM notes n
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant = $1 AND n.deleted_at IS NULL AND n.owner_sub = $2
			   AND ($4 OR nb.archived_at IS NULL)
			 ORDER BY n.updated_at DESC LIMIT $3`, t.Slug, sub, limit, archivados)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h Hit
			if err := rows.Scan(&h.NoteID, &h.NotebookID, &h.NotebookName, &h.Title,
				&h.Snippet, &h.Kind, &h.UpdatedAt); err != nil {
				return err
			}
			out = append(out, h)
		}
		return rows.Err()
	})
	return out, err
}

// NotesForTarget es la CONSULTA INVERSA de la referencia cruzada: dado un objeto
// de otra app (un evento de Calendar, una sala de Rooms, un documento de Docs),
// devuelve las notas que cuelgan de él y que quien pregunta puede ver.
//
// Es lo que hace bidireccional el enlace sin duplicarlo: Calendar no guarda nada,
// pregunta aquí con el token de quien mira, y la autorización sigue siendo la de
// Notes (una nota privada no aparece en el evento de otra persona).
func (s *Store) NotesForTarget(ctx context.Context, sub, targetKind, targetID string, limit int) ([]domain.Note, error) {
	if !domain.ValidTargetKind(targetKind) {
		return nil, domain.ErrInvalidInput
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	t := tenancy.MustFrom(ctx)
	var out []domain.Note
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+noteCols+`
			  FROM note_links nl
			  JOIN notes n ON n.id = nl.note_id AND n.deleted_at IS NULL
			  JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
			 WHERE nl.tenant = $1 AND nl.target_kind = $2 AND nl.target_id = $3
			   AND (nb.owner_sub = $4 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $4)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $4))
			   AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $4)
			 ORDER BY n.updated_at DESC LIMIT $5`, t.Slug, targetKind, targetID, sub, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			n, err := scanNote(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, n)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return s.adjuntarEnPaquete(ctx, tx, t.Slug, out)
	})
	return out, err
}
