// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// MaxCeldasBloque es el tope de celdas de un bloque (`MAX_CELDAS_BLOQUE`).
const MaxCeldasBloque = 5000

// bloqueHoja es el contenido de un bloque reutilizable de hoja, tal y como lo da el servicio.
type bloqueHoja struct {
	Cells   map[string]any          `json:"cells"`
	Formats map[string]formatoCrudo `json:"formats"`
	Styles  map[string]estiloCrudo  `json:"styles"`
	Charts  []json.RawMessage       `json:"charts"`
}

// extension es dónde cae cada referencia del bloque y el rectángulo que ocupan todas,
// para desplazarlo entero. false si alguna clave no es UNA celda.
func (b bloqueHoja) extension() (map[string]Celda, formula.Rect, bool) {
	caja := formula.Rect{C1: 1 << 30, R1: 1 << 30, C2: -1, R2: -1}
	pos := map[string]Celda{}
	ver := func(ref string) bool {
		r, ok := formula.ParseRangeRef(ref)
		if !ok || r.C1 != r.C2 || r.R1 != r.R2 {
			return false
		}
		pos[ref] = Celda{C: r.C1, F: r.R1}
		caja = formula.Rect{C1: min(caja.C1, r.C1), R1: min(caja.R1, r.R1), C2: max(caja.C2, r.C1), R2: max(caja.R2, r.R1)}
		return true
	}
	for ref := range b.Cells {
		if !ver(ref) {
			return nil, caja, false
		}
	}
	for ref := range b.Formats {
		if !ver(ref) {
			return nil, caja, false
		}
	}
	for ref := range b.Styles {
		if !ver(ref) {
			return nil, caja, false
		}
	}
	return pos, caja, true
}

// graficosDesplazados son los gráficos del bloque con su rango movido (dc, df), con
// identificador nuevo y en cascada tras los `ya` que tiene la hoja, como «Insertar gráfico».
func (b bloqueHoja) graficosDesplazados(ya, dc, df int) []Grafico {
	var out []Grafico
	for _, cr := range b.Charts {
		var g graficoCrudo
		if json.Unmarshal(cr, &g) != nil {
			continue
		}
		rango, _ := g.Range.(string)
		r, ok := formula.ParseRangeRef(rango)
		if !ok {
			continue
		}
		n := ya + len(out)
		g.ID = nuevoID()
		g.Range = Ref(r.C1+dc, r.R1+df) + ":" + Ref(r.C2+dc, r.R2+df)
		g.X, g.Y = float64(96+(n%8)*28), float64(48+(n%8)*28)
		if spec, ok := g.sanear(); ok {
			out = append(out, spec)
		}
	}
	return out
}

// ColocarBloque es `colocarBloque` de `sheets/bloque.ts`: coloca un bloque reutilizable
// —celdas, formatos, estilos y gráficos— con su esquina en (c, f) de la hoja activa. Las
// fórmulas se desplazan como al pegar y los rangos de los gráficos, con ellas. false, sin
// tocar nada, si el contenido no es un bloque, no tiene celdas, pasa del tope o no cabe.
func (l *Libro) ColocarBloque(crudo string, c, f int) bool {
	var b bloqueHoja
	if json.Unmarshal([]byte(crudo), &b) != nil || len(b.Cells) == 0 ||
		len(b.Cells)+len(b.Formats)+len(b.Styles) > MaxCeldasBloque {
		return false
	}
	pos, caja, ok := b.extension()
	dc, df := c-caja.C1, f-caja.R1
	if !ok || caja.C2+dc >= Columnas || caja.R2+df >= Filas {
		return false
	}
	en := func(ref string) Celda { return Celda{C: pos[ref].C + dc, F: pos[ref].F + df} }

	l.anotar()
	h := l.h()
	for ref, v := range b.Cells {
		if html, ok := v.(string); ok && html != "" {
			h.celdas[en(ref)] = desplazarCelda(html, dc, df)
		}
	}
	for ref, cr := range b.Formats {
		if fm, ok := cr.sanear(); ok {
			h.formatos[en(ref)] = fm
		}
	}
	for ref, cr := range b.Styles {
		if e, ok := cr.sanear(); ok {
			h.estilos[en(ref)] = e
		}
	}
	// Lista nueva y no `append` sobre la de la hoja: las fotos del deshacer la comparten.
	h.graficos = append(slices.Clone(h.graficos), b.graficosDesplazados(len(h.graficos), dc, df)...)
	l.restaurar(l.f) // el motor, de nuevo, con las celdas del bloque
	return true
}
