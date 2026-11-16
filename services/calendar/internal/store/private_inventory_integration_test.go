// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func TestPrivateInventoryPreservesEveryCalendarTableAndOriginalUnicodeKeys(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	if _, err := h.PoolSinRLS(t).Exec(ctx, `ALTER TABLE events ADD COLUMN future_integer bigint NOT NULL DEFAULT 9007199254740993`); err != nil {
		t.Fatal(err)
	}
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: scope.Subject, Name: "private", DisplayName: "Private source", Timezone: "Europe/Madrid"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2027, 3, 29, 7, 0, 0, 0, time.UTC)
	marker := "Exact original Unicode / content 漢字 é "
	data, err := caldav.EncodeEvent(domain.Event{UID: "private-source", Summary: marker, Description: strings.Repeat(marker, 1500), DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque, RRule: "FREQ=WEEKLY;COUNT=10"}, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err = caldav.ConRecordatorios(data, []int{15}, marker)
	if err != nil {
		t.Fatal(err)
	}
	ev, obj, err := st.CreateEvent(ctx, domain.EventInput{CalendarID: cal.ID, UID: "private-source", ResourceName: "private-source.ics", ICalData: data, Summary: marker, Description: strings.Repeat(marker, 1500), Status: domain.StatusConfirmed, Classification: domain.ClassPrivate, Transparency: domain.TranspOpaque, DTStart: start, DTEnd: start.Add(time.Hour), TZID: "Europe/Madrid", RRule: "FREQ=WEEKLY;COUNT=10", Attendees: []domain.Attendee{{Email: "bob@example.invalid", Role: domain.RoleRequired, PartStat: domain.PartAccepted, CUType: "INDIVIDUAL", RSVP: true}}})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := st.CreateResource(ctx, store.NewResource{Kind: domain.ResourceRoom, Name: "Managed private resource", CreatedBy: scope.Subject, Managers: []string{"bob"}, Capacity: 6})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "carol", Name: "foreign", DisplayName: "Must not leak"})
	if err != nil {
		t.Fatal(err)
	}
	incoming, invitation, outbox := uuid.New(), uuid.New(), uuid.New()
	requester := "人/é:bob"
	var rawObject, rawRequest []byte
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		statements := []struct {
			sql  string
			args []any
		}{
			{`UPDATE calendars SET read_only=true WHERE tenant=$1 AND id=$2`, []any{scope.Tenant, cal.ID}},
			{`INSERT INTO calendar_subscriptions(tenant,calendar_id,url,etag,last_modified) VALUES($1,$2,'https://feed.example.invalid/private-token','original-etag','original-modified')`, []any{scope.Tenant, cal.ID}},
			{`INSERT INTO calendar_shares(tenant,calendar_id,grantee_email,grantee_sub,permission,created_by) VALUES($1,$2,'bob@example.invalid','bob','reader',$3)`, []any{scope.Tenant, cal.ID, scope.Subject}},
			{`INSERT INTO calendar_peticiones_edicion(tenant,calendar_id,sujeto,nombre,correo) VALUES($1,$2,$3,'Unicode / requester','bob@example.invalid')`, []any{scope.Tenant, cal.ID, requester}},
			{`INSERT INTO invitations(id,tenant,event_id,uid,method,recipient,recipient_sub,data) VALUES($1,$2,$3,$4,'REQUEST','bob@example.invalid','bob',$5)`, []any{invitation, scope.Tenant, ev.ID, ev.UID, data}},
			{`INSERT INTO invitations(id,tenant,event_id,uid,method,recipient,recipient_sub,data) VALUES($1,$2,NULL,'external','REQUEST','alice@example.invalid',$3,$4)`, []any{incoming, scope.Tenant, scope.Subject, data}},
			{`UPDATE calendar_reminders SET fired_at=now(),dismissed_at=now() WHERE tenant=$1 AND owner_sub=$2`, []any{scope.Tenant, scope.Subject}},
			{`INSERT INTO events_outbox(id,tenant,channel,type,payload) VALUES($1,$2,'calendar','calendar.reminder.due',jsonb_build_object('event_id',$3::text,'summary',$4::text))`, []any{outbox, scope.Tenant, ev.ID, marker}},
			{`INSERT INTO events_outbox(id,tenant,channel,type,payload) VALUES(gen_random_uuid(),$1,'calendar','calendar.collection.created',jsonb_build_object('calendar_id',$2::text,'name','Foreign'))`, []any{scope.Tenant, outsider.ID}},
			{`UPDATE calendar_objects SET deleted_at=now() WHERE tenant=$1 AND id=$2`, []any{scope.Tenant, obj.ID}},
		}
		for _, statement := range statements {
			if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `SELECT convert_to(to_jsonb(o)::text,'UTF8') FROM calendar_objects o WHERE tenant=$1 AND id=$2`, scope.Tenant, obj.ID).Scan(&rawObject); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT convert_to(to_jsonb(p)::text,'UTF8') FROM calendar_peticiones_edicion p WHERE tenant=$1 AND calendar_id=$2 AND sujeto=$3`, scope.Tenant, cal.ID, requester).Scan(&rawRequest)
	})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot *model.Snapshot
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		snapshot, err = store.ReadPrivateStorageInventory(ctx, tx, scope)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]int{"calendar": 2, "object": 1, "event": 1, "attendee": 1, "invitation": 2, "reminder": 1, "subscription": 1, "share": 1, "request": 1, "resource": 1, "outbox": 1}
	requestID, err := model.RequestID(cal.ID.String(), requester)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, row := range snapshot.Rows() {
		seen[row.Kind]++
		switch row.Kind {
		case "object":
			if !bytes.Equal(row.JSON, rawObject) || !bytes.Contains(row.JSON, []byte("deleted_at")) {
				t.Fatal("original large ICS or trash changed")
			}
		case "event":
			if !bytes.Contains(row.JSON, []byte("9007199254740993")) {
				t.Fatal("future integer rounded or dropped")
			}
		case "request":
			if row.ID != requestID || !bytes.Equal(row.JSON, rawRequest) || strings.Contains(row.ID, "/") {
				t.Fatal("Unicode composite request changed")
			}
		case "resource":
			if row.ID != resource.ID.String() {
				t.Fatal("wrong resource captured")
			}
		case "outbox":
			if row.ID != outbox.String() || !bytes.Contains(row.JSON, []byte("summary")) {
				t.Fatal("foreign outbox or missing payload")
			}
		}
		var identity struct {
			ID    string `json:"id"`
			Owner string `json:"owner_sub"`
		}
		if json.Unmarshal(row.JSON, &identity) != nil || identity.ID == outsider.ID.String() || identity.Owner == "carol" {
			t.Fatal("foreign private inventory leaked")
		}
	}
	for kind, count := range expected {
		if seen[kind] != count {
			t.Fatal("source class missing or duplicated", kind, seen[kind], count)
		}
	}
}

func TestPrivateInventoryRejectsForeignScopeBeforeIO(t *testing.T) {
	scope := storage.Scope{Tenant: "other", Subject: "alice", Product: "calendar"}
	if _, err := store.ReadPrivateStorageInventory(tenantCtx("acme"), nil, scope); err == nil {
		t.Fatal("foreign scope accepted")
	}
}
