// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// Funciones de base de datos (functions/database.ts). Firma común: DFUN(base, campo,
// criterios). La primera fila de `base` son las cabeceras; `campo` elige la columna
// (número 1-based o nombre); un registro casa si cumple TODAS las condiciones de ALGUNA
// fila de `criterios` (Y dentro de la fila, O entre filas).

// headerEq compara dos cabeceras sin distinguir mayúsculas ni espacios extremos.
func headerEq(a Value, b string) bool {
	t, err := CoerceText(a)
	return err == "" && jsUpper(jsTrim(t)) == jsUpper(jsTrim(b))
}

func indiceCabecera(cabeceras []Value, nombre string) int {
	for i, h := range cabeceras {
		if headerEq(h, nombre) {
			return i
		}
	}
	return -1
}

// resolveField resuelve `campo` a un índice de columna 0-based.
func resolveField(cabeceras []Value, campo *Arg) (int, Error) {
	idx := -1
	switch x := campo.Value().(type) {
	case Error:
		return 0, x
	case float64:
		if i := math.Trunc(x); i >= 1 && i <= float64(len(cabeceras)) {
			idx = int(i) - 1
		}
	default:
		t, _ := CoerceText(x)
		idx = indiceCabecera(cabeceras, t)
	}
	if idx < 0 {
		return 0, ErrValue
	}
	return idx, ""
}

// casaFila: ¿el registro cumple todas las condiciones no vacías de esta fila de criterios?
func casaFila(registro, condiciones []Value, colMap []int) (bool, Error) {
	for c, destino := range colMap {
		celda := Value("")
		if c < len(condiciones) {
			celda = condiciones[c]
		}
		if celda == "" {
			continue // condición vacía: se ignora
		}
		if destino < 0 {
			return false, "" // cabecera de criterio sin columna
		}
		crit, err := parseCriterion(celda)
		if err != "" {
			return false, err
		}
		valor := Value("")
		if destino < len(registro) {
			valor = registro[destino]
		}
		if !matches(valor, crit) {
			return false, ""
		}
	}
	return true, ""
}

// collect reúne los valores de la columna `campo` de los registros que casan.
func collect(args []*Arg) ([]Value, Error) {
	if len(args) != 3 {
		return nil, ErrValue
	}
	db := args[0].Matrix()
	if len(db) < 1 {
		return nil, ErrValue
	}
	campo, err := resolveField(db[0], args[1])
	if err != "" {
		return nil, err
	}
	crit := args[2].Matrix()
	cabeceras := fila0(crit)
	colMap := make([]int, len(cabeceras))
	for i, h := range cabeceras {
		colMap[i] = -1
		if t, err := CoerceText(h); err == "" {
			colMap[i] = indiceCabecera(db[0], t)
		}
	}
	vals := []Value{}
	for _, registro := range db[1:] {
		for _, condiciones := range crit[min(1, len(crit)):] {
			casa, err := casaFila(registro, condiciones, colMap)
			if err != "" {
				return nil, err
			}
			if casa {
				vals = append(vals, en(Matrix{registro}, 0, campo, ""))
				break
			}
		}
	}
	return vals, ""
}

// conNumeros envuelve las que solo miran los números de la columna (propaga #error).
func conNumeros(f func(ns []float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		vals, err := collect(args)
		if err != "" {
			return err
		}
		ns := []float64{}
		for _, v := range vals {
			switch x := v.(type) {
			case Error:
				return x
			case float64:
				ns = append(ns, x)
			}
		}
		return f(ns)
	}
}

func sumar(ns []float64) float64 {
	s := 0.0
	for _, v := range ns {
		s += v
	}
	return s
}

// dispersion es la varianza o la desviación, muestral o poblacional. La muestral necesita
// AL MENOS DOS registros (con uno, n−1 es cero) y si no da #DIV/0!; la poblacional se
// conforma con uno. Esa asimetría es real y es lo que distingue a las dos familias.
func dispersion(muestral, raiz bool) fn {
	return conNumeros(func(ns []float64) Value {
		n := float64(len(ns))
		if n == 0 || (muestral && n < 2) {
			return ErrDiv0
		}
		media, sq := sumar(ns)/n, 0.0
		for _, v := range ns {
			sq += (v - media) * (v - media)
		}
		if muestral {
			n--
		}
		if raiz {
			return math.Sqrt(sq / n)
		}
		return sq / n
	})
}

// extremo0 es DMAX y DMIN: sin números, 0.
func extremo0(mejor func(a, b float64) float64) fn {
	return conNumeros(func(ns []float64) Value {
		if len(ns) == 0 {
			return 0.0
		}
		r := ns[0]
		for _, v := range ns[1:] {
			r = mejor(r, v)
		}
		return r
	})
}

func init() {
	registrar(map[string]fn{
		"DSUM": conNumeros(func(ns []float64) Value { return sumar(ns) }),
		"DPRODUCT": conNumeros(func(ns []float64) Value {
			p := 1.0
			for _, v := range ns {
				p *= v
			}
			return p
		}),
		"DAVERAGE": conNumeros(func(ns []float64) Value {
			if len(ns) == 0 {
				return ErrDiv0
			}
			return sumar(ns) / float64(len(ns))
		}),
		"DMAX":   extremo0(math.Max),
		"DMIN":   extremo0(math.Min),
		"DCOUNT": conNumeros(func(ns []float64) Value { return float64(len(ns)) }),
		"DCOUNTA": func(_ *FnCtx, args []*Arg) any {
			vals, err := collect(args)
			if err != "" {
				return err
			}
			n := 0.0
			for _, v := range vals {
				if v != "" {
					n++
				}
			}
			return n
		},
		// Ninguno → #VALUE!; más de uno → #NUM!.
		"DGET": func(_ *FnCtx, args []*Arg) any {
			vals, err := collect(args)
			switch {
			case err != "":
				return err
			case len(vals) == 0:
				return ErrValue
			case len(vals) > 1:
				return ErrNum
			}
			return vals[0]
		},
		// Como STDEV y VAR: sin P es MUESTRAL (n−1) y con P, POBLACIONAL (n).
		"DSTDEV":  dispersion(true, true),
		"DSTDEVP": dispersion(false, true),
		"DVAR":    dispersion(true, false),
		"DVARP":   dispersion(false, false),
	})
}
