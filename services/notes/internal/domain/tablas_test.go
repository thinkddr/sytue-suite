// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// Una tabla escrita en una nota tiene que sobrevivir al saneado y salir legible
// en texto plano: antes el saneador tiraba la rejilla y las celdas quedaban como
// una frase corrida.
func TestTablaSobreviveAlSaneado(t *testing.T) {
	in := `<table><thead><tr><th>Mes</th><th>Gasto</th></tr></thead>` +
		`<tbody><tr><td>Enero</td><td>120</td></tr></tbody></table>`
	got := domain.SanitizeHTML(in)
	for _, etiqueta := range []string{"<table>", "<thead>", "<tbody>", "<tr>", "<th>", "<td>"} {
		if !strings.Contains(got, etiqueta) {
			t.Errorf("falta %s en el saneado: %s", etiqueta, got)
		}
	}
}

func TestTablaEnTextoPlanoSeparaCeldasYFilas(t *testing.T) {
	in := `<table><tr><td>Enero</td><td>120</td></tr><tr><td>Febrero</td><td>90</td></tr></table>`
	got := domain.TextoDeHTML(in)
	if !strings.Contains(got, "Enero\t120") {
		t.Errorf("las celdas no se separan con tabulador: %q", got)
	}
	if !strings.Contains(got, "\n") || !strings.Contains(got, "Febrero") {
		t.Errorf("las filas no cortan línea: %q", got)
	}
}

// Lo peligroso sigue fuera aunque venga dentro de una tabla.
func TestTablaNoAbreLaPuertaAScript(t *testing.T) {
	got := domain.SanitizeHTML(`<table><tr><td><script>alert(1)</script>ok</td></tr></table>`)
	if strings.Contains(got, "script") || strings.Contains(got, "alert") {
		t.Errorf("el script sobrevive: %s", got)
	}
	if !strings.Contains(got, "ok") {
		t.Errorf("se perdió el texto de la celda: %s", got)
	}
}
