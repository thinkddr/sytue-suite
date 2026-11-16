// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// invitacionDeFuera es la copia que guarda Ana de una reunión que organiza alguien
// de fuera de la suite: con ella y con otro invitado.
const invitacionDeFuera = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Otro//ES\r\n" +
	"BEGIN:VEVENT\r\nUID:fuera-1@otro.example\r\nDTSTAMP:20260630T120000Z\r\n" +
	"DTSTART:20260701T090000Z\r\nDTEND:20260701T100000Z\r\nSEQUENCE:2\r\nSUMMARY:Revisión\r\n" +
	"ORGANIZER;CN=Luis:mailto:luis@otro.example\r\n" +
	"ATTENDEE;CN=Ana;PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:ana@sytue.eu\r\n" +
	"ATTENDEE;PARTSTAT=ACCEPTED:mailto:marta@otro.example\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

type correoEnviado struct {
	para, asunto, cuerpo, ics, metodo string
}

// servicioConCorreo es un servicio sin store con el relé cambiado por uno que apunta.
func servicioConCorreo(t *testing.T) (*Service, <-chan correoEnviado) {
	t.Helper()
	s := New(config.Config{PublicURL: "https://calendar.sytue.eu"}, nil, nil)
	enviados := make(chan correoEnviado, 4)
	s.SetCorreo(func(_ context.Context, para, asunto, cuerpo string, ics []byte, metodo string) error {
		enviados <- correoEnviado{para, asunto, cuerpo, string(ics), metodo}
		return nil
	})
	return s, enviados
}

func deAna() context.Context {
	return authn.Into(context.Background(), authn.Claims{Subject: "ana-1", Email: "ana@sytue.eu"})
}

// Cambiar de respuesta desde el evento de alguien de fuera le manda el REPLY por
// correo: a quien organiza, con método REPLY y la respuesta NUEVA de Ana, que es lo
// que su cliente (y RecibirRespuesta, si fuera de la suite) lee.
func TestLaRespuestaAUnOrganizadorDeFueraSalePorCorreo(t *testing.T) {
	s, enviados := servicioConCorreo(t)
	if !s.mandarRespuestaPorCorreo(deAna(), invitacionDeFuera, "ana@sytue.eu", domain.PartDeclined) {
		t.Fatal("la respuesta no sale")
	}
	var c correoEnviado
	select {
	case c = <-enviados:
	case <-time.After(5 * time.Second):
		t.Fatal("no llega el correo")
	}
	if c.para != "luis@otro.example" || c.metodo != "REPLY" {
		t.Errorf("para %q método %q", c.para, c.metodo)
	}
	if !strings.HasPrefix(c.asunto, "Rechazado: Revisión") || !strings.Contains(c.cuerpo, "ana@sytue.eu") {
		t.Errorf("asunto %q cuerpo %q", c.asunto, c.cuerpo)
	}
	uid, r, ok := LeerRespuesta(c.ics)
	if !ok || uid != "fuera-1@otro.example" || len(r) != 1 || r["ana@sytue.eu"] != domain.InvDeclined {
		t.Errorf("REPLY = %q %v %v:\n%s", uid, r, ok, c.ics)
	}
	if strings.Contains(c.ics, "marta@") {
		t.Errorf("el REPLY no lleva a los demás invitados:\n%s", c.ics)
	}
}

// Quien organiza desde dentro recibe el REPLY en su bandeja, no por correo; y no hay
// correo sin relé, sin figurar como invitado, a uno mismo ni con NEEDS-ACTION.
func TestLaRespuestaNoSalePorCorreoSiNoToca(t *testing.T) {
	s, enviados := servicioConCorreo(t)
	dentro := strings.Replace(invitacionDeFuera, "mailto:luis@otro.example", "mailto:luis@sytue.eu", 1)
	casos := map[string]func() bool{
		"organizador de dentro": func() bool {
			return s.mandarRespuestaPorCorreo(deAna(), dentro, "ana@sytue.eu", domain.PartAccepted)
		},
		"no invitado": func() bool {
			return s.mandarRespuestaPorCorreo(deAna(), invitacionDeFuera, "carla@sytue.eu", domain.PartAccepted)
		},
		"a uno mismo": func() bool {
			return s.mandarRespuestaPorCorreo(deAna(), invitacionDeFuera, "luis@otro.example", domain.PartAccepted)
		},
		"sin respuesta": func() bool {
			return s.mandarRespuestaPorCorreo(deAna(), invitacionDeFuera, "ana@sytue.eu", domain.PartNeedsAction)
		},
		"sin relé": func() bool {
			sinRele := New(config.Config{PublicURL: "https://calendar.sytue.eu"}, nil, nil)
			return sinRele.mandarRespuestaPorCorreo(deAna(), invitacionDeFuera, "ana@sytue.eu", domain.PartAccepted)
		},
	}
	for nombre, caso := range casos {
		if caso() {
			t.Errorf("%s: sale un correo", nombre)
		}
	}
	select {
	case c := <-enviados:
		t.Errorf("salió un correo a %q", c.para)
	case <-time.After(50 * time.Millisecond):
	}
}

// Las respuestas por correo cuentan en el mismo tope por hora que las invitaciones.
func TestLaRespuestaPorCorreoRespetaElTope(t *testing.T) {
	s, _ := servicioConCorreo(t)
	if !s.cupoCorreo.tomar("ana-1", maxCorreosAFueraPorHora, time.Now()) {
		t.Fatal("no se pudo agotar el cupo")
	}
	if s.mandarRespuestaPorCorreo(deAna(), invitacionDeFuera, "ana@sytue.eu", domain.PartAccepted) {
		t.Error("sin cupo no sale")
	}
}
