// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ReadPrivateStorageInventory captures the owned video graph and the caller's
// external contributions/access as exact rows. It is read-only and internal:
// no source route, fence, public visibility or live signaling is changed here.
// Moderation identities/audits belonging to other reporters stay in their
// original server store and are not exposed as this caller's private records.
func ReadPrivateStorageInventory(ctx context.Context, tx pgx.Tx, scope storage.Scope) (*model.Snapshot, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tx == nil || !scope.Valid() || scope.Product != "cast" || tenant.Slug != scope.Tenant {
		return nil, pg.ErrPrivateStorageScope
	}
	// The same RLS identity as the existing personal export; no maintenance or
	// public-link bypass is used to discover someone else's contributions.
	if _, err := tx.Exec(ctx, `SELECT set_config('app.cast_persona',$1,true)`, scope.Subject); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, castPrivateInventorySQL, scope.Tenant, scope.Subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []model.Row{}
	var total int
	defer func() {
		for _, value := range values {
			clear(value.JSON)
		}
	}()
	for rows.Next() {
		var kind string
		var raw []byte
		if err := rows.Scan(&kind, &raw); err != nil {
			return nil, err
		}
		total += len(raw)
		if raw == nil || total > model.MaxRowBytes {
			clear(raw)
			return nil, model.ErrSnapshot
		}
		value, err := model.RetainedRow(kind, raw)
		clear(raw)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return model.NewSnapshot(scope, values)
}

const castPrivateInventorySQL = `WITH owned_videos AS MATERIALIZED (
 SELECT v.* FROM videos v WHERE v.tenant=$1 AND v.owner_sub=$2
), source(kind,payload) AS (
 SELECT 'video',convert_to(to_jsonb(v)::text,'UTF8') FROM owned_videos v
 UNION ALL SELECT 'person',convert_to(to_jsonb(p)::text,'UTF8') FROM video_people p JOIN owned_videos v ON v.tenant=p.tenant AND v.id=p.video_id
 UNION ALL SELECT 'job',convert_to(to_jsonb(j)::text,'UTF8') FROM transcode_jobs j JOIN owned_videos v ON v.tenant=j.tenant AND v.id=j.video_id
 UNION ALL SELECT 'comment',convert_to(to_jsonb(c)::text,'UTF8') FROM comments c JOIN owned_videos v ON v.tenant=c.tenant AND v.id=c.video_id
 UNION ALL SELECT 'chat',convert_to(to_jsonb(c)::text,'UTF8') FROM chat_messages c JOIN owned_videos v ON v.tenant=c.tenant AND v.id=c.video_id
 UNION ALL SELECT 'stream_key',convert_to(to_jsonb(k)::text,'UTF8') FROM stream_keys k WHERE k.tenant=$1 AND k.owner_sub=$2
 UNION ALL SELECT 'viewer',convert_to(to_jsonb(w)::text,'UTF8') FROM live_viewers w JOIN owned_videos v ON v.tenant=w.tenant AND v.id=w.video_id
 UNION ALL SELECT 'presentation',convert_to(to_jsonb(p)::text,'UTF8') FROM live_presentations p JOIN owned_videos v ON v.tenant=p.tenant AND v.id=p.video_id
 UNION ALL SELECT 'slide_mark',convert_to(to_jsonb(m)::text,'UTF8') FROM slide_marks m JOIN owned_videos v ON v.tenant=m.tenant AND v.id=m.video_id
 UNION ALL SELECT 'report',convert_to(to_jsonb(r)::text,'UTF8') FROM reports r JOIN owned_videos v ON v.tenant=r.tenant AND v.id=r.video_id WHERE r.reporter_sub=$2
 UNION ALL SELECT 'person_block',convert_to(to_jsonb(b)::text,'UTF8') FROM person_blocks b WHERE b.blocker_sub=$2 OR b.blocked_sub=$2
 UNION ALL SELECT 'external_access',convert_to(to_jsonb(p)::text,'UTF8') FROM video_people p WHERE p.user_id=$2 AND NOT EXISTS(SELECT 1 FROM owned_videos v WHERE v.tenant=p.tenant AND v.id=p.video_id)
 UNION ALL SELECT 'external_comment',convert_to(to_jsonb(c)::text,'UTF8') FROM comments c WHERE c.author_sub=$2 AND NOT EXISTS(SELECT 1 FROM owned_videos v WHERE v.tenant=c.tenant AND v.id=c.video_id)
 UNION ALL SELECT 'external_report',convert_to(to_jsonb(r)::text,'UTF8') FROM reports r WHERE r.reporter_sub=$2 AND NOT EXISTS(SELECT 1 FROM owned_videos v WHERE v.tenant=r.tenant AND v.id=r.video_id)
)
SELECT kind,CASE WHEN octet_length(payload)<=33554432 THEN payload END FROM source ORDER BY kind COLLATE "C",payload`
