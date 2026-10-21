// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// ArchiveNotebook archiva un cuaderno con su rama, o la devuelve. Solo el dueño,
// como borrar: que un cuaderno de la organización se escriba entre varios no da
// a cualquiera el derecho a sacarlo del árbol de todos.
//
// Archivar pone a toda la rama la MISMA fecha (now() es fija en la transacción);
// desarchivar solo devuelve los que la comparten, así que un hijo que ya estaba
// archivado antes que su padre sigue archivado. Es lo que hace que la vuelta sea
// íntegra. Solo se devuelve la CABEZA de una rama archivada: un hijo con el padre
// aún archivado volvería colgando de uno que no está en el árbol, y no saldría ni
// en él ni en el Archivo (que lista cabezas). Los diarios no se archivan nunca
// (CHECK de la 0015): se saltan.
//
// UNION y no UNION ALL al bajar por la rama, como en DeleteNotebook: con un ciclo de
// `parent_id` la consulta acaba.
func (s *Store) ArchiveNotebook(ctx context.Context, id uuid.UUID, sub string, archivar bool) error {
	t := tenancy.MustFrom(ctx)
	sql := `
		WITH RECURSIVE rama AS (
		    SELECT id FROM notebooks
		     WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL
		       AND archived_at IS NULL AND daily_date = ''
		    UNION
		    SELECT h.id FROM notebooks h JOIN rama r ON h.parent_id = r.id
		     WHERE h.tenant=$1 AND h.deleted_at IS NULL
		)
		UPDATE notebooks SET archived_at = now(), updated_at = now()
		 WHERE tenant=$1 AND id IN (SELECT id FROM rama) AND archived_at IS NULL AND daily_date = ''`
	if !archivar {
		sql = `
		WITH RECURSIVE raiz AS (
		    SELECT id, archived_at FROM notebooks
		     WHERE tenant=$1 AND id=$2 AND owner_sub=$3 AND deleted_at IS NULL AND archived_at IS NOT NULL
		       AND NOT EXISTS (SELECT 1 FROM notebooks p
		                        WHERE p.tenant=$1 AND p.id = notebooks.parent_id
		                          AND p.deleted_at IS NULL AND p.archived_at IS NOT NULL)
		), rama AS (
		    SELECT id FROM raiz
		    UNION
		    SELECT h.id FROM notebooks h JOIN rama r ON h.parent_id = r.id
		     WHERE h.tenant=$1 AND h.deleted_at IS NULL
		)
		UPDATE notebooks SET archived_at = NULL, updated_at = now()
		 WHERE tenant=$1 AND id IN (SELECT id FROM rama) AND archived_at = (SELECT archived_at FROM raiz)`
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, sql, t.Slug, id, sub)
		if err != nil {
			return err
		}
		// Ni suyo, ni en el estado contrario, ni un diario, ni con el padre archivado:
		// lo mismo para quien pregunta.
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// PinnedNotes son las notas fijadas de todos los cuadernos que `sub` ve: la vista
// «Fijadas». Mismas verjas que la búsqueda salvo el índice (son notas, no un
// índice), y fuera lo archivado y lo cifrado.
//
// ponytail: sin las cifradas; su título y su texto son criptograma y aquí no hay
// clave. Si hacen falta, la vista tendría que pedir la contraseña de cada cuaderno.
func (s *Store) PinnedNotes(ctx context.Context, sub string, limit int) ([]Hit, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
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
			 WHERE n.tenant = $1 AND n.deleted_at IS NULL AND n.pinned
			   AND NOT nb.encrypted AND nb.archived_at IS NULL
			   AND (nb.owner_sub = $2 OR nb.visibility IN ('organization','public')
			        OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $2)
			        OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $2))
			   AND (n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $2)
			 ORDER BY n.updated_at DESC LIMIT $3`, t.Slug, sub, limit)
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
