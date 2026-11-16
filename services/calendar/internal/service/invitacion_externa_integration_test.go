// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

const requestDeGoogle = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Google Inc//EN\r\nMETHOD:REQUEST\r\n" +
	"BEGIN:VEVENT\r\nUID:g-1@google.com\r\nDTSTAMP:20261101T120000Z\r\n" +
	"DTSTART:20261112T090000Z\r\nDTEND:20261112T100000Z\r\nSEQUENCE:1\r\nSUMMARY:Revisión\r\n" +
	"ORGANIZER;CN=Luis:mailto:luis@otro.example\r\n" +
	"ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:ana@acme.example\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

// D-MAIL-04: contestar desde el lector la invitación de alguien de fuera guarda el
// evento en el calendario por defecto con la respuesta y manda el REPLY a quien
// organiza; volver a contestar cambia la respuesta sin duplicar el evento, y un correo
// viejo (SEQUENCE menor) no pisa una versión más nueva.
func TestLaInvitacionDeFueraSeContestaDesdeElCorreo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, PublicURL: "https://calendar.sytue.eu"}, st, nil)
	enviados := make(chan string, 4)
	svc.SetCorreo(func(_ context.Context, para, _, _ string, ics []byte, metodo string) error {
		enviados <- para + "|" + metodo + "|" + string(ics)
		return nil
	})
	ana := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}),
		authn.Claims{Subject: "ana-1", Email: "ana@personal.example", Mailbox: "ana@acme.example"})

	res, err := svc.ImportarInvitacion(ana, requestDeGoogle, domain.PartTentative)
	if err != nil {
		t.Fatal(err)
	}
	if res.UID != "g-1@google.com" || res.Attendee != "ana@acme.example" || !res.ReplySent {
		t.Errorf("resultado = %+v", res)
	}
	select {
	case c := <-enviados:
		if !strings.HasPrefix(c, "luis@otro.example|REPLY|") || !strings.Contains(c, "PARTSTAT=TENTATIVE") {
			t.Errorf("REPLY = %s", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no sale el REPLY")
	}
	obj, err := st.GetObjectByUID(ana, res.CalendarID, res.UID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(obj.Data, "METHOD") || !strings.Contains(obj.Data, "PARTSTAT=TENTATIVE") {
		t.Errorf("recurso guardado:\n%s", obj.Data)
	}

	// Cambiar de opinión: misma fila, respuesta nueva.
	if _, err := svc.ImportarInvitacion(ana, requestDeGoogle, domain.PartAccepted); err != nil {
		t.Fatal(err)
	}
	<-enviados
	objs, err := st.ListObjects(ana, res.CalendarID)
	if err != nil || len(objs) != 1 || !strings.Contains(objs[0].Data, "PARTSTAT=ACCEPTED") {
		t.Fatalf("objetos = %d, %v", len(objs), err)
	}

	// Llega la versión 5 (otra hora) y luego se contesta el correo viejo: no vuelve la hora de antes.
	nueva := strings.NewReplacer("SEQUENCE:1", "SEQUENCE:5", "T090000Z", "T150000Z", "T100000Z", "T160000Z").Replace(requestDeGoogle)
	if _, err := svc.ImportarInvitacion(ana, nueva, domain.PartAccepted); err != nil {
		t.Fatal(err)
	}
	<-enviados
	if _, err := svc.ImportarInvitacion(ana, requestDeGoogle, domain.PartDeclined); err != nil {
		t.Fatal(err)
	}
	obj, _ = st.GetObjectByUID(ana, res.CalendarID, res.UID)
	if !strings.Contains(obj.Data, "T150000Z") || !strings.Contains(obj.Data, "PARTSTAT=DECLINED") {
		t.Errorf("el correo viejo pisó la versión nueva:\n%s", obj.Data)
	}
}
