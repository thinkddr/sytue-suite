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

// LoadState carga el estado consolidado del documento aplicando snapshot inline
// + operaciones posteriores. Para snapshots en S3 devuelve BlobKey y BaseSeq sin
// aplicar (la capa superior descarga e hidrata).
//
//nolint:gocognit // reconstrucción snapshot+ops en una pasada; el flujo lineal es más legible que trocearlo.
func (s *Store) LoadState(ctx context.Context, docID string) (*domain.LoadedState, error) {
	res := &domain.LoadedState{State: domain.NewState()}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		var curSeq int64
		err := tx.QueryRow(ctx,
			`SELECT seq FROM documents WHERE id = $1 AND deleted_at IS NULL`, docID).Scan(&curSeq)
		if isNoRows(err) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		res.Seq = curSeq

		var (
			rawState []byte
			blobKey  *string
			baseSeq  int64
			haveSnap bool
		)
		err = tx.QueryRow(ctx, `
			SELECT state, blob_key, seq
			FROM doc_snapshots
			WHERE document_id = $1
			ORDER BY seq DESC
			LIMIT 1`, docID).Scan(&rawState, &blobKey, &baseSeq)
		switch {
		case isNoRows(err):
			haveSnap = false
		case err != nil:
			return err
		default:
			haveSnap = true
		}

		if haveSnap {
			res.BaseSeq = baseSeq
			if blobKey != nil && *blobKey != "" {
				res.BlobKey = *blobKey
				return nil // hidratación desde S3 fuera de la tx
			}
			st, err := domain.StateFromJSON(rawState)
			if err != nil {
				return err
			}
			res.State = st
		}

		// Aplicar operaciones posteriores al snapshot (o todas si no hay snapshot).
		const q = `
			SELECT op FROM doc_operations
			WHERE document_id = $1 AND seq > $2
			ORDER BY seq ASC`
		rows, err := tx.Query(ctx, q, docID, res.BaseSeq)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var op domain.Op
			if err := json.Unmarshal(raw, &op); err != nil {
				return fmt.Errorf("op corrupta: %w", err)
			}
			if _, err := res.State.Apply(op); err != nil {
				return err
			}
		}
		return rows.Err()
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("store: cargar estado: %w", err)
	}
	res.State.Seq = res.Seq
	return res, nil
}

// SaveSnapshot persiste un snapshot consolidado y lo marca como el actual del
// documento (documents.snapshot_id). Compacta la carga inicial futura.
func (s *Store) SaveSnapshot(ctx context.Context, p domain.SaveSnapshotParams) (string, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return "", err
	}
	var id string
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var state any
		if len(p.State) > 0 {
			state = json.RawMessage(p.State) // jsonb (no bytea)
		}
		var blob any
		if p.BlobKey != "" {
			blob = p.BlobKey
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO doc_snapshots (document_id, tenant, seq, state, blob_key, size_bytes)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id`,
			p.DocumentID, t.Slug, p.Seq, state, blob, p.SizeBytes).Scan(&id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE documents SET snapshot_id = $2 WHERE id = $1`, p.DocumentID, id)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("store: guardar snapshot: %w", err)
	}
	return id, nil
}

// SnapshotState carga y materializa el estado CRDT de un snapshot concreto (por
// id), hidratando desde S3 si el estado está externalizado. Devuelve el estado y
// la seq que cubre. Es la base de la restauración de versiones (el snapshot es
// inmutable, así que no hay que aplicar operaciones posteriores).
func (s *Store) SnapshotState(ctx context.Context, snapshotID string, hydrate func(ctx context.Context, blobKey string) (*domain.State, error)) (*domain.State, int64, error) {
	snap, err := s.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return nil, 0, err
	}
	if snap.BlobKey != "" {
		if hydrate == nil {
			return nil, 0, fmt.Errorf("store: snapshot %s en S3 sin hidratador", snapshotID)
		}
		st, err := hydrate(ctx, snap.BlobKey)
		if err != nil {
			return nil, 0, err
		}
		st.Seq = snap.Seq
		return st, snap.Seq, nil
	}
	st, err := domain.StateFromJSON(snap.State)
	if err != nil {
		return nil, 0, fmt.Errorf("store: snapshot %s inválido: %w", snapshotID, err)
	}
	st.Seq = snap.Seq
	return st, snap.Seq, nil
}

// GetSnapshot carga un snapshot por id (para restaurar versiones).
func (s *Store) GetSnapshot(ctx context.Context, id string) (*domain.Snapshot, error) {
	var snap domain.Snapshot
	var rawState []byte
	var blobKey *string
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, document_id, seq, state, blob_key, size_bytes, created_at
			FROM doc_snapshots WHERE id = $1`, id).
			Scan(&snap.ID, &snap.DocumentID, &snap.Seq, &rawState, &blobKey, &snap.SizeBytes, &snap.CreatedAt)
	})
	if isNoRows(err) {
		return nil, domain.ErrSnapshotMissing
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener snapshot: %w", err)
	}
	snap.State = rawState
	if blobKey != nil {
		snap.BlobKey = *blobKey
	}
	return &snap, nil
}
