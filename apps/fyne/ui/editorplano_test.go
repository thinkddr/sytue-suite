// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElAtajoVeLaCombinacionAntesQueElEditor: Fyne manda ⌘/Ctrl + Intro al campo
// enfocado como ATAJO y no como tecla, así que sin este gancho el redactor de Mail no
// puede enviar con el teclado. Lo que el gancho no se queda sigue siendo del editor:
// seleccionar todo tiene que seguir seleccionando.
//
// VISTO EN ROJO (2026-09-23): sin la sobrecarga de TypedShortcut, el gancho no se
// llama y el editor se traga la combinación.
func TestElAtajoVeLaCombinacionAntesQueElEditor(t *testing.T) {
	p := NuevoEditorPlano(tema.TextoBase, nil)
	p.PonerTexto("hola")
	enviado := false
	p.Atajo = func(s fyne.Shortcut) bool {
		c, ok := s.(*desktop.CustomShortcut)
		if ok && c.KeyName == fyne.KeyReturn {
			enviado = true
			return true
		}
		return false
	}
	var sc fyne.Shortcutable = p
	sc.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierControl})
	if !enviado {
		t.Fatal("el gancho no ha visto Ctrl + Intro")
	}
	sc.TypedShortcut(&fyne.ShortcutSelectAll{})
	if p.SelectedText() != "hola" {
		t.Errorf("seleccionar todo ya no llega al editor: seleccionado %q", p.SelectedText())
	}
}

// TestElEditorPlanoEscribeConLaTintaPropia: con `Tinta`, el texto sale del color del
// contenido y no del `text` de la interfaz —el blanco sobre una diapositiva oscura—.
// VISTO EN ROJO (2026-09-23, en el frontal): sin la tinta propia en el tema del editor,
// la «M» salía en la tinta de la interfaz y no había ni un píxel rojo.
func TestElEditorPlanoEscribeConLaTintaPropia(t *testing.T) {
	p := NuevoEditorPlano(tema.TextoXL, nil)
	rojo := color.NRGBA{R: 255, A: 255}
	p.Tinta(rojo, rojo)
	p.PonerTexto("MMMM")
	p.Colocar(fyne.NewPos(0, 0), fyne.NewSize(120, 40))
	img := pinta(t, 120, 40, p.Objeto())
	rojos := 0
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 > 200 && g>>8 < 80 && b>>8 < 80 {
				rojos++
			}
		}
	}
	if rojos < 20 {
		t.Errorf("solo %d píxeles rojos: el texto no sale con la tinta propia", rojos)
	}
}

// TestElEditorPlanoSabeSiHayMayus: el gancho `Tecla` puede quedarse Intro y dejar
// Mayús+Intro al editor, que parte la línea. Es lo que el compositor de Rooms
// necesita para enviar con Intro.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin apuntar la Mayús en KeyDown, «con
// Mayús pulsada, Mayus() dice que no». Deshecho.
func TestElEditorPlanoSabeSiHayMayus(t *testing.T) {
	e := NuevoEditorPlano(tema.TextoBase, nil)
	e.MultiLine = true
	w := test.NewWindow(e.Objeto())
	t.Cleanup(w.Close)
	enviados := 0
	e.Tecla = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn && !e.Mayus() {
			enviados++
			return true
		}
		return false
	}
	e.PonerTexto("hola")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if enviados != 1 || e.Texto() != "hola" {
		t.Fatalf("Intro sin Mayús: %d envíos y el texto %q", enviados, e.Texto())
	}
	e.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if !e.Mayus() {
		t.Fatal("con Mayús pulsada, Mayus() dice que no")
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	e.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if enviados != 1 || e.Texto() != "hola\n" || e.Mayus() {
		t.Fatalf("Mayús+Intro: %d envíos, texto %q y Mayus() = %v", enviados, e.Texto(), e.Mayus())
	}
}

// TestLaCaraSeMideComoSePinta: con `Cara`, el editor pinta con esa letra (el tema de
// su envoltorio) y el cursor se coloca midiendo LA MISMA (el de la aplicación, que es
// el que usa `widget.Entry`). Una «W» en negrita es más ancha que en regular, así que
// si la medida fuera la de Figtree regular el cursor caería dentro de la palabra.
func TestLaCaraSeMideComoSePinta(t *testing.T) {
	p := NuevoEditorPlano(tema.TextoBase, nil)
	p.Cara(tema.Negra, fyne.TextStyle{})
	const w = "WWWWWWWW"
	tam := tema.TextoBase.Tam
	quiero, _ := fyne.CurrentApp().Driver().RenderedTextSize(w, tam, fyne.TextStyle{}, tema.Negra)
	regular, _ := fyne.CurrentApp().Driver().RenderedTextSize(w, tam, fyne.TextStyle{}, tema.Regular)
	if quiero.Width == regular.Width {
		t.Fatal("la negrita y la regular miden igual: la prueba no distingue nada")
	}
	if got := fyne.MeasureText(w, tam, p.TextStyle); got.Width != quiero.Width {
		t.Errorf("el cursor mide %v y la letra pintada %v", got.Width, quiero.Width)
	}
	if got := p.th.Font(p.TextStyle); got != tema.Negra {
		t.Errorf("el envoltorio pinta con %v", got.Name())
	}
	p.Cara(nil, fyne.TextStyle{})
	if got := fyne.MeasureText(w, tam, p.TextStyle); got.Width != regular.Width {
		t.Errorf("sin cara vuelve a Figtree regular: %v y quiero %v", got.Width, regular.Width)
	}
}

// TestElDesplazamientoSeLee: el editor desplazable corre el texto para que el cursor se
// vea, como un `<input>`, y `Desplazamiento` dice cuánto —se lee de un campo que
// `widget.Entry` no publica, así que si Fyne lo renombra esto se pone rojo—. El de
// siempre no se desplaza.
func TestElDesplazamientoSeLee(t *testing.T) {
	p := NuevoEditorPlanoDesplazable(tema.TextoSM, nil)
	w := test.NewTempWindow(t, p.Objeto())
	w.Resize(fyne.NewSize(80, 30))
	p.Colocar(fyne.NewPos(0, 0), fyne.NewSize(80, 20))
	w.Canvas().Focus(p)
	for _, r := range "=SUMA(A1:A20;B1:B20;C1:C20)" {
		p.TypedRune(r)
	}
	if d := p.Desplazamiento(); d <= 0 {
		t.Errorf("con el cursor al final de una fórmula larga el texto no se ha corrido: %v", d)
	}
	quieto := NuevoEditorPlano(tema.TextoSM, nil)
	quieto.PonerTexto("=SUMA(A1:A20;B1:B20;C1:C20)")
	if d := quieto.Desplazamiento(); d != 0 {
		t.Errorf("el editor de siempre dice que se ha desplazado %v", d)
	}
}

// TestLaRuedaAvisaDelDesplazamiento: la rueda sobre el campo corre el texto sin tocar el
// cursor, y AlDesplazar lo avisa para que el tinte de la barra de fórmulas se ponga al
// día en ese momento y no con la siguiente tecla.
func TestLaRuedaAvisaDelDesplazamiento(t *testing.T) {
	p := NuevoEditorPlanoDesplazable(tema.TextoSM, nil)
	avisos := 0
	p.AlDesplazar(func() { avisos++ })
	w := test.NewTempWindow(t, p.Objeto())
	w.Resize(fyne.NewSize(80, 30))
	p.Colocar(fyne.NewPos(0, 0), fyne.NewSize(80, 20))
	w.Canvas().Focus(p)
	for _, r := range "=SUMA(A1:A20;B1:B20;C1:C20)" {
		p.TypedRune(r)
	}
	antes, n := p.Desplazamiento(), avisos
	test.Scroll(w.Canvas(), fyne.NewPos(40, 10), 30, 0)
	if p.Desplazamiento() == antes {
		t.Fatalf("la rueda no ha corrido el texto (%v)", antes)
	}
	if avisos == n {
		t.Error("la rueda corre el texto y AlDesplazar no avisa")
	}
}

// TestElAreaAvanzaUnaLineaDeCSSPorFila: un `<textarea>` avanza su `line-height` por
// renglón (Chromium: 15,75 px en `text-sm`, 19,25 en `text-base`). Fyne solo sumaba el
// interlineado del tema entre párrafos, y las filas de un área subían 2 px (`text-sm`)
// o 4,5 px (`text-base`) sobre la anterior; desde el fork `v2.8.1-sytue.7` las separa.
// El cursor tiene que caer en la misma fila que el texto.
//
// VISTO EN ROJO (2026-09-25) con `.6`: el paso era 13,66 y 14,70.
func TestElAreaAvanzaUnaLineaDeCSSPorFila(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		escalon tema.Escalon
	}{{"text-sm", tema.TextoSM}, {"text-base", tema.TextoBase}} {
		p := NuevoEditorPlanoMultilinea(c.escalon, nil)
		w := test.NewTempWindow(t, p.Objeto())
		w.Resize(fyne.NewSize(300, 200))
		p.SetText("uno\ndos\ntres")
		var filas []float32
		var recorrer func(o fyne.CanvasObject, y float32)
		recorrer = func(o fyne.CanvasObject, y float32) {
			y += o.Position().Y
			switch x := o.(type) {
			case *fcanvas.Text:
				if x.Text != "" {
					filas = append(filas, y)
				}
			case *fyne.Container:
				for _, h := range x.Objects {
					recorrer(h, y)
				}
			case fyne.Widget:
				for _, h := range test.WidgetRenderer(x).Objects() {
					recorrer(h, y)
				}
			}
		}
		recorrer(p.Objeto(), 0)
		if len(filas) != 3 {
			t.Fatalf("%s: %d filas pintadas, y son tres", c.nombre, len(filas))
		}
		for i := 1; i < 3; i++ {
			if paso := filas[i] - filas[i-1]; !casiIgual(paso, c.escalon.Linea) {
				t.Errorf("%s: la fila %d avanza %v y la web %v", c.nombre, i, paso, c.escalon.Linea)
			}
		}
		p.CursorRow = 2
		if y := p.CursorPosition().Y - func() float32 { p.CursorRow = 0; return p.CursorPosition().Y }(); !casiIgual(y, 2*c.escalon.Linea) {
			t.Errorf("%s: el cursor de la tercera fila baja %v y el texto %v", c.nombre, y, 2*c.escalon.Linea)
		}
	}
}

func casiIgual(a, b float32) bool { return a-b < 0.01 && b-a < 0.01 }
