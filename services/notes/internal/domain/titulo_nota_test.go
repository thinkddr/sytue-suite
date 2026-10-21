// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"testing"

	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// El nombre del fichero de una nota exportada sola es el de la NOTA: con el del
// cuaderno, «Mudanza.md» para una nota llamada «Presupuesto» no lo reconoce
// nadie en la carpeta de descargas.
func TestTituloDeNota(t *testing.T) {
	if got := domain.TituloDeNota(domain.Note{Title: "Presupuesto"}); got != "Presupuesto" {
		t.Errorf("TituloDeNota = %q; quería «Presupuesto»", got)
	}
	if got := domain.TituloDeNota(domain.Note{Title: "   "}); got != "nota" {
		t.Errorf("una nota sin título se llama %q; quería «nota»", got)
	}
}
