// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import "testing"

// Los casos de `sheets-bloque.test.ts`: el bloque se coloca con su esquina en el ancla, las
// fórmulas se desplazan, el formato y el estilo viajan (con la marca `tema`), el gráfico
// sigue a sus celdas y deshacer lo quita entero.
func TestColocarBloque(t *testing.T) {
	lz := libroDePrueba(t, `{"active":0,"sheets":[{"name":"H","cells":{"A1":"antes"}}]}`)
	l := lz.Libro
	const bloque = `{"cells":{"B2":"Concepto","C2":"Importe","B3":"Uno","C3":"10","B4":"Total","C4":"=SUM(C3:C3)"},
"formats":{"C3":{"id":"number","decimals":2}},
"styles":{"B2":{"fill":"#1F2937","color":"#FFFFFF","tema":"acento"},"C2":{"tema":"acento"}},
"charts":[{"type":"bar","range":"B2:C3","title":"Ventas"}]}`
	if !l.ColocarBloque(bloque, 3, 5) { // esquina B2 → D6
		t.Fatal("un bloque válido se coloca")
	}
	if got := l.Crudo(3, 5); got != "Concepto" {
		t.Errorf("la esquina va al ancla: D6 = %q", got)
	}
	if got := l.Crudo(4, 7); got != "=SUM(E7:E7)" {
		t.Errorf("la fórmula se desplaza con el bloque: E8 = %q", got)
	}
	if got := l.Texto(4, 7); got != "10" {
		t.Errorf("y calcula: %q", got)
	}
	if e := l.Estilo(3, 5); e.Relleno != "#1F2937" || e.Tema != "acento" {
		t.Errorf("el estilo viaja con su marca de tema: %+v", e)
	}
	// Retocar a mano una celda vestida por la marca le quita la marca de tema, como en la
	// web: «Actualizar a la marca actual» ya no debe pisar ese color.
	l.CambiarEstilo(func(c *estiloCrudo) { c.Relleno = "#FF0000" }, 3, 5, 3, 5)
	if e := l.Estilo(3, 5); e.Relleno != "#FF0000" || e.Tema != "" {
		t.Errorf("tras retocarla a mano: %+v", e)
	}
	l.Deshacer()
	if e := l.Estilo(4, 5); !e.vacio() {
		t.Errorf("una celda con solo la marca no gana estilo: %+v", e)
	}
	if fm := l.FormatoNumero(4, 6); fm.ID != "number" {
		t.Errorf("el formato de número viaja: %+v", fm)
	}
	gs := l.h().graficos
	if len(gs) != 1 || gs[0].Range != "D6:E7" || gs[0].ID == "" {
		t.Errorf("el gráfico sigue a sus celdas: %+v", gs)
	}
	if got := l.Crudo(0, 0); got != "antes" {
		t.Errorf("lo que había fuera del bloque sigue: %q", got)
	}
	if !l.Deshacer() || l.Crudo(3, 5) != "" || len(l.h().graficos) != 0 {
		t.Error("deshacer quita el bloque entero")
	}
	// Lo que no es un bloque no toca nada.
	for _, malo := range []string{"", "no es json", `{"cells":{}}`, `{"cells":{"A1:B2":"x"}}`} {
		if l.ColocarBloque(malo, 0, 0) {
			t.Errorf("%q no es un bloque y se colocó", malo)
		}
	}
	if l.ColocarBloque(`{"cells":{"A1":"x","B1":"y"}}`, Columnas-1, 0) {
		t.Error("un bloque que no cabe en la rejilla no se coloca")
	}
}
