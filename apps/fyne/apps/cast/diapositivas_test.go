// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// rotulosYParrafos son los textos de los rótulos y párrafos de la ventana, en orden.
func rotulosYParrafos(w fyne.Window) []string {
	var out []string
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Rotulo:
			out = append(out, x.Texto)
		case *ui.Parrafo:
			out = append(out, x.Texto)
		}
	})
	return out
}

// capitulosEn son las filas pulsables de la ventana.
func capitulosEn(w fyne.Window) []*ui.FilaPulsable {
	var out []*ui.FilaPulsable
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		if f, ok := o.(*ui.FilaPulsable); ok {
			out = append(out, f)
		}
	})
	return out
}

// grabacionDeDirecto es un directo ya terminado con su grabación lista.
func grabacionDeDirecto() api.CastVideo {
	g := directoDePrueba()
	g.ID, g.Status, g.DurationMS = "v41", api.CastStatusReady, 40*60*1000
	return g
}

// TestLaDiapositivaEnCursoDelDirecto: en un directo se lee «Diapositiva 3 de 12 ·
// Título»; se sondea mientras siga en directo y un cambio repinta. Sin sesión se
// pregunta por la ruta pública.
func TestLaDiapositivaEnCursoDelDirecto(t *testing.T) {
	s := servicioCompleto()
	s.presentacion = &api.CastPresentacion{Ref: "docs:d1", Index: 2, Total: 12, Title: "Resultados del trimestre"}
	a, w := appDePrueba(t, s, true)
	a.AbrirVideo("v10")
	if !contiene(s.vistas(), "GET /api/v1/videos/v10/slides") {
		t.Fatalf("el directo no pide su presentación: %q", s.vistas())
	}
	textos := rotulosYParrafos(w)
	if !slices.Contains(textos, "Diapositiva 3 de 12") || !slices.Contains(textos, " · Resultados del trimestre") {
		t.Fatalf("no se lee la diapositiva en curso: %q", textos)
	}
	// La diapositiva va antes que el título del vídeo, como en la web.
	if i, j := slices.Index(textos, "Diapositiva 3 de 12"), slices.Index(textos, "Reunión general de septiembre"); j < 0 || i > j {
		t.Errorf("la diapositiva (%d) tiene que ir encima del título (%d)", i, j)
	}
	if len(capitulosEn(w)) != 0 {
		t.Error("en directo no hay capítulos")
	}
	if !a.hayQueSondearDiapositivas() {
		t.Error("en directo la posición se sondea")
	}
	// Quien presenta avanza: el siguiente sondeo lo pinta.
	s.mu.Lock()
	s.presentacion = &api.CastPresentacion{Index: 3, Total: 12}
	s.mu.Unlock()
	a.pedirDiapositivas()
	textos = rotulosYParrafos(w)
	if !slices.Contains(textos, "Diapositiva 4 de 12") || slices.ContainsFunc(textos, func(x string) bool { return strings.HasPrefix(x, " · ") }) {
		t.Errorf("el avance no se pinta (sin título no hay « · »): %q", textos)
	}

	s2 := servicioCompleto()
	s2.presentacion = &api.CastPresentacion{Index: 0, Total: 4}
	b, w2 := appDePrueba(t, s2, false)
	b.AbrirVideo("v10")
	if !contiene(s2.vistas(), "GET /api/v1/public/videos/v10/slides") {
		t.Errorf("sin sesión se pregunta por la pública: %q", s2.vistas())
	}
	if !slices.Contains(rotulosYParrafos(w2), "Diapositiva 1 de 4") {
		t.Error("sin sesión también se lee la diapositiva")
	}
}

// TestLosCapitulosDeLaGrabacion: la grabación de un directo con marcas lista cada
// capítulo con su hora, y pulsarlo lleva el reproductor a ese instante. No se sondea.
func TestLosCapitulosDeLaGrabacion(t *testing.T) {
	s := servicioCompleto()
	s.mios = append(s.mios, grabacionDeDirecto())
	s.presentacion = &api.CastPresentacion{Index: 2, Total: 3, Marks: []api.CastMarca{
		{AtMS: 0, Index: 0, Title: "Portada"},
		{AtMS: 65_000, Index: 1, Title: "Plan"},
		{AtMS: 3_725_000, Index: 2},
	}}
	a, w := appDePrueba(t, s, true)
	a.AbrirVideo("v41")
	if a.hayQueSondearDiapositivas() {
		t.Error("la grabación no se sondea")
	}
	filas := capitulosEn(w)
	if len(filas) != 3 {
		t.Fatalf("hay %d capítulos y debería haber 3", len(filas))
	}
	nombres := []string{filas[0].AccessibilityLabel(), filas[1].AccessibilityLabel(), filas[2].AccessibilityLabel()}
	quiero := []string{"0:00 · Diapositiva 1 · Portada", "1:05 · Diapositiva 2 · Plan", "1:02:05 · Diapositiva 3"}
	if !slices.Equal(nombres, quiero) {
		t.Errorf("los capítulos son %q y deberían ser %q", nombres, quiero)
	}
	if !slices.Contains(rotulosYParrafos(w), "Diapositivas") {
		t.Error("falta el título de la sección")
	}
	m := &mandoFalso{dur: int64(40 * time.Minute)}
	a.visor.Mando = m
	test.Tap(filas[1])
	if got := time.Duration(m.pos); got != 65*time.Second {
		t.Errorf("pulsar «Plan» lleva a %v y debería a 1:05", got)
	}
}

// TestSinPresentacionNoSePintaNada: un directo sin presentación, una grabación sin
// marcas y un vídeo subido (que ni pregunta) no enseñan el bloque.
func TestSinPresentacionNoSePintaNada(t *testing.T) {
	s := servicioCompleto()
	s.mios = append(s.mios, grabacionDeDirecto())
	a, w := appDePrueba(t, s, true)
	a.AbrirVideo("v10")
	if slices.ContainsFunc(rotulosYParrafos(w), func(x string) bool { return strings.HasPrefix(x, "Diapositiva") }) {
		t.Error("un directo sin presentación no dice nada de diapositivas")
	}

	s.presentacion = &api.CastPresentacion{Index: 0, Total: 1}
	a.AbrirVideo("v41")
	if len(capitulosEn(w)) != 0 || slices.Contains(rotulosYParrafos(w), "Diapositivas") {
		t.Error("una grabación sin marcas no tiene capítulos")
	}

	a.AbrirVideo("v01")
	if contiene(s.vistas(), "GET /api/v1/videos/v01/slides") {
		t.Error("un vídeo subido no lleva presentación y no se pregunta")
	}
}
