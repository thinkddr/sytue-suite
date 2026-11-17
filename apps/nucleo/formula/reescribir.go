// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// # Insertar y borrar filas y columnas: `sheets/refshift.ts`
//
// Cuando se inserta o se borra una fila, las fórmulas tienen que enterarse: sin esto,
// `=SUMA(A1:A5)` seguiría sumando cinco filas cuando ya son seis, sin error ni aviso.
// Las reglas son las de Excel, y es la web la que las fija:
//
//   - lo de ANTES del corte no se mueve; lo del corte en adelante, sí;
//   - una celda borrada pasa a `#¡REF!` —mentir con un cero sería peor—;
//   - un rango al que le borran un trozo se encoge, y si se lo borran entero, `#¡REF!`;
//   - los `$` NO eximen: anclan al COPIAR, pero aquí el dato se ha movido de verdad.
//
// No es ShiftRefs (`desplazar.go`), que es la otra clase de desplazamiento —copiar y
// autorrellenar—, donde el `$` sí exime. Son dos reglas para dos cosas distintas.

// RefRota es lo que queda de una referencia que ha dejado de existir.
const RefRota = "#¡REF!"

// Desplazamiento es un tramo que se abre (Cantidad > 0) o se cierra (< 0) en un eje, a
// partir de Desde (base cero). Tope es el tamaño de la rejilla en ese eje: lo que
// rebosa deja de existir.
//
// Hoja es la hoja en la que se abre o se cierra el tramo (`hoja` en la web): con ella,
// una fórmula solo mueve las referencias que apuntan a ESA hoja —las suyas sin prefijo
// si vive en ella y las `Hoja!A1` que la nombran desde cualquier otra—. Vacía, se mueven
// todas (quien no sabe de libros).
type Desplazamiento struct {
	Filas    bool // el eje: filas o columnas
	Desde    int
	Cantidad int
	Tope     int
	Hoja     string
}

// Cómo trata `mover` una posición que cae en lo borrado: muere (una celda suelta), o se
// pega al borde de lo que queda —el inicio de un rango a la primera que sigue al corte,
// el fin a la última que lo precede—: así se encoge el rango en vez de romperse, y
// `A1:A5` sin las filas 3-5 es `A1:A2`, no `A1:A3`, que ya sería un dato ajeno.
type dentro int

const (
	muere dentro = iota
	sujeta
	sujetaFin
)

// mover es el índice nuevo, o false si deja de existir.
func (d Desplazamiento) mover(i int, que dentro) (int, bool) {
	if d.Cantidad > 0 {
		if i < d.Desde {
			return i, true
		}
		nuevo := i + d.Cantidad
		return nuevo, nuevo < d.Tope
	}
	n := -d.Cantidad
	switch {
	case i < d.Desde:
		return i, true
	case i < d.Desde+n:
		if que == sujetaFin {
			return d.Desde - 1, true
		}
		return d.Desde, que == sujeta
	}
	return i - n, true
}

var reExtremo = regexp.MustCompile(`^(\$?)([A-Za-z]*)(\$?)(\d*)$`)

type extremoTexto struct{ colDolar, col, filaDolar, fila string }

func partirExtremo(s string) (extremoTexto, bool) {
	m := reExtremo.FindStringSubmatch(s)
	if len(m) < 5 || (m[2] == "" && m[4] == "") {
		return extremoTexto{}, false
	}
	return extremoTexto{m[1], m[2], m[3], m[4]}, true
}

// moverExtremo desplaza un extremo; false si desaparece. Un extremo sin fila («A») o sin
// columna («5») no se toca en el eje que no tiene.
func (d Desplazamiento) moverExtremo(e extremoTexto, que dentro) (extremoTexto, bool) {
	if d.Filas {
		if e.fila == "" {
			return e, true
		}
		f, _ := strconv.Atoi(e.fila)
		i, ok := d.mover(f-1, que)
		e.fila = strconv.Itoa(i + 1)
		return e, ok
	}
	if e.col == "" {
		return e, true
	}
	i, ok := d.mover(ColToNum(e.col), que)
	if !ok {
		return e, false
	}
	letras := NumToCol(i)
	// Quien escribió la fórmula en minúsculas las conserva: pasarla a mayúsculas es un
	// cambio que nadie ha pedido.
	if e.col == strings.ToLower(e.col) {
		letras = strings.ToLower(letras)
	}
	e.col = letras
	return e, true
}

func (e extremoTexto) String() string { return e.colDolar + e.col + e.filaDolar + e.fila }

// MoverReferencia es `moverReferencia`: una referencia suelta (`A1`, `$B$4:C9`, `A:A`,
// con su hoja si la lleva) tras el desplazamiento, o RefRota si deja de existir.
func (d Desplazamiento) MoverReferencia(ref string) string {
	hoja, a1 := "", ref
	if i := strings.LastIndexByte(ref, '!'); i >= 0 {
		hoja, a1 = ref[:i+1], ref[i+1:]
	}
	partes := strings.Split(a1, ":")
	switch len(partes) {
	case 1:
		e, ok := partirExtremo(partes[0])
		if !ok {
			return ref
		}
		m, ok := d.moverExtremo(e, muere)
		if !ok {
			return RefRota
		}
		return hoja + m.String()
	case 2:
	default:
		return ref
	}
	a, okA := partirExtremo(partes[0])
	b, okB := partirExtremo(partes[1])
	if !okA || !okB {
		return ref
	}
	// Un rango solo muere si se lleva por delante sus DOS extremos.
	if _, vivo := d.moverExtremo(a, muere); !vivo {
		if _, vivo := d.moverExtremo(b, muere); !vivo {
			return RefRota
		}
	}
	// Cuál es el fin en el eje que se mueve: `A5:A1` también se encoge por el lado bueno.
	pos := func(e extremoTexto) int {
		if d.Filas {
			f, _ := strconv.Atoi(e.fila)
			return f
		}
		if e.col == "" {
			return 0
		}
		return ColToNum(e.col)
	}
	qa, qb := sujeta, sujetaFin
	if pos(a) > pos(b) {
		qa, qb = sujetaFin, sujeta
	}
	na, okA := d.moverExtremo(a, qa)
	nb, okB := d.moverExtremo(b, qb)
	if !okA || !okB {
		return RefRota
	}
	return hoja + na.String() + ":" + nb.String()
}

// Extremos son las posiciones (base cero) que un desplazamiento en ese eje mueve de la
// referencia `ref`: las mismas que lee MoverReferencia, en el mismo orden. Un extremo
// sin ese eje (la columna entera «A», la fila entera «5») no aporta ninguna, y una
// referencia que MoverReferencia no sabe leer —y deja tal cual—, tampoco. La protección
// del servidor las usa para saber qué desplazamientos dan el mismo resultado.
func Extremos(ref string, filas bool) []int {
	if i := strings.LastIndexByte(ref, '!'); i >= 0 {
		ref = ref[i+1:]
	}
	partes := strings.Split(ref, ":")
	if len(partes) > 2 {
		return nil
	}
	out := make([]int, 0, len(partes))
	for _, p := range partes {
		e, ok := partirExtremo(p)
		if !ok {
			return nil
		}
		switch {
		case filas && e.fila != "":
			f, _ := strconv.Atoi(e.fila)
			out = append(out, f-1)
		case !filas && e.col != "":
			out = append(out, ColToNum(e.col))
		}
	}
	return out
}

// ReescribirFormula es `reescribirFormula` para una fórmula que vive en la hoja del
// desplazamiento: sus referencias sin prefijo son de ella.
func (d Desplazamiento) ReescribirFormula(texto string) string {
	return d.ReescribirFormulaEn(texto, d.Hoja)
}

// ReescribirFormulaEn es `reescribirFormula` con su `base`: las referencias de una
// fórmula que vive en la hoja `base` y apuntan a la del desplazamiento (todas si
// d.Hoja está vacía), de derecha a izquierda para que las posiciones sigan valiendo
// mientras se sustituye. Como en Excel, el nombre de hoja no distingue mayúsculas.
func (d Desplazamiento) ReescribirFormulaEn(texto, base string) string {
	if !strings.HasPrefix(texto, "=") {
		return texto
	}
	refs := ExtractRefs(texto)
	slices.SortFunc(refs, func(x, y Referencia) int { return y.Inicio - x.Inicio })
	out := texto
	for _, r := range refs {
		// Una referencia a otro libro apunta a SUS filas, que aquí no se mueven nunca.
		if EsDeOtroLibro(r.Hoja) {
			continue
		}
		de := r.Hoja
		if de == "" {
			de = base
		}
		if d.Hoja != "" && !strings.EqualFold(de, d.Hoja) {
			continue
		}
		if nuevo := d.MoverReferencia(r.Texto); nuevo != r.Texto {
			out = out[:r.Inicio] + nuevo + out[r.Fin:]
		}
	}
	return out
}
