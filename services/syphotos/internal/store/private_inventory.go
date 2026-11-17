// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ReadPrivateStorageInventory reads complete retained rows, including trash,
// recipes, vectors, memberships and incoming/outgoing ACLs in the caller's
// transaction. It neither freezes nor activates Photos. The caller must reject
// incompatible shares and capture/verify original and derived bytes first.
func ReadPrivateStorageInventory(ctx context.Context, tx pgx.Tx, scope storage.Scope) (*model.Snapshot, error) {
	return readPrivateStorageInventory(ctx, tx, scope, 0)
}

func readPrivateStorageInventory(ctx context.Context, tx pgx.Tx, scope storage.Scope, limit int64) (*model.Snapshot, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tx == nil || !scope.Valid() || scope.Product != "photos" || tenant.Slug != scope.Tenant {
		return nil, pg.ErrPrivateStorageScope
	}
	query := photosPrivateInventorySQL
	args := []any{scope.Tenant, scope.Subject}
	if limit > 0 {
		query = `SELECT kind,id,CASE WHEN octet_length(payload)<=$3 THEN payload ELSE NULL END FROM (` + query + `) retained`
		args = append(args, limit)
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []model.Row{}
	var total int64
	defer func() {
		for _, value := range values {
			clear(value.JSON)
		}
	}()
	for rows.Next() {
		var value model.Row
		if err := rows.Scan(&value.Kind, &value.ID, &value.JSON); err != nil {
			return nil, err
		}
		if limit > 0 && (value.JSON == nil || int64(len(value.JSON)) > limit-total) {
			clear(value.JSON)
			return nil, model.ErrSnapshot
		}
		total += int64(len(value.JSON))
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return model.NewSnapshot(scope, values)
}

const photosPrivateInventorySQL = `WITH accessible_albums AS MATERIALIZED (
 SELECT a.* FROM albums a WHERE a.tenant=$1 AND (a.owner_sub=$2 OR a.id IN(SELECT p.album_id FROM album_personas p WHERE p.tenant=$1 AND p.sujeto=$2))
), retained_assets AS MATERIALIZED (
 SELECT a.* FROM assets a WHERE a.tenant=$1 AND (a.owner_sub=$2
 OR a.id IN(SELECT m.asset_id FROM album_assets m JOIN accessible_albums al ON al.tenant=m.tenant AND al.id=m.album_id)
 OR a.id IN(SELECT cover_asset_id FROM accessible_albums)
 OR a.id IN(SELECT e.asset_id FROM edits e WHERE e.tenant=$1 AND e.owner_sub=$2))
), source(kind,id,payload) AS (
 SELECT 'asset',a.id::text,convert_to(to_jsonb(a)::text,'UTF8') FROM retained_assets a
 UNION ALL SELECT 'album',a.id::text,convert_to(to_jsonb(a)::text,'UTF8') FROM accessible_albums a
 UNION ALL SELECT 'edit',e.id::text,convert_to(to_jsonb(e)::text,'UTF8') FROM edits e JOIN retained_assets a ON a.tenant=e.tenant AND a.id=e.asset_id
 UNION ALL SELECT 'album_asset',encode(sha256(convert_to(m.album_id::text||':'||m.asset_id::text,'UTF8')),'hex'),convert_to(to_jsonb(m)::text,'UTF8') FROM album_assets m JOIN accessible_albums a ON a.tenant=m.tenant AND a.id=m.album_id
 UNION ALL SELECT 'album_person',encode(sha256(convert_to(p.album_id::text||':'||p.sujeto,'UTF8')),'hex'),convert_to(to_jsonb(p)::text,'UTF8') FROM album_personas p JOIN accessible_albums a ON a.tenant=p.tenant AND a.id=p.album_id
)
SELECT kind,id,payload FROM source ORDER BY kind COLLATE "C",id COLLATE "C"`
