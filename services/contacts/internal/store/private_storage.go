// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

var (
	ErrPrivateStorageScope  = errors.New("contacts: invalid private storage scope")
	ErrPrivateStorageEpoch  = errors.New("contacts: retained snapshot does not match")
	ErrPrivateStorageFrozen = errors.New("contacts: legacy writes are frozen")
)

type PrivateStorageState struct {
	Version int `json:"v"`
	storage.LegacyMigrationState
	Shared bool `json:"shared"`
}

func PrivateStorageError(err error) error {
	var failure *pgconn.PgError
	if errors.As(err, &failure) && failure.Code == "PST01" {
		return ErrPrivateStorageFrozen
	}
	return err
}

func (s *Store) privateSourceTx(ctx context.Context, owner string, operation func(storage.Scope, pgx.Tx) error) error {
	tenant, err := tenancy.From(ctx)
	scope := storage.Scope{Tenant: tenant.Slug, Subject: owner, Product: "contacts"}
	if err != nil || !scope.Valid() {
		return ErrPrivateStorageScope
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.owner_sub',$1,true)", owner); err != nil {
			return err
		}
		return operation(scope, tx)
	})
}

func sourceState(scope storage.Scope, state pg.PrivateStorageState) PrivateStorageState {
	return PrivateStorageState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}}
}

func (s *Store) PrivateStorageState(ctx context.Context, owner string) (PrivateStorageState, error) {
	var result PrivateStorageState
	err := s.privateSourceTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, "contacts_private_storage_states", scope.Tenant, owner)
		if err == nil {
			result = sourceState(scope, state)
		}
		return err
	})
	return result, err
}

// BeginPrivateStorageMigration takes the exclusive state row lock paired with
// every legacy table's shared writer lock. Capture and publication of the frozen
// state are one transaction; a failed or interrupted capture leaves no fence.
func (s *Store) BeginPrivateStorageMigration(ctx context.Context, owner string, epoch uuid.UUID) (PrivateStorageState, error) {
	if epoch == uuid.Nil {
		return PrivateStorageState{}, ErrPrivateStorageEpoch
	}
	var result PrivateStorageState
	err := s.privateSourceTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		if err := pg.EnsurePrivateStorageState(ctx, tx, "contacts_private_storage_states", scope.Tenant, owner, nil); err != nil {
			return err
		}
		state, err := pg.LockPrivateStorageState(ctx, tx, "contacts_private_storage_states", scope.Tenant, owner)
		if err != nil {
			return err
		}
		if state.Mode != "legacy" {
			if state.Epoch != epoch.String() || (state.Mode != "frozen" && state.Mode != "private") {
				return ErrPrivateStorageEpoch
			}
			result = sourceState(scope, state)
			return nil
		}
		if _, err = tx.Exec(ctx, `UPDATE contacts_private_storage_states SET mode='building',epoch=$3,started_at=clock_timestamp() WHERE tenant=$1 AND owner_sub=$2`, scope.Tenant, owner, epoch); err != nil {
			return err
		}
		captured, err := tx.Exec(ctx, contactsSourceCaptureSQL, scope.Tenant, owner, epoch)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE contacts_private_storage_states SET mode='frozen',source_rows=$4,source_blobs=0 WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode='building'`, scope.Tenant, owner, epoch, captured.RowsAffected()); err != nil {
			return err
		}
		state, err = pg.ReadPrivateStorageState(ctx, tx, "contacts_private_storage_states", scope.Tenant, owner)
		if err == nil {
			result = sourceState(scope, state)
		}
		return err
	})
	return result, err
}

func privateSourceEpoch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) error {
	state, err := pg.ReadPrivateStorageState(ctx, tx, "contacts_private_storage_states", scope.Tenant, scope.Subject)
	if err != nil {
		return err
	}
	if epoch == uuid.Nil || state.Epoch != epoch.String() || (state.Mode != "frozen" && state.Mode != "private") {
		return ErrPrivateStorageEpoch
	}
	return nil
}

func (s *Store) PrivateStorageSourcePage(ctx context.Context, owner string, epoch uuid.UUID, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	result := storage.StreamSourcePage{Epoch: epoch.String(), Rows: []storage.StreamSourceRow{}}
	if epoch == uuid.Nil || after != (storage.LegacySourceCursor{}) && (!storage.SourceCursorValid(after) || !model.SourceKind(after.Kind)) {
		return result, ErrPrivateStorageEpoch
	}
	err := s.privateSourceTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		result.Scope = scope
		if err := privateSourceEpoch(ctx, tx, scope, epoch); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT kind,id,octet_length(payload),sha256 FROM contacts_private_storage_snapshot WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND (kind,id)>($4,$5) ORDER BY kind,id LIMIT $6`, scope.Tenant, owner, epoch, after.Kind, after.ID, storage.MaxLegacySourceRows+1)
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
		if len(result.Rows) > storage.MaxLegacySourceRows {
			result.Rows = result.Rows[:storage.MaxLegacySourceRows]
			last := result.Rows[len(result.Rows)-1]
			result.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
		}
		return result.Validate(scope, epoch.String(), after)
	})
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	return result, nil
}

func (s *Store) OpenPrivateStorageSource(ctx context.Context, owner string, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	descriptor := storage.StreamSourceRow{Kind: kind, ID: id}
	if epoch == uuid.Nil || !model.SourceKind(kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return descriptor, nil, ErrPrivateStorageEpoch
	}
	var payload []byte
	err := s.privateSourceTx(ctx, owner, func(scope storage.Scope, tx pgx.Tx) error {
		if err := privateSourceEpoch(ctx, tx, scope, epoch); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT payload,sha256 FROM contacts_private_storage_snapshot WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5`, scope.Tenant, owner, epoch, kind, id).Scan(&payload, &descriptor.SHA256)
	})
	if err != nil {
		clear(payload)
		if errors.Is(err, pgx.ErrNoRows) {
			err = domain.ErrNotFound
		}
		return descriptor, nil, err
	}
	descriptor.Size = int64(len(payload))
	return descriptor, &privateSourceBody{Reader: bytes.NewReader(payload), payload: payload}, nil
}

type privateSourceBody struct {
	*bytes.Reader
	payload []byte
}

func (r *privateSourceBody) Close() error { clear(r.payload); r.payload = nil; return nil }

const contactsSourceCaptureSQL = `WITH owned_books AS MATERIALIZED (
 SELECT b.* FROM address_books b WHERE b.tenant=$1 AND b.owner_sub=$2
), owned_contacts AS MATERIALIZED (
 SELECT c.* FROM contacts c JOIN owned_books b ON b.tenant=c.tenant AND b.id=c.address_book_id
), owned_groups AS MATERIALIZED (
 SELECT g.* FROM contact_groups g JOIN owned_books b ON b.tenant=g.tenant AND b.id=g.address_book_id
), source(kind,id,payload) AS (
 SELECT 'book',b.id::text,convert_to(to_jsonb(b)::text,'UTF8') FROM owned_books b
 UNION ALL SELECT 'card',o.id::text,convert_to(to_jsonb(o)::text,'UTF8') FROM card_objects o JOIN owned_books b ON b.tenant=o.tenant AND b.id=o.address_book_id
 UNION ALL SELECT 'contact',c.id::text,convert_to(to_jsonb(c)::text,'UTF8') FROM owned_contacts c
 UNION ALL SELECT 'email',e.id::text,convert_to(to_jsonb(e)::text,'UTF8') FROM contact_emails e JOIN owned_contacts c ON c.tenant=e.tenant AND c.id=e.contact_id
 UNION ALL SELECT 'phone',p.id::text,convert_to(to_jsonb(p)::text,'UTF8') FROM contact_phones p JOIN owned_contacts c ON c.tenant=p.tenant AND c.id=p.contact_id
 UNION ALL SELECT 'group',g.id::text,convert_to(to_jsonb(g)::text,'UTF8') FROM owned_groups g
 UNION ALL SELECT 'member',m.id::text,convert_to(to_jsonb(m)::text,'UTF8') FROM contact_group_members m JOIN owned_groups g ON g.tenant=m.tenant AND g.id=m.group_id
)
INSERT INTO contacts_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload)
 SELECT $1,$2,$3,kind,id,payload FROM source`
