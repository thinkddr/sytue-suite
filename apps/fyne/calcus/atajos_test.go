// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// ctrlMas es la pulsación de un atajo con la tecla de atajos del sistema.
func ctrlMas(r *Rejilla, k fyne.KeyName, shift bool) {
	m := fyne.KeyModifierShortcutDefault
	if shift {
		m |= fyne.KeyModifierShift
	}
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: k, Modifier: m})
}

// TestCtrlDRellenaHaciaAbajo: la primera fila del rango sobre las demás, con las
// referencias desplazadas como al arrastrar el asa; con una sola fila, la de encima; en
// la fila 1, nada. Es `sheets-atajos-hoja.test.tsx`.
func TestCtrlDRellenaHaciaAbajo(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	r.L.Escribir(0, 0, "7")
	r.L.Escribir(1, 0, "=A1*2")
	r.L.Escribir(0, 1, "x")
	r.Seleccionar(0, 0, false)
	r.Seleccionar(1, 2, true)
	ctrlMas(r, fyne.KeyD, false)
	for _, c := range []struct {
		c, f   int
		quiero string
	}{{0, 1, "7"}, {0, 2, "7"}, {1, 1, "=A2*2"}, {1, 2, "=A3*2"}} {
		if got := r.L.Crudo(c.c, c.f); got != c.quiero {
			t.Errorf("Ctrl+D deja %q en (%d,%d), quiero %q", got, c.c, c.f, c.quiero)
		}
	}
	if c1, f1, c2, f2 := r.Rango(); c1 != 0 || f1 != 0 || c2 != 1 || f2 != 2 {
		t.Errorf("Ctrl+D movió la selección a %d,%d–%d,%d", c1, f1, c2, f2)
	}

	r.L.Escribir(3, 0, "arriba")
	r.Seleccionar(3, 1, false)
	ctrlMas(r, fyne.KeyD, false)
	if got := r.L.Crudo(3, 1); got != "arriba" {
		t.Errorf("Ctrl+D en una sola celda copia %q y la de encima es «arriba»", got)
	}
	if r.Seleccion() != (Celda{C: 3, F: 1}) || r.ext != nil {
		t.Error("con una sola celda la selección se amplió a la de encima")
	}
	antes := r.L.version
	r.Seleccionar(4, 0, false)
	ctrlMas(r, fyne.KeyD, false)
	if r.L.version != antes {
		t.Error("Ctrl+D en la fila 1 escribió algo")
	}
}

// TestCtrlRRellenaALaDerecha es lo mismo por columnas.
func TestCtrlRRellenaALaDerecha(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	r.L.Escribir(0, 0, "=B9")
	r.L.Escribir(0, 1, "3")
	r.Seleccionar(0, 0, false)
	r.Seleccionar(2, 1, true)
	ctrlMas(r, fyne.KeyR, false)
	if r.L.Crudo(1, 0) != "=C9" || r.L.Crudo(2, 0) != "=D9" || r.L.Crudo(2, 1) != "3" {
		t.Errorf("Ctrl+R deja %q %q %q", r.L.Crudo(1, 0), r.L.Crudo(2, 0), r.L.Crudo(2, 1))
	}
}

// TestCtrlPuntoYComaEscribeLaFechaDeHoy: el valor (no `=HOY()`), el formato de fecha y
// la negrita que tuviera la celda, en un solo paso de deshacer.
func TestCtrlPuntoYComaEscribeLaFechaDeHoy(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	r.L.Escribir(1, 1, "viejo")
	r.L.NegritaOCursiva(true, 1, 1, 1, 1, 1, 1)
	r.Seleccionar(1, 1, false)
	r.EscribirHoy(time.Date(2026, 11, 13, 23, 59, 0, 0, time.Local))
	if got := r.L.Crudo(1, 1); got != "46339" {
		t.Errorf("Ctrl+; escribe %q y el 13/11/2026 es 46339", got)
	}
	if negrita, _, _ := r.L.Formato(1, 1); !negrita {
		t.Error("Ctrl+; se llevó la negrita de la celda")
	}
	if got := r.L.FormatoNumero(1, 1).ID; got != "date" {
		t.Errorf("Ctrl+; deja el formato %q y no el de fecha", got)
	}
	r.L.Deshacer()
	if got := r.L.Crudo(1, 1); got != "viejo" {
		t.Errorf("deshacer Ctrl+; deja %q: no fue un solo paso", got)
	}
	// Por el atajo, con la fecha de verdad.
	r.Seleccionar(2, 2, false)
	ctrlMas(r, fyne.KeySemicolon, false)
	if r.L.Crudo(2, 2) == "" {
		t.Error("Ctrl+; no escribió nada")
	}
	r.SoloLectura = true
	r.Seleccionar(3, 3, false)
	ctrlMas(r, fyne.KeySemicolon, false)
	ctrlMas(r, fyne.KeyD, false)
	if r.L.Crudo(3, 3) != "" {
		t.Error("en solo lectura Ctrl+; escribió")
	}
}

// TestEspacioConModificadorMarcaFilasYColumnas: Mayús+Espacio no empieza a escribir un
// espacio, marca las filas; Ctrl+Espacio, las columnas.
func TestEspacioConModificadorMarcaFilasYColumnas(t *testing.T) {
	r := rejillaDePrueba(400, 200)
	r.Seleccionar(1, 1, false)
	r.Seleccionar(2, 3, true)
	r.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	r.TypedRune(' ')
	r.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if r.Editando() {
		t.Fatal("Mayús+Espacio abrió la celda")
	}
	if c1, f1, c2, f2 := r.Rango(); c1 != 0 || f1 != 1 || c2 != r.Cols-1 || f2 != 3 {
		t.Errorf("Mayús+Espacio marca %d,%d–%d,%d y no las filas 2 a 4 enteras", c1, f1, c2, f2)
	}
	r.Seleccionar(2, 2, false)
	ctrlMas(r, fyne.KeySpace, false)
	if c1, f1, c2, f2 := r.Rango(); c1 != 2 || f1 != 0 || c2 != 2 || f2 != r.Fils-1 {
		t.Errorf("Ctrl+Espacio marca %d,%d–%d,%d y no la columna C entera", c1, f1, c2, f2)
	}
}

// TestCtrlBarraAbreLaHojaDeAtajos: Ctrl+/ (con Mayúsculas también: en un teclado español
// «/» va con ellas) y el botón del teclado de la barra de hojas.
func TestCtrlBarraAbreLaHojaDeAtajos(t *testing.T) {
	c := lienzoTraducido(t)
	w := test.NewWindow(c)
	defer w.Close()
	w.Resize(fyne.NewSize(560, 300))
	abiertas := 0
	c.Rejilla.AlAtajos = func() { abiertas++ }
	ctrlMas(c.Rejilla, fyne.KeySlash, false)
	ctrlMas(c.Rejilla, fyne.KeySlash, true)
	if abiertas != 2 {
		t.Errorf("Ctrl+/ abrió la hoja %d veces de 2", abiertas)
	}
	if got := c.Pestanas.teclado.Rotulo; got != "Atajos de teclado" {
		t.Errorf("el botón del teclado se llama %q", got)
	}
	c.Pestanas.teclado.Tapped(&fyne.PointEvent{})
	if !strings.Contains(textoDeLaVentana(w.Canvas().Overlays().Top()), "Rellenar hacia abajo") {
		t.Error("el botón del teclado no abre la hoja de atajos")
	}
}

// TestLaHojaDeAtajosEsLaDeLaWeb: los mismos grupos, filas y teclas que
// `gruposDeAtajosHoja` en castellano, sin claves sin traducir.
func TestLaHojaDeAtajosEsLaDeLaWeb(t *testing.T) {
	c := lienzoTraducido(t)
	grupos := GruposDeAtajosHoja(func(k string) string { return c.t(k, nil) })
	titulos := make([]string, 0, len(grupos))
	var teclas []string
	filas := 0
	for _, g := range grupos {
		titulos = append(titulos, g.Titulo)
		for _, a := range g.Atajos {
			filas++
			teclas = append(teclas, a.Teclas...)
			if strings.HasPrefix(a.Descripcion, "sheet.") || strings.HasPrefix(a.Descripcion, "cmd.") {
				t.Errorf("fila sin traducir: %q", a.Descripcion)
			}
		}
	}
	if !slices.Equal(titulos, []string{"Moverse", "Seleccionar", "Editar"}) {
		t.Errorf("grupos %v", titulos)
	}
	if filas != 28 {
		t.Errorf("la hoja tiene %d filas y la de la web 28", filas)
	}
	for _, k := range []string{"Mayús+Espacio", "Ctrl+Mayús+V", "Ctrl+D", "Ctrl+;", "Supr", "Mayús+Tab", "Mayús+F10"} {
		if !slices.Contains(teclas, k) {
			t.Errorf("falta la tecla %q en %v", k, teclas)
		}
	}
}

// textoDeLaVentana son los textos de un árbol ya colocado (el diálogo abierto).
func textoDeLaVentana(o fyne.CanvasObject) string {
	if o == nil {
		return ""
	}
	var b strings.Builder
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *canvas.Text:
			b.WriteString(x.Text + " ")
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				ir(h)
			}
		}
	}
	ir(o)
	return b.String()
}

// TestColumnaEnteraEsControlEspacioEnElMac: la hoja enseña ⌃Espacio en el Mac (⌘+Espacio
// es de Spotlight) y la rejilla atiende Control+Espacio aunque la tecla de atajos sea ⌘.
func TestColumnaEnteraEsControlEspacioEnElMac(t *testing.T) {
	c := lienzoTraducido(t)
	tr := func(k string) string { return c.t(k, nil) }
	if got := teclaDeHojaPara("darwin", "Control+{space}", tr); got != "⌃Espacio" {
		t.Errorf("en el Mac la columna entera se enseña %q", got)
	}
	if got := teclaDeHojaPara("linux", "Control+{space}", tr); got != "Ctrl+Espacio" {
		t.Errorf("en Linux la columna entera se enseña %q", got)
	}
	r := rejillaDePrueba(400, 200)
	r.Seleccionar(1, 1, false)
	r.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeySpace, Modifier: fyne.KeyModifierControl})
	if c1, f1, c2, f2 := r.Rango(); c1 != 1 || f1 != 0 || c2 != 1 || f2 != r.Fils-1 {
		t.Errorf("Control+Espacio marca %d,%d–%d,%d y no la columna B entera", c1, f1, c2, f2)
	}
}

// TestDentroDelEditorLaHojaDeAtajosEsUna: con el marco, Ctrl+/ (con Mayúsculas o sin
// ellas) y el botón del teclado abren la hoja del editor entero, la de Vista, y no una
// segunda solo de la rejilla; esa hoja acaba con los de la rejilla sin repetir ninguno.
func TestDentroDelEditorLaHojaDeAtajosEsUna(t *testing.T) {
	c := lienzoTraducido(t)
	w := test.NewWindow(c)
	defer w.Close()
	w.Resize(fyne.NewSize(560, 300))
	pedidas := 0
	c.Rejilla.Contexto = &editor.Contexto{Atajos: func() { pedidas++ }}
	ctrlMas(c.Rejilla, fyne.KeySlash, false)
	ctrlMas(c.Rejilla, fyne.KeySlash, true)
	c.Pestanas.teclado.Tapped(&fyne.PointEvent{})
	if pedidas != 3 {
		t.Errorf("la hoja del editor se pidió %d veces de 3", pedidas)
	}
	if n := len(w.Canvas().Overlays().List()); n != 0 {
		t.Errorf("se abrió además una hoja propia de la rejilla (%d capas)", n)
	}

	tr := func(k string) string { return c.t(k, nil) }
	propios := AtajosPropios(tr)
	vistas := map[string]bool{}
	var descripciones []string
	for _, g := range propios {
		for _, a := range g.Atajos {
			descripciones = append(descripciones, a.Descripcion)
			k := strings.Join(a.Teclas, " ")
			if vistas[k] {
				t.Errorf("%q sale dos veces", k)
			}
			vistas[k] = true
		}
	}
	for _, d := range []string{tr("sheet.shortcuts.fillDown"), tr("sheet.shortcuts.column")} {
		if !slices.Contains(descripciones, d) {
			t.Errorf("falta %q en los de la rejilla", d)
		}
	}
	for _, d := range []string{tr("sheet.shortcuts.help"), tr("cmd.copy")} {
		if slices.Contains(descripciones, d) {
			t.Errorf("%q ya está en el marco o en el registro y se repite", d)
		}
	}
}
