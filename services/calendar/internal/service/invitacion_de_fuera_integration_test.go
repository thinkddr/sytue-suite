// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"context"
	"errors"
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

// COM-15 (decidido por el dueño el 2026-10-17). Bea es de OTRA organización y la
// invitan a un evento de acme por su dirección de allí. Desde la suya ve la invitación
// en su bandeja, con la organización que convoca y el título y la hora del evento, y la
// responde: la respuesta llega a acme. Nadie más la ve ni la responde, tampoco Bea con un
// token delegado ni por su correo de entrar, y la bandeja no enseña nada más de acme.
// Antes la invitación se quedaba en acme, donde Bea no puede entrar.
func TestLaInvitacionDeOtraOrganizacionSeVeYSeRespondeDesdeLaPropia(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, PublicURL: "https://calendar.sytue.eu"}, st, nil)

	acme := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(acme, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	const beaMail = "bea@otra.example"
	in := nuevoEvento(cal.ID, "fuera-1")
	in.Attendees = []domain.Attendee{{Email: ownerEmail}, {Email: beaMail}, {Email: readerMail}}
	ev, err := svc.CreateEvent(acme, in)
	if err != nil {
		t.Fatal(err)
	}
	svc.InvitarNuevos(acme, ev, nil)

	desdeOtra := func(c authn.Claims) context.Context {
		return authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "otra"}), c)
	}
	bea := desdeOtra(authn.Claims{Subject: "bea-1", Email: "bea@personal.example", Mailbox: beaMail})
	lista, err := svc.ListInbox(bea, "bea@personal.example", true, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) == 1 && lista[0].OrganizationName != "" {
		t.Fatalf("sin Sytue ID no hay nombre: %+v", lista[0])
	}
	svc.SetNombresOrgs(nombresFijos{"acme": "Acme Ingeniería S.L.", "otra": "La propia"})
	if lista, err = svc.ListInbox(bea, "bea@personal.example", true, 50); err != nil || len(lista) != 1 ||
		lista[0].OrganizationName != "Acme Ingeniería S.L." {
		t.Fatalf("la organización que convoca sale por su nombre visible: %+v %v", lista, err)
	}
	if len(lista) != 1 || lista[0].Organization != "acme" || lista[0].Summary != "reunión" ||
		lista[0].Start == nil || !lista[0].Start.Equal(in.DTStart) || lista[0].Recipient != beaMail {
		t.Fatalf("la bandeja de Bea: %+v; quiero solo su invitación, de acme, con título y hora", lista)
	}
	id := lista[0].ID

	// Ni otra persona de su organización, ni Bea con un token delegado o sin su buzón.
	for nombre, c := range map[string]authn.Claims{
		"otra persona":        {Subject: "carla-1", Email: "carla@otra.example", Mailbox: "carla@otra.example"},
		"un token delegado":   {Subject: "bea-1", Mailbox: beaMail, Delegado: true, Actor: "app"},
		"su correo de entrar": {Subject: "bea-2", Email: beaMail},
	} {
		if l, err := svc.ListInbox(desdeOtra(c), c.Email, true, 50); err != nil || len(l) != 0 {
			t.Errorf("%s ve %+v (%v)", nombre, l, err)
		}
		if _, err := svc.RespondInvitation(desdeOtra(c), id, domain.InvDeclined); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s responde: %v", nombre, err)
		}
	}

	inv, err := svc.RespondInvitation(bea, id, domain.InvAccepted)
	if err != nil || inv.Status != domain.InvAccepted {
		t.Fatalf("Bea responde: %+v %v", inv, err)
	}
	guardado, err := st.GetEvent(acme, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	aceptada := false
	for _, a := range guardado.Attendees {
		aceptada = aceptada || (a.Email == beaMail && a.PartStat == domain.PartAccepted)
	}
	if !aceptada {
		t.Fatalf("en acme, la respuesta de Bea no consta: %+v", guardado.Attendees)
	}
	if l, _ := svc.ListInbox(bea, "", true, 50); len(l) != 0 {
		t.Fatalf("respondida, sigue pendiente: %+v", l)
	}
}

// nombresFijos da el nombre visible de unas organizaciones fijas.
type nombresFijos map[string]string

func (n nombresFijos) Nombres(_ context.Context, slugs []string) map[string]string {
	out := map[string]string{}
	for _, s := range slugs {
		if v, ok := n[s]; ok {
			out[s] = v
		}
	}
	return out
}
