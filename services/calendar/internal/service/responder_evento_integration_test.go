// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// El «¿vas a ir?» por UID escribe la respuesta en el .ics (lo que lee la web) y en
// los asistentes, solo la de quien contesta, y solo donde puede escribir.
func TestResponderEventoPorUIDEscribeElICS(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)

	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	for mail, perm := range map[string]domain.SharePermission{writerMail: domain.PermWriter, readerMail: domain.PermReader} {
		if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, mail, perm); err != nil {
			t.Fatal(err)
		}
	}
	const uid = "reunion-1@calendar.sytue.eu"
	in := nuevoEvento(cal.ID, uid)
	in.Attendees = []domain.Attendee{
		{Email: writerMail, PartStat: domain.PartNeedsAction},
		{Email: readerMail, PartStat: domain.PartAccepted},
	}
	if _, err := svc.CreateEvent(ctx, in); err != nil {
		t.Fatal(err)
	}

	// Quizá, y luego no: volver a responder cambia la respuesta.
	for _, ps := range []domain.ParticipationStatus{domain.PartTentative, domain.PartDeclined} {
		if _, err := svc.ResponderEvento(ctxDe(writerSub, writerMail), uid, writerMail, ps); err != nil {
			t.Fatalf("responder %s: %v", ps, err)
		}
		obj, err := st.GetObjectByUID(ctx, cal.ID, uid)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(obj.Data, "PARTSTAT="+string(ps)) {
			t.Errorf("el .ics no lleva la respuesta %s:\n%s", ps, obj.Data)
		}
		if !strings.Contains(obj.Data, "PARTSTAT=ACCEPTED") {
			t.Errorf("la respuesta de otro invitado se ha perdido:\n%s", obj.Data)
		}
	}

	// Quien solo puede leer el calendario no lo reescribe, ni un extraño.
	if _, err := svc.ResponderEvento(ctxDe(readerSub, readerMail), uid, readerMail, domain.PartDeclined); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("reader: err = %v, quiero ErrNotFound", err)
	}
	if _, err := svc.ResponderEvento(ctxDe(extranoSub, extranoEml), uid, extranoEml, domain.PartAccepted); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("extraño: err = %v, quiero ErrNotFound", err)
	}
	obj, err := st.GetObjectByUID(ctx, cal.ID, uid)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(obj.Data, "PARTSTAT=DECLINED") != 1 {
		t.Errorf("solo el writer ha dicho que no:\n%s", obj.Data)
	}
}

// D-CAL-01: cambiar la respuesta desde el evento avisa a quien organiza, como la
// bandeja. La invitación del writer deja de estar pendiente y toma cada respuesta
// nueva, el organizador recibe un REPLY en su bandeja por cada cambio (y ninguno si se
// repite la misma), y la bandeja también deja cambiar una respuesta ya dada.
func TestResponderDesdeElEventoAvisaAlOrganizador(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, PublicURL: "https://calendar.sytue.eu"}, st, nil)

	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, writerMail, domain.PermWriter); err != nil {
		t.Fatal(err)
	}
	const uid = "aviso-1@calendar.sytue.eu"
	in := nuevoEvento(cal.ID, uid)
	in.Send = true
	in.Attendees = []domain.Attendee{{Email: writerMail, PartStat: domain.PartNeedsAction, RSVP: true}}
	if _, err := svc.CreateEvent(ctx, in); err != nil {
		t.Fatal(err)
	}
	writer := ctxDe(writerSub, writerMail)
	pend, err := svc.ListInbox(writer, writerMail, true, 50)
	if err != nil || len(pend) != 1 {
		t.Fatalf("la invitación del writer: %+v %v", pend, err)
	}
	invID := pend[0].ID

	replies := func() []domain.Invitation {
		t.Helper()
		todas, err := svc.ListInbox(ctx, ownerEmail, false, 50)
		if err != nil {
			t.Fatal(err)
		}
		var out []domain.Invitation
		for _, i := range todas {
			if i.Method == domain.MethodReply && i.UID == uid {
				out = append(out, i)
			}
		}
		return out
	}
	estado := func() domain.InvitationStatus {
		t.Helper()
		inv, err := st.GetInvitation(ctx, invID)
		if err != nil {
			t.Fatal(err)
		}
		return inv.Status
	}

	pasos := []struct {
		ps      domain.ParticipationStatus
		quiero  domain.InvitationStatus
		replies int
	}{
		{domain.PartAccepted, domain.InvAccepted, 1},
		{domain.PartDeclined, domain.InvDeclined, 2},
		{domain.PartDeclined, domain.InvDeclined, 2}, // la misma otra vez: sin aviso nuevo
	}
	for _, p := range pasos {
		if _, err := svc.ResponderEvento(writer, uid, writerMail, p.ps); err != nil {
			t.Fatalf("responder %s: %v", p.ps, err)
		}
		if got := estado(); got != p.quiero {
			t.Errorf("tras %s la invitación está %q; quiero %q", p.ps, got, p.quiero)
		}
		if got := len(replies()); got != p.replies {
			t.Errorf("tras %s el organizador tiene %d REPLY; quiero %d", p.ps, got, p.replies)
		}
	}
	if l, err := svc.ListInbox(writer, writerMail, true, 50); err != nil || len(l) != 0 {
		t.Errorf("contestada desde el evento, ya no está pendiente en la bandeja: %+v %v", l, err)
	}

	// Desde la bandeja también se cambia una respuesta ya dada.
	if _, err := svc.RespondInvitation(writer, invID, domain.InvTentative); err != nil {
		t.Fatalf("cambiar desde la bandeja: %v", err)
	}
	if got := estado(); got != domain.InvTentative {
		t.Errorf("la invitación está %q; quiero tentative", got)
	}
	if got := len(replies()); got != 3 {
		t.Errorf("el organizador tiene %d REPLY; quiero 3", got)
	}
	// Pero no otra persona.
	if _, err := svc.RespondInvitation(ctxDe(extranoSub, extranoEml), invID, domain.InvAccepted); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("extraño: err = %v, quiero ErrNotFound", err)
	}
}
