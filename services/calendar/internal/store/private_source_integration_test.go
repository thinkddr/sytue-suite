// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

func stagedCalendarContext() context.Context {
	return authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Tenant: "acme", Subject: "alice", Email: "alice@example.invalid"})
}

func TestCalendarPrivateStagedSnapshotRetainsExactOriginalsAndBeginStaysClosed(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := New(h.Pool)
	ctx := stagedCalendarContext()
	if _, err := h.PoolSinRLS(t).Exec(ctx, `ALTER TABLE events ADD COLUMN future_integer bigint NOT NULL DEFAULT 9007199254740993`); err != nil {
		t.Fatal(err)
	}
	c, err := st.CreateCalendar(ctx, NewCalendar{OwnerSub: "alice", Name: "real-history"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2027, 3, 29, 7, 0, 0, 0, time.UTC)
	description := strings.Repeat("Exact original / 漢字 é ", 2000)
	data, err := calendarical.EncodeEvent(domain.Event{UID: "retained", Summary: "Original", Description: description, DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque, RRule: "FREQ=WEEKLY;COUNT=10"}, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err = calendarical.ConRecordatorios(data, []int{15}, "Original")
	if err != nil {
		t.Fatal(err)
	}
	e, o, err := st.CreateEvent(ctx, domain.EventInput{CalendarID: c.ID, UID: "retained", ResourceName: "retained.ics", ICalData: data, Description: description, Summary: "Original", DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque, RRule: "FREQ=WEEKLY;COUNT=10"})
	if err != nil {
		t.Fatal(err)
	}
	trash, err := st.CreateCalendar(ctx, NewCalendar{OwnerSub: "alice", Name: "trash"})
	if err != nil {
		t.Fatal(err)
	}
	var originalObject []byte
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE calendar_reminders SET fired_at=now(),dismissed_at=now() WHERE tenant='acme' AND owner_sub='alice'`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE calendars SET deleted_at=now() WHERE tenant='acme' AND id=$1`, trash.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO events_outbox(id,tenant,channel,type,payload,processed_at)VALUES(gen_random_uuid(),'acme','calendar','changed',jsonb_build_object('event_id',$1::text),now())`, e.ID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT convert_to(to_jsonb(o)::text,'UTF8') FROM calendar_objects o WHERE tenant='acme' AND id=$1`, o.ID).Scan(&originalObject)
	})
	if err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", epoch); !errors.Is(err, ErrPrivateStorageIngress) {
		t.Fatal("historical readiness opened Begin", err)
	}
	state, err := st.PrivateStorageState(ctx, "alice")
	if err != nil || state.Mode != "legacy" || state.PrivateIngressReady {
		t.Fatal("Begin modified state without a real ingress bridge", state, err)
	}
	var staged pg.PrivateStorageState
	stage := func(epoch uuid.UUID) error {
		return st.privateCalendarTx(ctx, "alice", func(identity privateCalendarIdentity, tx pgx.Tx) error {
			var err error
			staged, err = stagePrivateStorageSnapshot(ctx, tx, identity, epoch)
			return err
		})
	}
	if err := stage(epoch); err != nil {
		t.Fatal(err)
	}
	if staged.Mode != "frozen" || staged.Rows != 6 || staged.Blobs != 0 {
		t.Fatal("staged inventory incomplete", staged)
	}
	if err := stage(epoch); err != nil {
		t.Fatal("same epoch was not idempotent", err)
	}
	if err := stage(uuid.New()); !errors.Is(err, pg.ErrPrivateSnapshotEpoch) {
		t.Fatal("another epoch reused frozen source", err)
	}
	changed := authn.Into(ctx, authn.Claims{Tenant: "acme", Subject: "alice", Email: "reassigned@example.invalid"})
	err = st.privateCalendarTx(changed, "alice", func(identity privateCalendarIdentity, tx pgx.Tx) error {
		_, err := stagePrivateStorageSnapshot(changed, tx, identity, epoch)
		return err
	})
	if !errors.Is(err, pg.ErrPrivateStorageScope) {
		t.Fatal("new token aliases silently replaced frozen routing", err)
	}
	page, err := st.PrivateStorageSourcePage(ctx, "alice", epoch, storage.LegacySourceCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	count := len(page.Rows)
	for page.Next != nil {
		page, err = st.PrivateStorageSourcePage(ctx, "alice", epoch, *page.Next, 2)
		if err != nil {
			t.Fatal(err)
		}
		count += len(page.Rows)
	}
	if count != 6 {
		t.Fatal("source pages omitted rows", count)
	}
	descriptor, body, err := st.OpenPrivateStorageSource(ctx, "alice", epoch, "object", o.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	retained, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, originalObject) || descriptor.Size != int64(len(originalObject)) || len(retained) < 40<<10 {
		t.Fatal("source changed original large ICS JSON")
	}
	_, body, err = st.OpenPrivateStorageSource(ctx, "alice", epoch, "event", e.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	retained, err = io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Contains(retained, []byte("9007199254740993")) {
		t.Fatal("future event column lost or rounded", err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", epoch); !errors.Is(err, ErrPrivateStorageIngress) {
		t.Fatal("staged history was treated as continuing ingress", err)
	}
	if _, err := st.PrivateStorageSourcePage(ctx, "alice", uuid.New(), storage.LegacySourceCursor{}, 2); !errors.Is(err, ErrPrivateStorageEpoch) {
		t.Fatal("wrong epoch read source", err)
	}
	if _, _, err := st.OpenPrivateStorageSource(ctx, "bob", epoch, "object", o.ID.String()); !errors.Is(err, pg.ErrPrivateStorageScope) {
		t.Fatal("caller string overrode verified identity", err)
	}
	exportCalendarStagedFixture(ctx, t, st, epoch)
}

func TestCalendarPrivateStagingNeverProvisionsAnEmptyReplacement(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := New(h.Pool)
	ctx := stagedCalendarContext()
	err := st.privateCalendarTx(ctx, "alice", func(identity privateCalendarIdentity, tx pgx.Tx) error {
		_, err := stagePrivateStorageSnapshot(ctx, tx, identity, uuid.New())
		return err
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("missing Calendar source became a frozen empty account", err)
	}
	state, err := st.PrivateStorageState(ctx, "alice")
	if err != nil || state.Mode != "legacy" || state.Rows != 0 || state.Epoch != "" {
		t.Fatal("empty staging initialized retained state", state, err)
	}
}

func TestCalendarPrivateStagedSnapshotRollsBackUnsupportedCollaboration(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := New(h.Pool)
	ctx := stagedCalendarContext()
	c, err := st.CreateCalendar(ctx, NewCalendar{OwnerSub: "alice", Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO calendar_shares(tenant,calendar_id,grantee_sub,permission,created_by)VALUES('acme',$1,'bob','reader','alice')`, c.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = st.privateCalendarTx(ctx, "alice", func(identity privateCalendarIdentity, tx pgx.Tx) error {
		_, err := stagePrivateStorageSnapshot(ctx, tx, identity, uuid.New())
		return err
	})
	if !errors.Is(err, ErrPrivateStorageCollaboration) {
		t.Fatal("partial shared account froze", err)
	}
	state, err := st.PrivateStorageState(ctx, "alice")
	if err != nil || state.Mode != "legacy" || state.Rows != 0 || state.Epoch != "" {
		t.Fatal("rejected collaboration left a partial snapshot", state, err)
	}
}
