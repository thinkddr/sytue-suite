// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

// El CN de quien organiza: el de la sesión si organiza ella (el nombre de Sytue ID manda
// sobre lo que diga el cuerpo), el que el evento ya tenía al editar, y nunca uno
// inventado para la dirección de otra persona.
func TestNombreOrganizador(t *testing.T) {
	ana := authn.Into(context.Background(), authn.Claims{Subject: "ana", Email: "ana@sytue.eu", Mailbox: "ana@acme.sytue.eu", Name: "Ana Pérez"})
	sinNombre := authn.Into(context.Background(), authn.Claims{Subject: "ana", Email: "ana@sytue.eu"})
	for _, c := range []struct {
		nombre                   string
		ctx                      context.Context
		organizer, pedido, tenia string
		quiere                   string
	}{
		{"organiza ella", ana, "ana@sytue.eu", "", "", "Ana Pérez"},
		{"con su buzón", ana, "mailto:ana@acme.sytue.eu", "", "", "Ana Pérez"},
		{"el token manda sobre el cuerpo", ana, "ana@sytue.eu", "La Jefa", "", "Ana Pérez"},
		{"sin nombre en el token, el del cuerpo", sinNombre, "ana@sytue.eu", "Ana P.", "", "Ana P."},
		{"editar conserva el que tenía", ana, "ana@sytue.eu", "", "Ana (Acme)", "Ana (Acme)"},
		{"de otra persona se conserva", ana, "beto@sytue.eu", "Ana Pérez", "Beto", "Beto"},
		{"de otra persona no se inventa", ana, "beto@sytue.eu", "Ana Pérez", "", ""},
		{"se sanea", sinNombre, "ana@sytue.eu", "Ana\r\nX:\"y\"", "", "AnaX:y"},
	} {
		if got := nombreOrganizador(c.ctx, c.organizer, c.pedido, c.tenia); got != c.quiere {
			t.Errorf("%s: %q, y debía ser %q", c.nombre, got, c.quiere)
		}
	}
}

// toStoreInput lleva el CN a la proyección y al iCalendar que sirve CalDAV.
func TestToStoreInputLlevaElCNDelOrganizador(t *testing.T) {
	in := EventInput{Summary: "Repaso", Organizer: "ana@sytue.eu", OrganizerName: "Ana Pérez"}
	in.DTStart = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	in.DTEnd = in.DTStart.Add(time.Hour)
	si, err := (&Service{}).toStoreInput(in)
	if err != nil {
		t.Fatal(err)
	}
	if si.OrganizerName != "Ana Pérez" {
		t.Errorf("proyección: %q", si.OrganizerName)
	}
	if !strings.Contains(si.ICalData, "ORGANIZER;CN=Ana Pérez:mailto:ana@sytue.eu") {
		t.Errorf("el iCalendar no lleva el CN:\n%s", si.ICalData)
	}
}
