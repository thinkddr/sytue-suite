// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notify

import "testing"

func recibido(ch <-chan Evento) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Un aviso le llega a la PERSONA en la sesión de cualquiera de sus organizaciones
// (ADR 0053); un evento de estado, solo en la organización en la que ocurrió. Y a
// nadie más: ni a otra persona de la misma organización ni a otra de otra.
func TestDispatchPorPersona(t *testing.T) {
	h := NewHub(4)
	beaGlobex, c1 := h.Subscribe("globex", []string{"bea", "mail:bea@corp.test"}, "bea", "mail:bea@corp.test", "mail:bea@gmail.test")
	defer c1()
	anaAcme, c2 := h.Subscribe("acme", []string{"ana"}, "ana")
	defer c2()
	carlaGlobex, c3 := h.Subscribe("globex", []string{"carla"}, "carla")
	defer c3()

	h.Dispatch("acme", "bea", Evento{Nombre: "notification"})
	if !recibido(beaGlobex) {
		t.Error("el aviso de acme no le llega a bea en su sesión de globex")
	}
	if recibido(anaAcme) || recibido(carlaGlobex) {
		t.Error("un aviso de bea le llega a otra persona")
	}

	h.Dispatch("acme", "mail:bea@corp.test", Evento{Nombre: "notification"})
	if !recibido(beaGlobex) {
		t.Error("el aviso a su buzón de organización no le llega")
	}
	// El correo de entrar no es personal (ADR 0053): su aviso no cruza de organización.
	h.Dispatch("acme", "mail:bea@gmail.test", Evento{Nombre: "notification"})
	if recibido(beaGlobex) {
		t.Error("un aviso de acme a su correo de entrar cruza a globex")
	}
	h.Dispatch("globex", "mail:bea@gmail.test", Evento{Nombre: "notification"})
	if !recibido(beaGlobex) {
		t.Error("el aviso a su correo de entrar en su organización no le llega")
	}

	h.Dispatch("acme", "bea", Evento{Nombre: "avatar"})
	if recibido(beaGlobex) {
		t.Error("un evento de estado de acme cruza a su sesión de globex")
	}
	h.Dispatch("globex", "bea", Evento{Nombre: "avatar"})
	if !recibido(beaGlobex) {
		t.Error("un evento de estado de su organización no le llega")
	}
}

// Darse de baja la quita de los dos índices.
func TestSubscribeBajaDeLosDosIndices(t *testing.T) {
	h := NewHub(1)
	_, cancel := h.Subscribe("globex", []string{"bea"}, "bea")
	cancel()
	if len(h.subs) != 0 || len(h.porPersona) != 0 {
		t.Fatalf("quedan suscripciones: %v %v", h.subs, h.porPersona)
	}
}
