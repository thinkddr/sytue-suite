// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Con varias cuentas que conocen gente, Sytue Office pone la cuenta detrás del nombre
// de cada persona («Luis Pérez · @ana:servidor», `local/chat`) y `ui.Iniciales`, que es
// el espejo de la web y no se toca, sacaba «L@». El avatar sale del nombre sin la cuenta
// (`PlainName`, que pone el agregador); sin él —la edición conectada—, del nombre, como
// siempre. El nombre que se LEE sigue llevando la cuenta: es lo que distingue a dos Luis.
func TestElAvatarNoLlevaLaCuentaDetras(t *testing.T) {
	test.NewTempApp(t)
	const conCuenta = "Luis Pérez · @ana:servidor"
	if ui.Iniciales(conCuenta) != "L@" {
		t.Fatalf("el caso de partida ya no es el que era: %q", ui.Iniciales(conCuenta))
	}
	casos := []struct {
		que, nombre, sinCuenta, quiero string
	}{
		{"con la cuenta detrás", conCuenta, "Luis Pérez", "LP"},
		{"sin PlainName (la conectada)", "Marta Ruiz", "", "MR"},
	}
	for _, c := range casos {
		p := NuevaFilaPersona(Persona{ID: "x", Nombre: c.nombre, NombreSinCuenta: c.sinCuenta}, false, func() {})
		test.WidgetRenderer(p)
		if got := ui.Iniciales(p.avatar.Nombre); got != c.quiero {
			t.Errorf("%s: «Nuevo mensaje» pinta %q de iniciales; quiero %q", c.que, got, c.quiero)
		}
		m := NuevaFilaMiembro(Miembro{ID: "x", Nombre: c.nombre, NombreSinCuenta: c.sinCuenta}, "", "", nil)
		test.WidgetRenderer(m)
		if got := ui.Iniciales(m.avatar.Nombre); got != c.quiero {
			t.Errorf("%s: «Miembros» pinta %q de iniciales; quiero %q", c.que, got, c.quiero)
		}
		if m.Miembro.Nombre != c.nombre || p.Persona.Nombre != c.nombre {
			t.Errorf("%s: el nombre que se lee tiene que seguir siendo %q", c.que, c.nombre)
		}
	}

	// Lo que contesta el servicio llega a la pantalla: la gente del equipo y la persona
	// encontrada, con y sin el nombre sin cuenta.
	if m := miembroDe(api.RoomsMember{Subject: "mx1|@luis:x", Name: conCuenta, PlainName: "Luis Pérez"}); m.Nombre != conCuenta || m.NombreDePersona() != "Luis Pérez" {
		t.Errorf("miembroDe con PlainName: %+v", m)
	}
	if m := miembroDe(api.RoomsMember{Subject: "s", Name: "Ana"}); m.NombreDePersona() != "Ana" || m.NombreSinCuenta != "" {
		t.Errorf("miembroDe sin PlainName: %+v", m)
	}
}
