// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestOrdenadasNoCopiaElCuadernoEnCadaRepintado: con la misma lista, ordenar otra vez no
// pide memoria (era copiar las mil notas, ~0,7 MB por repintado); da lo mismo que
// Ordenar en los cuatro órdenes; y lo que se cambia EN SU SITIO —el título, la posición—
// sale en la salida, que se reescribe entera. VISTO EN ROJO con `Ordenar` en
// `sincronizar` (una copia por llamada).
func TestOrdenadasNoCopiaElCuadernoEnCadaRepintado(t *testing.T) {
	notas := DeNotas(notasDeBanco(1000))
	p := &Pantalla{}
	for _, o := range Ordenes {
		p.orden = o
		if got, want := p.ordenadas(notas), Ordenar(notas, o, p.loc); !slices.EqualFunc(got, want, func(a, b Nota) bool { return a.ID == b.ID }) {
			t.Fatalf("en el orden %s no sale lo mismo que con Ordenar", o)
		}
	}
	p.orden = OrdenManual
	p.ordenadas(notas)
	if n := testing.AllocsPerRun(20, func() { p.ordenadas(notas) }); n != 0 {
		t.Errorf("ordenar otra vez la misma lista pide %v reservas de memoria", n)
	}

	// Un cambio en su sitio: el título se ve y la posición reordena.
	notas[5].Titulo = "Cambiada en su sitio"
	if i := slices.IndexFunc(p.ordenadas(notas), func(n Nota) bool { return n.ID == notas[5].ID }); i < 0 || p.ordenadas(notas)[i].Titulo != "Cambiada en su sitio" {
		t.Error("la salida enseña la nota de antes")
	}
	notas[999].Posicion = -1
	if got := p.ordenadas(notas); got[slices.IndexFunc(got, func(n Nota) bool { return !n.Fijada })].ID != notas[999].ID {
		t.Error("una posición cambiada en su sitio no reordena")
	}

	// En el orden en que ya venían, la entrada tal cual.
	sueltas := DeNotas([]api.Note{{ID: "a", Position: 0}, {ID: "b", Position: 1}})
	if got := p.ordenadas(sueltas); &got[0] != &sueltas[0] {
		t.Error("ya ordenadas, se copian igualmente")
	}
}
