// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package calendarhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Una ocurrencia quitada de una serie («solo este») sigue quitada después de editar la
// serie por la emulación de Google Calendar, en la columna `exdate` (la expansión del
// servicio) y en el recurso que sincronizan los clientes CalDAV. La emulación no enseña
// las EXDATE —`recurrence` lleva solo la RRULE—, así que el cliente que guarda lo que
// leyó no las manda: el events.update y el events.patch las borraban y la ocurrencia
// volvía. Con la serie movida de hora, la EXDATE se mueve con ella.
func TestGoogleEditarUnaSerieConservaSusEXDATE(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)
	const sub, email = "owner-1", "owner@sytue.eu"
	sesion := func(ctx context.Context) context.Context {
		return authn.Into(tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"}), authn.Claims{Subject: sub, Email: email})
	}
	ctx := sesion(t.Context())
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: sub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	semana := func(n, hora int) time.Time { return time.Date(2026, 12, 2+7*n, hora, 0, 0, 0, madrid) }

	ev, err := svc.CreateEvent(ctx, service.EventInput{
		CalendarID: cal.ID, UID: "serie-google", Summary: "Seguimiento", Organizer: email,
		DTStart: semana(0, 10), DTEnd: semana(0, 11), TZID: "Europe/Madrid", RRule: "FREQ=WEEKLY;COUNT=6",
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if _, err := svc.AmendRecurrence(ctx, cal.ID, ev.UID, semana(1, 10), string(caldav.AmendExclude), uuid.Nil); err != nil {
		t.Fatalf("AmendRecurrence: %v", err)
	}

	router := chi.NewRouter()
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(sesion(r.Context())))
		})
	}
	mountCompat(router, &compatHandlers{svc: svc, cfg: config.Config{
		PublicURL: "https://calendar.sytue.eu", MaxRangeWindow: 32 * 24 * time.Hour,
	}}, auth, auth)
	evento := "/compat/google/calendar/v3/calendars/" + cal.ID.String() + "/events/" + ev.ID.String()

	comprobar := func(cuando string, hora int) {
		t.Helper()
		quitada := semana(1, hora)
		leido, err := svc.GetEvent(ctx, ev.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(leido.ExDate) != 1 || !leido.ExDate[0].Equal(quitada) {
			t.Errorf("%s: la columna exdate %v, quiero %v", cuando, leido.ExDate, quitada)
		}
		obj, err := st.GetObjectByUID(ctx, cal.ID, ev.UID)
		if err != nil {
			t.Fatal(err)
		}
		po, err := caldav.DecodeCalendarObject(obj.Data, cal.ID.String())
		if err != nil {
			t.Fatal(err)
		}
		if len(po.Master.ExDate) != 1 || !po.Master.ExDate[0].Equal(quitada) {
			t.Errorf("%s: el recurso que sincronizan los clientes no quita la ocurrencia del %v: EXDATE %v\n%s",
				cuando, quitada, po.Master.ExDate, obj.Data)
		}
		inst, err := svc.ListInstances(ctx, cal.ID, semana(0, 0), semana(6, 0))
		if err != nil {
			t.Fatal(err)
		}
		if len(inst) != 5 {
			t.Errorf("%s: la serie tiene %d ocurrencias, quiero 5", cuando, len(inst))
		}
		for _, i := range inst {
			if i.Start.Equal(quitada) {
				t.Errorf("%s: la expansión del servicio enseña la ocurrencia quitada", cuando)
			}
		}
	}

	comprobar("tras «solo este»", 10)

	// events.update con lo que leyó el cliente (GET), con otro título.
	rec := compatDo(router, http.MethodGet, evento, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d (%s)", rec.Code, rec.Body.String())
	}
	var leido map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &leido); err != nil {
		t.Fatal(err)
	}
	cuerpo := map[string]any{"summary": "Seguimiento semanal"}
	for _, k := range []string{"start", "end", "recurrence", "status", "transparency", "organizer"} {
		cuerpo[k] = leido[k]
	}
	put, err := json.Marshal(cuerpo)
	if err != nil {
		t.Fatal(err)
	}
	if rec := compatDo(router, http.MethodPut, evento, string(put)); rec.Code != http.StatusOK {
		t.Fatalf("update: %d (%s)", rec.Code, rec.Body.String())
	}
	comprobar("tras events.update del título", 10)

	// events.patch que mueve la serie una hora.
	mover := `{"start":{"dateTime":"2026-12-02T11:00:00+01:00","timeZone":"Europe/Madrid"},` +
		`"end":{"dateTime":"2026-12-02T12:00:00+01:00","timeZone":"Europe/Madrid"}}`
	if rec := compatDo(router, http.MethodPatch, evento, mover); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d (%s)", rec.Code, rec.Body.String())
	}
	comprobar("tras events.patch que mueve la serie", 11)
	if obj, err := st.GetObjectByUID(ctx, cal.ID, ev.UID); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(obj.Data, "EXDATE;TZID=Europe/Madrid:20261209T110000\r\n") {
		t.Errorf("la EXDATE movida no va como el DTSTART:\n%s", obj.Data)
	}
}
