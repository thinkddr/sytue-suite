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

// RecordBlob registra los metadatos de un export/import materializado en S3.
// El binario ya debe estar subido a Object Storage (bucket docs-blobs) bajo
// blob_key; aquí solo se persiste la referencia.
func (s *Store) RecordBlob(ctx context.Context, b domain.Blob) (*domain.Blob, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	var out domain.Blob
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO doc_blobs
			    (document_id, tenant, format, blob_key, seq, size_bytes, content_type, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, document_id, format, blob_key, seq, size_bytes, content_type, created_by, created_at`,
			b.DocumentID, t.Slug, string(b.Format), b.BlobKey, b.Seq, b.SizeBytes, b.ContentType, b.CreatedBy).
			Scan(&out.ID, &out.DocumentID, &out.Format, &out.BlobKey, &out.Seq,
				&out.SizeBytes, &out.ContentType, &out.CreatedBy, &out.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("store: registrar blob: %w", err)
	}
	return &out, nil
}

// GetBlob carga los metadatos de un export por id (para descargarlo de S3).
func (s *Store) GetBlob(ctx context.Context, docID, blobID string) (*domain.Blob, error) {
	var b domain.Blob
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, document_id, format, blob_key, seq, size_bytes, content_type, created_by, created_at
			FROM doc_blobs WHERE id = $1 AND document_id = $2`, blobID, docID).
			Scan(&b.ID, &b.DocumentID, &b.Format, &b.BlobKey, &b.Seq,
				&b.SizeBytes, &b.ContentType, &b.CreatedBy, &b.CreatedAt)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener blob: %w", err)
	}
	return &b, nil
}

// ListBlobs lista los exports de un documento (más reciente primero).
func (s *Store) ListBlobs(ctx context.Context, docID string) ([]domain.Blob, error) {
	out := make([]domain.Blob, 0, 8)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, document_id, format, blob_key, seq, size_bytes, content_type, created_by, created_at
			FROM doc_blobs WHERE document_id = $1 ORDER BY created_at DESC`, docID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b domain.Blob
			if err := rows.Scan(&b.ID, &b.DocumentID, &b.Format, &b.BlobKey, &b.Seq,
				&b.SizeBytes, &b.ContentType, &b.CreatedBy, &b.CreatedAt); err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar blobs: %w", err)
	}
	return out, nil
}
