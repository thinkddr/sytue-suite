// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// almacenDePrueba es un `localStorage` en memoria.
type almacenDePrueba map[string]string

func (a almacenDePrueba) Leer(k string) string { return a[k] }
func (a almacenDePrueba) Poner(k, v string) {
	if v == "" {
		delete(a, k)
	} else {
		a[k] = v
	}
}

// sesentaLineas es un mazo de dos diapositivas cuya primera lleva 60 líneas de notas.
func sesentaLineas(t *testing.T) *Modelo {
	t.Helper()
	ls := make([]string, 60)
	for i := range ls {
		ls[i] = fmt.Sprintf("Línea %d de las notas.", i+1)
	}
	notas, _ := json.Marshal(strings.Join(ls, "\n"))
	m, err := Leer([]byte(`{"slides":[{"id":"a","boxes":[],"notes":` + string(notas) + `},{"id":"b","boxes":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// alFinal dice si, bajando el desplazador del todo, la última línea se ve entera (su
// caja de glifos, que ColocarTexto ya baja dentro de la caja de línea).
func alFinal(d *container.Scroll, ultima fyne.CanvasObject) bool {
	d.ScrollToBottom()
	fin := ultima.Position().Y + ultima.Size().Height - d.Offset.Y
	return d.Offset.Y > 0 && fin > 0 && fin <= d.Size().Height+0.5
}

// TestLasNotasLargasDelModeradorSeDesplazan: unas notas de 60 líneas se leen enteras en
// su franja (el `overflow: auto` de la web), sin moverse con el tic del reloj.
//
// VISTO EN ROJO (2026-11-12, antes de DIAP-08): solo se pintaban las líneas que cabían
// encima del pie (siete), y el resto no había forma de leerlo.
func TestLasNotasLargasDelModeradorSeDesplazan(t *testing.T) {
	p := NuevaPresentacion(sesentaLineas(t), 0, traductor(t))
	mod := NuevoModerador(p, "es")
	w := test.NewTempWindow(t, mod)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(AnchoModerador, AltoModerador))
	r := test.WidgetRenderer(mod).(*moderadorRenderer)

	if len(r.notas) != 60 || r.notas[59].Text != "Línea 60 de las notas." {
		t.Fatalf("hay %d líneas de notas", len(r.notas))
	}
	if r.desplaza.Size().Height >= r.desplaza.Content.MinSize().Height {
		t.Fatalf("las 60 líneas caben en %v: la prueba no prueba nada", r.desplaza.Size())
	}
	if !alFinal(r.desplaza, r.notas[59]) {
		t.Errorf("bajando del todo no se ve la última línea (desplazado %v)", r.desplaza.Offset)
	}
	bajado := r.desplaza.Offset
	mod.Refresh() // el tic del reloj
	if r.desplaza.Offset != bajado {
		t.Errorf("el tic del reloj mueve las notas de %v a %v", bajado, r.desplaza.Offset)
	}
	p.Ir(1, 0)
	mod.Refresh()
	if r.desplaza.Offset.Y != 0 || len(r.notas) != 1 || r.notas[0].Text != "Sin notas para esta diapositiva." {
		t.Errorf("en otra diapositiva las notas siguen desplazadas (%v) o no dicen «sin notas»: %v", r.desplaza.Offset, r.notas)
	}
}

// TestElTamanoDeLasNotasVaDeSMa2XLYSeRecuerda: A− y A+ recorren la escala de la suite de
// `text-sm` a `text-2xl` con topes, el botón del tope se apaga, y el tamaño sobrevive a
// cerrar y abrir la vista (con la misma clave y valores que la web).
func TestElTamanoDeLasNotasVaDeSMa2XLYSeRecuerda(t *testing.T) {
	almacen := almacenDePrueba{}
	abrir := func() (*Moderador, *moderadorRenderer, fyne.Window) {
		p := NuevaPresentacion(sesentaLineas(t), 0, traductor(t))
		p.Almacen = almacen
		mod := NuevoModerador(p, "es")
		w := test.NewTempWindow(t, mod)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(AnchoModerador, AltoModerador))
		return mod, test.WidgetRenderer(mod).(*moderadorRenderer), w
	}
	mod, r, w := abrir()
	if mod.TamNotas() != "md" || r.notas[0].TextSize != tema.TextoMD.Tam {
		t.Fatalf("de entrada %q a %v", mod.TamNotas(), r.notas[0].TextSize)
	}
	if r.menos.AccessibilityLabel() != "Reducir el texto de las notas" || r.mas.AccessibilityLabel() != "Ampliar el texto de las notas" {
		t.Errorf("A−/A+ se anuncian %q y %q", r.menos.AccessibilityLabel(), r.mas.AccessibilityLabel())
	}
	for range 5 {
		test.Tap(r.mas)
	}
	if mod.TamNotas() != "2xl" || r.notas[0].TextSize != tema.Texto2XL.Tam || !r.mas.apagado || r.menos.apagado {
		t.Errorf("tras cinco A+: %q a %v (A+ apagado %v)", mod.TamNotas(), r.notas[0].TextSize, r.mas.apagado)
	}
	if almacen[ClaveTamNotas] != "2xl" {
		t.Errorf("se guarda %q", almacen[ClaveTamNotas])
	}
	// Más grande, más líneas que desplazar, pero se siguen leyendo todas.
	if !alFinal(r.desplaza, r.notas[len(r.notas)-1]) {
		t.Error("en 2xl la última línea no se alcanza")
	}
	guarda(t, "moderador-notas-2xl.png", w.Canvas().Capture())

	// Cerrar y abrir: el tamaño sigue.
	mod, r, _ = abrir()
	if mod.TamNotas() != "2xl" || r.notas[0].TextSize != tema.Texto2XL.Tam {
		t.Errorf("al volver a abrir: %q", mod.TamNotas())
	}
	for range 7 {
		test.Tap(r.menos)
	}
	if mod.TamNotas() != "sm" || r.notas[0].TextSize != tema.TextoSM.Tam || !r.menos.apagado {
		t.Errorf("tras siete A−: %q a %v", mod.TamNotas(), r.notas[0].TextSize)
	}
	// Volver al de siempre no deja rastro, como en la web.
	test.Tap(r.mas)
	test.Tap(r.mas)
	if mod.TamNotas() != "md" || len(almacen) != 0 {
		t.Errorf("en md queda guardado %v", almacen)
	}
	// Un valor ajeno se ignora.
	almacen[ClaveTamNotas] = "9xl"
	if mod, _, _ = abrir(); mod.TamNotas() != "md" {
		t.Errorf("con basura guardada abre en %q", mod.TamNotas())
	}
}

// TestLasNotasLargasDeLaCapaNSeDesplazan: la capa N del visor se queda en su 32 % y las
// 60 líneas se alcanzan desplazándola; tocarla no pasa de diapositiva.
//
// VISTO EN ROJO (2026-11-12, antes de DIAP-08): las líneas de más del 32 % se cortaban.
func TestLasNotasLargasDeLaCapaNSeDesplazan(t *testing.T) {
	p := NuevaPresentacion(sesentaLineas(t), 0, traductor(t))
	w := test.NewTempWindow(t, p)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(960, 540))
	p.Runa('n')
	r := p.r
	if len(r.notas) != 60 {
		t.Fatalf("hay %d líneas en la capa", len(r.notas))
	}
	if h := r.papelNotas.Size().Height; !cerca32(h, 540*0.32, 0.01) || r.desplaza.Size().Height != h {
		t.Errorf("la capa mide %v y el desplazador %v; el tope es el 32 %%", h, r.desplaza.Size())
	}
	r.desplaza.ScrollToBottom()
	// La última línea queda por encima del `pb-16` (los mandos), dentro de lo visible.
	fin := r.notas[59].Position().Y + r.notas[59].Size().Height - r.desplaza.Offset.Y
	if fin > r.desplaza.Size().Height-tema.Espacio(16)+0.5 || fin < 0 {
		t.Errorf("la última línea acaba en %v de %v", fin, r.desplaza.Size().Height)
	}
	test.TapAt(p, fyne.NewPos(480, 500))
	if p.Diapo != 0 {
		t.Error("tocar las notas pasa de diapositiva")
	}
	test.TapAt(p, fyne.NewPos(480, 100))
	if p.Diapo != 1 {
		t.Error("tocar la diapositiva ya no avanza")
	}
}

// TestElTamanoDeLasNotasSigueAlReabrirElModeradorSinAlmacen: sin dónde recordarlo (sin
// sesión), cerrar y abrir la vista con P en la misma presentación no lo pierde, como el
// estado de `Presentation` en la web.
func TestElTamanoDeLasNotasSigueAlReabrirElModeradorSinAlmacen(t *testing.T) {
	p := NuevaPresentacion(sesentaLineas(t), 0, traductor(t))
	mod := NuevoModerador(p, "es")
	mod.CambiarTamNotas(1)
	if mod = NuevoModerador(p, "es"); mod.TamNotas() != "lg" {
		t.Errorf("al reabrir el moderador las notas vuelven a %q", mod.TamNotas())
	}
}
