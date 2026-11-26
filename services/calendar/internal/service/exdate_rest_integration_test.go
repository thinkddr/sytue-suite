// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// comoLoMandaREST es el cuerpo del PUT /events/{id} de un cliente REST que guarda el
// evento que leyó con GET (la edición conectada: `api.Event.Input`), con sus EXDATE.
func comoLoMandaREST(ev domain.Event) service.EventInput {
	return service.EventInput{
		CalendarID: ev.CalendarID, UID: ev.UID, Summary: ev.Summary, Description: ev.Description,
		Location: ev.Location, Organizer: ev.Organizer, DTStart: ev.DTStart, DTEnd: ev.DTEnd,
		TZID: ev.TZID, AllDay: ev.AllDay, Status: ev.Status, Classification: ev.Classification,
		Transparency: ev.Transparency, Color: ev.Color, ConferenceURL: ev.ConferenceURL,
		RRule: ev.RRule, RDate: ev.RDate, ExDate: ev.ExDate, Attendees: ev.Attendees,
	}
}

// Una ocurrencia quitada de una serie («solo este») tiene que seguir quitada en el
// recurso que sincronizan los clientes CalDAV después de editar la serie por REST. El
// servicio la guardaba en la columna `exdate` —y su propia expansión la ocultaba— pero
// regeneraba el iCalendar sin la EXDATE, así que iCloud o Thunderbird la volvían a
// enseñar. La EXDATE va escrita como el DTSTART del maestro (en la hora de Madrid, con su
// TZID), y la excepción de otra ocurrencia (RECURRENCE-ID) sobrevive a la edición, también
// con la serie movida de hora.
func TestEditarPorRESTConservaLasEXDATEDelRecurso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)
	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	semana := func(n, hora int) time.Time {
		return time.Date(2026, 12, 2+7*n, hora, 0, 0, 0, madrid)
	}

	// Una serie semanal creada por REST, en la hora de Madrid.
	in := nuevoEvento(cal.ID, "serie-exdate")
	in.Summary = "Seguimiento"
	in.DTStart, in.DTEnd, in.TZID = semana(0, 10), semana(0, 11), "Europe/Madrid"
	in.RRule = "FREQ=WEEKLY;COUNT=6"
	ev, err := svc.CreateEvent(ctx, in)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	// «Solo este» sobre la segunda: amend-recurrence con exclude.
	if _, err := svc.AmendRecurrence(ctx, cal.ID, ev.UID, semana(1, 10), string(caldav.AmendExclude), uuid.Nil); err != nil {
		t.Fatalf("AmendRecurrence: %v", err)
	}
	// Un cliente CalDAV cambia la tercera (una excepción con RECURRENCE-ID).
	obj, err := st.GetObjectByUID(ctx, cal.ID, ev.UID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(obj.Data, "EXDATE;TZID=Europe/Madrid:20261209T100000\r\n") {
		t.Errorf("la EXDATE de «solo este» no va como el DTSTART:\n%s", obj.Data)
	}
	excepcion := "BEGIN:VEVENT\r\nUID:" + ev.UID + "\r\nDTSTAMP:20261120T080000Z\r\n" +
		"RECURRENCE-ID;TZID=Europe/Madrid:20261216T100000\r\n" +
		"DTSTART;TZID=Europe/Madrid:20261216T100000\r\nDTEND;TZID=Europe/Madrid:20261216T110000\r\n" +
		"SUMMARY:Seguimiento en la sala 2\r\nEND:VEVENT\r\nEND:VCALENDAR"
	conExcepcion := strings.Replace(obj.Data, "END:VCALENDAR", excepcion, 1)
	if _, err := svc.PutObject(ctx, cal.ID, conExcepcion, obj.ETag, false); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	// El GET que hace el cliente REST trae la EXDATE en el evento.
	ev, err = svc.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.ExDate) != 1 || !ev.ExDate[0].Equal(semana(1, 10)) {
		t.Fatalf("la columna exdate: %v", ev.ExDate)
	}

	comprobar := func(cuando string, hora int) {
		t.Helper()
		obj, err := st.GetObjectByUID(ctx, cal.ID, ev.UID)
		if err != nil {
			t.Fatal(err)
		}
		po, err := caldav.DecodeCalendarObject(obj.Data, cal.ID.String())
		if err != nil {
			t.Fatal(err)
		}
		if len(po.Master.ExDate) != 1 || !po.Master.ExDate[0].Equal(semana(1, hora)) {
			t.Errorf("%s: el recurso que sincronizan los clientes no quita la ocurrencia del %v: EXDATE %v\n%s",
				cuando, semana(1, hora), po.Master.ExDate, obj.Data)
		}
		if quiero := "EXDATE;TZID=Europe/Madrid:20261209T" + time.Date(0, 1, 1, hora, 0, 0, 0, time.UTC).Format("150405") + "\r\n"; !strings.Contains(obj.Data, quiero) {
			t.Errorf("%s: falta %q (la EXDATE como el DTSTART)", cuando, quiero)
		}
		if len(po.Overrides) != 1 || po.Overrides[0].RecurrenceID == nil || !po.Overrides[0].RecurrenceID.Equal(semana(2, hora)) ||
			po.Overrides[0].Summary != "Seguimiento en la sala 2" {
			t.Errorf("%s: la excepción de la tercera no ha sobrevivido: %+v\n%s", cuando, po.Overrides, obj.Data)
		}
		// La expansión del servicio, que lee la columna, sigue sin la quitada.
		inst, err := svc.ListInstances(ctx, cal.ID, semana(0, 0), semana(6, 0))
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range inst {
			if i.Start.Equal(semana(1, hora)) {
				t.Errorf("%s: la expansión del servicio enseña la ocurrencia quitada", cuando)
			}
		}
	}

	// Editar el título por REST.
	cambio := comoLoMandaREST(ev)
	cambio.Summary = "Seguimiento semanal"
	if ev, err = svc.UpdateEvent(ctx, cambio); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	comprobar("tras cambiar el título", 10)

	// Mover la serie una hora por REST: la pantalla manda la EXDATE movida con ella.
	cambio = comoLoMandaREST(ev)
	cambio.DTStart, cambio.DTEnd = cambio.DTStart.Add(time.Hour), cambio.DTEnd.Add(time.Hour)
	cambio.ExDate = []time.Time{semana(1, 11)}
	if _, err = svc.UpdateEvent(ctx, cambio); err != nil {
		t.Fatalf("UpdateEvent moviendo la serie: %v", err)
	}
	comprobar("tras mover la serie", 11)
}

// Una serie que puso un cliente CalDAV (iCloud: la regla con INTERVAL=1, la EXDATE en la
// hora de su zona) y que se edita por REST desde un formulario que rehace la regla a su
// manera (el de las apps Fyne no escribe INTERVAL=1): es la misma serie, así que sus
// EXDATE y sus excepciones siguen en el recurso.
func TestEditarPorRESTUnaSerieDeCalDAVConservaEXDATEYExcepciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)
	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	deICloud := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:serie@icloud\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:1\r\n" +
		"DTSTART;TZID=Europe/Madrid:20261202T100000\r\nDTEND;TZID=Europe/Madrid:20261202T110000\r\n" +
		"RRULE:FREQ=WEEKLY;INTERVAL=1;COUNT=6\r\nEXDATE;TZID=Europe/Madrid:20261209T100000\r\n" +
		"SUMMARY:Seguimiento\r\nORGANIZER:mailto:" + ownerEmail + "\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:serie@icloud\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:1\r\n" +
		"RECURRENCE-ID;TZID=Europe/Madrid:20261216T100000\r\n" +
		"DTSTART;TZID=Europe/Madrid:20261216T120000\r\nDTEND;TZID=Europe/Madrid:20261216T130000\r\n" +
		"SUMMARY:Seguimiento (a mediodía)\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := svc.PutObject(ctx, cal.ID, deICloud, uuid.Nil, true); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	evs, err := svc.ListEventsInRange(ctx, cal.ID, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var maestro domain.Event
	for _, e := range evs {
		if e.RecurrenceID == nil {
			maestro = e
		}
	}
	if maestro.UID != "serie@icloud" {
		t.Fatalf("no está el maestro: %+v", evs)
	}

	cambio := comoLoMandaREST(maestro)
	cambio.Summary = "Seguimiento semanal"
	cambio.RRule = "FREQ=WEEKLY;COUNT=6" // como la rehace el formulario
	if _, err := svc.UpdateEvent(ctx, cambio); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	obj, err := st.GetObjectByUID(ctx, cal.ID, "serie@icloud")
	if err != nil {
		t.Fatal(err)
	}
	po, err := caldav.DecodeCalendarObject(obj.Data, cal.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if po.Master.Summary != "Seguimiento semanal" {
		t.Errorf("el cambio no está: %q", po.Master.Summary)
	}
	quitada := time.Date(2026, 12, 9, 10, 0, 0, 0, madrid)
	if len(po.Master.ExDate) != 1 || !po.Master.ExDate[0].Equal(quitada) || !strings.Contains(obj.Data, "EXDATE;TZID=Europe/Madrid:20261209T100000\r\n") {
		t.Errorf("la EXDATE no sigue en el recurso: %v\n%s", po.Master.ExDate, obj.Data)
	}
	movida := time.Date(2026, 12, 16, 10, 0, 0, 0, madrid)
	if len(po.Overrides) != 1 || po.Overrides[0].RecurrenceID == nil || !po.Overrides[0].RecurrenceID.Equal(movida) ||
		po.Overrides[0].Summary != "Seguimiento (a mediodía)" {
		t.Errorf("la excepción no ha sobrevivido a la edición por REST: %+v\n%s", po.Overrides, obj.Data)
	}
}
