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

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func privateCalendarContext(tenant, owner string) context.Context {
	return authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: tenant}), authn.Claims{Tenant: tenant, Subject: owner, Email: owner + "@example.invalid"})
}

func privateCalendarEvent(ctx context.Context, t *testing.T, st *store.Store, owner string) (domain.Calendar, domain.Event, domain.CalendarObject) {
	t.Helper()
	c, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: owner, Name: "retained", DisplayName: "Original private calendar"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2027, 3, 29, 7, 0, 0, 0, time.UTC)
	data, err := caldav.EncodeEvent(domain.Event{UID: "retained", Summary: "Original", DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque}, "")
	if err != nil {
		t.Fatal(err)
	}
	e, o, err := st.CreateEvent(ctx, domain.EventInput{CalendarID: c.ID, UID: "retained", ResourceName: "retained.ics", ICalData: data, Summary: "Original", DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque})
	if err != nil {
		t.Fatal(err)
	}
	return c, e, o
}

func TestCalendarPrivatePreflightBlocksUnsupportedRowsBeforeChangingState(t *testing.T) {
	for _, test := range []struct {
		name, tenant, sql string
		want              error
	}{
		{"owned_share", "acme", `INSERT INTO calendar_shares(tenant,calendar_id,grantee_sub,permission,created_by)VALUES('acme',$1,'bob','reader','alice')`, store.ErrPrivateStorageCollaboration},
		{"foreign_invitation_reassigned_email", "foreign", `INSERT INTO invitations(tenant,uid,method,recipient,recipient_sub,data)VALUES('foreign','historical','REQUEST','alice@example.invalid','old-identity','original before reassignment')`, store.ErrPrivateStorageCollaboration},
		{"foreign_share", "foreign", `WITH created AS(INSERT INTO calendars(tenant,owner_sub,name)VALUES('foreign','bob','shared')RETURNING id)INSERT INTO calendar_shares(tenant,calendar_id,grantee_email,permission,created_by)SELECT 'foreign',id,'alice@example.invalid','reader','bob' FROM created`, store.ErrPrivateStorageCollaboration},
		{"attendee", "acme", `INSERT INTO attendees(tenant,event_id,email)VALUES('acme',$2,'bob@example.invalid')`, store.ErrPrivateStorageCollaboration},
		{"request", "acme", `INSERT INTO calendar_peticiones_edicion(tenant,calendar_id,sujeto)VALUES('acme',$1,'bob')`, store.ErrPrivateStorageCollaboration},
		{"subscription", "acme", `INSERT INTO calendar_subscriptions(tenant,calendar_id,url)VALUES('acme',$1,'https://example.invalid/original-feed')`, store.ErrPrivateStorageSubscriptions},
		{"pending_reminder", "acme", `INSERT INTO calendar_reminders(tenant,owner_sub,calendar_id,object_id,event_id,uid,dtstart,trigger_at)VALUES('acme','alice',$1,$3,$2,'retained',now(),now())`, store.ErrPrivateStorageReminders},
		{"pending_outbox", "acme", `INSERT INTO events_outbox(id,tenant,channel,type,payload)VALUES(gen_random_uuid(),'acme','calendar','changed',jsonb_build_object('event_id',$2::text))`, store.ErrPrivateStorageOutbox},
		{"unroutable_processed_outbox", "acme", `INSERT INTO events_outbox(id,tenant,channel,type,payload,processed_at)VALUES(gen_random_uuid(),'acme','calendar','old_event','{"original":"must not vanish"}',now())`, store.ErrPrivateStorageAmbiguous},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := pgtest.New(t, db.Migrations, db.MigrationsDir)
			ctx := privateCalendarContext("acme", "alice")
			st := store.New(h.Pool)
			c, e, o := privateCalendarEvent(ctx, t, st, "alice")
			err := h.Pool.InTenantTx(privateCalendarContext(test.tenant, "bob"), func(tx pgx.Tx) error {
				// PostgreSQL's simple protocol permits the two ordered fixture writes;
				// normal production operations remain parameterized extended queries.
				_, err := tx.Exec(ctx, `SELECT $1::uuid,$2::uuid,$3::uuid;`+test.sql, pgx.QueryExecModeSimpleProtocol, c.ID, e.ID, o.ID)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			compat, err := st.PrivateStorageCompatibility(ctx, "alice")
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(compat.Err(), test.want) {
				t.Fatal("unsupported retained class was accepted", compat, compat.Err())
			}
			err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `SELECT set_config('app.owner_sub','alice',true)`); err != nil {
					return err
				}
				var mode string
				var epoch *string
				var rows, aliases int
				if err := tx.QueryRow(ctx, `SELECT mode,epoch::text FROM calendar_private_storage_states WHERE tenant='acme' AND owner_sub='alice'`).Scan(&mode, &epoch); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM calendar_private_storage_snapshot WHERE tenant='acme' AND owner_sub='alice'`).Scan(&rows); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM calendar_private_storage_aliases WHERE tenant='acme' AND owner_sub='alice'`).Scan(&aliases); err != nil {
					return err
				}
				if mode != "legacy" || epoch != nil || rows != 0 || aliases != 0 {
					return errors.New("preflight modified or partially froze legacy data")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCalendarPrivatePreflightBlocksSharedResourcesAndAcceptsOnlyQuiescentHistory(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := privateCalendarContext("acme", "alice")
	st := store.New(h.Pool)
	c, e, o := privateCalendarEvent(ctx, t, st, "alice")
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO calendar_reminders(tenant,owner_sub,calendar_id,object_id,event_id,uid,dtstart,trigger_at,fired_at,dismissed_at)VALUES('acme','alice',$1,$2,$3,'retained',now(),now(),now(),now())`, c.ID, o.ID, e.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO events_outbox(id,tenant,channel,type,payload,processed_at)VALUES(gen_random_uuid(),'acme','calendar','changed',jsonb_build_object('event_id',$1::text),now())`, e.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	compat, err := st.PrivateStorageCompatibility(ctx, "alice")
	if err != nil || compat.Err() != nil {
		t.Fatal("retained fired/dismissed history was dropped or blocked", compat, err)
	}
	if _, err := st.CreateResource(ctx, store.NewResource{Kind: domain.ResourceRoom, Name: "Tenant resource managed by another user", CreatedBy: "bob"}); err != nil {
		t.Fatal(err)
	}
	compat, err = st.PrivateStorageCompatibility(ctx, "alice")
	if err != nil || !errors.Is(compat.Err(), store.ErrPrivateStorageResources) {
		t.Fatal("tenant resource catalog disappeared from compatibility", compat, err)
	}
}

func TestCalendarPrivateIdentityCannotComeFromCallerStringsOrDelegation(t *testing.T) {
	st := store.New(nil)
	for _, ctx := range []context.Context{
		tenantCtx("acme"),
		privateCalendarContext("acme", "bob"),
		authn.Into(tenantCtx("acme"), authn.Claims{Tenant: "foreign", Subject: "alice", Email: "alice@example.invalid"}),
		authn.Into(tenantCtx("acme"), authn.Claims{Tenant: "acme", Subject: "alice", Email: "alice@example.invalid", Delegado: true}),
		authn.Into(tenantCtx("acme"), authn.Claims{Tenant: "acme", Subject: "alice", Email: "not-an-address"}),
	} {
		if _, err := st.PrivateStorageCompatibility(ctx, "alice"); err == nil {
			t.Fatal("unverified identity reached storage")
		}
	}
}

func TestCalendarPrivatePreflightPreservesOrphanedInvitationAfterRealMasterReplacement(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := privateCalendarContext("acme", "alice")
	st := store.New(h.Pool)
	c, e, o := privateCalendarEvent(ctx, t, st, "alice")
	invitation, err := st.CreateInvitation(ctx, store.NewInvitation{EventID: &e.ID, UID: e.UID, Method: domain.MethodRequest, Organizer: "former-assignee@example.invalid", Recipient: "bob@example.invalid", RecipientSub: "bob", Data: o.Data})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM events WHERE tenant='acme' AND id=$1`, e.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	updated, _, err := st.UpdateEvent(ctx, domain.EventInput{CalendarID: c.ID, UID: e.UID, ResourceName: o.ResourceName, ICalData: o.Data, Summary: e.Summary, DTStart: e.DTStart, DTEnd: e.DTEnd, TZID: e.TZID, Status: e.Status, Classification: e.Classification, Transparency: e.Transparency}, o.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID == e.ID {
		t.Fatal("fixture did not exercise real master replacement")
	}
	var linked *uuid.UUID
	var original string
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT event_id,data FROM invitations WHERE tenant='acme' AND id=$1`, invitation.ID).Scan(&linked, &original)
	})
	if err != nil {
		t.Fatal(err)
	}
	if linked != nil || original != o.Data {
		t.Fatal("historical invitation was dropped or altered instead of SET NULL")
	}
	compat, err := st.PrivateStorageCompatibility(ctx, "alice")
	if err != nil || !errors.Is(compat.Err(), store.ErrPrivateStorageCollaboration) {
		t.Fatal("orphaned historical outgoing invitation was silently omitted", compat, err)
	}
	if _, err := st.BeginPrivateStorageMigration(ctx, "alice", uuid.New()); !errors.Is(err, store.ErrPrivateStorageCollaboration) {
		t.Fatal("unattributed invitation history passed preflight", err)
	}
	state, err := st.PrivateStorageState(ctx, "alice")
	if err != nil || state.Mode != "legacy" || state.Rows != 0 || state.Epoch != "" {
		t.Fatal("history blocker partially froze the account", state, err)
	}
}
