// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrPrivateSource = errors.New("pg: retained private source does not match")

// PrivateSourceConfig supplies domain kind validation, never SQL identifiers.
// The caller owns a tenant transaction with the product's owner GUC set. Mail's
// external MIME routing remains internal and is never read into a page or JSON.
type PrivateSourceConfig struct {
	Scope storage.Scope
	Epoch uuid.UUID
	Kind  func(string) bool
}

type privateSourceSchema struct {
	table, ownerSetting, size, key string
	maxJSON                        int64
	externalKind                   func(string) bool
}

func privateSourceRegistry(cfg PrivateSourceConfig) (privateSourceSchema, error) {
	if !cfg.Scope.Valid() || cfg.Epoch == uuid.Nil || cfg.Kind == nil {
		return privateSourceSchema{}, ErrPrivateSource
	}
	schema := privateSourceSchema{ownerSetting: "app.owner_sub", size: "size_bytes", key: "NULL::text"}
	switch cfg.Scope.Product {
	case "calendar", "contacts":
		schema.table = cfg.Scope.Product + "_private_storage_snapshot"
		schema.maxJSON = 8 << 20
		if cfg.Scope.Product == "contacts" {
			schema.size = "octet_length(payload)::bigint"
		}
	case "notes", "hub", "bases", "erp":
		schema.table = cfg.Scope.Product + "_private_storage_snapshot"
		schema.maxJSON = 32 << 20
	case "mail":
		schema.table = "mail_private_storage_source"
		schema.ownerSetting = "app.private_mail_owner"
		schema.key = "storage_key"
		schema.maxJSON = 32 << 20
		schema.externalKind = func(kind string) bool { return kind == "mime_message" || kind == "mime_outbound" }
	case "photos":
		schema.table = "photos_private_storage_snapshot"
		schema.key = "storage_key"
		schema.maxJSON = 32 << 20
		schema.externalKind = func(kind string) bool { return kind == "variant" }
	case "docs", "sheets", "slides":
		schema.table = cfg.Scope.Product + "_private_storage_snapshot"
		schema.key = "storage_key"
		schema.maxJSON = 32 << 20
		schema.externalKind = func(kind string) bool { return kind == "office_bytes" }
	case "cast":
		schema.table = "cast_private_storage_snapshot"
		schema.key = "storage_key"
		schema.maxJSON = 32 << 20
		schema.externalKind = func(kind string) bool { return kind == "media_bytes" }
	default:
		return privateSourceSchema{}, ErrPrivateSource
	}
	return schema, nil
}

func privateSourceEpoch(ctx context.Context, tx pgx.Tx, cfg PrivateSourceConfig, schema privateSourceSchema) (PrivateStorageState, error) {
	if tx == nil {
		return PrivateStorageState{}, ErrPrivateSource
	}
	if _, err := privateStorageTable(ctx, cfg.Scope.Product+"_private_storage_states", cfg.Scope.Tenant, cfg.Scope.Subject); err != nil {
		return PrivateStorageState{}, err
	}
	var tenant, owner, maintenance, merge string
	if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.tenant',true),''),coalesce(current_setting($1,true),''),coalesce(current_setting('app.maintenance',true),''),coalesce(current_setting('app.merge',true),'')`, schema.ownerSetting).Scan(&tenant, &owner, &maintenance, &merge); err != nil {
		return PrivateStorageState{}, err
	}
	if tenant != cfg.Scope.Tenant || owner != cfg.Scope.Subject || maintenance == "on" || merge == "on" {
		return PrivateStorageState{}, ErrPrivateSource
	}
	state, err := ReadPrivateStorageState(ctx, tx, cfg.Scope.Product+"_private_storage_states", cfg.Scope.Tenant, cfg.Scope.Subject)
	if err != nil {
		return PrivateStorageState{}, err
	}
	if (state.Mode != "frozen" && state.Mode != "private") || state.Epoch != cfg.Epoch.String() || validatePrivateSnapshotState(cfg.Scope, state) != nil {
		return PrivateStorageState{}, ErrPrivateSource
	}
	return state, nil
}

func privateSourceCursor(cfg PrivateSourceConfig, after storage.LegacySourceCursor) bool {
	return after == (storage.LegacySourceCursor{}) || storage.SourceCursorValid(after) && cfg.Kind(after.Kind)
}

func privateSourceDescriptor(cfg PrivateSourceConfig, schema privateSourceSchema, row storage.StreamSourceRow, external bool) bool {
	if !cfg.Kind(row.Kind) || row.Validate() != nil {
		return false
	}
	if external {
		return schema.externalKind != nil && schema.externalKind(row.Kind)
	}
	if schema.externalKind != nil && schema.externalKind(row.Kind) {
		return false
	}
	return row.Size <= schema.maxJSON
}

func privateSourcePageCount(rows int, declared int64, after storage.LegacySourceCursor) bool {
	if int64(rows) > declared {
		return false
	}
	return rows > 0 || after != (storage.LegacySourceCursor{}) || declared == 0
}

// ReadPrivateStorageSourcePage reads descriptors only, in the byte ordering used
// by storage's cursor validator. Scope/epoch checks run in this same transaction.
func ReadPrivateStorageSourcePage(ctx context.Context, tx pgx.Tx, cfg PrivateSourceConfig, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	schema, err := privateSourceRegistry(cfg)
	if err != nil || limit < 1 || limit > storage.MaxLegacySourceRows || !privateSourceCursor(cfg, after) {
		return storage.StreamSourcePage{}, ErrPrivateSource
	}
	state, err := privateSourceEpoch(ctx, tx, cfg, schema)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	page := storage.StreamSourcePage{Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Rows: []storage.StreamSourceRow{}}
	if after != (storage.LegacySourceCursor{}) {
		var present bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.`+schema.table+` WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, after.Kind, after.ID).Scan(&present); err != nil {
			return storage.StreamSourcePage{}, err
		}
		if !present {
			return storage.StreamSourcePage{}, ErrPrivateSource
		}
	}
	query := `SELECT kind,id,` + schema.size + `,sha256,` + schema.key + ` IS NOT NULL FROM public.` + schema.table + ` WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND (kind COLLATE "C",id COLLATE "C")>($4,$5) ORDER BY kind COLLATE "C",id COLLATE "C" LIMIT $6`
	rows, err := tx.Query(ctx, query, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, after.Kind, after.ID, limit+1)
	if err != nil {
		return storage.StreamSourcePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row storage.StreamSourceRow
		var external bool
		if err := rows.Scan(&row.Kind, &row.ID, &row.Size, &row.SHA256, &external); err != nil {
			return storage.StreamSourcePage{}, err
		}
		if !privateSourceDescriptor(cfg, schema, row, external) {
			return storage.StreamSourcePage{}, ErrPrivateSource
		}
		page.Rows = append(page.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return storage.StreamSourcePage{}, err
	}
	if !privateSourcePageCount(len(page.Rows), state.Rows, after) {
		return storage.StreamSourcePage{}, ErrPrivateSource
	}
	if len(page.Rows) > limit {
		page.Rows = page.Rows[:limit]
		last := page.Rows[limit-1]
		page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
	}
	if page.Validate(cfg.Scope, cfg.Epoch.String(), after) != nil {
		return storage.StreamSourcePage{}, ErrPrivateSource
	}
	return page, nil
}

// PrivateSourceBody contains either verified, bounded JSON or an external blob
// reference. Only Descriptor may leave the service. External consumers open the
// immutable object through their existing store and verify the streamed bytes.
type PrivateSourceBody struct {
	Descriptor storage.StreamSourceRow
	StorageKey string `json:"-"`
	payload    []byte
}

func ReadPrivateStorageSourceBody(ctx context.Context, tx pgx.Tx, cfg PrivateSourceConfig, kind, id string) (PrivateSourceBody, error) {
	schema, err := privateSourceRegistry(cfg)
	if err != nil || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) || !cfg.Kind(kind) {
		return PrivateSourceBody{}, ErrPrivateSource
	}
	if _, err := privateSourceEpoch(ctx, tx, cfg, schema); err != nil {
		return PrivateSourceBody{}, err
	}
	body := PrivateSourceBody{Descriptor: storage.StreamSourceRow{Kind: kind, ID: id}}
	query := `SELECT CASE WHEN ` + schema.key + ` IS NULL AND ` + schema.size + ` BETWEEN 0 AND $6 THEN payload ELSE NULL END,coalesce(` + schema.key + `,''),` + schema.size + `,sha256 FROM public.` + schema.table + ` WHERE tenant=$1 AND owner_sub=$2 AND epoch=$3 AND kind=$4 AND id=$5`
	err = tx.QueryRow(ctx, query, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, kind, id, schema.maxJSON).Scan(&body.payload, &body.StorageKey, &body.Descriptor.Size, &body.Descriptor.SHA256)
	if err != nil {
		clear(body.payload)
		return PrivateSourceBody{}, err
	}
	if err := ctx.Err(); err != nil {
		clear(body.payload)
		return PrivateSourceBody{}, err
	}
	if body.Descriptor.Validate() != nil {
		clear(body.payload)
		return PrivateSourceBody{}, ErrPrivateSource
	}
	if body.StorageKey != "" {
		if schema.externalKind == nil || !schema.externalKind(kind) || body.payload != nil {
			return PrivateSourceBody{}, ErrPrivateSource
		}
		return body, nil
	}
	if schema.externalKind != nil && schema.externalKind(kind) {
		clear(body.payload)
		return PrivateSourceBody{}, ErrPrivateSource
	}
	if body.payload == nil || int64(len(body.payload)) != body.Descriptor.Size || body.Descriptor.Size > schema.maxJSON || !json.Valid(body.payload) {
		clear(body.payload)
		return PrivateSourceBody{}, ErrPrivateSource
	}
	sum := sha256.Sum256(body.payload)
	if hex.EncodeToString(sum[:]) != body.Descriptor.SHA256 {
		clear(body.payload)
		return PrivateSourceBody{}, ErrPrivateSource
	}
	return body, nil
}

// OpenJSON transfers the verified buffer to a reader and clears it on close.
// It cannot be used for MIME routing or reopened after ownership is transferred.
func (b *PrivateSourceBody) OpenJSON(ctx context.Context) (io.ReadCloser, error) {
	if b == nil || b.StorageKey != "" || b.payload == nil {
		return nil, ErrPrivateSource
	}
	if err := ctx.Err(); err != nil {
		clear(b.payload)
		b.payload = nil
		return nil, err
	}
	payload := b.payload
	b.payload = nil
	return &privateSourceReader{ctx: ctx, reader: bytes.NewReader(payload), payload: payload}, nil
}

type privateSourceReader struct {
	mu      sync.Mutex
	ctx     context.Context
	reader  *bytes.Reader
	payload []byte
}

func (r *privateSourceReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (r *privateSourceReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.payload)
	r.payload = nil
	r.reader = bytes.NewReader(nil)
	return nil
}
