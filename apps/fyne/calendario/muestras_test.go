// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaPildoraDeUnInvitadoEsLaEtiquetaComun: el invitado del formulario es `ui.Etiqueta`
// (la `<Tag>` de EventModal.tsx): el aspa nombra al invitado y tiene la diana de 24 con
// ratón y 44 con el dedo —la copia local se quedó en 20—, y la respuesta que ya dio va
// entre el correo y el aspa.
func TestLaPildoraDeUnInvitadoEsLaEtiquetaComun(t *testing.T) {
	for _, c := range []struct {
		tactil bool
		lado   float32
	}{{false, 24}, {true, 44}} {
		func() {
			defer ui.TactilDePrueba(c.tactil)()
			p := pantallaDeMuestra(t)
			quitados := 0
			e := nuevaPildoraInvitado(p, "ana@sytue.eu", "ACCEPTED", func() { quitados++ })
			test.NewTempWindow(t, e)
			e.Resize(e.MinSize())

			a := e.Aspa()
			if a == nil {
				t.Fatal("el invitado no tiene aspa para quitarlo")
			}
			if s := a.Size(); s.Width < c.lado || s.Height < c.lado {
				t.Errorf("táctil=%v: el aspa mide %v y la diana pide %v", c.tactil, s, c.lado)
			}
			if got, want := a.AccessibilityLabel(), p.t("event.guestRemove", i18n.Vars{"email": "ana@sytue.eu"}); got != want {
				t.Errorf("el aspa se llama %q y debería llamarse %q", got, want)
			}
			test.Tap(a)
			if quitados != 1 {
				t.Errorf("pulsar el aspa quita %d veces", quitados)
			}

			ins, es := e.Anexo().(*ui.Insignia)
			if !es {
				t.Fatalf("la respuesta es un %T y debería ser una Insignia", e.Anexo())
			}
			if d := ins.Position().X + ins.Size().Width; d > a.Position().X+0.01 {
				t.Errorf("la insignia acaba en %v y el aspa empieza en %v", d, a.Position().X)
			}
		}()
	}

	// Un recién añadido no ha respondido: sin insignia.
	if e := nuevaPildoraInvitado(pantallaDeMuestra(t), "luis@sytue.eu", "", func() {}); e.Anexo() != nil {
		t.Error("un invitado sin respuesta lleva insignia")
	}
}
