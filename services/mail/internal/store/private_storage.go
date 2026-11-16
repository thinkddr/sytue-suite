// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

var (
	ErrPrivateStorageFrozen        = errors.New("mail: legacy writes are frozen for private migration")
	ErrPrivateStorageEpoch         = errors.New("mail: private source epoch does not match")
	ErrPrivateStorageCollaboration = errors.New("mail: collaborative private source is not supported yet")
	ErrPrivateStorageBusy          = errors.New("mail: an outbound delivery is already in flight")
)

// PrivateSourceBody is internal routing. Only Descriptor goes on the wire.
type PrivateSourceBody struct {
	Descriptor storage.StreamSourceRow
	Payload    []byte
	StorageKey string
}

func PrivateStorageError(err error) error {
	var failure *pgconn.PgError
	if errors.As(err, &failure) && failure.Code == "PST01" {
		return ErrPrivateStorageFrozen
	}
	return err
}

func (s *Store) privateTx(ctx context.Context, owner string, fn func(pgx.Tx) error) error {
	t, err := tenancy.From(ctx)
	if err != nil || !(storage.Scope{Tenant: t.Slug, Subject: owner, Product: "mail"}).Valid() {
		return domain.ErrNotFound
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.private_mail_owner',$1,true)`, owner); err != nil {
			return err
		}
		return fn(tx)
	})
}

func privateState(ctx context.Context, tx pgx.Tx, t, owner string) (storage.LegacyMigrationState, error) {
	value, err := pg.ReadPrivateStorageState(ctx, tx, "mail_private_storage_states", t, owner)
	return storage.LegacyMigrationState{Scope: storage.Scope{Tenant: t, Subject: owner, Product: "mail"}, Mode: value.Mode, Epoch: value.Epoch, StartedAt: value.StartedAt, Rows: value.Rows, Blobs: value.Blobs}, err
}

func (s *Store) PrivateStorageState(ctx context.Context, owner string) (storage.LegacyMigrationState, error) {
	var result storage.LegacyMigrationState
	err := s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		var err error
		result, err = privateState(ctx, tx, tenantOf(ctx), owner)
		return err
	})
	return result, err
}

// BeginPrivateStorageMigration captures ALL extant account-owned rows, including
// soft-deleted messages and scheduled/cancelled/bounced queue entries. Mail has no
// historical message-version table: no previously purged history is fabricated.
// MIME is hashed from retained originals before the same transaction commits.
// This is an explicit migration operation, never an automatic activation. The
// caller must first arrange private ingress/egress on authorized devices.
type privateMailCapture struct {
	tenant, owner  string
	account, epoch uuid.UUID
}
type privateMailBody struct {
	kind, id, key string
	size          int64
}

func (s *Store) BeginPrivateStorageMigration(ctx context.Context, owner string, epoch uuid.UUID, open func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error) {
	if epoch == uuid.Nil || open == nil {
		return storage.LegacyMigrationState{}, ErrPrivateStorageEpoch
	}
	var result storage.LegacyMigrationState
	err := s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		var err error
		result, err = s.beginPrivateStorageTx(ctx, tx, owner, epoch, open)
		return err
	})
	return result, err
}

func (s *Store) beginPrivateStorageTx(ctx context.Context, tx pgx.Tx, owner string, epoch uuid.UUID, open func(context.Context, string) (io.ReadCloser, error)) (storage.LegacyMigrationState, error) {
	var result storage.LegacyMigrationState
	err := func() error {
		plan, resumed, err := lockPrivateMailCapture(ctx, tx, tenantOf(ctx), owner, epoch)
		if err != nil {
			return err
		}
		if !resumed {
			rows, err := plan.captureMetadata(ctx, tx)
			if err != nil {
				return err
			}
			bodies, err := plan.captureBodies(ctx, tx, open)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE mail_private_storage_states SET mode='frozen',source_rows=$5,source_blobs=$6 WHERE tenant=$1 AND account_id=$2 AND owner_sub=$3 AND epoch=$4 AND mode='building'`, plan.tenant, plan.account, plan.owner, plan.epoch, rows, bodies); err != nil {
				return err
			}
		}
		result, err = privateState(ctx, tx, plan.tenant, plan.owner)
		return err
	}()
	return result, err
}

func lockPrivateMailCapture(ctx context.Context, tx pgx.Tx, tenant, owner string, epoch uuid.UUID) (privateMailCapture, bool, error) {
	plan := privateMailCapture{tenant: tenant, owner: owner, epoch: epoch}
	if err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE tenant=$1 AND subject=$2 AND kind='user'`, tenant, owner).Scan(&plan.account); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return plan, false, domain.ErrNotFound
		}
		return plan, false, err
	}
	if err := pg.EnsurePrivateStorageState(ctx, tx, "mail_private_storage_states", tenant, owner, &plan.account); err != nil {
		return plan, false, err
	}
	locked, err := pg.LockPrivateStorageState(ctx, tx, "mail_private_storage_states", tenant, owner)
	if err != nil {
		return plan, false, err
	}
	if locked.Mode == "legacy" {
		return plan, false, nil
	}
	if locked.Mode != "frozen" || locked.Epoch != epoch.String() {
		return plan, false, ErrPrivateStorageEpoch
	}
	return plan, true, nil
}

func (p privateMailCapture) captureMetadata(ctx context.Context, tx pgx.Tx) (int64, error) {
	var sending, shared bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM outbound WHERE tenant=$1 AND account_id=$2 AND state='sending'),EXISTS(SELECT 1 FROM mailbox_acl WHERE tenant=$1 AND account_id=$2)`, p.tenant, p.account).Scan(&sending, &shared); err != nil {
		return 0, err
	}
	if sending {
		return 0, ErrPrivateStorageBusy
	}
	if shared {
		return 0, ErrPrivateStorageCollaboration
	}
	if _, err := tx.Exec(ctx, `UPDATE mail_private_storage_states SET mode='building',epoch=$4,started_at=clock_timestamp() WHERE tenant=$1 AND account_id=$2 AND owner_sub=$3`, p.tenant, p.account, p.owner, p.epoch); err != nil {
		return 0, err
	}
	rows, err := tx.Exec(ctx, privateMailCaptureSQL, p.tenant, p.account, p.owner, p.epoch)
	if err != nil {
		return 0, fmt.Errorf("mail: capture retained source: %w", err)
	}
	return rows.RowsAffected(), nil
}

func (p privateMailCapture) bodyPage(ctx context.Context, tx pgx.Tx, kind, id string) ([]privateMailBody, error) {
	rows, err := tx.Query(ctx, `WITH bodies AS(
  SELECT 'mime_message' AS kind,id::text AS id,blob_key AS key,size_bytes AS size FROM messages WHERE tenant=$1 AND account_id=$2
  UNION ALL SELECT 'mime_outbound',id::text,blob_key,-1::bigint FROM outbound WHERE tenant=$1 AND account_id=$2)
 SELECT kind,id,key,size FROM bodies WHERE(kind,id)>($3,$4) ORDER BY kind,id LIMIT 50`, p.tenant, p.account, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []privateMailBody{}
	for rows.Next() {
		var body privateMailBody
		if err := rows.Scan(&body.kind, &body.id, &body.key, &body.size); err != nil {
			return nil, err
		}
		result = append(result, body)
	}
	return result, rows.Err()
}

func (p privateMailCapture) captureBodies(ctx context.Context, tx pgx.Tx, open func(context.Context, string) (io.ReadCloser, error)) (int64, error) {
	var count int64
	kind, id := "", ""
	for {
		page, err := p.bodyPage(ctx, tx, kind, id)
		if err != nil {
			return 0, err
		}
		if len(page) == 0 {
			return count, nil
		}
		for _, body := range page {
			if err := p.captureBody(ctx, tx, body, open); err != nil {
				return 0, err
			}
			count++
		}
		last := page[len(page)-1]
		kind, id = last.kind, last.id
	}
}

func (p privateMailCapture) captureBody(ctx context.Context, tx pgx.Tx, body privateMailBody, open func(context.Context, string) (io.ReadCloser, error)) error {
	source, err := open(ctx, body.key)
	if err != nil {
		return err
	}
	sum := sha256.New()
	n, readErr := io.Copy(sum, io.LimitReader(source, storage.MaxFileBytes+1))
	closeErr := source.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	if n > storage.MaxFileBytes || body.size >= 0 && n != body.size {
		return domain.ErrMessageTooLarge
	}
	_, err = tx.Exec(ctx, `INSERT INTO mail_private_storage_source(tenant,account_id,owner_sub,epoch,kind,id,storage_key,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.tenant, p.account, p.owner, p.epoch, body.kind, body.id, body.key, n, hex.EncodeToString(sum.Sum(nil)))
	return err
}

func privateMailEpoch(ctx context.Context, tx pgx.Tx, t, owner string, epoch uuid.UUID) error {
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mail_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND mode='frozen')`, t, owner, epoch).Scan(&valid); err != nil {
		return err
	}
	if epoch == uuid.Nil || !valid {
		return ErrPrivateStorageEpoch
	}
	return nil
}

func (s *Store) PrivateStorageSourcePage(ctx context.Context, owner string, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	result := storage.StreamSourcePage{Scope: storage.Scope{Tenant: tenantOf(ctx), Subject: owner, Product: "mail"}, Epoch: epoch.String(), Rows: []storage.StreamSourceRow{}}
	if limit < 1 || limit > storage.MaxLegacySourceRows || after != (storage.LegacySourceCursor{}) && !storage.SourceCursorValid(after) {
		return result, ErrPrivateStorageEpoch
	}
	err := s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		if err := privateMailEpoch(ctx, tx, tenantOf(ctx), owner, epoch); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT kind,id,size_bytes,sha256 FROM mail_private_storage_source WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND (kind,id)>($4,$5) ORDER BY kind,id LIMIT $6`, tenantOf(ctx), owner, epoch, after.Kind, after.ID, limit+1)
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
		return nil
	})
	return result, err
}

func (s *Store) PrivateStorageSourceBody(ctx context.Context, owner string, epoch uuid.UUID, kind, id string) (PrivateSourceBody, error) {
	result := PrivateSourceBody{Descriptor: storage.StreamSourceRow{Kind: kind, ID: id}}
	if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return result, domain.ErrNotFound
	}
	err := s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		if err := privateMailEpoch(ctx, tx, tenantOf(ctx), owner, epoch); err != nil {
			return err
		}
		var key *string
		err := tx.QueryRow(ctx, `SELECT payload,storage_key,size_bytes,sha256 FROM mail_private_storage_source WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5`, tenantOf(ctx), owner, epoch, kind, id).Scan(&result.Payload, &key, &result.Descriptor.Size, &result.Descriptor.SHA256)
		if key != nil {
			result.StorageKey = *key
		}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.ErrNotFound
	}
	return result, err
}

// OpenPrivateStorageJSON never exposes routing and returns a reader on a copy;
// MIME callers must use their objectstore port with the internal StorageKey.
func (b PrivateSourceBody) OpenJSON() (io.ReadCloser, error) {
	if b.StorageKey != "" || b.Payload == nil {
		return nil, domain.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b.Payload)), nil
}

// SQL columns deliberately differ from Drive/Notes: these are Mail's actual
// retained domain rows, not copied transport or cryptographic plumbing.
const privateMailCaptureSQL = `WITH source AS (
 SELECT 'account' AS kind,a.id::text AS id,to_jsonb(a)-'tenant' AS payload FROM accounts a WHERE a.tenant=$1 AND a.id=$2
 UNION ALL SELECT 'alias',v.id::text,to_jsonb(v)-'tenant' FROM aliases v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'mailbox',v.id::text,to_jsonb(v)-'tenant' FROM mailboxes v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'message',v.id::text,to_jsonb(v)-'tenant'-'blob_key'-'busqueda' FROM messages v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'thread',v.id::text,to_jsonb(v)-'tenant' FROM threads v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'outbound',v.id::text,to_jsonb(v)-'tenant'-'blob_key' FROM outbound v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'label',v.id::text,to_jsonb(v)-'tenant' FROM mail_labels v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'filter',v.id::text,to_jsonb(v)-'tenant' FROM mail_filters v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'vacation',v.account_id::text,to_jsonb(v)-'tenant' FROM mail_vacation v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'vacation_log',encode(convert_to(v.sender,'UTF8'),'hex'),to_jsonb(v)-'tenant' FROM mail_vacation_log v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'acl',v.id::text,to_jsonb(v)-'tenant' FROM mailbox_acl v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'smime_identity',v.account_id::text,to_jsonb(v)-'tenant' FROM smime_identities v WHERE v.tenant=$1 AND v.account_id=$2
 UNION ALL SELECT 'request',encode(convert_to(v.sujeto,'UTF8'),'hex'),to_jsonb(v)-'tenant' FROM mailbox_peticiones_edicion v WHERE v.tenant=$1 AND v.account_id=$2
),encoded AS(SELECT kind,id,convert_to(payload::text,'UTF8') AS payload FROM source)
INSERT INTO mail_private_storage_source(tenant,account_id,owner_sub,epoch,kind,id,payload,size_bytes,sha256)
 SELECT $1,$2,$3,$4,kind,id,payload,octet_length(payload),encode(sha256(payload),'hex') FROM encoded`

func (s *Store) PrivateStorageShared(ctx context.Context, owner string) (bool, error) {
	var shared bool
	err := s.privateTx(ctx, owner, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT a.kind<>'user' OR EXISTS(SELECT 1 FROM mailbox_acl x WHERE x.tenant=$1 AND x.account_id=a.id)
   FROM accounts a WHERE a.tenant=$1 AND a.subject=$2`, tenantOf(ctx), owner).Scan(&shared)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return shared, err
}

// PrivateStorageOwnsMessage is a routing-only lookup for the personal HTTP
// gate. A delegated reader must still pass the existing handler's ACL check.
func (s *Store) PrivateStorageOwnsMessage(ctx context.Context, owner string, id uuid.UUID) (bool, error) {
	var own bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce((SELECT m.account_id=a.id FROM messages m JOIN accounts a ON a.tenant=m.tenant
   WHERE m.tenant=$1 AND a.tenant=$1 AND a.subject=$2 AND m.id=$3),false)`, tenantOf(ctx), owner, id).Scan(&own)
	})
	return own, err
}
