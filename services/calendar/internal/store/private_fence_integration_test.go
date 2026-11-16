// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func freezeCalendarFixture(ctx context.Context, t *testing.T, h *pgtest.Harness, owner string) {
	t.Helper()
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT calendar_private_lock_identities($1,$2)`, owner, []string{owner + "@example.invalid"}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
			return err
		}
		if err := pg.EnsurePrivateStorageState(ctx, tx, "calendar_private_storage_states", "acme", owner, nil); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE calendar_private_storage_states SET mode='building',epoch=gen_random_uuid(),started_at=clock_timestamp() WHERE tenant='acme' AND owner_sub=$1`, owner)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO calendar_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload) SELECT tenant,owner_sub,epoch,'calendar',gen_random_uuid()::text,convert_to('{"retained":true}','UTF8') FROM calendar_private_storage_states WHERE tenant='acme' AND owner_sub=$1`, owner)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO calendar_private_storage_aliases(tenant,owner_sub,epoch,address) SELECT tenant,owner_sub,epoch,$2 FROM calendar_private_storage_states WHERE tenant='acme' AND owner_sub=$1`, owner, owner+"@example.invalid")
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE calendar_private_storage_states SET mode='frozen',source_rows=1 WHERE tenant='acme' AND owner_sub=$1`, owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requireCalendarFence(t *testing.T, err error) {
	t.Helper()
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "PST01" {
		t.Fatalf("expected private storage fence, got %v", err)
	}
}

func TestCalendarPrivateFenceCoversEveryWriterAndRetainsSource(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	st := store.New(h.Pool)
	c, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "alice", Name: "private"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(time.Hour)
	e, o, err := st.CreateEvent(ctx, domain.EventInput{CalendarID: c.ID, UID: "private", ResourceName: "private.ics", ICalData: "original", DTStart: start, DTEnd: start.Add(time.Hour), Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := st.CreateResource(ctx, store.NewResource{Kind: domain.ResourceRoom, Name: "Room", CreatedBy: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO attendees(tenant,event_id,email) VALUES('acme','` + e.ID.String() + `','bob@example.invalid')`,
		`INSERT INTO invitations(tenant,event_id,uid,method,recipient) VALUES('acme','` + e.ID.String() + `','private','REQUEST','bob@example.invalid')`,
		`INSERT INTO calendar_reminders(tenant,owner_sub,calendar_id,object_id,event_id,uid,dtstart,trigger_at) VALUES('acme','alice','` + c.ID.String() + `','` + o.ID.String() + `','` + e.ID.String() + `','private',now(),now())`,
		`INSERT INTO calendar_subscriptions(tenant,calendar_id,url) VALUES('acme','` + c.ID.String() + `','https://example.invalid/feed')`,
		`INSERT INTO calendar_shares(tenant,calendar_id,grantee_sub,permission,created_by) VALUES('acme','` + c.ID.String() + `','bob','reader','alice')`,
		`INSERT INTO calendar_peticiones_edicion(tenant,calendar_id,sujeto) VALUES('acme','` + c.ID.String() + `','bob')`,
		`INSERT INTO events_outbox(id,tenant,channel,type,payload) VALUES(gen_random_uuid(),'acme','calendar','changed',jsonb_build_object('event_id','` + e.ID.String() + `'))`,
	}
	for _, statement := range statements {
		if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, statement); return err }); err != nil {
			t.Fatal(err)
		}
	}
	freezeCalendarFixture(ctx, t, h, "alice")
	for _, table := range []string{"calendars", "calendar_objects", "events", "attendees", "invitations", "calendar_reminders", "calendar_subscriptions", "calendar_shares", "calendar_peticiones_edicion", "calendar_resources", "events_outbox"} {
		t.Run(table, func(t *testing.T) {
			for _, sql := range []string{`UPDATE ` + table + ` SET tenant=tenant WHERE tenant='acme'`, `DELETE FROM ` + table + ` WHERE tenant='acme'`} {
				err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, sql); return err })
				requireCalendarFence(t, err)
			}
		})
	}
	_, err = st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "alice", Name: "late"})
	requireCalendarFence(t, err)
	_, err = st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "bob", Name: "legacy"})
	if err != nil {
		t.Fatal("unrelated legacy owner blocked", err)
	}
	lateName := "late"
	_, err = st.UpdateResource(ctx, resource.ID, "alice", false, store.ResourcePatch{Name: &lateName})
	requireCalendarFence(t, err)
	for _, sql := range []string{
		`UPDATE calendar_private_storage_snapshot SET payload=convert_to('{}','UTF8') WHERE tenant='acme'`,
		`DELETE FROM calendar_private_storage_snapshot WHERE tenant='acme'`,
		`UPDATE calendar_private_storage_states SET tenant='other' WHERE tenant='acme' AND owner_sub='alice'`,
		`UPDATE calendar_private_storage_states SET mode='legacy',epoch=NULL,started_at=NULL WHERE tenant='acme' AND owner_sub='alice'`,
		`INSERT INTO calendar_private_storage_snapshot(tenant,owner_sub,epoch,kind,id,payload) SELECT tenant,owner_sub,epoch,'event',gen_random_uuid()::text,convert_to('{}','UTF8') FROM calendar_private_storage_states WHERE tenant='acme' AND owner_sub='alice'`,
		`UPDATE calendar_private_storage_aliases SET address='reassigned@example.invalid' WHERE tenant='acme' AND owner_sub='alice'`,
	} {
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','alice',true),set_config('app.merge','on',true)`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, sql)
			return err
		})
		requireCalendarFence(t, err)
	}
	for _, owner := range []string{"alice", "bob"} {
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub',$1,true)`, owner); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM calendar_private_storage_snapshot WHERE tenant='acme'`).Scan(&count); err != nil {
				return err
			}
			want := 0
			if owner == "alice" {
				want = 1
			}
			if count != want {
				t.Fatalf("owner RLS: %s saw %d rows", owner, count)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCalendarPrivateIngressGuardsForeignTenantSubjectAndVerifiedAliases(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	freezeCalendarFixture(tenantCtx("acme"), t, h, "alice")
	ctx := tenantCtx("foreign")
	st := store.New(h.Pool)
	c, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "bob", Name: "foreign"})
	if err != nil {
		t.Fatal(err)
	}
	for _, recipient := range []struct{ sub, email string }{{"alice", "unknown@example.invalid"}, {"", " ALICE@example.invalid "}} {
		err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO invitations(tenant,uid,method,recipient,recipient_sub,data) VALUES('foreign','external','REQUEST',$1,$2,'original private ICS')`, recipient.email, recipient.sub)
			return err
		})
		requireCalendarFence(t, err)
		err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO calendar_shares(tenant,calendar_id,grantee_sub,grantee_email,permission,created_by) VALUES('foreign',$1,$2,$3,'reader','bob')`, c.ID, recipient.sub, recipient.email)
			return err
		})
		requireCalendarFence(t, err)
	}
	_, err = st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "alice", Name: "another-scope"})
	if err != nil {
		t.Fatal("explicit unrelated owner scope was blocked", err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO invitations(tenant,uid,method,recipient,recipient_sub,data) VALUES('foreign','external','REQUEST','carol@example.invalid','carol','other invitation')`)
		return err
	})
	if err != nil {
		t.Fatal("unrelated ingress blocked", err)
	}
}

func TestCalendarPrivateIngressRejectsRepeatableReadMissingAlias(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("foreign")
	tx, err := h.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := pg.SetTenant(ctx, tx, "foreign"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM calendar_private_storage_aliases WHERE tenant='foreign'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	freezeCalendarFixture(tenantCtx("acme"), t, h, "alice")
	_, err = tx.Exec(ctx, `INSERT INTO invitations(tenant,uid,method,recipient,data) VALUES('foreign','external','REQUEST','alice@example.invalid','must not escape stale aliases')`)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("stale ingress was not rejected: %v", err)
	}
}

func waitCalendarBlocked(ctx context.Context, t *testing.T, pool *pg.Pool, blocked, blocker int32) {
	t.Helper()
	pgtest.WaitBlocked(ctx, t, pool, blocked, blocker)
}

func TestCalendarPrivateIngressBeforeIdentityRegistrationMustCommitBeforePreflight(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(tenantCtx("foreign"), 10*time.Second)
	defer cancel()
	writer, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	if err := pg.SetTenant(ctx, writer, "foreign"); err != nil {
		t.Fatal(err)
	}
	var writerPID int32
	if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `INSERT INTO invitations(tenant,uid,method,recipient,data) VALUES('foreign','external','REQUEST','alice@example.invalid','writer before aliases')`); err != nil {
		t.Fatal(err)
	}
	pid := make(chan int32, 1)
	done := make(chan error, 1)
	var observed int
	go func() {
		done <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			var p int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&p); err != nil {
				return err
			}
			pid <- p
			if _, err := tx.Exec(ctx, `SELECT calendar_private_lock_identities('alice',ARRAY['alice@example.invalid'])`); err != nil {
				return err
			}
			if err := pg.SetMaintenance(ctx, tx); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT count(*) FROM invitations WHERE tenant='foreign' AND recipient='alice@example.invalid'`).Scan(&observed)
		})
	}()
	var current int32
	select {
	case current = <-pid:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitCalendarBlocked(ctx, t, h.Pool, current, writerPID)
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil || observed != 1 {
		t.Fatal("preflight missed committed foreign ingress", observed, err)
	}
}

func TestCalendarPrivateIngressDuringRegistrationWaitsThenRejectsFrozenRecipient(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(tenantCtx("acme"), 10*time.Second)
	defer cancel()
	freeze, err := h.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = freeze.Rollback(context.Background()) }()
	if err := pg.SetTenant(ctx, freeze, "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := freeze.Exec(ctx, `SELECT calendar_private_lock_identities('alice',ARRAY['alice@example.invalid']),set_config('app.owner_sub','alice',true)`); err != nil {
		t.Fatal(err)
	}
	if err := pg.EnsurePrivateStorageState(ctx, freeze, "calendar_private_storage_states", "acme", "alice", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := freeze.Exec(ctx, `UPDATE calendar_private_storage_states SET mode='building',epoch=gen_random_uuid(),started_at=clock_timestamp() WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
		t.Fatal(err)
	}
	if _, err := freeze.Exec(ctx, `INSERT INTO calendar_private_storage_aliases(tenant,owner_sub,epoch,address) SELECT tenant,owner_sub,epoch,'alice@example.invalid' FROM calendar_private_storage_states WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
		t.Fatal(err)
	}
	var freezePID int32
	if err := freeze.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&freezePID); err != nil {
		t.Fatal(err)
	}
	pid := make(chan int32, 1)
	done := make(chan error, 1)
	go func() {
		done <- h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, "foreign"); err != nil {
				return err
			}
			var p int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&p); err != nil {
				return err
			}
			pid <- p
			_, err := tx.Exec(ctx, `INSERT INTO invitations(tenant,uid,method,recipient,data) VALUES('foreign','external','REQUEST','alice@example.invalid','during freeze')`)
			return err
		})
	}()
	var current int32
	select {
	case current = <-pid:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitCalendarBlocked(ctx, t, h.Pool, current, freezePID)
	if _, err := freeze.Exec(ctx, `UPDATE calendar_private_storage_states SET mode='frozen' WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
		t.Fatal(err)
	}
	if err := freeze.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	requireCalendarFence(t, <-done)
}

func TestCalendarPrivateWorkersSkipFrozenOwnersWithoutStoppingLegacyWork(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	st := store.New(h.Pool)
	for _, owner := range []string{"alice", "bob"} {
		c, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: owner, Name: "calendar"})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now().UTC().Add(time.Hour)
		e, o, err := st.CreateEvent(ctx, domain.EventInput{CalendarID: c.ID, UID: owner, ResourceName: owner + ".ics", ICalData: "original", DTStart: start, DTEnd: start.Add(time.Hour), Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque})
		if err != nil {
			t.Fatal(err)
		}
		err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO calendar_reminders(tenant,owner_sub,calendar_id,object_id,event_id,uid,dtstart,trigger_at) VALUES('acme',$1,$2,$3,$4,$1,now(),now()-interval '1 hour')`, owner, c.ID, o.ID, e.ID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO calendar_subscriptions(tenant,calendar_id,url) VALUES('acme',$1,'https://example.invalid/feed')`, c.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	freezeCalendarFixture(ctx, t, h, "alice")
	n, err := st.ClaimDueReminders(ctx, time.Now().UTC(), 100, func(_ context.Context, _ pg.Querier, r store.DueReminder) error {
		if r.OwnerSub != "bob" {
			t.Fatal("frozen reminder published")
		}
		return nil
	})
	if err != nil || n != 1 {
		t.Fatal("legacy reminder batch failed", n, err)
	}
	jobs, err := st.ClaimDueSubscriptions(ctx, 100)
	if err != nil || len(jobs) != 1 {
		t.Fatal("legacy subscription batch failed", len(jobs), err)
	}
	if jobs[0].ID == uuid.Nil {
		t.Fatal("empty legacy subscription")
	}
}
