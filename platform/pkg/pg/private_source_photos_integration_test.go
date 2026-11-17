// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestPrivateSourcePhotosExternalPolicyRetainsVariantWithoutMaterializingMedia(t *testing.T) {
	h := newPrivateSourceHarness(t)
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: "acme"})
	cfg := privateSourceConfig("photos")
	cfg.Kind = func(k string) bool { return k == "calendar" || k == "object" || k == "variant" || k == "mime_message" }
	seedPrivateSource(ctx, t, h, cfg)
	const key = "a/retained-asset/old-recipe/thumb.png"
	err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE photos_private_storage_states SET source_rows=4,source_blobs=1 WHERE tenant=$1 AND owner_sub=$2`, cfg.Scope.Tenant, cfg.Scope.Subject); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO photos_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload,size_bytes,sha256,storage_key)VALUES($1,$2,$3,'variant','retained',NULL,1099511627776,repeat('a',64),$4)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Epoch, key)
		if err != nil {
			return err
		}
		page, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{Kind: "object", ID: "z"}, 10)
		if err != nil || len(page.Rows) != 1 || page.Rows[0].Kind != "variant" || page.Rows[0].Size != 1<<40 {
			return privateSourceFailure("external media descriptor lost", err)
		}
		body, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, "variant", "retained")
		if err != nil || body.StorageKey != key || body.Descriptor != page.Rows[0] {
			return privateSourceFailure("external media routing lost", err)
		}
		if _, err := body.OpenJSON(ctx); !errors.Is(err, pg.ErrPrivateSource) {
			return privateSourceFailure("external media exposed as JSON", err)
		}
		raw, err := json.Marshal(body)
		if err != nil || bytes.Contains(raw, []byte(key)) {
			return privateSourceFailure("internal object key escaped descriptor", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"missing key", "inline media", "metadata external", "other product kind"} {
		t.Run(change, func(t *testing.T) {
			err := privateSourceTestTx(ctx, h, cfg, func(tx pgx.Tx) error {
				// Roll back each substitution independently of the retained source.
				if _, err := tx.Exec(ctx, "SAVEPOINT changed_variant"); err != nil {
					return err
				}
				query := `UPDATE photos_private_storage_snapshot SET storage_key=NULL WHERE kind='variant'`
				kind, id := "variant", "retained"
				switch change {
				case "inline media":
					query = `UPDATE photos_private_storage_snapshot SET storage_key=NULL,payload=convert_to('{}','UTF8'),size_bytes=2,sha256=encode(sha256(convert_to('{}','UTF8')),'hex') WHERE kind='variant'`
				case "metadata external":
					query = `UPDATE photos_private_storage_snapshot SET storage_key='a/retained-asset/old-recipe/thumb.png' WHERE kind='object'`
					kind, id = "object", "z"
				case "other product kind":
					query = `UPDATE photos_private_storage_snapshot SET kind='mime_message' WHERE kind='variant'`
					kind = "mime_message"
				}
				if _, err := tx.Exec(ctx, query); err != nil {
					return err
				}
				if _, err := pg.ReadPrivateStorageSourceBody(ctx, tx, cfg, kind, id); !errors.Is(err, pg.ErrPrivateSource) {
					return privateSourceFailure("wrong external routing opened", change, err)
				}
				if _, err := pg.ReadPrivateStorageSourcePage(ctx, tx, cfg, storage.LegacySourceCursor{}, 10); !errors.Is(err, pg.ErrPrivateSource) {
					return privateSourceFailure("wrong external routing paged", change, err)
				}
				_, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT changed_variant")
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
