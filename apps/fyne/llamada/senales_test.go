// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package llamada

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Las señales son las de la web: el mismo JSON sale y entra, una reacción que no está
// en la lista no pasa, y la lista es la de `salaDatos.ts`.
//
// VISTO EN ROJO (2026-09-26, en el frontal): con `omitempty` en la mano, bajarla
// mandaba `{"t":"mano"}` y la web la descartaba (exige `alzada` booleano).
func TestLasSenalesSonLasDeLaWeb(t *testing.T) {
	if got := string(Mano(false).Codificar()); got != `{"t":"mano","alzada":false}` {
		t.Errorf("bajar la mano: %s", got)
	}
	if got := string(Reaccion("👏").Codificar()); got != `{"t":"reaccion","emoji":"👏"}` {
		t.Errorf("aplauso: %s", got)
	}
	for _, c := range []struct {
		in string
		ok bool
	}{
		{`{"t":"mano","alzada":true}`, true},
		{`{"t":"mano"}`, false},
		{`{"t":"reaccion","emoji":"🎉"}`, true},
		{`{"t":"reaccion","emoji":"💩"}`, false},
		{`no`, false},
	} {
		if _, ok := DescodificarSenal([]byte(c.in)); ok != c.ok {
			t.Errorf("%s: %v", c.in, ok)
		}
	}
	web := webref.Read(t, "apps/frontend/apps/rooms/src/lib/salaDatos.ts")
	if !strings.Contains(web, "['"+strings.Join(Reacciones, "', '")+"']") {
		t.Error("las reacciones de la web ya no son estas: revisar Reacciones")
	}
}
