// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// CreateComment inserta un comentario anclado a una referencia CRDT estable.
func (s *Store) CreateComment(ctx context.Context, p domain.CreateCommentParams) (*domain.Comment, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	// Pasar como json.RawMessage para que pgx lo codifique como jsonb (no bytea).
	anchor := json.RawMessage(p.Anchor)
	if len(anchor) == 0 {
		anchor = json.RawMessage(`{}`)
	}
	var c domain.Comment
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO doc_comments (document_id, tenant, parent_id, author_sub, anchor, body)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, document_id, parent_id, author_sub, anchor, body,
			          resolved, resolved_by, created_at, updated_at`,
			p.DocumentID, t.Slug, p.ParentID, p.AuthorSub, anchor, p.Body).
			Scan(&c.ID, &c.DocumentID, &c.ParentID, &c.AuthorSub, &c.Anchor, &c.Body,
				&c.Resolved, &c.ResolvedBy, &c.CreatedAt, &c.UpdatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("store: crear comentario: %w", err)
	}
	return &c, nil
}

// ListComments devuelve los comentarios vivos de un documento.
func (s *Store) ListComments(ctx context.Context, docID string, includeResolved bool) ([]domain.Comment, error) {
	out := make([]domain.Comment, 0, 16)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, document_id, parent_id, author_sub, anchor, body,
			       resolved, resolved_by, created_at, updated_at
			FROM doc_comments
			WHERE document_id = $1 AND deleted_at IS NULL
			  AND ($2 OR resolved = false)
			ORDER BY created_at ASC`, docID, includeResolved)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c domain.Comment
			if err := rows.Scan(&c.ID, &c.DocumentID, &c.ParentID, &c.AuthorSub,
				&c.Anchor, &c.Body, &c.Resolved, &c.ResolvedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar comentarios: %w", err)
	}
	return out, nil
}

// ResolveComment marca un comentario como resuelto/reabierto.
func (s *Store) ResolveComment(ctx context.Context, docID, commentID, bySub string, resolved bool) error {
	var by any
	if resolved {
		by = bySub
	}
	return s.execAffecting(ctx, "resolver comentario",
		`UPDATE doc_comments SET resolved = $3, resolved_by = $4, updated_at = now()
		 WHERE id = $1 AND document_id = $2 AND deleted_at IS NULL`,
		commentID, docID, resolved, by)
}

// DeleteComment hace soft-delete de un comentario o de una respuesta. Solo lo borra su
// autor o quien modera (`moderador`: editor o propietario del documento). La regla va en
// la propia sentencia, no en una lectura previa: así no hay carrera entre comprobar y
// borrar. Sin fila afectada, el comentario o no existe (ErrNotFound) o es de otra
// persona (ErrForbidden).
func (s *Store) DeleteComment(ctx context.Context, docID, commentID, bySub string, moderador bool) error {
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE doc_comments SET deleted_at = now(), updated_at = now()
			 WHERE id = $1 AND document_id = $2 AND deleted_at IS NULL
			   AND ((author_sub = $3 AND $3 <> '') OR $4)`,
			commentID, docID, bySub, moderador)
		if err != nil || tag.RowsAffected() > 0 {
			return err
		}
		var vivo int
		err = tx.QueryRow(ctx,
			`SELECT 1 FROM doc_comments WHERE id = $1 AND document_id = $2 AND deleted_at IS NULL`,
			commentID, docID).Scan(&vivo)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		return domain.ErrForbidden
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrForbidden) {
		return fmt.Errorf("store: borrar comentario: %w", err)
	}
	return err
}
