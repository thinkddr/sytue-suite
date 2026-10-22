// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"math"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func referencia(t *testing.T) *Modelo {
	t.Helper()
	crudo, err := os.ReadFile("testdata/referencia.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := Leer(crudo)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// cerca32 es «a ± tol».
func cerca32(a, b, tol float32) bool { return math.Abs(float64(a-b)) <= float64(tol) }

// TestLaVistaDelModeradorEsLaDeLaWeb: las medidas de `PresenterView` en su ventana de
// 1040 × 720, leídas en Chromium el 2026-10-04 sobre el componente real con rem 16
// y Figtree: cabecera de 58 con «Reiniciar» de 32 en y = 13 (items-baseline); vistas
// previas de 484 × 272,25 en (24, 106) y (532, 106); notas del 26 % que empiezan en
// 471,8125; pie de 61 con «← Anterior» de 98,671875 × 40 en (20, 670) y «Terminar»
// pegado al margen derecho. En la última, la siguiente es el hueco con «Fin de la presentación» y,
// sin notas, «Sin notas…». Las flechas usan el recurso compartido Noto Sans Math
// 3.000, la misma cara comprobada por CDP en el componente web.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con un píxel de más en el ancho de la
// vista previa, salían de 485 × 272,81 y la primera en x = 23.
func TestLaVistaDelModeradorEsLaDeLaWeb(t *testing.T) {
	p := NuevaPresentacion(referencia(t), 0, traductor(t))
	mod := NuevoModerador(p, "es")
	fijo := time.Date(2026, 9, 23, 1, 23, 0, 0, time.Local)
	mod.ahora = func() time.Time { return fijo }
	mod.inicio = fijo.Add(-3 * time.Second)
	w := test.NewTempWindow(t, mod)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(AnchoModerador, AltoModerador))
	r := test.WidgetRenderer(mod).(*moderadorRenderer)

	if r.tiempoTexto != "0:03" || r.reloj.Text != "01:23" || r.posicion.Text != "Diapositiva 1 de 4" {
		t.Errorf("cabecera = %q %q %q", r.tiempoTexto, r.reloj.Text, r.posicion.Text)
	}
	// El span real mide 63,765625 con tnum y letter-spacing .02em; también deja
	// «Reiniciar» en x = 20 + 63,765625 + 14, sin saltos al cambiar las cifras.
	if ancho := r.tiempo.Ancho(); !cerca32(ancho, 63.765625, 0.03) {
		t.Errorf("el cronómetro ocupa %v; Chromium da 63,765625", ancho)
	}
	if x := r.reinicia.Position().X; !cerca32(x, 97.765625, 0.03) {
		t.Errorf("«Reiniciar» empieza en %v; Chromium da 97,765625", x)
	}
	if pos, tam := r.reinicia.Position(), r.reinicia.Size(); pos.Y != 13 || tam.Height != 32 {
		t.Errorf("«Reiniciar» en %v, %v", pos, tam)
	}
	if y := r.rayas[0].Position().Y; y != 57 {
		t.Errorf("la raya de la cabecera está en %v", y)
	}
	for i, l := range []*Lienzo{r.actual, r.siguiente} {
		x := 24 + float32(i)*508
		if pos, tam := l.Position(), l.Size(); pos != fyne.NewPos(x, 106) || tam != fyne.NewSize(484, 272.25) {
			t.Errorf("vista previa %d en %v con %v", i, pos, tam)
		}
	}
	if r.siguiente.Indice != 1 || r.siguiente.Paso != 0 || r.actual.Indice != 0 {
		t.Errorf("actual %d, siguiente %d paso %d", r.actual.Indice, r.siguiente.Indice, r.siguiente.Paso)
	}
	if y := r.rayas[1].Position().Y; !cerca32(y, 471.8125, 0.02) {
		t.Errorf("las notas empiezan en %v", y)
	}
	if len(r.notas) != 1 || r.notas[0].Text != "Presentar en dos minutos." || r.notas[0].TextSize != 16 {
		t.Errorf("notas = %v", r.notas)
	}
	if pos, tam := r.anterior.Position(), r.anterior.Size(); pos != fyne.NewPos(20, 670) ||
		tam.Height != 40 || !cerca32(tam.Width, 98.671875, 0.03) {
		t.Errorf("«← Anterior» en %v con %v", pos, tam)
	}
	for _, boton := range []*botonModerador{r.anterior, r.siguien} {
		pintado := test.WidgetRenderer(boton).(*botonModeradorRenderer)
		if pintado.flecha == nil || pintado.flecha.FontSource != tema.Matematica {
			t.Errorf("%q debe pintar la flecha con el recurso matemático canónico", boton.texto)
		}
	}
	if pos, tam := r.terminar.Position(), r.terminar.Size(); pos.X+tam.Width != 1020 || pos.Y != 674 {
		t.Errorf("«Terminar» en %v con %v", pos, tam)
	}
	guarda(t, "moderador.png", w.Canvas().Capture())

	p.Ir(3, 0)
	mod.Refresh()
	if r.haySiguiente || r.marcoFin == nil || r.marcoFin.Position() != fyne.NewPos(532, 106) {
		t.Error("en la última, la siguiente es el hueco discontinuo")
	}
	if len(r.notas) != 1 || r.notas[0].Text != "Sin notas para esta diapositiva." {
		t.Errorf("sin notas se lee %v", r.notas)
	}
	guarda(t, "moderador-fin.png", w.Canvas().Capture())

	test.Tap(r.anterior)
	if p.Diapo != 2 {
		t.Errorf("«← Anterior» lleva a %d", p.Diapo)
	}
	fijo = fijo.Add(time.Minute)
	test.Tap(r.reinicia)
	if r.tiempoTexto != "0:00" {
		t.Errorf("«Reiniciar» deja %q", r.tiempoTexto)
	}
	if ancho := r.tiempo.Ancho(); !cerca32(ancho, 63.765625, 0.03) {
		t.Errorf("el cronómetro reiniciado ocupa %v: las cifras tabulares conservan el ancho", ancho)
	}
	salido := false
	p.AlSalir = func() { salido = true }
	test.Tap(r.terminar)
	if !salido {
		t.Error("«Terminar» sale de la presentación")
	}
}

// TestLaPAbreYCierraLaVentanaDelModerador: P abre la segunda ventana, que sigue a la
// presentación; cerrarla a mano apaga el moderador, y cerrar la presentación la cierra.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin apagar el moderador al cerrar su
// ventana, `p.Moderador` seguía encendido y la siguiente P la «cerraba» en vez de abrirla.
func TestLaPAbreYCierraLaVentanaDelModerador(t *testing.T) {
	antes := cadaSegundo
	cadaSegundo = func(func()) func() { return func() {} }
	t.Cleanup(func() { cadaSegundo = antes })
	ventana := func() (fyne.Window, *Moderador) {
		for _, w := range fyne.CurrentApp().Driver().AllWindows() {
			if mod, ok := w.Content().(*Moderador); ok {
				return w, mod
			}
		}
		return nil, nil
	}

	// Con -count>1 otra prueba del paquete deja su moderador abierto en el driver de
	// pruebas; se cierra aquí para que «sin pedirlo no hay moderador» mire solo este.
	for wm, _ := ventana(); wm != nil; wm, _ = ventana() {
		wm.Close()
	}
	p := Presentar(referencia(t), 0, false, "es", traductor(t))
	if wm, _ := ventana(); wm != nil || p.Moderador {
		t.Fatal("sin pedirlo no hay moderador")
	}
	p.Runa('p')
	wm, mod := ventana()
	if wm == nil || !p.Moderador {
		t.Fatal("P abre la ventana del moderador")
	}
	if s := wm.Canvas().Size(); s != fyne.NewSize(AnchoModerador, AltoModerador) {
		t.Errorf("la ventana mide %v", s)
	}
	p.Avanzar(1)
	if r := test.WidgetRenderer(mod).(*moderadorRenderer); r.diapo != p.Diapo || r.paso != p.Paso {
		t.Errorf("el moderador está en (%d, %d) y la presentación en (%d, %d)", r.diapo, r.paso, p.Diapo, p.Paso)
	}
	wm.Close()
	if p.Moderador {
		t.Error("cerrar la ventana a mano apaga el moderador")
	}
	p.Runa('p')
	if wm, _ = ventana(); wm == nil {
		t.Fatal("P la vuelve a abrir")
	}
	p.AlSalir()
	if wm, _ = ventana(); wm != nil {
		t.Error("salir de la presentación cierra también el moderador")
	}
}

// TestElModeradorDelTelefonoVaEnLaMismaVentana: donde no hay segunda ventana, la P (o
// «Vista del moderador») pasa la misma ventana a la vista del moderador, que sigue a la
// presentación, y tocar su diapositiva actual vuelve a la presentación.
//
// VISTO EN ROJO (2026-09-26, en el frontal): con el `AlVolver` sin poner, el toque en la
// diapositiva actual no volvía y el teléfono solo salía de la vista con «Terminar».
func TestElModeradorDelTelefonoVaEnLaMismaVentana(t *testing.T) {
	p := NuevaPresentacion(leeModelo(t), 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.Resize(fyne.NewSize(AnchoModerador, AltoModerador))
	p.AlCambiarModerador = ModeradorEnLaMisma(w, p, "es")
	p.Runa('p')
	mod, ok := w.Canvas().Content().(*Moderador)
	if !ok || !p.Moderador {
		t.Fatalf("la P deja en la ventana %T", w.Canvas().Content())
	}
	p.Avanzar(1)
	if mod.r.actual.Indice != p.Diapo || mod.r.actual.Paso != p.Paso {
		t.Error("la vista sigue a la presentación")
	}
	a := mod.r.actual
	test.TapAt(mod, a.Position().AddXY(a.Size().Width/2, a.Size().Height/2))
	if w.Canvas().Content() != p || p.Moderador {
		t.Fatalf("tocar la diapositiva actual deja %T", w.Canvas().Content())
	}
}
