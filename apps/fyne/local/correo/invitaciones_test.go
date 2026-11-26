// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"path/filepath"
	"strings"
	"testing"

	almacen "git.sytue.eu/sytue/one/apps/nucleo/local/correo"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

func calendarioEnCorreo(metodo, partstat string) string {
	return strings.ReplaceAll(`From: Luis <luis@ejemplo.eu>
To: ana@ejemplo.eu
Subject: Revisión
Message-ID: <`+metodo+`@ejemplo.eu>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="b"

--b
Content-Type: text/plain; charset=utf-8

Revisión.
--b
Content-Type: text/calendar; charset=utf-8; method=`+metodo+`

BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//prueba//ES
METHOD:`+metodo+`
BEGIN:VEVENT
UID:evento-1@ejemplo.eu
DTSTAMP:20261120T090000Z
DTSTART:20261201T090000Z
DTEND:20261201T100000Z
SUMMARY:Revisión
ORGANIZER:mailto:ana@ejemplo.eu
ATTENDEE;PARTSTAT=`+partstat+`:mailto:luis@ejemplo.eu
END:VEVENT
END:VCALENDAR
--b--
`, "\n", "\r\n")
}

// TestUnaRespuestaNoEsUnaInvitacion: el lector enseña la tarjeta de una invitación
// (REQUEST), pero no la de la respuesta de un invitado (REPLY), que Mail deja para
// Calendar.
func TestUnaRespuestaNoEsUnaInvitacion(t *testing.T) {
	dir := t.TempDir()
	b, err := almacen.Abrir(dir, secretos.SoloFichero(filepath.Join(dir, "secretos")), almacen.Red{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cerrar()
	x := Invitaciones{B: b}
	req, err := b.ImportarEML(t.Context(), []byte(calendarioEnCorreo("REQUEST", "NEEDS-ACTION")))
	if err != nil {
		t.Fatal(err)
	}
	if r, ok, err := x.LeerInvitacion(t.Context(), req.ID); err != nil || !ok || r.Resumen != "Revisión" {
		t.Fatalf("una invitación sale: %+v %v %v", r, ok, err)
	}
	rep, err := b.ImportarEML(t.Context(), []byte(calendarioEnCorreo("REPLY", "ACCEPTED")))
	if err != nil {
		t.Fatal(err)
	}
	if r, ok, err := x.LeerInvitacion(t.Context(), rep.ID); err != nil || ok {
		t.Fatalf("una respuesta no sale como invitación: %+v %v %v", r, ok, err)
	}
}
