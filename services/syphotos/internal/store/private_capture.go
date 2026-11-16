// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// photoCaptureSummary is independent of library size. The complete transient
// capture lives in PostgreSQL's transaction-local table, not Go slices; raw JSON
// is transferred and validated one entity at a time. Only source commitments
// survive preflight, never another permanent copy of legacy EXIF/vectors.
type photoCaptureSummary struct {
	SHA256                 string
	Rows, Blobs, Originals int64
}

func (c photoCaptureSummary) matches(row photoPreflight) bool {
	return c.SHA256 == row.inventory && c.Rows == row.rows && c.Blobs == row.blobs && c.Originals == row.originals
}

const photoCaptureTempDDL = `CREATE TEMP TABLE sytue_photos_capture (
 kind text COLLATE "C" NOT NULL,id text COLLATE "C" NOT NULL,payload bytea,
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 0 AND 140737488355328),
 sha256 text NOT NULL,storage_key text,
 PRIMARY KEY(kind,id),
 CHECK((kind='variant' AND payload IS NULL AND storage_key IS NOT NULL) OR
       (kind<>'variant' AND payload IS NOT NULL AND storage_key IS NULL AND octet_length(payload) BETWEEN 2 AND 33554432))
) ON COMMIT DROP`

func capturePhotoSourceBounded(ctx context.Context, tx pgx.Tx, objects retainedVariantStore, scope storage.Scope, epoch string) (photoCaptureSummary, error) {
	if !storage.MigrationEpoch(epoch) || objects == nil {
		return photoCaptureSummary{}, ErrPrivateSource
	}
	var shared bool
	if err := tx.QueryRow(ctx, photoSourceSharedSQL, scope.Tenant, scope.Subject).Scan(&shared); err != nil {
		return photoCaptureSummary{}, err
	}
	if shared {
		return photoCaptureSummary{}, model.ErrShared
	}
	if _, err := tx.Exec(ctx, photoCaptureTempDDL); err != nil {
		return photoCaptureSummary{}, err
	}
	_, err := tx.Exec(ctx, `INSERT INTO pg_temp.sytue_photos_capture(kind,id,payload,size_bytes,sha256)
 SELECT kind,id,payload,octet_length(payload),encode(digest(payload,'sha256'),'hex') FROM (`+photosPrivateInventorySQL+`) retained`, scope.Tenant, scope.Subject)
	if err != nil {
		return photoCaptureSummary{}, err
	}
	if err := validatePhotoCaptureRows(ctx, tx, scope); err != nil {
		return photoCaptureSummary{}, err
	}
	if err := validatePhotoCaptureGraph(ctx, tx); err != nil {
		return photoCaptureSummary{}, err
	}
	count, err := capturePhotoVariantsBounded(ctx, tx, objects, scope)
	if err != nil {
		return photoCaptureSummary{}, err
	}
	result, err := summarizePhotoCapture(ctx, tx, scope, epoch)
	result.Originals = count
	return result, err
}

// Match Snapshot.validateGraph using indexed identity references only. The
// original tables normally enforce these relationships, but retained legacy
// corruption must still abort before a source is frozen.
func validatePhotoCaptureGraph(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `CREATE TEMP TABLE sytue_photos_capture_refs ON COMMIT DROP AS
 SELECT kind,id,j->>'asset_id' asset,j->>'album_id' album,j->>'drive_file_id' drive,
 j->>'owner_sub' owner,j->>'cover_asset_id' cover
 FROM pg_temp.sytue_photos_capture c CROSS JOIN LATERAL(SELECT convert_from(c.payload,'UTF8')::jsonb j)x;
 CREATE UNIQUE INDEX ON pg_temp.sytue_photos_capture_refs(kind,id)`)
	if err != nil {
		return err
	}
	var invalid bool
	err = tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM pg_temp.sytue_photos_capture_refs WHERE kind='asset' GROUP BY drive HAVING count(*)<>1)
 OR EXISTS(SELECT 1 FROM pg_temp.sytue_photos_capture_refs WHERE kind='edit' GROUP BY asset,owner HAVING count(*)<>1)
 OR EXISTS(SELECT 1 FROM pg_temp.sytue_photos_capture_refs r WHERE
 (r.kind='album' AND r.cover IS NOT NULL AND NOT EXISTS(SELECT 1 FROM pg_temp.sytue_photos_capture_refs a WHERE a.kind='asset' AND a.id=r.cover))
 OR(r.kind IN('edit','album_asset') AND NOT EXISTS(SELECT 1 FROM pg_temp.sytue_photos_capture_refs a WHERE a.kind='asset' AND a.id=r.asset))
 OR(r.kind IN('album_asset','album_person') AND NOT EXISTS(SELECT 1 FROM pg_temp.sytue_photos_capture_refs a WHERE a.kind='album' AND a.id=r.album)))`).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return model.ErrSnapshot
	}
	return nil
}

func validatePhotoCaptureRows(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
	rows, err := tx.Query(ctx, `SELECT kind,id,payload FROM pg_temp.sytue_photos_capture ORDER BY kind,id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row model.Row
		if err := rows.Scan(&row.Kind, &row.ID, &row.JSON); err != nil {
			return err
		}
		err := model.ValidateRow(scope, row)
		clear(row.JSON)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return rows.Err()
}

func capturePhotoVariantsBounded(ctx context.Context, tx pgx.Tx, objects retainedVariantStore, scope storage.Scope) (int64, error) {
	after := ""
	var originals int64
	for {
		var asset string
		err := tx.QueryRow(ctx, `SELECT id FROM pg_temp.sytue_photos_capture WHERE kind='asset' AND id>$1 ORDER BY id LIMIT 1`, after).Scan(&asset)
		if errors.Is(err, pgx.ErrNoRows) {
			return originals, nil
		}
		if err != nil {
			return 0, err
		}
		if err := walkAssetVariants(ctx, objects, scope, asset, func(value retainedVariant) error { return insertPhotoCapturedVariant(ctx, tx, value) }); err != nil {
			return 0, err
		}
		after = asset
		originals++
	}
}

func walkAssetVariants(ctx context.Context, objects retainedVariantStore, scope storage.Scope, asset string, consume func(retainedVariant) error) error {
	cursor, last := "", ""
	for {
		page, err := objects.List(ctx, tenancy.Tenant{Slug: scope.Tenant}, model.PrefijoDelActivo(uuid.MustParse(asset)), cursor, 1000)
		if err != nil {
			return err
		}
		if len(page.Objects) > 1000 || len(page.Objects) == 0 && page.Next != "" || page.Next != "" && page.Next == cursor {
			return ErrPrivateSource
		}
		for _, object := range page.Objects {
			if object.Key <= last {
				return ErrPrivateSource
			}
			last = object.Key
			value, err := retainedVariantMedia(ctx, objects, scope, asset, object)
			if err != nil {
				return err
			}
			if err := consume(value); err != nil {
				return err
			}
		}
		if page.Next == "" {
			return ctx.Err()
		}
		cursor = page.Next
	}
}

func insertPhotoCapturedVariant(ctx context.Context, tx pgx.Tx, value retainedVariant) error {
	raw, err := json.Marshal(value.Media)
	if err != nil {
		return err
	}
	defer clear(raw)
	descriptor := value.Media.Descriptor()
	_, err = tx.Exec(ctx, `INSERT INTO pg_temp.sytue_photos_capture(kind,id,payload,size_bytes,sha256,storage_key) VALUES
 ('variant',$1,NULL,$2,$3,$4),('media',$1,$5,octet_length($5::bytea),encode(digest($5::bytea,'sha256'),'hex'),NULL)`, descriptor.ID, descriptor.Size, descriptor.SHA256, value.Key, raw)
	return err
}

func summarizePhotoCapture(ctx context.Context, tx pgx.Tx, scope storage.Scope, epoch string) (photoCaptureSummary, error) {
	rows, err := tx.Query(ctx, `SELECT kind,id,size_bytes,sha256 FROM pg_temp.sytue_photos_capture ORDER BY kind,id`)
	if err != nil {
		return photoCaptureSummary{}, err
	}
	defer rows.Close()
	var result photoCaptureSummary
	sum, err := model.SourceInventoryHash(scope, epoch)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var row storage.StreamSourceRow
		if err := rows.Scan(&row.Kind, &row.ID, &row.Size, &row.SHA256); err != nil {
			return photoCaptureSummary{}, err
		}
		if err := model.AppendSourceInventoryHash(sum, row); err != nil {
			return photoCaptureSummary{}, err
		}
		result.Rows++
		if row.Kind == "variant" {
			result.Blobs++
		}
	}
	result.SHA256 = hex.EncodeToString(sum.Sum(nil))
	return result, rows.Err()
}
