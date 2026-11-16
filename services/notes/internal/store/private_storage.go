// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

var (
	ErrPrivateStorageFrozen     = errors.New("notes: legacy storage is read-only after migration starts")
	ErrPrivateStorageEpoch      = errors.New("notes: retained source does not match the migration")
	ErrPrivateStorageShared     = errors.New("notes: shared data needs encrypted recipient migration")
	ErrPrivateStorageAttachment = errors.New("notes: attachment content needs private import before migration")
)

const (
	privateNotesStateTable  = "notes_private_storage_states"
	maxPrivateNotesRowBytes = 32 << 20
)

type PrivateSourceBody struct {
	Data   []byte
	Size   int64
	SHA256 string
}

func PrivateStorageError(err error) error {
	var failure *pgconn.PgError
	if errors.As(err, &failure) && failure.Code == "PST01" {
		return ErrPrivateStorageFrozen
	}
	return err
}

func privateNotesScope(ctx context.Context, owner string) (storage.Scope, error) {
	tenant, err := tenancy.From(ctx)
	scope := storage.Scope{Tenant: tenant.Slug, Subject: owner, Product: "notes"}
	if err != nil || !scope.Valid() {
		return scope, domain.ErrForbidden
	}
	return scope, nil
}

func (s *Store) privateNotesTx(ctx context.Context, owner string, fn func(pgx.Tx) error) error {
	if _, err := privateNotesScope(ctx, owner); err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
			return err
		}
		return fn(tx)
	})
}

func privateNotesState(scope storage.Scope, state pg.PrivateStorageState, shared bool) (storage.AppMigrationState, error) {
	result := storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}, Shared: shared}
	return result, result.Validate(scope)
}

func privateNotesPreflight(ctx context.Context, tx pgx.Tx, scope storage.Scope) (bool, bool, error) {
	var shared, attachments bool
	err := tx.QueryRow(ctx, privateNotesPreflightSQL, scope.Tenant, scope.Subject).Scan(&shared, &attachments)
	return shared, attachments, err
}

func (s *Store) PrivateStorageState(ctx context.Context, owner string) (storage.AppMigrationState, error) {
	scope, err := privateNotesScope(ctx, owner)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	var result storage.AppMigrationState
	err = s.privateNotesTx(ctx, owner, func(tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, privateNotesStateTable, scope.Tenant, owner)
		if err != nil {
			return err
		}
		shared, _, err := privateNotesPreflight(ctx, tx, scope)
		if err != nil {
			return err
		}
		result, err = privateNotesState(scope, state, shared)
		return err
	})
	return result, err
}

// BeginPrivateStorageMigration preflights before closing legacy writes. Source
// capture, state transition and counts commit together; originals are retained.
func (s *Store) BeginPrivateStorageMigration(ctx context.Context, owner string, epoch uuid.UUID) (storage.AppMigrationState, error) {
	scope, err := privateNotesScope(ctx, owner)
	if err != nil {
		return storage.AppMigrationState{}, err
	}
	if epoch == uuid.Nil {
		return storage.AppMigrationState{}, ErrPrivateStorageEpoch
	}
	var result storage.AppMigrationState
	err = s.privateNotesTx(ctx, owner, func(tx pgx.Tx) error {
		if err := pg.EnsurePrivateStorageState(ctx, tx, privateNotesStateTable, scope.Tenant, owner, nil); err != nil {
			return err
		}
		state, err := pg.LockPrivateStorageState(ctx, tx, privateNotesStateTable, scope.Tenant, owner)
		if err != nil {
			return err
		}
		if state.Mode != "legacy" {
			if state.Mode != "frozen" || state.Epoch != epoch.String() {
				return ErrPrivateStorageEpoch
			}
			result, err = privateNotesState(scope, state, false)
			return err
		}
		shared, attachments, err := privateNotesPreflight(ctx, tx, scope)
		if err != nil {
			return err
		}
		if shared {
			return ErrPrivateStorageShared
		}
		if attachments {
			return ErrPrivateStorageAttachment
		}
		if _, err := tx.Exec(ctx, `UPDATE notes_private_storage_states SET mode='building',epoch=$3,started_at=clock_timestamp() WHERE tenant=$1 AND owner_sub=$2`, scope.Tenant, owner, epoch); err != nil {
			return err
		}
		captured, err := tx.Exec(ctx, privateNotesCaptureSQL, scope.Tenant, owner, epoch)
		if err != nil {
			return fmt.Errorf("notes: capture retained private source: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE notes_private_storage_states SET mode='frozen',source_rows=$4 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode='building'`, scope.Tenant, owner, epoch, captured.RowsAffected()); err != nil {
			return err
		}
		state, err = pg.ReadPrivateStorageState(ctx, tx, privateNotesStateTable, scope.Tenant, owner)
		if err != nil {
			return err
		}
		result, err = privateNotesState(scope, state, false)
		return err
	})
	return result, err
}

func privateNotesEpoch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) error {
	state, err := pg.ReadPrivateStorageState(ctx, tx, privateNotesStateTable, scope.Tenant, scope.Subject)
	if err != nil {
		return err
	}
	if epoch == uuid.Nil || state.Mode != "frozen" || state.Epoch != epoch.String() {
		return ErrPrivateStorageEpoch
	}
	return nil
}

func privateNotesKind(kind string) bool {
	switch kind {
	case "notebook", "note", "tag", "tag_state", "tag_mark", "note_link", "reminder", "audit":
		return true
	default:
		return false
	}
}

func (s *Store) PrivateStorageSourcePage(ctx context.Context, owner string, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	scope, err := privateNotesScope(ctx, owner)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	if limit < 1 || limit > storage.MaxLegacySourceRows || after != (storage.LegacySourceCursor{}) && (!storage.SourceCursorValid(after) || !privateNotesKind(after.Kind)) {
		return storage.StreamSourcePage{}, ErrPrivateStorageEpoch
	}
	result := storage.StreamSourcePage{Scope: scope, Epoch: epoch.String(), Rows: []storage.StreamSourceRow{}}
	err = s.privateNotesTx(ctx, owner, func(tx pgx.Tx) error {
		if err := privateNotesEpoch(ctx, tx, scope, epoch); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT kind,id,size_bytes,sha256 FROM notes_private_storage_snapshot
   WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND (kind,id)>($4,$5)
   ORDER BY kind,id LIMIT $6`, scope.Tenant, owner, epoch, after.Kind, after.ID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row storage.StreamSourceRow
			if err := rows.Scan(&row.Kind, &row.ID, &row.Size, &row.SHA256); err != nil {
				return err
			}
			result.Rows = append(result.Rows, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(result.Rows) > limit {
			result.Rows = result.Rows[:limit]
			last := result.Rows[limit-1]
			result.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
		}
		return result.Validate(scope, epoch.String(), after)
	})
	return result, err
}

func (s *Store) PrivateStorageSourceBody(ctx context.Context, owner string, epoch uuid.UUID, kind, id string) (PrivateSourceBody, error) {
	scope, err := privateNotesScope(ctx, owner)
	if err != nil {
		return PrivateSourceBody{}, err
	}
	if !privateNotesKind(kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return PrivateSourceBody{}, ErrPrivateStorageEpoch
	}
	var body PrivateSourceBody
	err = s.privateNotesTx(ctx, owner, func(tx pgx.Tx) error {
		if err := privateNotesEpoch(ctx, tx, scope, epoch); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT payload,size_bytes,sha256 FROM notes_private_storage_snapshot WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5`, scope.Tenant, owner, epoch, kind, id).Scan(&body.Data, &body.Size, &body.SHA256)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	})
	if err == nil && (int64(len(body.Data)) != body.Size || body.Size > maxPrivateNotesRowBytes) {
		clear(body.Data)
		return PrivateSourceBody{}, ErrPrivateStorageEpoch
	}
	return body, err
}

const privateNotesPreflightSQL = `WITH own_books AS MATERIALIZED(
 SELECT * FROM notebooks WHERE tenant=$1 AND owner_sub=$2
),own_notes AS MATERIALIZED(
 SELECT * FROM notes WHERE tenant=$1 AND owner_sub=$2
)
SELECT (
 EXISTS(SELECT 1 FROM own_books WHERE visibility IN('organization','public') OR public_token<>'' OR room_id<>'')
 OR EXISTS(SELECT 1 FROM notebooks b WHERE b.tenant=$1 AND b.owner_sub<>$2 AND b.deleted_at IS NULL AND b.visibility IN('organization','public'))
 OR EXISTS(SELECT 1 FROM own_books b JOIN notebooks p ON p.tenant=b.tenant AND p.id=b.parent_id WHERE p.owner_sub<>$2)
 OR EXISTS(SELECT 1 FROM own_notes n JOIN notebooks b ON b.tenant=n.tenant AND b.id=n.notebook_id WHERE b.owner_sub<>$2 OR n.visibility IN('organization','public'))
 OR EXISTS(SELECT 1 FROM notes n JOIN own_books b ON b.id=n.notebook_id WHERE n.tenant=$1 AND n.owner_sub<>$2)
 OR EXISTS(SELECT 1 FROM notebook_lectores p WHERE p.tenant=$1 AND (p.notebook_id IN(SELECT id FROM own_books) OR p.sujeto=$2))
 OR EXISTS(SELECT 1 FROM notebook_editores p WHERE p.tenant=$1 AND (p.notebook_id IN(SELECT id FROM own_books) OR p.sujeto=$2))
 OR EXISTS(SELECT 1 FROM notebook_peticiones_edicion p WHERE p.tenant=$1 AND (p.notebook_id IN(SELECT id FROM own_books) OR p.sujeto=$2))
 OR EXISTS(SELECT 1 FROM note_personas p WHERE p.tenant=$1 AND (p.note_id IN(SELECT id FROM own_notes) OR p.sujeto=$2))
 OR EXISTS(SELECT 1 FROM note_reminders p WHERE p.tenant=$1 AND p.note_id IN(SELECT id FROM own_notes) AND p.owner_sub<>$2)
 OR EXISTS(SELECT 1 FROM note_reminders p JOIN notes n ON n.tenant=p.tenant AND n.id=p.note_id WHERE p.tenant=$1 AND p.owner_sub=$2 AND n.owner_sub<>$2)
 OR EXISTS(SELECT 1 FROM note_tag_states p JOIN tags t ON t.tenant=p.tenant AND t.id=p.tag_id WHERE p.tenant=$1 AND p.note_id IN(SELECT id FROM own_notes) AND t.owner_sub<>$2)
 OR EXISTS(SELECT 1 FROM note_tag_states p JOIN tags t ON t.tenant=p.tenant AND t.id=p.tag_id JOIN notes n ON n.tenant=p.tenant AND n.id=p.note_id WHERE p.tenant=$1 AND t.owner_sub=$2 AND n.owner_sub<>$2)
),(EXISTS(SELECT 1 FROM own_notes WHERE kind IN('image','file','drive','doc'))
 OR EXISTS(SELECT 1 FROM note_links l JOIN own_notes n ON n.tenant=l.tenant AND n.id=l.note_id WHERE l.target_kind IN('drive_file','docs_document')))`

const privateNotesCaptureSQL = `WITH own_books AS MATERIALIZED(
 SELECT * FROM notebooks WHERE tenant=$1 AND owner_sub=$2
),own_notes AS MATERIALIZED(
 SELECT * FROM notes WHERE tenant=$1 AND owner_sub=$2
),own_tags AS MATERIALIZED(
 SELECT * FROM tags WHERE tenant=$1 AND owner_sub=$2
),source_rows AS(
 SELECT 'notebook' AS kind,b.id::text AS id,to_jsonb(b)-'tenant' AS payload FROM own_books b
 UNION ALL SELECT 'note',n.id::text,to_jsonb(n)-'tenant' FROM own_notes n
 UNION ALL SELECT 'tag',t.id::text,to_jsonb(t)-'tenant' FROM own_tags t
 UNION ALL SELECT 'tag_state',s.id::text,to_jsonb(s)-'tenant' FROM tag_states s JOIN own_tags t ON t.tenant=s.tenant AND t.id=s.tag_id
 UNION ALL SELECT 'tag_mark',encode(digest(convert_to(m.note_id::text||':'||m.tag_id::text,'UTF8'),'sha256'),'hex'),to_jsonb(m)-'tenant' FROM note_tag_states m JOIN own_notes n ON n.tenant=m.tenant AND n.id=m.note_id
 UNION ALL SELECT 'note_link',l.id::text,to_jsonb(l)-'tenant' FROM note_links l JOIN own_notes n ON n.tenant=l.tenant AND n.id=l.note_id
 UNION ALL SELECT 'reminder',encode(digest(convert_to(r.note_id::text||':'||r.owner_sub,'UTF8'),'sha256'),'hex'),to_jsonb(r)-'tenant' FROM note_reminders r WHERE r.tenant=$1 AND r.owner_sub=$2
 UNION ALL SELECT 'audit',a.id::text,to_jsonb(a)-'tenant' FROM audit_log a WHERE a.tenant=$1 AND (a.target IN(SELECT id::text FROM own_books) OR a.target IN(SELECT id::text FROM own_notes))
)
INSERT INTO notes_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload)
 SELECT $1,$2,$3,kind,id,convert_to(payload::text,'UTF8') FROM source_rows`
