// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"strings"
	"testing"
)

const conZona = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\nUID:r@prueba\r\nDTSTAMP:20260101T000000Z\r\n" +
	"DTSTART;TZID=Europe/Madrid:20261202T100000\r\nDTEND;TZID=Europe/Madrid:20261202T110000\r\nRRULE:FREQ=WEEKLY\r\n" +
	"EXDATE;TZID=Europe/Madrid:20261209T100000\r\nSUMMARY:Seguimiento\r\nORGANIZER:mailto:ana@example.org\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:luis@example.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// La firma de la invitación es lo que SIGNIFICA el evento: la EXDATE en UTC o el STATUS
// CONFIRMED que no traía no son otra invitación; un título nuevo o otra respuesta, sí.
// Las firmas que guardó una versión anterior (el texto de cada propiedad) se siguen
// entendiendo: lo mismo, escrito igual, es la misma firma; tras actualizar no sale una
// invitación por cada evento que se suba.
func TestLaFirmaEsLoQueSignifica(t *testing.T) {
	enUTC := strings.Replace(conZona, "EXDATE;TZID=Europe/Madrid:20261209T100000", "EXDATE:20261209T090000Z\r\nSTATUS:CONFIRMED", 1)
	a, b := firmaInvitacion(conZona), firmaInvitacion(enUTC)
	if a.vacia() || !a.igual(b) || !a.mismoContenido(b) || !strings.HasPrefix(a.String(), versionFirma) {
		t.Fatalf("lo mismo escrito de otra forma: %s / %s", a, b)
	}
	if otra := firmaInvitacion(strings.Replace(conZona, "SUMMARY:Seguimiento", "SUMMARY:Otro", 1)); otra.mismoContenido(a) {
		t.Fatal("otro título es otra invitación")
	}
	respuesta := firmaInvitacion(strings.Replace(conZona, "PARTSTAT=ACCEPTED", "PARTSTAT=DECLINED", 1))
	if !respuesta.mismoContenido(a) || respuesta.igual(a) {
		t.Fatal("otra respuesta es el mismo contenido y otra firma")
	}

	// Una firma de antes (guardada en el estado) y la de ahora del mismo texto.
	deAntes := leerFirma(firma{contenido: a.textual, invitados: a.invitados}.String())
	if !deAntes.deAntes() || !a.igual(deAntes) || !deAntes.igual(a) || !a.mismoContenido(deAntes) {
		t.Fatalf("la firma de antes del mismo texto: %s / %s", deAntes, a)
	}
	if b.igual(deAntes) {
		t.Fatal("con una firma de antes solo se sabe comparar el mismo texto")
	}
	if guardada := leerFirma(a.String()); guardada.deAntes() || !guardada.igual(a) || !guardada.igual(b) {
		t.Fatalf("la firma de ahora, guardada y leída: %s", guardada)
	}
	if !(firma{}).igual(leerFirma("")) || a.igual(firma{}) {
		t.Fatal("sin invitados no hay firma")
	}
}
