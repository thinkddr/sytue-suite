// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElNombreDelSaludoNoSeParte: en un teléfono el saludo de la portada («Buenas
// tardes, Maximiliano») no cabe en una línea, y el nombre tiene que pasar ENTERO a la
// de abajo, nunca partido con guion como hacía la web con su `hyphens: auto` (lo vio
// el dueño, 2026-11-26). El título se reparte por palabras (`ui.Repartir`): esto
// vigila que siga así a los anchos de los teléfonos.
func TestElNombreDelSaludoNoSeParte(t *testing.T) {
	for _, nombre := range []string{"Maximiliano", "Anunciación", "Bartholomew"} {
		c := nuevaCabecera(tema.Claro, "jueves, 26 de noviembre", "Buenas tardes, "+nombre, "")
		for _, ancho := range []float32{272, 312, 343} {
			lineas := c.lineasTitulo(ancho)
			entero := false
			for _, l := range lineas {
				if strings.HasSuffix(l, "-") {
					t.Errorf("a %v, «%s» se parte con guion: %q", ancho, nombre, lineas)
				}
				for _, palabra := range strings.Fields(l) {
					if palabra == nombre {
						entero = true
					}
				}
			}
			if !entero {
				t.Errorf("a %v, «%s» no está entero en ninguna línea: %q", ancho, nombre, lineas)
			}
		}
	}
}
