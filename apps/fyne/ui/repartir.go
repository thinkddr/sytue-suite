// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// RepartirEnLineas es el `flex-wrap` del navegador: reparte en líneas los índices de
// unos elementos de `anchos` dados para que cada línea quepa en `cabe`. Un elemento
// entra en la línea si cabe con su hueco por delante; si no, abre otra. El primero de
// cada línea va siempre, aunque no quepa ni él solo: el navegador tampoco lo parte.
//
// `hueco(i)` es el hueco que va DELANTE del elemento i cuando no abre línea; nil es
// sin hueco. Es una función y no un número porque la barra de lote cuenta su `flex-1`
// como un elemento más, y ese hueco es doble.
//
// Antes eran dos copias del mismo bucle —las acciones de `CabeceraApp` y la barra de
// lote— y el alta del Hub iba a ser la tercera; ahora las tres llaman aquí.
func RepartirEnLineas(anchos []float32, cabe float32, hueco func(i int) float32) [][]int {
	var lineas [][]int
	linea := make([]int, 0, len(anchos))
	usado := float32(0)
	for i, w := range anchos {
		h := float32(0)
		if hueco != nil {
			h = hueco(i)
		}
		if len(linea) > 0 && usado+h+w > cabe {
			lineas, linea, usado = append(lineas, linea), make([]int, 0, len(anchos)-i), 0
		} else if len(linea) > 0 {
			usado += h
		}
		linea, usado = append(linea, i), usado+w
	}
	if len(linea) > 0 {
		lineas = append(lineas, linea)
	}
	return lineas
}
