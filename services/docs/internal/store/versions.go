// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// CreateVersion registra una versión apuntando a un snapshot ya persistido.
func (s *Store) CreateVersion(ctx context.Context, docID, snapshotID, authorSub string, seq int64, label *string, auto bool) (*domain.Version, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var v domain.Version
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO doc_versions (document_id, tenant, snapshot_id, label, author_sub, seq, auto)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, document_id, snapshot_id, label, author_sub, seq, auto, created_at`,
			docID, t.Slug, snapshotID, label, authorSub, seq, auto).
			Scan(&v.ID, &v.DocumentID, &v.SnapshotID, &v.Label, &v.AuthorSub, &v.Seq, &v.Auto, &v.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("store: crear versión: %w", err)
	}
	return &v, nil
}

// ListVersions devuelve el historial de versiones de un documento (más reciente
// primero).
func (s *Store) ListVersions(ctx context.Context, docID string, limit int) ([]domain.Version, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := make([]domain.Version, 0, limit)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, document_id, snapshot_id, label, author_sub, seq, auto, created_at
			FROM doc_versions WHERE document_id = $1
			ORDER BY created_at DESC LIMIT $2`, docID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v domain.Version
			if err := rows.Scan(&v.ID, &v.DocumentID, &v.SnapshotID, &v.Label,
				&v.AuthorSub, &v.Seq, &v.Auto, &v.CreatedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar versiones: %w", err)
	}
	return out, nil
}

// GetVersion carga una versión por id.
func (s *Store) GetVersion(ctx context.Context, docID, versionID string) (*domain.Version, error) {
	var v domain.Version
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, document_id, snapshot_id, label, author_sub, seq, auto, created_at
			FROM doc_versions WHERE id = $1 AND document_id = $2`, versionID, docID).
			Scan(&v.ID, &v.DocumentID, &v.SnapshotID, &v.Label, &v.AuthorSub, &v.Seq, &v.Auto, &v.CreatedAt)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener versión: %w", err)
	}
	return &v, nil
}
