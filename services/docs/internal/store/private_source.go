// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// RetainedOfficeObjects returns original logical bytes, including transparent
// decompression of historical snapshots. No object keys appear in source pages.
type RetainedOfficeObjects interface {
	Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error)
}

type PrivateSource struct {
	store   *Store
	objects RetainedOfficeObjects
}

func NewPrivateSource(store *Store, objects RetainedOfficeObjects) (*PrivateSource, error) {
	if store == nil || store.pool == nil || objects == nil {
		return nil, model.ErrSource
	}
	return &PrivateSource{store: store, objects: objects}, nil
}

func (p *PrivateSource) ownerTx(ctx context.Context, scope storage.Scope, fn func(pgx.Tx) error) error {
	tenant, err := tenancy.From(ctx)
	_, office := model.ProductKind(scope.Product)
	if err != nil || !scope.Valid() || !office || tenant.Slug != scope.Tenant || p == nil || p.store == nil || p.objects == nil {
		return pg.ErrPrivateStorageScope
	}
	return p.store.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, scope.Subject); err != nil {
			return err
		}
		return fn(tx)
	})
}

func officeState(scope storage.Scope, state pg.PrivateStorageState, shared bool) storage.AppMigrationState {
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: state.Mode, Epoch: state.Epoch, StartedAt: state.StartedAt, Rows: state.Rows, Blobs: state.Blobs}, Shared: shared}
}

func (p *PrivateSource) State(ctx context.Context, scope storage.Scope) (storage.AppMigrationState, error) {
	var result storage.AppMigrationState
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		state, err := pg.ReadPrivateStorageState(ctx, tx, scope.Product+"_private_storage_states", scope.Tenant, scope.Subject)
		if err != nil {
			return err
		}
		shared, err := officeShared(ctx, tx, scope)
		if err != nil {
			return err
		}
		result = officeState(scope, state, shared)
		return result.Validate(scope)
	})
	return result, err
}

func officeShared(ctx context.Context, tx pgx.Tx, scope storage.Scope) (bool, error) {
	kind, _ := model.ProductKind(scope.Product)
	var shared bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM documents d WHERE d.tenant=$1 AND d.owner_sub=$2 AND d.kind::text=$3 AND (
 EXISTS(SELECT 1 FROM doc_permissions p WHERE p.tenant=d.tenant AND p.document_id=d.id AND (p.principal_kind<>'user' OR p.principal_id<>d.owner_sub)) OR EXISTS(SELECT 1 FROM doc_share_links l WHERE l.tenant=d.tenant AND l.document_id=d.id AND l.revoked_at IS NULL) OR EXISTS(SELECT 1 FROM doc_org_templates t WHERE t.tenant=d.tenant AND t.document_id=d.id)))
 OR EXISTS(SELECT 1 FROM doc_vistas_filtro v JOIN documents d ON d.tenant=v.tenant AND d.id=v.document_id WHERE v.tenant=$1 AND v.sub=$2 AND d.owner_sub<>$2 AND d.kind::text=$3)
 OR EXISTS(SELECT 1 FROM doc_scripts s WHERE s.tenant=$1 AND s.author_sub=$2 AND s.kind=$3 AND s.scope='org')
 OR ($3='spreadsheet' AND EXISTS(SELECT 1 FROM data_queries q JOIN data_connections c ON c.tenant=q.tenant AND c.id=q.connection_id WHERE q.tenant=$1 AND q.created_by=$2 AND c.created_by<>$2))`, scope.Tenant, scope.Subject, string(kind)).Scan(&shared)
	return shared, err
}

// Begin retains shared documents as well as personal ones. This is a source
// snapshot, not authority to enable an owner-only private collaboration path.
func (p *PrivateSource) Begin(ctx context.Context, scope storage.Scope, epoch uuid.UUID) (storage.AppMigrationState, error) {
	var result storage.AppMigrationState
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		state, err := pg.BeginPrivateStorageSnapshot(ctx, tx, pg.PrivateSnapshotConfig{Scope: scope, Epoch: epoch, Capture: func(ctx context.Context, tx pgx.Tx) (pg.PrivateSnapshotCounts, error) {
			return p.captureOffice(ctx, tx, scope, epoch)
		}})
		if err != nil {
			return err
		}
		shared, err := officeShared(ctx, tx, scope)
		if err != nil {
			return err
		}
		result = officeState(scope, state, shared)
		return result.Validate(scope)
	})
	return result, err
}

func (p *PrivateSource) Page(ctx context.Context, scope storage.Scope, epoch uuid.UUID, after storage.LegacySourceCursor, limit int) (storage.StreamSourcePage, error) {
	var result storage.StreamSourcePage
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		result, err = pg.ReadPrivateStorageSourcePage(ctx, tx, pg.PrivateSourceConfig{Scope: scope, Epoch: epoch, Kind: model.SourceKind}, after, limit)
		return err
	})
	return result, err
}

func (p *PrivateSource) Open(ctx context.Context, scope storage.Scope, epoch uuid.UUID, kind, id string) (storage.StreamSourceRow, io.ReadCloser, error) {
	var body pg.PrivateSourceBody
	err := p.ownerTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		body, err = pg.ReadPrivateStorageSourceBody(ctx, tx, pg.PrivateSourceConfig{Scope: scope, Epoch: epoch, Kind: model.SourceKind}, kind, id)
		return err
	})
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	if body.StorageKey == "" {
		reader, err := body.OpenJSON(ctx)
		return body.Descriptor, reader, err
	}
	// The immutable row was admitted only after resolving its actual document,
	// original key, size and complete SHA-256 under the owner's exclusive fence.
	reader, err := p.objects.Get(ctx, tenancy.Tenant{Slug: scope.Tenant}, body.StorageKey)
	if err != nil {
		if reader != nil {
			_ = reader.Close()
		}
		return storage.StreamSourceRow{}, nil, err
	}
	if reader == nil {
		return storage.StreamSourceRow{}, nil, model.ErrSource
	}
	return body.Descriptor, reader, nil
}

type officeRelation struct {
	kind, table string
	route       string
	onlyProduct string
}

// This is the domain inventory. The generic state transition, paging, source
// epoch and hashing contract remain in platform/pkg/pg and apps/nucleo/storage.
var officeRelations = []officeRelation{
	{"document", "documents", "document", ""},
	{"operation", "doc_operations", "document_child", ""},
	{"snapshot", "doc_snapshots", "document_child", ""},
	{"version", "doc_versions", "document_child", ""},
	{"permission", "doc_permissions", "document_child", ""},
	{"share_link", "doc_share_links", "document_child", ""},
	{"comment", "doc_comments", "document_child", ""},
	{"blob", "doc_blobs", "document_child", ""},
	{"edit_request", "doc_peticiones_edicion", "document_child", ""},
	{"protection", "doc_proteccion", "document_child", ""},
	{"protection_key", "doc_proteccion_claves", "document_child", ""},
	{"filter_view", "doc_vistas_filtro", "filter", ""},
	{"template", "doc_org_templates", "document_child", ""},
	{"template_log", "doc_org_template_log", "template_log", ""},
	{"script", "doc_scripts", "script", ""},
	{"script_version", "doc_script_versions", "script_version", ""},
	{"script_run", "doc_script_runs", "script_run", ""},
	{"brand", "doc_org_brand", "tenant", ""},
	{"link_policy", "doc_org_politica_enlaces", "tenant", ""},
	{"connection", "data_connections", "creator", "sheets"},
	{"query", "data_queries", "query", "sheets"},
}

func officeRelationQuery(r officeRelation) (string, error) {
	table := pgx.Identifier{r.table}.Sanitize()
	fields := make([]string, 0, len(model.SourcePrimaryFields(r.kind))*2)
	for _, key := range model.SourcePrimaryFields(r.kind) {
		fields = append(fields, "'"+key+"'", "r."+pgx.Identifier{key}.Sanitize())
	}
	primary := "jsonb_build_object(" + strings.Join(fields, ",") + ")"
	ownDoc := `EXISTS(SELECT 1 FROM documents d WHERE d.tenant=$1 AND d.owner_sub=$2 AND d.kind::text=$3 AND d.id=r.document_id)`
	var where string
	switch r.route {
	case "document":
		where = "r.owner_sub=$2 AND r.kind::text=$3"
	case "document_child":
		where = ownDoc
	case "filter":
		where = "r.sub=$2 AND EXISTS(SELECT 1 FROM documents d WHERE d.tenant=$1 AND d.kind::text=$3 AND d.id=r.document_id)"
	case "template_log":
		where = "(" + ownDoc + " OR r.actor_sub=$2)"
	case "script":
		where = "r.author_sub=$2 AND r.kind=$3"
	case "script_version":
		where = "EXISTS(SELECT 1 FROM doc_scripts s WHERE s.tenant=$1 AND s.id=r.script_id AND s.author_sub=$2 AND s.kind=$3)"
	case "script_run":
		where = "r.sub=$2 AND (" + ownDoc + " OR EXISTS(SELECT 1 FROM doc_scripts s WHERE s.tenant=$1 AND s.id=r.script_id AND s.author_sub=$2 AND s.kind=$3))"
	case "tenant":
		where = "true"
	case "creator":
		where = "r.created_by=$2"
	case "query":
		where = "r.created_by=$2 OR EXISTS(SELECT 1 FROM data_connections c WHERE c.tenant=$1 AND c.id=r.connection_id AND c.created_by=$2)"
	default:
		return "", model.ErrSource
	}
	return "SELECT " + primary + "::text,to_jsonb(r)::text FROM " + table + " r WHERE r.tenant=$1 AND $2::text<>'' AND $3::text<>'' AND (" + where + ") AND (" + primary + ")::text COLLATE \"C\">$4 ORDER BY (" + primary + ")::text COLLATE \"C\" LIMIT 128", nil
}

type officeCapturedRow struct {
	id      string
	payload []byte
}

func (p *PrivateSource) captureOffice(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID) (pg.PrivateSnapshotCounts, error) {
	counts := pg.PrivateSnapshotCounts{}
	if err := validateOfficeSourceSchema(ctx, tx); err != nil {
		return counts, err
	}
	kind, _ := model.ProductKind(scope.Product)
	table := pgx.Identifier{scope.Product + "_private_storage_snapshot"}.Sanitize()
	for _, relation := range officeRelations {
		if relation.onlyProduct != "" && relation.onlyProduct != scope.Product {
			continue
		}
		relCounts, err := p.captureOfficeRelation(ctx, tx, scope, epoch, string(kind), table, relation)
		counts.Rows += relCounts.Rows
		counts.Blobs += relCounts.Blobs
		if err != nil {
			return counts, err
		}
	}
	return counts, nil
}

func (p *PrivateSource) captureOfficeRelation(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, kind, table string, relation officeRelation) (pg.PrivateSnapshotCounts, error) {
	counts := pg.PrivateSnapshotCounts{}
	query, err := officeRelationQuery(relation)
	if err != nil {
		return counts, err
	}
	after := ""
	for {
		batch, nextAfter, err := readOfficeRelationBatch(ctx, tx, query, scope, kind, relation.kind, after)
		if err != nil {
			return counts, err
		}
		if len(batch) == 0 {
			return counts, nil
		}
		after = nextAfter
		relCounts, err := p.insertOfficeRelationBatch(ctx, tx, scope, epoch, table, relation.kind, batch)
		counts.Rows += relCounts.Rows
		counts.Blobs += relCounts.Blobs
		if err != nil {
			return counts, err
		}
	}
}

func readOfficeRelationBatch(ctx context.Context, tx pgx.Tx, query string, scope storage.Scope, kind, relKind, after string) ([]officeCapturedRow, string, error) {
	rows, err := tx.Query(ctx, query, scope.Tenant, scope.Subject, kind, after)
	if err != nil {
		return nil, after, err
	}
	defer rows.Close()
	batch := []officeCapturedRow{}
	bytes := 0
	for rows.Next() {
		row, nextKey, err := scanOfficeCapturedRow(rows, scope, relKind, after)
		if err != nil {
			for _, v := range batch {
				clear(v.payload)
			}
			return nil, after, err
		}
		after = nextKey
		batch = append(batch, row)
		bytes += len(row.payload)
		if bytes >= model.MaxSourceRowBytes {
			break
		}
	}
	if err := rows.Err(); err != nil {
		for _, v := range batch {
			clear(v.payload)
		}
		return nil, after, err
	}
	return batch, after, nil
}

func scanOfficeCapturedRow(rows pgx.Rows, scope storage.Scope, relKind, after string) (officeCapturedRow, string, error) {
	var key string
	var raw []byte
	if err := rows.Scan(&key, &raw); err != nil {
		return officeCapturedRow{}, after, err
	}
	if key <= after {
		clear(raw)
		return officeCapturedRow{}, after, model.ErrSource
	}
	var primary map[string]json.RawMessage
	if json.Unmarshal([]byte(key), &primary) != nil {
		clear(raw)
		return officeCapturedRow{}, after, model.ErrSource
	}
	id, err := model.SourceObjectID(relKind, primary)
	if err != nil {
		clear(raw)
		return officeCapturedRow{}, after, err
	}
	if err := model.ValidateSourceRow(scope, relKind, id, raw); err != nil {
		clear(raw)
		return officeCapturedRow{}, after, err
	}
	return officeCapturedRow{id: id, payload: raw}, key, nil
}

func (p *PrivateSource) insertOfficeRelationBatch(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, table, relKind string, batch []officeCapturedRow) (pg.PrivateSnapshotCounts, error) {
	counts := pg.PrivateSnapshotCounts{}
	for _, row := range batch {
		digest := sha256.Sum256(row.payload)
		_, err := tx.Exec(ctx, "INSERT INTO "+table+"(tenant,owner_sub,epoch,kind,id,payload,size_bytes,sha256)VALUES($1,$2,$3,$4,$5,$6,$7,$8)", scope.Tenant, scope.Subject, epoch, relKind, row.id, row.payload, len(row.payload), hex.EncodeToString(digest[:]))
		if err == nil && (relKind == "snapshot" || relKind == "blob") {
			var added int64
			added, err = p.captureOfficeObject(ctx, tx, scope, epoch, relKind, row, table)
			counts.Blobs += added
			counts.Rows += added
		}
		clear(row.payload)
		if err != nil {
			for _, value := range batch {
				clear(value.payload)
			}
			return counts, err
		}
		counts.Rows++
	}
	return counts, nil
}

func (p *PrivateSource) captureOfficeObject(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch uuid.UUID, sourceKind string, row officeCapturedRow, table string) (int64, error) {
	var object struct {
		DocumentID string `json:"document_id"`
		Key        string `json:"blob_key"`
		Size       int64  `json:"size_bytes"`
	}
	if json.Unmarshal(row.payload, &object) != nil {
		return 0, model.ErrSource
	}
	if object.Key == "" {
		if sourceKind == "snapshot" {
			return 0, nil
		}
		return 0, model.ErrSource
	}
	if id, err := uuid.Parse(object.DocumentID); err != nil || id == uuid.Nil || id.String() != object.DocumentID || !strings.HasPrefix(object.Key, "docs/"+object.DocumentID+"/") || object.Size < 0 || object.Size > storage.MaxFileBytes {
		return 0, model.ErrSource
	}
	reader, err := p.objects.Get(ctx, tenancy.Tenant{Slug: scope.Tenant}, object.Key)
	if err != nil {
		if reader != nil {
			_ = reader.Close()
		}
		return 0, err
	}
	if reader == nil {
		return 0, model.ErrSource
	}
	stop := context.AfterFunc(ctx, func() { _ = reader.Close() })
	defer stop()
	digest := sha256.New()
	buf := make([]byte, 64<<10)
	defer clear(buf)
	n, readErr := io.CopyBuffer(digest, io.LimitReader(reader, object.Size+1), buf)
	if err := errors.Join(readErr, reader.Close(), ctx.Err()); err != nil {
		return 0, err
	}
	if n != object.Size {
		return 0, model.ErrSource
	}
	id, idErr := model.BinarySourceID(sourceKind, row.id)
	if idErr != nil {
		return 0, idErr
	}
	_, err = tx.Exec(ctx, "INSERT INTO "+table+"(tenant,owner_sub,epoch,kind,id,size_bytes,sha256,storage_key)VALUES($1,$2,$3,$4,$5,$6,$7,$8)", scope.Tenant, scope.Subject, epoch, model.BinarySourceKind, id, n, hex.EncodeToString(digest.Sum(nil)), object.Key)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// Unknown content-bearing relations are an explicit migration gap, never a
// successful snapshot that quietly loses features introduced by another build.
func validateOfficeSourceSchema(ctx context.Context, tx pgx.Tx) error {
	// Bootstrap and pending roster captures contain public routing/SDK evidence;
	// their owner_sub is not another legacy content relation. Keep these exact
	// metadata names: unknown application relations must still fail closed.
	base := []string{"resource_e2e_bootstraps", "resource_e2e_roster_captures", "doc_call_access_fences", "docs_private_storage_states", "docs_private_storage_snapshot", "sheets_private_storage_states", "sheets_private_storage_snapshot", "slides_private_storage_states", "slides_private_storage_snapshot"}
	known := make([]string, 0, len(base)+len(officeRelations))
	known = append(known, base...)
	for _, relation := range officeRelations {
		known = append(known, relation.table)
	}
	var missing bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid WHERE n.nspname='public' AND c.relkind IN('r','p') AND a.attnum>0 AND NOT a.attisdropped AND a.attname IN('document_id','owner_sub','author_sub','created_by') AND c.relname<>ALL($1::text[]))`, known).Scan(&missing)
	if err != nil {
		return err
	}
	if missing {
		return model.ErrSource
	}
	return nil
}
