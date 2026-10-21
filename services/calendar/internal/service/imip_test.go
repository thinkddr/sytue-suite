// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/calendar/internal/config"
)

// Quién recibe la invitación por correo: solo las direcciones cuyo dominio no es de la
// suite (ni un subdominio suyo), y el apex sale de la URL pública del servicio.
func TestQuienEsDeFuera(t *testing.T) {
	if got := apexDe("https://calendar.sytue.eu"); got != "sytue.eu" {
		t.Errorf("apexDe = %q", got)
	}
	if got := apexDe("https://calendar.sytue.localhost:8443"); got != "sytue.localhost" {
		t.Errorf("apexDe con puerto = %q", got)
	}
	internos := []string{"sytue.eu", "Sytue.COM"}
	for correo, fuera := range map[string]bool{
		"ana@sytue.eu":           false,
		"ana@corporate.sytue.eu": false,
		"ANA@SYTUE.COM":          false,
		"ana@gmail.com":          true,
		"ana@nosytue.eu":         true, // no es subdominio: es otro dominio que acaba igual
		"sin-arroba":             false,
		"":                       false,
	} {
		if got := EsDeFuera(correo, internos); got != fuera {
			t.Errorf("EsDeFuera(%q) = %v; quiero %v", correo, got, fuera)
		}
	}
}

// Un REPLY se lee como `parseReminders` lee sus alarmas: UID y la respuesta de cada
// asistente; lo que no es REPLY, o no dice nada, no vale.
func TestLeerRespuesta(t *testing.T) {
	uid, r, ok := LeerRespuesta("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:u1\r\nDTSTAMP:20260701T080000Z\r\n" +
		"ATTENDEE;PARTSTAT=TENTATIVE:mailto:Luis@Gmail.com\r\nATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:otro@x.es\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	if !ok || uid != "u1" || len(r) != 1 || r["luis@gmail.com"] != "tentative" {
		t.Fatalf("LeerRespuesta = %q %v %v", uid, r, ok)
	}
	if _, _, ok := LeerRespuesta("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:u1\r\nDTSTAMP:20260701T080000Z\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:a@b.c\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"); ok {
		t.Fatal("un REQUEST no es una respuesta")
	}
	if _, _, ok := LeerRespuesta("no es iCalendar"); ok {
		t.Fatal("basura")
	}
}

// Los dominios de dentro son los que Sytue Mail tiene dados de alta, pedidos con el
// secreto de la suite, más el apex de la URL pública, que cuenta aunque Mail no conteste.
func TestLosDominiosDeDentroSonLosDeMail(t *testing.T) {
	mail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/domains" || r.Header.Get("Authorization") != "Bearer s3creto" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"domains":["acme.eu","sytue.eu"]}`))
	}))
	defer mail.Close()
	s := &Service{cfg: config.Config{PublicURL: "https://calendar.sytue.eu"}}
	if got := s.dominiosInternos(); !slices.Equal(got, []string{"sytue.eu"}) {
		t.Fatalf("sin Mail = %v, quiero solo el apex", got)
	}
	if _, err := DominiosDeMail(context.Background(), mail.URL, "otro"); err == nil {
		t.Fatal("sin el secreto, Mail no da los dominios")
	}
	ds, err := DominiosDeMail(context.Background(), mail.URL+"/", "s3creto")
	if err != nil {
		t.Fatal(err)
	}
	s.SetDominiosInternos(ds)
	if EsDeFuera("ana@acme.eu", s.dominiosInternos()) || !EsDeFuera("ana@gmail.com", s.dominiosInternos()) {
		t.Fatalf("con los de Mail = %v", s.dominiosInternos())
	}
}
