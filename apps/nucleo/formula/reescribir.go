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
type Desplazamiento struct {
	Filas    bool // el eje: filas o columnas
	Desde    int
	Cantidad int
	Tope     int
}

// mover es el índice nuevo, o false si deja de existir. Con `sujeta`, un extremo de
// rango que cae en lo borrado se pega al borde de lo que queda: así se encoge el rango
// en vez de romperse.
func (d Desplazamiento) mover(i int, sujeta bool) (int, bool) {
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
		return d.Desde, sujeta
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
func (d Desplazamiento) moverExtremo(e extremoTexto, sujeta bool) (extremoTexto, bool) {
	if d.Filas {
		if e.fila == "" {
			return e, true
		}
		f, _ := strconv.Atoi(e.fila)
		i, ok := d.mover(f-1, sujeta)
		e.fila = strconv.Itoa(i + 1)
		return e, ok
	}
	if e.col == "" {
		return e, true
	}
	i, ok := d.mover(ColToNum(e.col), sujeta)
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
		m, ok := d.moverExtremo(e, false)
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
	if _, vivo := d.moverExtremo(a, false); !vivo {
		if _, vivo := d.moverExtremo(b, false); !vivo {
			return RefRota
		}
	}
	na, okA := d.moverExtremo(a, true)
	nb, okB := d.moverExtremo(b, true)
	if !okA || !okB {
		return RefRota
	}
	return hoja + na.String() + ":" + nb.String()
}

// ReescribirFormula es `reescribirFormula`: todas las referencias de una fórmula, de
// derecha a izquierda para que las posiciones sigan valiendo mientras se sustituye.
//
// ponytail: como en la web, se reescriben TODAS las referencias de la fórmula, también
// las que apuntan a otra hoja (`Hoja2!A5`), aunque lo que se ha movido es la hoja
// activa; y las fórmulas de las otras hojas que apuntan a esta no se tocan. Se
// arregla en los dos sitios a la vez, o el mismo documento daría dos números.
func (d Desplazamiento) ReescribirFormula(texto string) string {
	if !strings.HasPrefix(texto, "=") {
		return texto
	}
	refs := ExtractRefs(texto)
	slices.SortFunc(refs, func(x, y Referencia) int { return y.Inicio - x.Inicio })
	out := texto
	for _, r := range refs {
		if nuevo := d.MoverReferencia(r.Texto); nuevo != r.Texto {
			out = out[:r.Inicio] + nuevo + out[r.Fin:]
		}
	}
	return out
}
