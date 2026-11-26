// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Sin personas (Sytue Office) no se pregunta por «Compartidos conmigo» ni por las
// invitaciones, no se ofrece invitar ni reservar, guardar no es invitar, y con
// PistaEnlace el formulario lleva un campo libre para el enlace de la reunión.
func TestSinPersonasYEnlaceLibre(t *testing.T) {
	s := &servicioFalso{}
	a := appDePrueba(t, s)
	a.P.SinPersonas = true
	a.P.PistaEnlace = "Pega el enlace"
	a.Arrancar()
	for _, pedido := range s.pedidos {
		if strings.Contains(pedido, "/shared") || strings.Contains(pedido, "/invitations") {
			t.Fatalf("sin personas no se pide %q", pedido)
		}
	}
	if len(a.P.Compartidos) != 0 || a.P.Invitaciones != 0 {
		t.Fatal("ni compartidos ni invitaciones", a.P.Compartidos, a.P.Invitaciones)
	}

	e := Edicion{
		Inicio: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC), Fin: time.Date(2026, 9, 22, 11, 0, 0, 0, time.UTC),
		Invitados: []string{"ana@ejemplo.org"},
	}
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	if f.a.Recursos != nil {
		t.Fatal("sin personas no se reservan salas ni recursos")
	}
	if f.enlace == nil {
		t.Fatal("con PistaEnlace hay campo libre para el enlace")
	}
	f.enlace.PonerTexto("  https://meet.example/abc  ")
	in := f.e.EntradaDe(a.P)
	if in.ConferenceURL != "https://meet.example/abc" {
		t.Fatalf("el enlace no llega al evento: %q", in.ConferenceURL)
	}
	if len(in.Attendees) != 1 || in.Send {
		t.Fatalf("los invitados que traía se conservan, pero no se les manda nada: %+v send=%v", in.Attendees, in.Send)
	}

	// Sin las opciones, todo como siempre.
	b := appDePrueba(t, &servicioFalso{})
	fb := b.P.nuevoFormulario(e, b.destinos(), b.accionesFormulario(e))
	if fb.enlace != nil || !fb.e.EntradaDe(b.P).Send {
		t.Fatal("la edición conectada no cambia")
	}
	_ = api.Event{}
}
