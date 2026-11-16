// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El teclado de la lista (CONT-04): una libreta de 1.000 contactos se recorre sin ratón.
// La fila es `Focusable` (Intro y Espacio abren), solo la del cursor está en el
// recorrido del tabulador, las flechas la mueven y las letras de la web suben a
// `App.Atajo` aunque la fila tenga el foco.

// appConLibreta monta la app con n contactos cargados y el lienzo escuchando las letras,
// como `cmd/contacts`.
func appConLibreta(t *testing.T, n int) (*App, fyne.Canvas) {
	t.Helper()
	cs := make([]api.Contact, n)
	for i := range cs {
		cs[i] = persona(fmt.Sprintf("c%04d", i), fmt.Sprintf("Contacto %04d", i))
	}
	s := &servicioFalso{contactos: cs}
	a := appDePrueba(t, s)
	a.Cargar()
	s.correrCola()
	if len(a.Pantalla.Contactos) != n {
		t.Fatalf("cargados %d de %d", len(a.Pantalla.Contactos), n)
	}
	c := a.lienzo()
	if c == nil {
		t.Fatal("la app no está montada")
	}
	c.SetOnTypedRune(a.Atajo)
	return a, c
}

// filaEnfocada es la fila con el foco del lienzo, o nil.
func filaEnfocada(c fyne.Canvas) *FilaContacto {
	f, _ := c.Focused().(*FilaContacto)
	return f
}

func pulsar(c fyne.Canvas, k fyne.KeyName) {
	if f := c.Focused(); f != nil {
		f.TypedKey(&fyne.KeyEvent{Name: k})
	}
}

func teclear(c fyne.Canvas, r rune) {
	if f := c.Focused(); f != nil {
		f.TypedRune(r)
		return
	}
	if h := c.OnTypedRune(); h != nil {
		h(r)
	}
}

func TestTecladoRecorreMilContactosConLasFlechas(t *testing.T) {
	a, c := appConLibreta(t, 1000)
	p := a.Pantalla

	// La primera j, sin foco ni ficha, aterriza en el cursor (la 0) sin saltársela.
	teclear(c, 'j')
	f := filaEnfocada(c)
	if f == nil || f.Nombre != "Contacto 0000" {
		t.Fatalf("la primera j debe enfocar la fila 0; enfocado %v", c.Focused())
	}
	pulsar(c, fyne.KeyDown)
	pulsar(c, fyne.KeyDown)
	pulsar(c, fyne.KeyUp)
	if f := filaEnfocada(c); f == nil || f.Nombre != "Contacto 0001" || p.CursorFila() != 1 {
		t.Fatalf("↓ ↓ ↑ debe dejar el foco en la 1; cursor %d", p.CursorFila())
	}
	// Fin: la 999 está muy lejos de lo pintado; la lista la trae y la enfoca.
	pulsar(c, fyne.KeyEnd)
	if f := filaEnfocada(c); f == nil || f.Nombre != "Contacto 0999" || !f.EsCursor {
		t.Fatalf("Fin debe enfocar la última; enfocado %v", c.Focused())
	}
	pulsar(c, fyne.KeyDown) // en la última se queda
	if f := filaEnfocada(c); f == nil || f.Nombre != "Contacto 0999" {
		t.Fatal("↓ en la última no debe salirse de la lista")
	}
	pulsar(c, fyne.KeyHome)
	if f := filaEnfocada(c); f == nil || f.Nombre != "Contacto 0000" {
		t.Fatal("Inicio debe volver a la primera")
	}
	// Moverse no abre ninguna ficha.
	if p.Seleccionado != nil {
		t.Errorf("las flechas no deben abrir fichas; abierta %q", p.Seleccionado.DisplayName())
	}
}

func TestTecladoUnaSolaParadaDeTabulador(t *testing.T) {
	a, c := appConLibreta(t, 50)
	teclear(c, 'j')
	pulsar(c, fyne.KeyDown)
	pulsar(c, fyne.KeyDown)
	en := 0
	for f, i := range a.Pantalla.filas {
		if !f.Visible() || i >= len(a.Pantalla.Contactos) {
			continue
		}
		r, ok := test.WidgetRenderer(f).(*filaContactoRenderer)
		if !ok {
			t.Fatal("la fila no tiene su renderer")
		}
		// La casilla y la estrella cuentan igual: en la web solo la fila del cursor
		// tiene `tabIndex=0` en las tres.
		for _, d := range []fyne.Disableable{f, r.casilla, r.boton} {
			if !d.Disabled() {
				en++
				if i != 2 {
					t.Errorf("la fila %d tiene una parada del tabulador y el cursor está en la 2", i)
				}
			}
		}
	}
	if en != 3 {
		t.Fatalf("solo la fila del cursor debe estar en el recorrido (fila, casilla y estrella); hay %d paradas", en)
	}
	// Fuera del recorrido no es inerte: un toque la sigue abriendo.
	for f, i := range a.Pantalla.filas {
		if i == 5 && f.Visible() {
			f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(150, 20)})
		}
	}
	if s := a.Pantalla.Seleccionado; s == nil || s.DisplayName() != "Contacto 0005" {
		t.Fatal("una fila fuera del recorrido debe seguir abriéndose con el puntero")
	}
}

func TestTecladoIntroYEspacioAbrenLaFicha(t *testing.T) {
	a, c := appConLibreta(t, 20)
	teclear(c, 'j')
	pulsar(c, fyne.KeyDown)
	pulsar(c, fyne.KeyReturn)
	if s := a.Pantalla.Seleccionado; s == nil || s.DisplayName() != "Contacto 0001" {
		t.Fatal("Intro debe abrir la fila enfocada")
	}
	pulsar(c, fyne.KeyDown)
	teclear(c, ' ')
	if s := a.Pantalla.Seleccionado; s == nil || s.DisplayName() != "Contacto 0002" {
		t.Fatal("Espacio debe abrir la fila enfocada")
	}
}

func TestTecladoConLaFichaAbiertaJYKLaLlevan(t *testing.T) {
	a, c := appConLibreta(t, 20)
	a.elegir(a.Pantalla.Contactos[3])
	if a.Pantalla.CursorFila() != 3 {
		t.Fatalf("el cursor debe seguir a la ficha abierta; está en %d", a.Pantalla.CursorFila())
	}
	teclear(c, 'j')
	if s := a.Pantalla.Seleccionado; s == nil || s.DisplayName() != "Contacto 0004" {
		t.Fatal("j con una ficha abierta debe abrir la siguiente")
	}
	if f := filaEnfocada(c); f == nil || f.Nombre != "Contacto 0004" {
		t.Fatal("y el foco va con ella a su fila")
	}
	// Ya desde la fila enfocada, k sube a la que la precede.
	teclear(c, 'k')
	teclear(c, 'k')
	if s := a.Pantalla.Seleccionado; s == nil || s.DisplayName() != "Contacto 0002" {
		t.Fatalf("k k debe volver a la 2; abierta %v", s)
	}
}

func TestTecladoAtajosXEyBarra(t *testing.T) {
	a, c := appConLibreta(t, 20)
	teclear(c, 'j')
	pulsar(c, fyne.KeyDown)
	// x marca la fila ENFOCADA (no abre nada) y el foco se queda donde estaba.
	teclear(c, 'x')
	if !a.marcados["c0001"] || len(a.marcados) != 1 {
		t.Fatalf("x debe marcar la fila 1 y solo ella; marcados %v", a.marcados)
	}
	if f := filaEnfocada(c); f == nil || f.Nombre != "Contacto 0001" {
		t.Fatal("marcar no debe mover el foco")
	}
	// e edita la enfocada: abre el formulario encima.
	teclear(c, 'e')
	if c.Overlays().Top() == nil {
		t.Fatal("e debe abrir el formulario de la fila enfocada")
	}
	// Con una capa abierta las letras son de ella: x no marca nada detrás.
	a.Atajo('x')
	if len(a.marcados) != 1 {
		t.Fatal("con un diálogo abierto, x no debe tocar la lista")
	}
	ui.Descartar(c)
	for c.Overlays().Top() != nil {
		c.Overlays().Remove(c.Overlays().Top())
	}
	// «/» lleva al buscador.
	c.Unfocus()
	teclear(c, '/')
	if c.Focused() != fyne.Focusable(a.Pantalla.buscador) {
		t.Fatalf("/ debe enfocar el buscador; enfocado %v", c.Focused())
	}
}

func TestTecladoInterrogacionAbreLaHoja(t *testing.T) {
	a, c := appConLibreta(t, 5)
	teclear(c, '?')
	top := c.Overlays().Top()
	if top == nil {
		t.Fatal("? debe abrir la hoja de atajos")
	}
	textos := textosHondos(top)
	for _, quiere := range []string{
		a.Pantalla.t("shortcuts.title", nil), a.Pantalla.t("shortcuts.next", nil),
		a.Pantalla.t("shortcuts.star", nil), a.Pantalla.t("shortcuts.select", nil),
	} {
		if !contiene(textos, quiere) {
			t.Errorf("la hoja no dice %q; dice %v", quiere, textos)
		}
	}
	// Con la hoja abierta, j no mueve nada detrás.
	a.Atajo('j')
	if filaEnfocada(c) != nil {
		t.Error("con la hoja abierta, j no debe enfocar filas")
	}
}

func TestTecladoLaFilaEnfocadaPintaSuAro(t *testing.T) {
	f := NuevaFilaContacto()
	f.Nombre, f.EsCursor = "Ana", true
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(anchoPanel-1, altoFila))
	r, ok := test.WidgetRenderer(f).(*filaContactoRenderer)
	if !ok {
		t.Fatal("renderer inesperado")
	}
	visibles := func() int {
		n := 0
		for _, o := range r.aro.Objetos() {
			if o.Visible() {
				n++
			}
		}
		return n
	}
	if visibles() != 0 {
		t.Fatal("sin foco no hay aro")
	}
	w.Canvas().Focus(f)
	if visibles() == 0 {
		t.Fatal("con el foco, el aro debe verse")
	}
	// Dentro de la fila: el `inset` de la web, que el `overflow` del panel no recorta.
	for _, o := range r.aro.Objetos() {
		if o.Position().X < 0 || o.Position().Y < 0 ||
			o.Position().X+o.Size().Width > f.Size().Width || o.Position().Y+o.Size().Height > f.Size().Height {
			t.Errorf("el aro se sale de la fila: %v %v", o.Position(), o.Size())
		}
	}
	w.Canvas().Unfocus()
	if visibles() != 0 {
		t.Fatal("al perder el foco, el aro se apaga")
	}
}

// textosHondos recoge los `canvas.Text` de un árbol, entrando también en los widgets:
// la hoja pinta sus filas con un renderer propio.
func textosHondos(o fyne.CanvasObject) []string {
	var fuera []string
	switch v := o.(type) {
	case *canvas.Text:
		fuera = append(fuera, v.Text)
	case *fyne.Container:
		for _, h := range v.Objects {
			fuera = append(fuera, textosHondos(h)...)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(v).Objects() {
			fuera = append(fuera, textosHondos(h)...)
		}
	}
	return fuera
}

func contiene(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
