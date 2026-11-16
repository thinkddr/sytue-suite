// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"cmp"
	"slices"
)

// Las filas y columnas OCULTAS A MANO (`SheetData.hiddenRows` y `hiddenCols`), el puerto de
// `packages/office/src/editor/sheets/ocultas.ts` con sus mismos vectores
// (testdata/ocultas.json). No son el esquema: un grupo contraído oculta su detalle y se
// vuelve a abrir con su botón; esto es «Ocultar filas» y «Mostrar filas» sobre lo que haya
// seleccionado, sin grupo de por medio.
//
// Se guardan como TRAMOS [Start, End] (0-based, ambos dentro) y no fila a fila: seleccionar
// la columna entera y ocultar desde la fila 10 son un millón de filas, y en tramos es uno.

// Tramo son las unidades (filas o columnas) [Start, End], ambos extremos incluidos.
type Tramo struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// normalizar ordena los tramos y junta los que se solapan o se tocan: así una misma
// ocultación tiene una sola forma, que es lo que se compara al fusionar y al guardar.
func normalizar(ts []Tramo) []Tramo {
	slices.SortFunc(ts, func(a, b Tramo) int { return cmp.Or(cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End)) })
	out := []Tramo{}
	for _, t := range ts {
		if n := len(out); n > 0 && t.Start <= out[n-1].End+1 {
			out[n-1].End = max(out[n-1].End, t.End)
			continue
		}
		out = append(out, t)
	}
	return out
}

// SanearTramos es `sanitizeSpans`: los tramos del documento que valen contra `tope`
// unidades (recortados a la rejilla), ordenados y juntados. Vacío = el campo no se guarda.
func SanearTramos(crudo any, tope int) []Tramo {
	lista, _ := crudo.([]any)
	var ts []Tramo
	for _, x := range lista {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		s, ok1 := campoEntero(m, "start")
		e, ok2 := campoEntero(m, "end")
		if ok1 && ok2 {
			ts = append(ts, Tramo{Start: s, End: e})
		}
	}
	return RecortarTramos(ts, tope)
}

// RecortarTramos deja los tramos ya tipados (los de un modelo leído con encoding/json, o los
// de un fichero importado) como los dejaría `sanitizeSpans`: los que caen en la rejilla,
// recortados a ella, ordenados y juntados.
func RecortarTramos(ts []Tramo, tope int) []Tramo {
	var out []Tramo
	for _, t := range ts {
		if t.Start > t.End || t.End < 0 || t.Start >= tope {
			continue
		}
		out = append(out, Tramo{Start: max(0, t.Start), End: min(tope-1, t.End)})
	}
	return normalizar(out)
}

// campoEntero lee un entero de un objeto JSON como `Math.trunc(Number(v))`; una clave
// que falta no vale (Number(undefined) es NaN).
func campoEntero(m map[string]any, k string) (int, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	return entero(v)
}

// TramoOculto dice si la unidad u está en algún tramo.
func TramoOculto(ts []Tramo, u int) bool {
	for _, t := range ts {
		if u >= t.Start && u <= t.End {
			return true
		}
	}
	return false
}

// OcultarTramo es `hideSpan`: añade [a, b] (en cualquier orden, recortado a la rejilla).
// false si así no quedaría ninguna unidad a la vista: una hoja sin una sola fila que
// seleccionar no deja ni volver a mostrarlas, y Excel tampoco lo permite.
func OcultarTramo(ts []Tramo, a, b, tope int) ([]Tramo, bool) {
	s, e := max(0, min(a, b)), min(tope-1, max(a, b))
	if s > e {
		return slices.Clone(ts), true
	}
	out := normalizar(append(slices.Clone(ts), Tramo{Start: s, End: e}))
	// El primer tramo y no «el único»: lo que haya más allá de `tope` (los tramos de la web
	// en la rejilla corta de la app nativa) no deja nada a la vista.
	if out[0].Start == 0 && out[0].End >= tope-1 {
		return slices.Clone(ts), false
	}
	return out, true
}

// MostrarTramo es `showSpan`: vuelve a mostrar lo oculto dentro de [a, b]. Si dentro no hay
// nada oculto, muestra el bloque oculto que toca la selección por arriba o por abajo
// (por la izquierda o la derecha): es como se recupera la fila 1 oculta, que no hay forma
// de seleccionar a ambos lados.
func MostrarTramo(ts []Tramo, a, b int) []Tramo {
	s, e := min(a, b), max(a, b)
	if !slices.ContainsFunc(ts, func(t Tramo) bool { return t.Start <= e && t.End >= s }) {
		for _, t := range ts {
			if t.End == s-1 {
				s = t.Start
			}
			if t.Start == e+1 {
				e = t.End
			}
		}
	}
	out := []Tramo{}
	for _, t := range ts {
		if t.End < s || t.Start > e {
			out = append(out, t)
			continue
		}
		if t.Start < s {
			out = append(out, Tramo{Start: t.Start, End: s - 1})
		}
		if t.End > e {
			out = append(out, Tramo{Start: e + 1, End: t.End})
		}
	}
	return out
}

// MoverTramos es lo que les pasa a los tramos al insertar (`cantidad` > 0) o borrar
// (`cantidad` < 0) unidades desde `desde`: lo de detrás se desplaza, lo borrado desaparece
// y lo que se sale de la rejilla se pierde, como las celdas. Una inserción DENTRO de un
// tramo lo parte y deja lo insertado a la vista.
func MoverTramos(ts []Tramo, desde, cantidad, tope int) []Tramo {
	var out []Tramo
	for _, t := range ts {
		if t.Start < desde {
			out = append(out, Tramo{Start: t.Start, End: min(t.End, desde-1)})
		}
		if cantidad > 0 {
			if t.End >= desde {
				s, e := max(t.Start, desde)+cantidad, min(t.End+cantidad, tope-1)
				if s <= e {
					out = append(out, Tramo{Start: s, End: e})
				}
			}
			continue
		}
		finBorrado := desde - cantidad - 1
		if t.End > finBorrado {
			out = append(out, Tramo{Start: max(t.Start, finBorrado+1) + cantidad, End: t.End + cantidad})
		}
	}
	return normalizar(out)
}

// UnidadesDeTramos son las unidades ocultas por debajo de `limite` (lo que se pinta o lo que
// tiene datos): un tramo hasta el final de la rejilla no se recorre entero.
func UnidadesDeTramos(ts []Tramo, limite int) map[int]bool {
	out := map[int]bool{}
	for _, t := range ts {
		for u := t.Start; u <= t.End && u < limite; u++ {
			out[u] = true
		}
	}
	return out
}
