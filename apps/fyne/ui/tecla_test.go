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
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TestLaTeclaMideLoQuePinta: la medida declarada, la que pide a Fyne y la PINTADA son
// la misma caja. Es la regla 2 del paquete.
//
// Se mide la tecla del papel, que sobre el lienzo blanco de prueba se ve entera. Con un
// texto largo, para que mande el texto y no el `min-w-5`.
//
// Este guardián SE HA VISTO EN ROJO por un fallo real: la pastilla nacía con la caja
// transparente y la pintaba después, y `Caja` decide al nacer si lleva filete. Salía
// sin borde, y lo pintado eran solo las letras.
func TestLaTeclaMideLoQuePinta(t *testing.T) {
	k := TeclaSuperficie("Ctrl+Mayús+G")
	k.Fondo, k.Borde = "bg-subtle", "border-strong" // un filete que se distinga del blanco
	declarada := MedidaTecla(k)
	if m := k.MinSize(); m != declarada {
		t.Errorf("MinSize = %v y MedidaTecla = %v: tienen que ser la misma cuenta", m, declarada)
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

// TestLasDosTeclasSonLaKbdDeLaWeb: los dos presets son la `Kbd` de `@sytue/ui` —`h-5
// min-w-5 px-1.5 rounded border text-xs`— y solo cambian de colores. Una letra suelta
// sale cuadrada (17,5 × 17,5) y una palabra crece a lo ancho con su `px-1.5` y el
// filete, sin crecer en alto.
//
// VISTO EN ROJO: con las recetas de antes, la de Rooms medía 16 de alto (`text-2xs`,
// `px-1`) y la de la ventana de atajos 21 (`h-6`).
func TestLasDosTeclasSonLaKbdDeLaWeb(t *testing.T) {
	lado := float32(5 * 0.25 * tema.Rem)
	for _, c := range []struct {
		que                 string
		k                   *Tecla
		nueva               func(string) *Tecla
		fondo, tinta, borde string
	}{
		{"del papel", nil, TeclaSuperficie, "bg-subtle", "text-muted", "border"},
		{"del cromo", nil, TeclaCromo, TokenCarril(), "on-chrome-muted", "chrome-border"},
	} {
		c.k = c.nueva("j")
		if m := MedidaTecla(c.k); m != fyne.NewSize(lado, lado) {
			t.Errorf("la tecla %s con una letra mide %v y la web %v×%v", c.que, m, lado, lado)
		}
		if c.k.Fondo != c.fondo || c.k.Tinta != c.tinta || c.k.Borde != c.borde {
			t.Errorf("la tecla %s pinta %q/%q/%q y la web %q/%q/%q", c.que,
				c.k.Fondo, c.k.Tinta, c.k.Borde, c.fondo, c.tinta, c.borde)
		}
		if c.k.Escalon != tema.TextoXS || c.k.Radio != tema.RadioSM {
			t.Errorf("la tecla %s no es `text-xs rounded`", c.que)
		}
		palabra := c.nueva("Ctrl+Mayús+G")
		quiere := Medir(palabra.Texto, tema.TextoXS, Normal).Width + 2*(tema.Espacio(1.5)+tema.Borde)
		if m := MedidaTecla(palabra); m.Width != quiere || m.Height != lado {
			t.Errorf("la tecla %s con una palabra mide %v y la web %v×%v", c.que, m, quiere, lado)
		}
	}
}

// TestLaTeclaSobreUnVeloLoCompone: con `Debajo`, el fondo translúcido se compone sobre
// él. Sin componer, `Caja` deja ver el filete por todo el interior (lo pinta debajo).
func TestLaTeclaSobreUnVeloLoCompone(t *testing.T) {
	k := TeclaCromo("⌘K")
	k.Fondo, k.Debajo = "sidebar-hover-bg", "sidebar-bg"
	relleno, _ := k.colores(tema.Oscuro)
	if relleno.A != 255 {
		t.Errorf("el relleno sobre el velo sale con alfa %d: dejaría ver el filete", relleno.A)
	}
	if quiere := tokens.SobreSRGB(tema.Color("sidebar-hover-bg", tema.Oscuro), tema.Color("sidebar-bg", tema.Oscuro)); relleno != quiere {
		t.Errorf("relleno %v, se esperaba el velo compuesto %v", relleno, quiere)
	}
}

// TestLaTeclaSinBordeNoLoCuenta: sin token de borde no hay filete, y por tanto la caja
// no crece los dos píxeles. Es lo que separa las pastillas que no son teclas (la cuenta
// de módulos del ERP) de las dos teclas.
func TestLaTeclaSinBordeNoLoCuenta(t *testing.T) {
	k := NuevaTecla("⌘K", tema.TextoXS, Normal)
	k.Fondo, k.Tinta = "bg-subtle", "text-subtle"
	if quiere := tema.TextoXS.Linea; MedidaTecla(k).Height != quiere {
		t.Errorf("alto %v, se esperaba %v (sin borde no se suman filetes)", MedidaTecla(k).Height, quiere)
	}
	k.Minimo = fyne.NewSize(24, 24)
	if m := MedidaTecla(k); m.Width < 24 || m.Height != 24 {
		t.Errorf("con un mínimo de 24×24 la pastilla mide %v", m)
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
