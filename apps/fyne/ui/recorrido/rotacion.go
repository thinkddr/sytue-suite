// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recorrido

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// ComprobarRotacion gira un fixture real antes del recorrido, en Marco y Carril.
// tactil recibe ui.TactilDePrueba desde la app para no introducir un ciclo con ui.
// Restaura forma, modo táctil y tamaño antes de devolver el control al recorrido.
//
// La geometría comprueba mandos y viewports, no exige que el contenido de un Scroll
// quepa en él. Las capturas requieren revisión visual: la geometría no la sustituye.
// capturar elige una escena por app: SYTUE_ROTACION_CAPTURAS guarda solo los dos
// horizontales representativos por forma, con nombres únicos de app/escena.
// preparacion termina el trabajo propio del fixture tras cada Resize y antes de pintar.
func ComprobarRotacion(t testing.TB, w fyne.Window, nombre string, tactil func(bool) func(), capturar bool, preparacion ...func()) {
	t.Helper()
	forma := "marco"
	if !tema.EsMarco() {
		forma = "carril"
	}
	tamano := w.Canvas().Size()
	apariencia := fyne.CurrentApp().Settings().Theme()
	restaurar := tactil(true)
	defer func() {
		fallo := recover()
		restaurar()
		tema.ElegirForma(forma)
		if fallo != nil {
			panic(fallo) // SetTheme puede conservar un mutex tomado tras el panic.
		}
		fyne.CurrentApp().Settings().SetTheme(apariencia)
		w.Resize(tamano)
		w.Canvas().Content().Refresh()
		fyne.DoAndWait(func() {})
	}()
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		// Ya estamos en la rutina del test, como el TestMain de Suite. El
		// DoAndWait vacío de abajo drena el trabajo pendiente tras el cambio.
		fyne.CurrentApp().Settings().SetTheme(apariencia)
		for paso, tam := range []fyne.Size{
			{Width: 320, Height: 700},
			{Width: 568, Height: 320},
			{Width: 844, Height: 390},
			{Width: 1024, Height: 768},
			{Width: 768, Height: 1024},
			{Width: 320, Height: 700},
		} {
			w.Resize(tam)
			w.Canvas().Content().Refresh()
			fyne.DoAndWait(func() {})
			for _, preparar := range preparacion {
				preparar()
			}
			fyne.DoAndWait(func() {})
			img := w.Canvas().Capture()
			controles, fallos := geometriaRotacion(w.Canvas())
			if w.Canvas().Size() != tam {
				fallos = append(fallos, fmt.Sprintf("viewport=%v; quiero %v", w.Canvas().Size(), tam))
			}
			if controles == 0 {
				fallos = append(fallos, "fixture sin mandos con área")
			}
			for _, fallo := range fallos {
				t.Errorf("rotación %s/%s/%d %v: %s", nombre, forma, paso, tam, fallo)
			}
			t.Logf("rotación %s/%s/%d %v: %d mandos, %d fallos de geometría", nombre, forma, paso, tam, controles, len(fallos))
			if capturar && (paso == 1 || paso == 3) {
				guardarRotacion(t, w.Canvas(), img, nombre, forma, paso, tam)
			}
		}
	}
}

// PrepararCaptura termina la descarga y la tesela del tamaño actual. La primera
// pintura pide los bytes; la segunda prepara sus píxeles. No espera al reloj.
// esperar y repintar son los de la fototeca y la app que conserva el fixture.
func PrepararCaptura(w fyne.Window, esperar, repintar func()) {
	for range 2 {
		fyne.DoAndWait(func() {})
		w.Canvas().Capture()
		esperar()
		repintar()
	}
}

func guardarRotacion(t testing.TB, c fyne.Canvas, img image.Image, nombre, forma string, paso int, tam fyne.Size) {
	t.Helper()
	dir := os.Getenv("SYTUE_ROTACION_CAPTURAS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // la ruta la pone quien lanza el test
		t.Fatal(err)
	}
	nombre = limpiar(strings.ReplaceAll(nombre, "/", "-"))
	ruta := filepath.Join(dir, fmt.Sprintf("native-rotacion-%s-%s-%d-%gx%g.png", nombre, forma, paso, tam.Width, tam.Height))
	datos, err := imagenPNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, datos, 0o600); err != nil { //nolint:gosec // G703: la ruta procede del entorno del test
		t.Fatal(err)
	}
	if os.Getenv("SYTUE_CAPTURAS_VOLCADO") != "" {
		if err := Volcar(c, strings.TrimSuffix(ruta, ".png")+".json"); err != nil {
			t.Fatal(err)
		}
	}
}

type sondaRotacion struct {
	ventana fyne.Size
	mandos  int
	fallos  []string
}

func geometriaRotacion(c fyne.Canvas) (int, []string) {
	s := sondaRotacion{ventana: c.Size()}
	raiz := c.Content()
	if raiz == nil || raiz.Size().Width <= 0 || raiz.Size().Height <= 0 {
		return 0, []string{"contenido raíz sin área"}
	}
	s.ir(raiz, fyne.Position{}, false)
	return s.mandos, s.fallos
}

func (s *sondaRotacion) ir(o fyne.CanvasObject, base fyne.Position, dentroScroll bool) {
	if o == nil || !o.Visible() {
		return
	}
	p, tam := base.Add(o.Position()), o.Size()
	if !geometriaFinita(p, tam) {
		s.fallos = append(s.fallos, fmt.Sprintf("%s tiene geometría inválida: %v/%v", nombre(o), p, tam))
		return
	}
	// El pintor de Fyne recorta también desplazadores propios de la suite:
	// no todos son container.Scroll (la cinta usa fyne.Scrollable).
	_, scroll := o.(fyne.Scrollable)
	if tam.Width > 0 && tam.Height > 0 && (scroll || esMando(o)) {
		s.mandos++
		if !dentroScroll && (p.X < -1 || p.Y < -1 || p.X+tam.Width > s.ventana.Width+1 || p.Y+tam.Height > s.ventana.Height+1) {
			s.fallos = append(s.fallos, fmt.Sprintf("%s fuera del viewport: %v/%v en %v", nombre(o), p, tam, s.ventana))
		}
	}
	for _, h := range hijos(o) {
		s.ir(h, p, dentroScroll || scroll)
	}
}

func esMando(o fyne.CanvasObject) bool {
	switch o.(type) {
	case fyne.Tappable, fyne.Focusable:
		return true
	}
	return false
}

func geometriaFinita(p fyne.Position, tam fyne.Size) bool {
	for _, valor := range []float32{p.X, p.Y, tam.Width, tam.Height} {
		if math.IsNaN(float64(valor)) || math.IsInf(float64(valor), 0) {
			return false
		}
	}
	return tam.Width >= 0 && tam.Height >= 0
}
