// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// The same shape as every domain: a v1 fence migration, then the release
// migration a service adds after it.
var releaseFixture = pg.WithPrivateStorageDDL(fstest.MapFS{
	"migrations/0001_state.up.sql": {Data: []byte(`
CREATE TABLE fixture_private_storage_states (
 tenant text NOT NULL, owner_sub text NOT NULL CHECK(owner_sub<>''),
 mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN('legacy','building','frozen','private')),
 epoch uuid, started_at timestamptz,
 source_rows bigint NOT NULL DEFAULT 0, source_blobs bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(tenant,owner_sub),
 CHECK((mode='legacy' AND epoch IS NULL AND started_at IS NULL) OR (mode<>'legacy' AND epoch IS NOT NULL AND started_at IS NOT NULL)));
ALTER TABLE fixture_private_storage_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE fixture_private_storage_states FORCE ROW LEVEL SECURITY;
CREATE POLICY fixture_owner ON fixture_private_storage_states
 USING(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND owner_sub=current_setting('app.owner_sub',true));
CREATE POLICY fixture_maintenance ON fixture_private_storage_states
 USING(current_setting('app.maintenance',true)='on') WITH CHECK(current_setting('app.maintenance',true)='on');
CREATE TABLE fixture_rows(tenant text NOT NULL, owner_sub text NOT NULL, body text NOT NULL);
-- sytue:private-storage-fence-v1
CREATE TRIGGER fixture_state_guard BEFORE UPDATE OR DELETE ON fixture_private_storage_states
 FOR EACH ROW EXECUTE FUNCTION sytue_private_storage_state_scope_guard();
CREATE FUNCTION fixture_row_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM sytue_private_storage_guard('public.fixture_private_storage_states'::regclass,NEW.tenant,NEW.owner_sub);
 RETURN NEW;
END $$;
CREATE TRIGGER fixture_rows_guard BEFORE INSERT OR UPDATE ON fixture_rows FOR EACH ROW EXECUTE FUNCTION fixture_row_guard();
`)},
	"migrations/0001_state.down.sql": {Data: []byte(`DROP TABLE fixture_rows; DROP TABLE fixture_private_storage_states;`)},
	"migrations/0002_release.up.sql": {Data: []byte(`
-- sytue:private-storage-release-v1
SELECT sytue_private_storage_release_install('public.fixture_private_storage_states'::regclass);
`)},
	"migrations/0002_release.down.sql": {Data: []byte(`SELECT sytue_private_storage_release_uninstall('public.fixture_private_storage_states'::regclass);`)},
})

const releaseTable = "fixture_private_storage_states"

func releaseTx(ctx context.Context, t *testing.T, h *pgtest.Harness, owner string, fn func(pgx.Tx) error) error {
	t.Helper()
	return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
			return err
		}
		return fn(tx)
	})
}

func freezeFixture(ctx context.Context, t *testing.T, h *pgtest.Harness, owner string) uuid.UUID {
	t.Helper()
	epoch := uuid.New()
	if err := releaseTx(ctx, t, h, owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO fixture_rows VALUES('acme',$1,'before the freeze')`, owner); err != nil {
			return err
		}
		if err := pg.EnsurePrivateStorageState(ctx, tx, releaseTable, "acme", owner, nil); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE fixture_private_storage_states SET mode='frozen',epoch=$2,started_at=now(),source_rows=1 WHERE tenant='acme' AND owner_sub=$1`, owner, epoch)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return epoch
}

func TestSoltarLaFuenteSoloPorLaOperacionYConAuditoria(t *testing.T) {
	h := pgtest.New(t, releaseFixture, "migrations")
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	epoch := freezeFixture(ctx, t, h, "ada")

	// Frozen: the writer guard and the state guard keep rejecting everything else.
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO fixture_rows VALUES('acme','ada','after the freeze')`)
		return err
	}); err == nil || !strings.Contains(err.Error(), "private_storage_legacy_write_blocked") {
		t.Fatal("a frozen source accepted a legacy write", err)
	}
	for _, update := range []string{
		`UPDATE fixture_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL,source_rows=0 WHERE owner_sub='ada'`,
		`UPDATE fixture_private_storage_states SET releases='[{"epoch":"x"}]' WHERE owner_sub='ada'`,
	} {
		if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error { _, err := tx.Exec(ctx, update); return err }); err == nil {
			t.Fatal("a plain UPDATE reopened or rewrote a frozen source:", update)
		}
	}
	// A forged GUC for another epoch does not open it either.
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.private_storage_release',sytue_private_storage_release_key('fixture_private_storage_states','acme','ada',$1),true)`, uuid.New()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE fixture_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL,source_rows=0,releases=releases||'[{"epoch":"x"}]' WHERE owner_sub='ada'`)
		return err
	}); err == nil {
		t.Fatal("a release key for another epoch reopened the source")
	}
	// A well-formed reopening without the operation's key is still rejected…
	wellFormed := `UPDATE fixture_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL,source_rows=0,releases=releases||jsonb_build_array(jsonb_build_object('epoch',$1::text)) WHERE owner_sub='ada'`
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error { _, err := tx.Exec(ctx, wellFormed, epoch); return err }); err == nil {
		t.Fatal("a plain UPDATE with a valid audit entry reopened the source")
	}
	// …and the right key does not accept an audit entry for another epoch.
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.private_storage_release',sytue_private_storage_release_key('fixture_private_storage_states','acme','ada',$1),true)`, epoch); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, wellFormed, uuid.New())
		return err
	}); err == nil {
		t.Fatal("the release key accepted an audit entry for another epoch")
	}

	authTime := time.Now().Add(-time.Minute).Truncate(time.Second)
	release := pg.PrivateStorageRelease{Epoch: uuid.New(), AuthTime: authTime, AMR: []string{"pwd", "otp"}}
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		_, err := pg.ReleasePrivateStorageState(ctx, tx, releaseTable, "acme", "ada", release)
		return err
	}); !errors.Is(err, pg.ErrPrivateStorageReleaseStale) {
		t.Fatal("a stale epoch released the source", err)
	}
	// Under maintenance the release is refused even with the right epoch.
	release.Epoch = epoch
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.maintenance','on',true)`); err != nil {
			return err
		}
		_, err := pg.ReleasePrivateStorageState(ctx, tx, releaseTable, "acme", "ada", release)
		return err
	}); err == nil {
		t.Fatal("release ran under maintenance")
	}

	var state pg.PrivateStorageState
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		var err error
		state, err = pg.ReleasePrivateStorageState(ctx, tx, releaseTable, "acme", "ada", release)
		return err
	}); err != nil || state.Mode != "legacy" || state.Epoch != "" || state.Rows != 0 {
		t.Fatal("release did not return to legacy", state, err)
	}
	// Legacy again: the server data as it was at the freeze, and writable.
	var audit []byte
	var rows int
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO fixture_rows VALUES('acme','ada','after the release')`); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fixture_rows WHERE owner_sub='ada'`).Scan(&rows); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT releases FROM fixture_private_storage_states WHERE owner_sub='ada'`).Scan(&audit)
	}); err != nil || rows != 2 {
		t.Fatal("released source is not writable", rows, err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(audit, &entries); err != nil || len(entries) != 1 {
		t.Fatal("release audit", string(audit), err)
	}
	if entries[0]["epoch"] != epoch.String() || entries[0]["mode"] != "frozen" || entries[0]["source_rows"] != float64(1) || entries[0]["released_at"] == nil || entries[0]["auth_time"] == nil {
		t.Fatal("release audit is incomplete", entries[0])
	}
	// The audit only grows through another release.
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE fixture_private_storage_states SET releases='[]' WHERE owner_sub='ada'`)
		return err
	}); err == nil {
		t.Fatal("a legacy row rewrote its release audit")
	}
	// Releasing a legacy source is stale, never a second audit entry.
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		_, err := pg.ReleasePrivateStorageState(ctx, tx, releaseTable, "acme", "ada", release)
		return err
	}); !errors.Is(err, pg.ErrPrivateStorageReleaseStale) {
		t.Fatal("released twice", err)
	}
	// Another owner cannot release ada's source: RLS hides the row.
	other := freezeFixture(ctx, t, h, "bob")
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		_, err := pg.ReleasePrivateStorageState(ctx, tx, releaseTable, "acme", "bob", pg.PrivateStorageRelease{Epoch: other, AuthTime: authTime})
		return err
	}); err == nil {
		t.Fatal("one owner released another owner's source")
	}
	// The frozen → private → legacy path through the operation, too.
	if err := releaseTx(ctx, t, h, "bob", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE fixture_private_storage_states SET mode='private' WHERE owner_sub='bob'`); err != nil {
			return err
		}
		state, err := pg.ReleasePrivateStorageState(ctx, tx, releaseTable, "acme", "bob", pg.PrivateStorageRelease{Epoch: other, AuthTime: authTime})
		if err == nil && state.Mode != "legacy" {
			return errors.New(state.Mode)
		}
		return err
	}); err != nil {
		t.Fatal("private source did not release", err)
	}
	// And the released source can be migrated again with a new epoch.
	if err := releaseTx(ctx, t, h, "ada", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE fixture_private_storage_states SET mode='frozen',epoch=$1,started_at=now(),source_rows=2 WHERE owner_sub='ada'`, uuid.New())
		return err
	}); err != nil {
		t.Fatal("a released source cannot be migrated again", err)
	}
}
