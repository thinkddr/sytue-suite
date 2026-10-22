// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// Rangos editables por persona o grupo dentro de una hoja protegida (ROADMAP-OFFICE
// permisos-2). El modelo los guarda dentro de `protect`:
//
//	"protect": {"ranges": [{"ref": "B2:D10", "title": "Ventas", "users": ["<sub>"], "groups": ["<grupo>"]}]}
//
// Como viven en `protect`, cambiarlos es cambiar la protección: solo el propietario o
// quien trae la contraseña (transicion en proteccion.go). Aquí solo se decide si las
// celdas de un rango se consideran DESBLOQUEADAS para quien escribe.
//
// Deny-by-default: un rango ilegible, sin referencia válida o sin nadie concedido no
// desbloquea nada. Las personas se guardan por su `sub` (la identidad que el servicio
// ya resuelve), no por correo.

// rangoEditable es un `ranges[i]` ya leído.
type rangoEditable struct {
	c1, f1, c2, f2 int // columnas y filas, 1-based; 0 en f* = columna entera, 0 en c* = fila entera
}

// rangosEditables son los rangos de una protección que aplican a una persona concreta.
type rangosEditables []rangoEditable

// rangosDe devuelve los rangos de `protect` concedidos a `quien` (por su sub o por uno
// de sus grupos). Sin sub no hay concesión posible.
func rangosDe(protect map[string]json.RawMessage, quien Subject) rangosEditables {
	if quien.Sub == "" {
		return nil
	}
	var lista []struct {
		Ref    string   `json:"ref"`
		Users  []string `json:"users"`
		Groups []string `json:"groups"`
	}
	if json.Unmarshal(protect["ranges"], &lista) != nil {
		return nil
	}
	var out rangosEditables
	for _, r := range lista {
		concedido := slices.Contains(r.Users, quien.Sub)
		for _, g := range r.Groups {
			concedido = concedido || slices.Contains(quien.Groups, g)
		}
		if !concedido {
			continue
		}
		if rg, ok := leerRangoEditable(r.Ref); ok {
			out = append(out, rg)
		}
	}
	return out
}

// contiene dice si la celda «A1» cae en alguno de los rangos.
func (rs rangosEditables) contiene(ref string) bool {
	if len(rs) == 0 {
		return false
	}
	col, fila, ok := leerCelda(ref)
	if !ok {
		return false
	}
	for _, r := range rs {
		if (r.c1 == 0 || (col >= r.c1 && col <= r.c2)) && (r.f1 == 0 || (fila >= r.f1 && fila <= r.f2)) {
			return true
		}
	}
	return false
}

// leerRangoEditable acepta «B2», «B2:D10», columnas enteras «B:D» y filas enteras «3:5».
func leerRangoEditable(ref string) (rangoEditable, bool) {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	a, b, par := strings.Cut(ref, ":")
	if !par {
		b = a
	}
	if c1, f1, ok := leerCelda(a); ok {
		c2, f2, ok2 := leerCelda(b)
		if !ok2 {
			return rangoEditable{}, false
		}
		return rangoEditable{min(c1, c2), min(f1, f2), max(c1, c2), max(f1, f2)}, true
	}
	if c1, ok := leerColumna(a); ok { // «B:D»
		if c2, ok2 := leerColumna(b); ok2 {
			return rangoEditable{c1: min(c1, c2), c2: max(c1, c2)}, true
		}
		return rangoEditable{}, false
	}
	f1, e1 := strconv.Atoi(a) // «3:5»
	f2, e2 := strconv.Atoi(b)
	if e1 != nil || e2 != nil || f1 < 1 || f2 < 1 || f1 > maxFila || f2 > maxFila {
		return rangoEditable{}, false
	}
	return rangoEditable{f1: min(f1, f2), f2: max(f1, f2)}, true
}

const (
	maxColumna = 16384   // XFD, el tope de Excel
	maxFila    = 1048576 // idem
)

// leerColumna: «A» → 1, «AA» → 27.
func leerColumna(s string) (int, bool) {
	if s == "" || len(s) > 3 {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return 0, false
		}
		n = n*26 + int(r-'A') + 1
	}
	return n, n <= maxColumna
}

// leerCelda: «B12» → (2, 12).
func leerCelda(ref string) (col, fila int, ok bool) {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		i++
	}
	c, okc := leerColumna(ref[:i])
	f, err := strconv.Atoi(ref[i:])
	if !okc || err != nil || f < 1 || f > maxFila || strings.ContainsAny(ref[i:], "+-") {
		return 0, 0, false
	}
	return c, f, true
}
