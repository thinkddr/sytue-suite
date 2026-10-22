// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// Un invitado añadido al guardar por CalDAV —el camino de la web— recibe su
// invitación en la bandeja y su aviso en la campana, que abre esa bandeja. Solo
// invita quien organiza, y solo a los que no estaban.
func TestUnInvitadoNuevoRecibeInvitacionYAviso(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, PublicURL: "https://calendar.sytue.eu"}, st, nil)
	avisos := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	svc.SetHubNotifier(hubnotify.New(hub.URL, "s"))

	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	in := nuevoEvento(cal.ID, "inv-1")
	in.Attendees = []domain.Attendee{{Email: ownerEmail}, {Email: readerMail}}
	ev, err := svc.CreateEvent(ctx, in) // sin `Send`: como guarda la web
	if err != nil {
		t.Fatal(err)
	}

	// Un invitado que guarda su copia no invita a nadie.
	svc.InvitarNuevos(ctxDe(readerSub, readerMail), ev, nil)
	// Quien organiza, sí, y solo al que no estaba.
	svc.InvitarNuevos(ctx, ev, nil)
	svc.InvitarNuevos(ctx, ev, ev.Attendees)

	select {
	case n := <-avisos:
		if n.Subject != hubnotify.PorCorreo(readerMail) || n.Type != "calendar.invitation" || n.Tenant != "acme" {
			t.Fatalf("aviso = %+v", n)
		}
		if n.URL != "https://calendar.sytue.eu/calendar/?invitations=1" || n.Params["title"] != "reunión" {
			t.Fatalf("enlace o variables: %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("el invitado no recibe aviso")
	}
	select {
	case n := <-avisos:
		t.Fatalf("aviso de más: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
	bandeja, err := st.ListInbox(ctxDe(readerSub, readerMail), readerMail, true, 10)
	if err != nil || len(bandeja) != 1 {
		t.Fatalf("bandeja del invitado: %v %+v", err, bandeja)
	}
}

// A quien no es de la suite la invitación le llega POR CORREO (iMIP): su REQUEST con el
// iCalendar del evento y su ORGANIZER, que es adónde vuelve el REPLY. A quien es de la
// suite, no: ya la tiene en la bandeja y en la campana. Y un CANCEL sale igual.
func TestLaInvitacionSaleConIMIPSoloHaciaFuera(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, PublicURL: "https://calendar.sytue.eu"}, st, nil)
	type correo struct{ para, asunto, metodo, ics string }
	enviados := make(chan correo, 8)
	svc.SetCorreo(func(_ context.Context, para, asunto, _ string, ics []byte, metodo string) error {
		enviados <- correo{para, asunto, metodo, string(ics)}
		return nil
	})

	ctx := ctxDe(ownerSub, "owner@sytue.eu")
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	in := nuevoEvento(cal.ID, "imip-1")
	in.Organizer = "owner@sytue.eu"
	in.Attendees = []domain.Attendee{{Email: "owner@sytue.eu"}, {Email: "ana@corporate.sytue.eu"}, {Email: "luis@gmail.com"}}
	in.Send = true
	ev, err := svc.CreateEvent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	espera := func() correo {
		t.Helper()
		select {
		case c := <-enviados:
			return c
		case <-time.After(5 * time.Second):
			t.Fatal("no ha salido ningún correo")
		}
		return correo{}
	}
	c := espera()
	if c.para != "luis@gmail.com" || c.metodo != "REQUEST" || !strings.Contains(c.ics, "METHOD:REQUEST") ||
		!strings.Contains(c.ics, "UID:imip-1") || !strings.Contains(c.ics, "ORGANIZER:mailto:owner@sytue.eu") ||
		!strings.HasPrefix(c.asunto, "Invitación: ") {
		t.Fatalf("correo inesperado: %+v", c)
	}
	select {
	case otro := <-enviados:
		t.Fatalf("ha salido un correo de más (a alguien de la suite o a quien organiza): %+v", otro)
	case <-time.After(300 * time.Millisecond):
	}

	// La respuesta vuelve por correo: Mail la entrega con la dirección que la firma y
	// sin sesión de nadie. Solo cuenta la de quien firma.
	reply := func(quien, partstat string) string {
		return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Otro//ES\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\n" +
			"UID:imip-1\r\nDTSTAMP:20260701T080000Z\r\nDTSTART:20260701T090000Z\r\n" +
			"ORGANIZER:mailto:owner@sytue.eu\r\nATTENDEE;PARTSTAT=" + partstat + ":mailto:" + quien + "\r\n" +
			"END:VEVENT\r\nEND:VCALENDAR\r\n"
	}
	sinSesion := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if err := svc.RecibirRespuesta(sinSesion, "eva@fuera.example", reply("luis@gmail.com", "DECLINED")); !errors.Is(err, service.ErrRespuestaAjena) {
		t.Fatalf("un REPLY de luis firmado por otra persona: %v", err)
	}
	if err := svc.RecibirRespuesta(sinSesion, "LUIS@gmail.com", reply("luis@gmail.com", "ACCEPTED")); err != nil {
		t.Fatalf("RecibirRespuesta: %v", err)
	}
	visto, err := svc.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range visto.Attendees {
		if a.Email == "luis@gmail.com" && a.PartStat != domain.PartAccepted {
			t.Fatalf("el PARTSTAT de luis es %q y contestó que sí", a.PartStat)
		}
	}
	if err := svc.RecibirRespuesta(sinSesion, "luis@gmail.com", reply("luis@gmail.com", "ACCEPTED")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("contestar dos veces la misma invitación: %v", err)
	}

	if err := svc.DeleteEventNotifying(ctx, ev.ID, true); err != nil {
		t.Fatal(err)
	}
	if c := espera(); c.para != "luis@gmail.com" || c.metodo != "CANCEL" || !strings.Contains(c.ics, "METHOD:CANCEL") {
		t.Fatalf("la cancelación no sale por correo: %+v", c)
	}
}
