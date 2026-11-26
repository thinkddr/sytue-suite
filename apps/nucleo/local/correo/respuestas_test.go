// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"strings"
	"testing"
)

// replyDe es un correo con el REPLY de iTIP de `asistente` a lo que organiza
// `organizador`, mandado por `de` (como los de Google y Outlook: un multipart/alternative
// con el texto y la parte `text/calendar; method=REPLY`).
func replyDe(id, de, asistente, organizador, partstat string) string {
	return `From: ` + de + `
To: ` + organizador + `
Subject: Accepted: Revisión trimestral
Date: Tue, 24 Nov 2026 10:00:00 +0100
Message-ID: <` + id + `@ejemplo.eu>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="b"

--b
Content-Type: text/plain; charset=utf-8

Ha contestado a la invitación.
--b
Content-Type: text/calendar; charset=utf-8; method=REPLY

BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Prueba//ES
METHOD:REPLY
BEGIN:VEVENT
UID:evento-9@ejemplo.eu
SEQUENCE:0
DTSTAMP:20261124T090000Z
DTSTART:20261201T090000Z
DTEND:20261201T100000Z
SUMMARY:Revisión trimestral
ORGANIZER:mailto:` + organizador + `
ATTENDEE;CN=Invitado;PARTSTAT=` + partstat + `:mailto:` + asistente + `
END:VEVENT
END:VCALENDAR
--b--
`
}

// TestLasRespuestasDeLosInvitadosVanACalendar: lo que llega NUEVO a la entrada con un
// REPLY a lo que organiza la persona (la dirección de una cuenta de Mail) y lo manda
// quien contesta se deja en la cola de Calendar y se marca como procesado; el que manda
// otro, o a lo que organiza otro, se marca sin dejarlo; una invitación no se toca; lo de
// la primera bajada no se mira; el lector no lo tiene por invitación, y el mismo mensaje
// que vuelve (otro cliente lo movió y lo devolvió) no se deja dos veces.
func TestLasRespuestasDeLosInvitadosVanACalendar(t *testing.T) {
	ctx := context.Background()
	si, ss := nuevoIMAP(t), nuevoSMTP(t)
	// Lo que ya estaba antes de la primera vuelta no se mira.
	si.entregar(t, "INBOX", replyDe("viejo", "Marta <marta@ejemplo.eu>", "marta@ejemplo.eu", usuario, "DECLINED"))
	b := abrir(t, si, ss)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if rs, err := RespuestasPorAplicar(b.raiz); err != nil || len(rs) != 0 {
		t.Fatalf("lo de la primera bajada no va a la cola: %v %v", rs, err)
	}

	si.entregar(t, "INBOX", replyDe("bueno", "Luis <luis@ejemplo.eu>", "luis@ejemplo.eu", usuario, "ACCEPTED"))
	si.entregar(t, "INBOX", replyDe("suplantado", "Intruso <intruso@ejemplo.eu>", "marta@ejemplo.eu", usuario, "ACCEPTED"))
	si.entregar(t, "INBOX", replyDe("ajeno", "Luis <luis@ejemplo.eu>", "luis@ejemplo.eu", "pedro@otro.eu", "ACCEPTED"))
	si.entregar(t, "INBOX", invitacion)
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	rs, err := RespuestasPorAplicar(b.raiz)
	if err != nil || len(rs) != 1 {
		t.Fatalf("cola: %d %v", len(rs), err)
	}
	ics := strings.ReplaceAll(string(rs[0].ICS), "\r\n", "\n")
	if !strings.Contains(ics, "METHOD:REPLY") || !strings.Contains(ics, "UID:evento-9@ejemplo.eu") || !strings.Contains(ics, "PARTSTAT=ACCEPTED:mailto:luis@ejemplo.eu") {
		t.Fatalf("lo dejado es el REPLY tal cual: %s", ics)
	}
	if got := RespuestasEnCola(b.raiz); len(got) != 1 {
		t.Fatalf("nombres en la cola: %v", got)
	}

	ms, _, _ := b.Messages(ctx, "rol:inbox", "", 50)
	porMID := map[string]string{}
	for _, m := range ms {
		porMID[m.MessageID] = m.ID
	}
	for mid, quiero := range map[string]bool{"viejo@ejemplo.eu": false, "bueno@ejemplo.eu": true, "suplantado@ejemplo.eu": true, "ajeno@ejemplo.eu": true, "inv@ejemplo.eu": false} {
		id := porMID[mid]
		if id == "" {
			t.Fatalf("no está %s: %v", mid, porMID)
		}
		if ya, err := b.RespuestaProcesada(ctx, id); err != nil || ya != quiero {
			t.Errorf("%s procesado = %v (%v), quiero %v", mid, ya, err, quiero)
		}
	}
	// El lector no lo enseña como invitación (no se contesta).
	inv, err := b.Invitacion(ctx, porMID["bueno@ejemplo.eu"])
	if err != nil || !inv.EsRespuesta() || inv.Contestable() {
		t.Fatalf("un REPLY no es una invitación: %+v %v", inv, err)
	}

	// Calendar la recoge.
	if err := rs[0].Quitar(); err != nil {
		t.Fatal(err)
	}
	if rs, _ := RespuestasPorAplicar(b.raiz); len(rs) != 0 {
		t.Fatalf("recogida: %v", rs)
	}
	// El mismo mensaje vuelve con otro UID: ya estaba procesado.
	si.entregar(t, "INBOX", replyDe("bueno", "Luis <luis@ejemplo.eu>", "luis@ejemplo.eu", usuario, "ACCEPTED"))
	if err := b.Sincronizar(ctx); err != nil {
		t.Fatal(err)
	}
	if rs, _ := RespuestasPorAplicar(b.raiz); len(rs) != 0 {
		t.Fatalf("el mismo REPLY no se deja dos veces: %d", len(rs))
	}
}
