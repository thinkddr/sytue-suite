// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// guardarVersion guarda cómo estaba la nota ANTES del guardado en curso, si toca
// (domain.GuardaVersion), y poda lo que sobre. Corre en la transacción del guardado y
// con la nota ya bloqueada: dos guardados a la vez no abren dos versiones.
func guardarVersion(ctx context.Context, tx pgx.Tx, tenant, sub string, antes domain.Note, in domain.NoteInput) error {
	var ultima *domain.UltimaVersion
	var u domain.UltimaVersion
	err := tx.QueryRow(ctx, `
		SELECT replaced_by, created_at FROM note_versions
		 WHERE tenant = $1 AND note_id = $2
		 ORDER BY created_at DESC LIMIT 1`, tenant, antes.ID).Scan(&u.ReplacedBy, &u.CreatedAt)
	switch {
	case err == nil:
		ultima = &u
	case !isNoRows(err):
		return err
	}
	if !domain.GuardaVersion(antes, in, ultima, sub, time.Now()) {
		return nil
	}
	// `replaced_by` es quien guardó ENCIMA (la persona de esta tanda), que es lo
	// único cierto: quién dejó la nota así no se registra en `notes`.
	if _, err := tx.Exec(ctx, `
		INSERT INTO note_versions (note_id, tenant, replaced_by, title, content, content_cipher, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		antes.ID, tenant, sub, antes.Title, antes.Content, antes.ContentCipher, nullableJSON(antes.Data)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM note_versions v
		 WHERE v.tenant = $1 AND v.note_id = $2
		   AND (v.created_at < now() - make_interval(secs => $3)
		        OR v.id NOT IN (SELECT id FROM note_versions
		                         WHERE tenant = $1 AND note_id = $2
		                         ORDER BY created_at DESC LIMIT $4))`,
		tenant, antes.ID, domain.RetencionVersiones.Seconds(), domain.MaxVersiones)
	return err
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// Versiones son las versiones guardadas de una nota, de la más reciente a la más
// antigua, sin el texto (la lista solo enseña cuándo y quién). Solo para quien puede
// escribir la nota: volver a una versión es escribirla.
func (s *Store) Versiones(ctx context.Context, noteID uuid.UUID, sub string) ([]domain.NoteVersion, error) {
	t := tenancy.MustFrom(ctx)
	out := []domain.NoteVersion{}
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := notaConHistorial(ctx, tx, t.Slug, noteID, sub); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, note_id, replaced_by, title, created_at FROM note_versions
			 WHERE tenant = $1 AND note_id = $2 AND created_at >= now() - make_interval(secs => $3)
			 ORDER BY created_at DESC LIMIT $4`,
			t.Slug, noteID, domain.RetencionVersiones.Seconds(), domain.MaxVersiones)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.NoteVersion
			if err := rows.Scan(&v.ID, &v.NoteID, &v.ReplacedBy, &v.Title, &v.CreatedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// Version es una versión entera, para verla o volver a ella.
func (s *Store) Version(ctx context.Context, noteID, versionID uuid.UUID, sub string) (domain.NoteVersion, error) {
	t := tenancy.MustFrom(ctx)
	var v domain.NoteVersion
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := notaConHistorial(ctx, tx, t.Slug, noteID, sub); err != nil {
			return err
		}
		var data []byte
		e := tx.QueryRow(ctx, `
			SELECT id, note_id, replaced_by, title, content, content_cipher, data, created_at FROM note_versions
			 WHERE tenant = $1 AND note_id = $2 AND id = $3 AND created_at >= now() - make_interval(secs => $4)`,
			t.Slug, noteID, versionID, domain.RetencionVersiones.Seconds()).
			Scan(&v.ID, &v.NoteID, &v.ReplacedBy, &v.Title, &v.Content, &v.ContentCipher, &data, &v.CreatedAt)
		if isNoRows(e) {
			return domain.ErrNotFound
		}
		v.Data = data
		return e
	})
	return v, err
}

// notaConHistorial comprueba que `sub` puede escribir la nota (y que no está en la
// papelera): es quien puede ver su historial y volver a una versión.
func notaConHistorial(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string) error {
	var ok bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM notes n JOIN notebooks nb ON nb.id = n.notebook_id AND nb.deleted_at IS NULL
		 WHERE n.tenant = $1 AND n.id = $2 AND n.deleted_at IS NULL AND `+notaEscribible("$3"),
		tenant, id, sub).Scan(&ok)
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	return err
}
