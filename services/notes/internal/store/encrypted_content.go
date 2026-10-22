// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

type noteWriteState struct {
	note                                 domain.Note
	byNotebook, encrypted, cryptoMatches bool
}

func normalizeNewNoteWrite(ctx context.Context, tx pgx.Tx, tenant string, notebookID uuid.UUID, sub string, in *domain.NoteInput) error {
	var encrypted, cryptoMatches bool
	err := tx.QueryRow(ctx, `SELECT nb.encrypted, COALESCE(nb.crypto=$4::jsonb, false) FROM notebooks nb
		WHERE nb.tenant=$1 AND nb.id=$2 AND nb.deleted_at IS NULL
		  AND (nb.owner_sub=$3 OR nb.visibility='organization' OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id=nb.id AND e.sujeto=$3))
		FOR UPDATE OF nb`, tenant, notebookID, sub, safeExpectedCrypto(in.ExpectedCrypto)).Scan(&encrypted, &cryptoMatches)
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if encrypted || in.ContentCipher != "" {
		if err := in.Normalize(encrypted); err != nil {
			return err
		}
	}
	if encrypted && !cryptoMatches {
		return domain.ErrNotebookCryptoChanged
	}
	return nil
}

// Invalid input must never reach a PostgreSQL jsonb cast. A missing or malformed
// expected value cannot match the public encryption parameters under the lock.
func safeExpectedCrypto(raw json.RawMessage) []byte {
	if len(raw) == 0 || len(raw) > 4096 || !json.Valid(raw) {
		return nil
	}
	return []byte(raw)
}

// Lock the notebook as well as the note so a concurrent key/flag change cannot
// slip between the encryption check and the write. The permission predicate is
// shared by ordinary updates and migration; an inaccessible note is always 404.
func lockNoteWrite(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, sub string, expectedCrypto json.RawMessage) (noteWriteState, error) {
	var state noteWriteState
	note, err := scanNote(tx.QueryRow(ctx, `
		SELECT `+noteCols+`, `+escribiblePorCuaderno("$3")+`,
		       nb.encrypted, COALESCE(nb.crypto=$4::jsonb, false)
		  FROM notes n JOIN notebooks nb ON nb.id=n.notebook_id AND nb.deleted_at IS NULL
		 WHERE n.tenant=$1 AND n.id=$2 AND n.deleted_at IS NULL
		   AND `+notaEscribible("$3")+`
		 FOR UPDATE OF n, nb`, tenant, id, sub, safeExpectedCrypto(expectedCrypto)), tenant,
		&state.byNotebook, &state.encrypted, &state.cryptoMatches)
	if isNoRows(err) {
		return state, domain.ErrNotFound
	}
	state.note = note
	return state, err
}

// Preserve only controls authored by the service, never arbitrary legacy Data.
const taskControls = `jsonb_strip_nulls(jsonb_build_object(
	'repeated_into', n.data->'repeated_into',
	'carried_from', n.data->'carried_from',
	'carry_count', n.data->'carry_count'))`

func (s *Store) MigrateEncryptedContent(ctx context.Context, id uuid.UUID, sub string, in domain.EncryptedContentInput) (domain.Note, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.Note
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		state, err := lockNoteWrite(ctx, tx, t.Slug, id, sub, in.ExpectedCrypto)
		if err != nil {
			return err
		}
		if !state.encrypted {
			return domain.ErrEncrypted
		}
		if err := in.Normalize(); err != nil {
			return err
		}
		if !state.cryptoMatches {
			return domain.ErrNotebookCryptoChanged
		}
		if !state.note.UpdatedAt.Equal(in.ExpectedUpdatedAt) {
			return domain.ErrNoteChanged
		}
		oldID := (&domain.NoteInput{Data: state.note.Data}).ClientID()
		newID := (&domain.NoteInput{Data: in.Data}).ClientID()
		if oldID != "" && newID != "" && oldID != newID {
			return domain.ErrInvalidInput
		}
		note, err := scanNote(tx.QueryRow(ctx, `
			UPDATE notes n SET title='', content='', content_cipher=$3,
			       data=$4::jsonb || `+taskControls+` ||
			            CASE WHEN n.data ? 'client_id' THEN jsonb_build_object('client_id', n.data->'client_id') ELSE '{}'::jsonb END,
			       indexable=false, updated_at=clock_timestamp()
			 WHERE n.tenant=$1 AND n.id=$2 RETURNING `+noteCols,
			t.Slug, id, in.ContentCipher, []byte(in.Data)), t.Slug)
		if err != nil {
			return err
		}
		notes := []domain.Note{note}
		if err := s.adjuntarEnPaquete(ctx, tx, t.Slug, notes); err != nil {
			return err
		}
		out = notes[0]
		return nil
	})
	return out, err
}

// A capture ID makes offline retries idempotent, including two concurrent clients.
func existingCapture(ctx context.Context, tx pgx.Tx, tenant string, notebookID uuid.UUID, sub string, in domain.NoteInput) (domain.Note, bool, error) {
	cid := in.ClientID()
	if cid == "" {
		return domain.Note{}, false, nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant+"/"+notebookID.String()+"/"+sub+"/"+cid); err != nil {
		return domain.Note{}, false, err
	}
	got, err := scanNote(tx.QueryRow(ctx, `SELECT `+noteColsBase+` FROM notes
		WHERE tenant=$1 AND notebook_id=$2 AND owner_sub=$3 AND deleted_at IS NULL
		  AND data->>'client_id'=$4 LIMIT 1`, tenant, notebookID, sub, cid), tenant)
	if isNoRows(err) {
		return domain.Note{}, false, nil
	}
	if err != nil {
		return domain.Note{}, false, err
	}
	got.Links = in.Links
	return got, true, nil
}
