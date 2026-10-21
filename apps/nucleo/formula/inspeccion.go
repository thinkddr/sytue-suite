// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"slices"
	"strings"
)

// Uso describe lo que una fórmula usa, SIN evaluarla. Es para quien admite solo una
// parte del lenguaje (los campos de fórmula de Sytue Bases): con esto decide si la
// acepta antes de dársela a un motor.
type Uso struct {
	// Funciones son las que llama, en mayúsculas, ordenadas y sin repetir.
	Funciones []string
	// Referencias cuenta celdas, rangos, nombres y referencias estructuradas.
	Referencias int
	// Matrices cuenta las constantes matriciales ({1,2;3,4}).
	Matrices int
	// Nodos y Profundidad miden el árbol: una cota del coste de evaluarla.
	Nodos, Profundidad int
}

// Inspeccionar parsea una fórmula (con o sin el «=») y dice qué usa. Falla con
// ErrSintaxis si no se entiende.
func Inspeccionar(src string) (Uso, error) {
	ast, err := parseFormula(src)
	if err != nil {
		return Uso{}, err
	}
	var u Uso
	u.recorrer(ast, 1)
	slices.Sort(u.Funciones)
	u.Funciones = slices.Compact(u.Funciones)
	return u, nil
}

func (u *Uso) recorrer(n *nodo, nivel int) {
	if n == nil {
		return
	}
	u.Nodos++
	u.Profundidad = max(u.Profundidad, nivel)
	switch n.clase { //nolint:exhaustive // los literales no cuentan más que como nodo
	case nCall:
		u.Funciones = append(u.Funciones, strings.ToUpper(n.str))
	case nRef, nRange, nName, nStructured:
		u.Referencias++
	case nArray:
		u.Matrices++
	}
	u.recorrer(n.izq, nivel+1)
	u.recorrer(n.der, nivel+1)
	for _, a := range n.args {
		u.recorrer(a, nivel+1)
	}
}
