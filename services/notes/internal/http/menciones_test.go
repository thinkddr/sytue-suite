// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// TestUnaMencionAlFinalDelParrafoAvisa: con la mención al final de un párrafo el editor
// guarda `@ana@acme.test&nbsp;`, y lo que sigue va en otro párrafo. Al pasar a texto ese
// `&nbsp;` es un U+00A0 pegado a la dirección; antes de 2026-11-06 se leía como parte de
// ella y no se avisaba a nadie. Visto en rojo con el `\s` de RE2 en hubnotify.
func TestUnaMencionAlFinalDelParrafoAvisa(t *testing.T) {
	texto := textoDeMenciones("Reunión", "<p>Hablar con @ana@acme.test&nbsp;</p><p>mañana a las diez</p>")
	if got := hubnotify.Mencionados(texto); !slices.Equal(got, []string{"ana@acme.test"}) {
		t.Fatalf("la nota menciona %q en %q", got, texto)
	}
}
