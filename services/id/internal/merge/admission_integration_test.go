// SPDX-License-Identifier: Apache-2.0
//go:build integration

package merge

import (
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	calendardb "git.sytue.eu/sytue/one/services/calendar/db"
	contactsdb "git.sytue.eu/sytue/one/services/contacts/db"
	drivedb "git.sytue.eu/sytue/one/services/drive/db"
	maildb "git.sytue.eu/sytue/one/services/mail/db"
	notesdb "git.sytue.eu/sytue/one/services/notes/db"
	roomsdb "git.sytue.eu/sytue/one/services/rooms/db"
	photosdb "git.sytue.eu/sytue/one/services/syphotos/db"
)

func TestScopeAdmissionCoversEveryRetainedScopeInSevenServiceSchemas(t *testing.T) {
	schemas := map[string]struct {
		fs  fs.FS
		dir string
	}{
		"calendar": {calendardb.Migrations, calendardb.MigrationsDir}, "contacts": {contactsdb.Migrations, contactsdb.MigrationsDir},
		"drive": {drivedb.Migrations, drivedb.MigrationsDir}, "mail": {maildb.Migrations, maildb.MigrationsDir},
		"notes": {notesdb.Migrations, notesdb.MigrationsDir}, "rooms": {roomsdb.Migrations, roomsdb.MigrationsDir}, "syphotos": {photosdb.Migrations, photosdb.MigrationsDir},
	}
	for service, schema := range schemas {
		t.Run(service, func(t *testing.T) {
			h := pgtest.New(t, schema.fs, schema.dir)
			inventory, err := pg.ConsultaInventarioDeFusion(t.Context(), h.Pool)
			if err != nil {
				t.Fatal(err)
			}
			if len(inventory.ScopeBound)+len(inventory.PrivateStates) == 0 {
				t.Fatal("service lost all Scope-bound contract markers")
			}
			if len(inventory.Missing) != 0 {
				t.Fatal("Scope admission changed RLS merge contract", inventory.Missing)
			}
			if err := pg.RequireScopeAdmissionCoverage(t.Context(), h.Pool, inventory); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScopeAdmissionPhotosLegacyMovesButCapturedScopesRemainUnchanged(t *testing.T) {
	for _, capture := range []bool{false, true} {
		name := "legacy"
		if capture {
			name = "captured"
		}
		t.Run(name, func(t *testing.T) {
			h := pgtest.New(t, photosdb.Migrations, photosdb.MigrationsDir)
			ctx := t.Context()
			admin := h.PoolSinRLS(t)
			if _, err := admin.Exec(ctx, `INSERT INTO assets(tenant,owner_sub,drive_file_id,name,media_type,bytes,width,height) VALUES('source','alice','retained-original','Synthetic photo','image/jpeg',17,1,1)`); err != nil {
				t.Fatal(err)
			}
			if capture {
				if _, err := admin.Exec(ctx, `INSERT INTO photos_private_storage_preflights(tenant,owner_sub,epoch,ticket,inventory_sha256,backend_rows,backend_blobs,original_blobs) VALUES('source','alice',gen_random_uuid(),gen_random_uuid(),repeat('a',64),0,0,1)`); err != nil {
					t.Fatal(err)
				}
			}
			_, err := MoveService(ctx, h.Pool, "source", "target")
			expected := "target"
			if capture {
				expected = "source"
				if !errors.Is(err, pg.ErrScopeBoundMerge) {
					t.Fatal("captured Photos scope was accepted for retagging", err)
				}
			} else if err != nil {
				t.Fatal("legacy Photos merge failed", err)
			}
			var tenant string
			if err := admin.QueryRow(ctx, `SELECT tenant FROM assets WHERE drive_file_id='retained-original'`).Scan(&tenant); err != nil || tenant != expected {
				t.Fatal("Photos domain moved despite retained capture", tenant, expected, err)
			}
			if capture {
				if err := admin.QueryRow(ctx, `SELECT tenant FROM photos_private_storage_preflights`).Scan(&tenant); err != nil || tenant != "source" {
					t.Fatal("Photos capture scope was rewritten", tenant, err)
				}
			}
		})
	}
}

func TestScopeAdmissionDisabledStateGuardRemainsInventoriedAndBlocksMerge(t *testing.T) {
	for _, action := range []string{"DISABLE TRIGGER", "DROP TRIGGER"} {
		t.Run(action, func(t *testing.T) {
			h := pgtest.New(t, drivedb.Migrations, drivedb.MigrationsDir)
			ctx := t.Context()
			admin := h.PoolSinRLS(t)
			statement := `ALTER TABLE drive_private_storage_states DISABLE TRIGGER drive_private_storage_state_scope_guard`
			if action == "DROP TRIGGER" {
				statement = `DROP TRIGGER drive_private_storage_state_scope_guard ON drive_private_storage_states`
			}
			if _, err := admin.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			inventory, err := pg.ConsultaInventarioDeFusion(ctx, h.Pool)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(inventory.PrivateStates, "drive_private_storage_states") {
				t.Fatal("missing immutable state guard converted frozen Scope to movable data", inventory)
			}
			if err := pg.RequireScopeAdmissionCoverage(ctx, h.Pool, inventory); err == nil {
				t.Fatal("missing immutable state guard accepted for suite merge")
			}
		})
	}
}

func TestScopeAdmissionPhotosCaptureIdentityAndAcceptedRoutingAreImmutable(t *testing.T) {
	h := pgtest.New(t, photosdb.Migrations, photosdb.MigrationsDir)
	ctx := t.Context()
	admin := h.PoolSinRLS(t)
	if _, err := admin.Exec(ctx, `INSERT INTO photos_private_storage_preflights(tenant,owner_sub,epoch,ticket,inventory_sha256,backend_rows,backend_blobs,original_blobs) VALUES('source','alice',gen_random_uuid(),gen_random_uuid(),repeat('a',64),0,0,1)`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE photos_private_storage_preflights SET tenant='target'`,
		`UPDATE photos_private_storage_preflights SET owner_sub='different-owner'`,
		`UPDATE photos_private_storage_preflights SET epoch=gen_random_uuid()`,
		`UPDATE photos_private_storage_preflights SET inventory_sha256=repeat('b',64)`,
		`DELETE FROM photos_private_storage_preflights`,
	} {
		_, err := admin.Exec(ctx, statement)
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "PST01" {
			t.Fatal("maintenance changed retained capture identity", statement, err)
		}
	}
	if _, err := admin.Exec(ctx, `UPDATE photos_private_storage_preflights SET plan_room_id='!first:fixture.invalid',plan_event_id='$first'`); err != nil {
		t.Fatal("initial plan CAS rejected", err)
	}
	if _, err := admin.Exec(ctx, `UPDATE photos_private_storage_preflights SET plan_room_id='!first:fixture.invalid',plan_event_id='$first'`); err != nil {
		t.Fatal("same plan CAS was not idempotent", err)
	}
	_, err := admin.Exec(ctx, `UPDATE photos_private_storage_preflights SET plan_room_id='!different:fixture.invalid',plan_event_id='$different'`)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "PST01" {
		t.Fatal("maintenance replaced accepted plan routing", err)
	}
	var n int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM photos_private_storage_states WHERE tenant='target' OR owner_sub='different-owner'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("rejected retagging left a new scope behind", n, err)
	}
}
