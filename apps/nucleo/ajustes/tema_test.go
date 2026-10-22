// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ajustes

import (
	"testing"
)

func TestCambiarOtroAjusteConservaLaAparienciaElegida(t *testing.T) {
	for _, tema := range []string{"claro", "oscuro", "traslucido", ""} {
		t.Run(tema, func(t *testing.T) {
			aparte(t)
			if err := Cambiar(func(a *Ajustes) {
				a.Tema, a.Idioma, a.Bloqueo, a.Invisible = tema, "es-ES", true, true
			}); err != nil {
				t.Fatal(err)
			}
			if err := Cambiar(func(a *Ajustes) { a.Idioma = "is" }); err != nil {
				t.Fatal(err)
			}
			if e := Leer(); e.Tema != tema || e.Idioma != "is" || !e.Bloqueo || !e.Invisible {
				t.Errorf("cambiar el idioma ha perdido preferencias guardadas: %+v", e)
			}
		})
	}
}
