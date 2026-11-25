// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// # El buscador de una vista
//
// Cada app escribía el suyo y la auditoría del 2026-11-24 los encontró distintos: el de
// Mail a medio ancho en el teléfono, el del ERP sin lupa, el de Cast de 24 px con letra de
// 12 y el de Photos cortando su marca de agua. El de la web es UNO —`<Input>` de Form.tsx
// con `leadingIcon` de lupa, `type="search"` y, en el teléfono, `w-full`—, y este es su
// preajuste: la talla `md` (36, y 44 con el dedo), la lupa de 16 en `left-3`, Escape que
// vacía lo escrito, y a todo el ancho por debajo de `CorteSM`.

// NuevoBuscador crea el buscador de una vista con su marca de agua, YA traducida.
func NuevoBuscador(marca string) *Entrada {
	e := NuevaEntrada(marca)
	e.Icono = iconos.Buscar
	e.Busqueda = true
	return e
}

// AnchoBuscador es el ancho que le toca al buscador `e`: en el teléfono (ventana por
// debajo de `CorteSM`), todo el `disponible`; si no, el `propio` de su app sin pasar del
// disponible. La ventana es la del lienzo del buscador; sin lienzo, cuenta como ancha.
func AnchoBuscador(e fyne.CanvasObject, propio, disponible float32) float32 {
	disponible = max(disponible, 0)
	if c := lienzoDe(e); c != nil && c.Size().Width > 0 && c.Size().Width < CorteSM {
		return disponible
	}
	return min(propio, disponible)
}
