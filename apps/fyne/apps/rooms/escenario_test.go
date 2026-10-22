// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func llamadaDe(id string) llamada.Participante { return llamada.Participante{Identidad: id} }

// El protagonista sigue las reglas de `escenario.ts` y en su orden: lo fijado gana a la
// pantalla, la pantalla gana al orador, el orador solo en su modo, y nadie que se haya
// ido. El orden de la web se lee de su fichero: si cambia allí, esto se entera.
//
// VISTO EN ROJO (2026-09-27, en el frontal): con la pantalla antes que lo fijado,
// fijar a alguien no servía en cuanto otro compartía.
func TestElProtagonistaDelEscenario(t *testing.T) {
	presentes := []string{"ana", "beto", idPantalla("beto")}
	for _, c := range []struct {
		e     Escena
		quien string
	}{
		{Escena{Modo: ModoMosaico}, ""},
		{Escena{Modo: ModoOrador, UltimoHablante: "ana"}, "ana"},
		{Escena{Modo: ModoMosaico, UltimoHablante: "ana"}, ""},
		{Escena{Modo: ModoMosaico, Compartiendo: idPantalla("beto"), UltimoHablante: "ana"}, idPantalla("beto")},
		{Escena{Modo: ModoOrador, Fijado: "ana", Compartiendo: idPantalla("beto")}, "ana"},
		{Escena{Modo: ModoOrador, Fijado: "eva", UltimoHablante: "eva"}, ""},
	} {
		c.e.Presentes = presentes
		if got := Protagonista(c.e); got != c.quien {
			t.Errorf("%+v: %q y es %q", c.e, got, c.quien)
		}
	}
	web := webref.Read(t, "apps/frontend/apps/rooms/src/lib/escenario.ts")
	if !strings.Contains(web, "presente(e.fijado) ??\n    presente(e.compartiendo) ??\n    (e.modo === 'orador' ? presente(e.ultimoHablante) : null)") {
		t.Error("el orden de las reglas de escenario.ts ya no es este: revisar Protagonista")
	}
	ts := []*TeselaLlamada{{Participante: llamadaDe("a")}, {Participante: llamadaDe("b")}, {Participante: llamadaDe("c")}}
	if got := OrdenarPorProtagonista(ts, "c"); got[0].Identidad != "c" || got[1].Identidad != "a" || got[2].Identidad != "b" {
		t.Errorf("ordenar: %s %s %s", got[0].Identidad, got[1].Identidad, got[2].Identidad)
	}
}
