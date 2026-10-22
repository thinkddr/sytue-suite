// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Leave room for JSON framing, escaping, and metadata. The HTTP encoder also
// checks the exact representation before writing any headers or response bytes.
const notePageBudget = 24 << 20

type NotePage struct {
	Notes      []domain.Note `json:"notes"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func (s *Store) ListNotesPage(ctx context.Context, notebookID uuid.UUID, sub string, limit int, after *uuid.UUID) (NotePage, error) {
	if limit < 1 || limit > 100 {
		return NotePage{}, domain.ErrInvalidInput
	}
	return s.boundedNotes(ctx, notebookID, sub, limit, after, false)
}

// Legacy readers expect the whole list. Never give them a partial list without
// a cursor they understand; small notebooks of over 100 notes remain readable.
func (s *Store) ListNotesLegacyBounded(ctx context.Context, notebookID uuid.UUID, sub string) (NotePage, error) {
	page, err := s.boundedNotes(ctx, notebookID, sub, 1000, nil, true)
	if err == nil && page.NextCursor != "" {
		return NotePage{}, domain.ErrNotePaginationRequired
	}
	return page, err
}

func (s *Store) boundedNotes(ctx context.Context, notebookID uuid.UUID, sub string, limit int, after *uuid.UUID, legacy bool) (NotePage, error) {
	order, budget, expansion := "n.id", int64(notePageBudget), int64(6)
	if legacy {
		order, budget, expansion = "n.pinned DESC, n.col, n.position, n.created_at, n.id", domain.MaxNotePageBytes, 1
	}
	t := tenancy.MustFrom(ctx)
	page := NotePage{Notes: []domain.Note{}}
	err := s.pool.InTxOpts(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, t.Slug); err != nil {
			return err
		}
		// Only identifiers and byte counts cross the connection here. In
		// particular, never fetch limit+1 large ciphertexts just to discard them.
		// Per-item overhead accounts for the decoded slices/structs too; metadata
		// is measured in the same repeatable snapshot used by adjuntarEnPaquete.
		rows, err := tx.Query(ctx, `WITH candidates AS MATERIALIZED (
			SELECT n.id,
			       2048::bigint + octet_length(n.content_cipher) + 2::bigint*octet_length(n.data::text)
			       + octet_length(n.title) + octet_length(n.content) + octet_length(n.owner_sub)
			       + COALESCE((SELECT sum(512::bigint + octet_length(t.name) + octet_length(ts.name)
			                           + octet_length(ts.emoji) + octet_length(ts.color))
			                    FROM note_tag_states nts JOIN tags t ON t.id=nts.tag_id
			                    JOIN tag_states ts ON ts.id=nts.tag_state_id
			                   WHERE nts.tenant=$1 AND nts.note_id=n.id),0)
			       + COALESCE((SELECT sum(512::bigint + octet_length(l.target_kind) + octet_length(l.target_id)
			                           + octet_length(l.label) + octet_length(l.url))
			                    FROM note_links l WHERE l.tenant=$1 AND l.note_id=n.id),0) AS memory_bytes
			  FROM notes n JOIN notebooks nb ON nb.id=n.notebook_id AND nb.deleted_at IS NULL
			 WHERE n.tenant=$1 AND n.notebook_id=$2 AND n.deleted_at IS NULL
			   AND `+legiblePorCuaderno("$3")+`
			   AND ($4::uuid IS NULL OR n.id>$4)
			 ORDER BY n.id LIMIT $5
		)
		SELECT id, memory_bytes, sum(memory_bytes) OVER (ORDER BY id) AS memory_total
		  FROM candidates ORDER BY id`, t.Slug, notebookID, sub, after, limit+1)
		if err != nil {
			return err
		}
		ids, next, err := notePageIDs(rows, limit, budget, expansion)
		if err != nil || len(ids) == 0 {
			return err
		}
		page.NextCursor = next
		rows, err = tx.Query(ctx, `SELECT `+noteCols+` FROM notes n WHERE n.tenant=$1 AND n.id=ANY($2) ORDER BY `+order, t.Slug, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			note, err := scanNote(rows, t.Slug)
			if err != nil {
				rows.Close()
				return err
			}
			page.Notes = append(page.Notes, note)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		return s.adjuntarEnPaquete(ctx, tx, t.Slug, page.Notes)
	})
	return page, err
}

func notePageIDs(rows pgx.Rows, limit int, budget, expansion int64) ([]uuid.UUID, string, error) {
	defer rows.Close()
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		var memoryBytes, memoryTotal int64
		if err := rows.Scan(&id, &memoryBytes, &memoryTotal); err != nil {
			return nil, "", err
		}
		if len(ids) == 0 && memoryBytes > domain.MaxNotePageBytes {
			return nil, "", domain.ErrNotePageTooLarge
		}
		if len(ids) == limit || (len(ids) > 0 && memoryTotal > budget/expansion) {
			return ids, ids[len(ids)-1].String(), nil
		}
		ids = append(ids, id)
	}
	return ids, "", rows.Err()
}
