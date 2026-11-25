// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasDescripcionesCompactasNoSeCortanEnElTelefono: en Marco la tesela compacta
// lleva la descripción en UNA línea, y a 390 las dos columnas le dejan unos 90 px:
// «Files and coll…», «Text docume…» (auditoría estética del 2026-11-24). En el
// teléfono va en hasta dos líneas y la fila crece; en escritorio sigue en una, a 62.
//
// VISTO EN ROJO con la tesela de antes: a 390 la de Drive mide 62 y enseña una línea.
func TestLasDescripcionesCompactasNoSeCortanEnElTelefono(t *testing.T) {
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	for _, c := range []struct {
		ancho  float32
		alto   float32
		lineas int
	}{
		{390, AltoTeselaCompactaCon(2), 2},
		{1280, AltoTeselaCompacta, 1},
	} {
		l := lanzadorDePrueba(t)
		_, r := pinta(t, l, c.ancho)
		var drive *TeselaApp
		for _, ts := range r.teselas {
			if ts.Producto.Marca.Clave() == "drive" {
				drive = ts
			}
		}
		if drive == nil {
			t.Fatal("no está la tesela de Drive")
		}
		if !cerca(drive.Size().Height, c.alto) {
			t.Errorf("a %v la tesela de Drive mide %v y tocaban %v", c.ancho, drive.Size().Height, c.alto)
		}
		visibles := 0
		for _, d := range test.WidgetRenderer(drive).(*teselaRenderer).desc {
			if d.Visible() && d.Text != "" {
				visibles++
				if fin := d.Position().Y + EscalonDescripcion.Linea; fin > drive.Size().Height {
					t.Errorf("a %v la descripción acaba en %v, fuera de la tesela de %v", c.ancho, fin, drive.Size().Height)
				}
			}
		}
		if visibles != c.lineas {
			t.Errorf("a %v la descripción de Drive va en %d líneas y tocaban %d", c.ancho, visibles, c.lineas)
		}
	}
}
