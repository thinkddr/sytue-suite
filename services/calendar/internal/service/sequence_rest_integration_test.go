// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Editar un evento por REST sube su SEQUENCE (RFC 5546) en la columna `sequence_no` y en
// el recurso iCalendar que sincronizan los clientes CalDAV. El recurso salía sin ella:
// un cliente que compara SEQUENCE veía la versión 0 —también la que iCloud ya tenía en la
// 3— y podía no tomar el cambio por nuevo; y lo que reescribe el recurso tal como está
// («solo este», un PUT de CalDAV) devolvía la columna a 0, así que la edición siguiente
// repetía una SEQUENCE ya enviada a los invitados.
func TestEditarPorRESTSubeLaSEQUENCEDelRecurso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)
	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	secuencia := func(cuando, uid string, quiero int) {
		t.Helper()
		obj, err := st.GetObjectByUID(ctx, cal.ID, uid)
		if err != nil {
			t.Fatal(err)
		}
		po, err := caldav.DecodeCalendarObject(obj.Data, cal.ID.String())
		if err != nil {
			t.Fatal(err)
		}
		if obj.SequenceNo != quiero || po.Master.SequenceNo != quiero {
			t.Errorf("%s: SEQUENCE %d en la columna y %d en el recurso, quiero %d\n%s", cuando, obj.SequenceNo, po.Master.SequenceNo, quiero, obj.Data)
		}
		if linea := "\r\nSEQUENCE:" + strconv.Itoa(quiero) + "\r\n"; quiero > 0 && !strings.Contains(obj.Data, linea) {
			t.Errorf("%s: el recurso no lleva %q (un entero, sin VALUE=TEXT):\n%s", cuando, strings.TrimSpace(linea), obj.Data)
		}
	}

	in := nuevoEvento(cal.ID, "secuencia-rest")
	in.RRule = "FREQ=WEEKLY;COUNT=4"
	ev, err := svc.CreateEvent(ctx, in)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	secuencia("al crearlo", ev.UID, 0)

	cambio := comoLoMandaREST(ev)
	cambio.Summary = "reunión de seguimiento"
	if ev, err = svc.UpdateEvent(ctx, cambio); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	secuencia("tras editar el título", ev.UID, 1)

	// «Solo este» reescribe el recurso tal como está: la SEQUENCE no baja.
	if _, err := svc.AmendRecurrence(ctx, cal.ID, ev.UID, in.DTStart.AddDate(0, 0, 7), string(caldav.AmendExclude), uuid.Nil); err != nil {
		t.Fatalf("AmendRecurrence: %v", err)
	}
	secuencia("tras quitar una ocurrencia", ev.UID, 1)

	if ev, err = svc.GetEvent(ctx, ev.ID); err != nil {
		t.Fatal(err)
	}
	cambio = comoLoMandaREST(ev)
	cambio.Location = "Sala 2"
	if _, err = svc.UpdateEvent(ctx, cambio); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	secuencia("tras la segunda edición", ev.UID, 2)

	// Un evento de iCloud, ya en la versión 3, editado por REST: la 4, no la 0.
	deICloud := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Apple Inc.//iCloud//ES\r\n" +
		"BEGIN:VEVENT\r\nUID:secuencia@icloud\r\nDTSTAMP:20261101T080000Z\r\nSEQUENCE:3\r\n" +
		"DTSTART:20261202T090000Z\r\nDTEND:20261202T100000Z\r\nSUMMARY:Revisión\r\n" +
		"ORGANIZER:mailto:" + ownerEmail + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := svc.PutObject(ctx, cal.ID, deICloud, uuid.Nil, true); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	secuencia("el de iCloud", "secuencia@icloud", 3)
	evs, err := svc.ListEventsInRange(ctx, cal.ID, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.UID != "secuencia@icloud" {
			continue
		}
		cambio := comoLoMandaREST(e)
		cambio.Summary = "Revisión trimestral"
		if _, err := svc.UpdateEvent(ctx, cambio); err != nil {
			t.Fatalf("UpdateEvent: %v", err)
		}
	}
	secuencia("el de iCloud editado por REST", "secuencia@icloud", 4)
}
