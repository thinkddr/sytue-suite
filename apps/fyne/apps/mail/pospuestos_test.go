// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaCarpetaDePospuestosEsLaDeLaWeb: el buzón de rol `snoozed` (D-MAIL-07) se
// pinta como en la web aunque Fyne aún no pospone: justo debajo de Recibidos, con el
// reloj y sin contador, y no es destino de «Mover a…» ni de una regla, porque lo que
// entra ahí sin posponerlo no tiene fecha de vuelta.
func TestLaCarpetaDePospuestosEsLaDeLaWeb(t *testing.T) {
	cs := ordenarCarpetas("es", []api.Mailbox{
		{ID: "ar", Name: "Archive", Role: "archive"},
		{ID: "pr", Name: "Proyectos"},
		{ID: "sn", Name: "Snoozed", Role: "snoozed"},
		{ID: "dr", Name: "Drafts", Role: "drafts"},
		{ID: "in", Name: "Inbox", Role: "inbox"},
	})
	orden := make([]string, 0, len(cs))
	for _, c := range cs {
		orden = append(orden, c.ID)
	}
	if want := []string{"in", "sn", "dr", "ar", "pr"}; !slices.Equal(orden, want) {
		t.Errorf("orden %v, quería %v", orden, want)
	}
	if GlifoCarpeta("snoozed") != RelojD {
		t.Error("«Pospuestos» no lleva el reloj de la web")
	}

	m := &Menu{T: traductor(t), Locale: "es", EstadoCarpetas: Listo, AlElegirCarpeta: func(Carpeta) {}}
	if f := m.filaCarpeta(Carpeta{ID: "sn", Nombre: "Snoozed", Rol: "snoozed", SinLeer: 3}, tema.Claro); f.Contador != "" {
		t.Errorf("«Pospuestos» enseña el contador %q", f.Contador)
	}

	s := servicioConTodas()
	s.carpetas = append(s.carpetas, api.Mailbox{ID: "sn", Name: "Snoozed", Role: "snoozed"})
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	for _, d := range b.P.Acciones.Destinos {
		if d.Rol == "snoozed" {
			t.Error("«Mover a…» ofrece «Pospuestos»")
		}
	}
	g := &gestorReglas{b: b}
	roles, otras := g.destinos()
	if _, hay := roles["snoozed"]; hay || slices.ContainsFunc(otras, func(m api.Mailbox) bool { return m.Role == "snoozed" }) {
		t.Error("una regla puede mandar el correo a «Pospuestos»")
	}
}
