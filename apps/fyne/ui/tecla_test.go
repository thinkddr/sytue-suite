// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// teclaDeRooms es la pastilla de «Ir a un canal…»: `text-2xs border-sidebar-fg-muted/40
// rounded border px-1`.
func teclaDeRooms() *Tecla {
	k := NuevaTecla("Ctrl+G", tema.Texto2XS, Normal)
	k.Tinta, k.Borde, k.OpacidadBorde = "sidebar-fg-muted", "sidebar-fg-muted", 0.4
	k.RellenoX = 1 * 0.25 * tema.Rem
	return k
}

// TestLaTeclaMideLoQuePinta: la medida declarada, la que pide a Fyne y la PINTADA son
// la misma caja. Es la regla 2 del paquete.
//
// Se mide la geometría de Rooms con tokens que se VEAN sobre el papel blanco del
// lienzo de prueba (la de verdad es clara sobre el carril oscuro).
//
// Este guardián SE HA VISTO EN ROJO por un fallo real: la pastilla nacía con la caja
// transparente y la pintaba después, y `Caja` decide al nacer si lleva filete. Salía
// sin borde, y lo pintado eran solo las letras: 29 × 7,5 donde la caja son 38,3 × 16.
func TestLaTeclaMideLoQuePinta(t *testing.T) {
	k := teclaDeRooms()
	k.Tinta, k.Borde, k.OpacidadBorde = "text", "border-strong", 0
	declarada := MedidaTecla(k)
	if m := k.MinSize(); m != declarada {
		t.Errorf("MinSize = %v y MedidaTecla = %v: tienen que ser la misma cuenta", m, declarada)
	}
	// El texto con `px-1` y el borde por dentro: sin relleno vertical, el alto es la
	// caja de línea de `text-2xs` más los dos filetes.
	if quiere := tema.Texto2XS.Linea + 2*tema.Borde; declarada.Height != quiere {
		t.Errorf("alto %v, se esperaba %v", declarada.Height, quiere)
	}
	k.Resize(declarada)
	img := pinta(t, declarada.Width+4, declarada.Height+4, k)
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	b := img.Bounds()
	x0, x1, y0, y1 := 9999, -1, 9999, -1
	for y := range b.Dy() {
		for x := range b.Dx() {
			if !cerca(px(img, x, y), blanco) {
				x0, x1, y0, y1 = min(x0, x), max(x1, x), min(y0, y), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		t.Fatal("la tecla no pintó ni un píxel")
	}
	mismaCaja(t, "la tecla", declarada, x0, x1, y0, y1)
}

// TestLaTeclaSinBordeNoLoCuenta: sin token de borde no hay filete, y por tanto la caja
// no crece los dos píxeles. Es lo que separa el `<kbd>` del ERP —solo fondo— del de
// Rooms.
func TestLaTeclaSinBordeNoLoCuenta(t *testing.T) {
	k := NuevaTecla("⌘K", tema.TextoXS, Normal)
	k.Fondo, k.Tinta = "bg-subtle", "text-subtle"
	if quiere := tema.TextoXS.Linea; MedidaTecla(k).Height != quiere {
		t.Errorf("alto %v, se esperaba %v (sin borde no se suman filetes)", MedidaTecla(k).Height, quiere)
	}
	k.Minimo = fyne.NewSize(24, 24)
	if m := MedidaTecla(k); m.Width < 24 || m.Height != 24 {
		t.Errorf("con `min-w-6 h-6` la pastilla mide %v y tenía que medir 24×24", m)
	}
}

// TestLaTablaPintaElObjetoDeLaCelda: con `CeldaObjeto` la celda deja de ser texto y la
// fila cuelga el objeto; sin él, todo sigue siendo texto.
//
// Es el guardián del gancho que pedía el `ponytail:` del ERP: la insignia de estado.
func TestLaTablaPintaElObjetoDeLaCelda(t *testing.T) {
	cols := []ColumnaTabla{{Clave: "ref", Rotulo: "Ref"}, {Clave: "estado", Rotulo: "Estado"}}
	tb := NuevaTabla(cols, func() int { return 2 },
		func(fila, col int) string { return []string{"F-001", "Pagada"}[col] })
	insignia := NuevaInsignia("Pagada", InsigniaExito)
	tb.CeldaObjeto = func(fila, col int) fyne.CanvasObject {
		if col == 1 && fila == 0 {
			return insignia
		}
		return nil
	}
	f := nuevaFilaTabla(tb)
	r := test.WidgetRenderer(f)

	f.Poner(0)
	if !slices.Contains(r.Objects(), fyne.CanvasObject(insignia)) {
		t.Error("la fila con objeto de celda no lo cuelga: la insignia no se pintaría")
	}
	fr, ok := r.(*filaTablaRenderer)
	if !ok {
		t.Fatalf("renderer inesperado: %T", r)
	}
	if fr.textos[1] != "" {
		t.Errorf("la celda con objeto sigue pintando el texto %q: saldrían los dos encima", fr.textos[1])
	}
	if fr.textos[0] != "F-001" {
		t.Errorf("la celda de texto se perdió: %q", fr.textos[0])
	}

	// La fila se RECICLA: al ponerle otro índice, el objeto se va y vuelve el texto.
	f.Poner(1)
	if slices.Contains(r.Objects(), fyne.CanvasObject(insignia)) {
		t.Error("la fila reciclada se quedó con la insignia de la anterior")
	}
	if fr.textos[1] != "Pagada" {
		t.Errorf("sin objeto la celda tiene que volver a su texto, y tiene %q", fr.textos[1])
	}
}
