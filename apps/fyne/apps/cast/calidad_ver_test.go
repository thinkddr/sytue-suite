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
)

func TestDecidirAutoPorTamanoYRed(t *testing.T) {
	tres := []int{1080, 720, 360}
	for _, c := range []struct {
		nombre string
		alto   float32
		quiere int
	}{
		{"un cuadro pequeño no baja 1080", 300, 360},
		{"360 justo", 360, 360},
		{"un poco más de 360: 720", 420, 720},
		{"720 cubre hasta ~790", 780, 720},
		{"pantalla completa en 1080", 1080, 1080},
		{"un monitor de 1440: la mayor que hay", 1440, 1080},
	} {
		if got := DecidirAuto(c.alto, tres, 0, 0, 0); got != c.quiere {
			t.Errorf("%s: alto %v → %d, se esperaba %d", c.nombre, c.alto, got, c.quiere)
		}
	}
	if got := DecidirAuto(1080, []int{720}, 0, 0, 0); got != 720 {
		t.Errorf("con una sola, esa: %d", got)
	}
	if got := DecidirAuto(500, nil, 0, 0, 0); got != 0 {
		t.Errorf("sin calidades, ninguna: %d", got)
	}
	// La red: suena 720p a 1900 kbps. Para 1080 harían falta ~4300 (×1,3): con 3000
	// no llega y se queda en 720; con 2000 tampoco llega a 720 y baja a 360.
	if got := DecidirAuto(1080, tres, 3000, 720, 1900); got != 720 {
		t.Errorf("red de 3 Mbit/s a pantalla completa: %d", got)
	}
	if got := DecidirAuto(1080, tres, 2000, 720, 1900); got != 360 {
		t.Errorf("red de 2 Mbit/s: %d", got)
	}
	if got := DecidirAuto(1080, tres, 50_000, 720, 1900); got != 1080 {
		t.Errorf("red de sobra: %d", got)
	}
}

// verConEscalera abre la página de un vídeo con los tres WebM y espera al primer
// fotograma; devuelve la app, la cola de la interfaz y el servicio.
func verConEscalera(t *testing.T) (*App, fyne.Window, func(string, func() bool), *servicioFalso) {
	t.Helper()
	s := servicioCompleto()
	s.escalera = true
	a, w := appDePrueba(t, s, true)
	vaciar := colaDeInterfaz(t, a)
	a.AbrirMedia = nil // el cliente de verdad: HEAD y Range contra el servicio falso
	a.AbrirVideo("v01")
	vaciar("el primer fotograma", func() bool {
		return a.reproductor != nil && a.visor.imagen.Image != nil && a.visor.Mando != nil
	})
	return a, w, vaciar, s
}

// TestCambiarDeCalidadConservaPosicionYEstado: el menú lista «Automática» y las tres;
// elegir una abre SU WebM, lo busca en el instante en que iba el otro y conserva la
// pausa; con el vídeo sonando, el nuevo sigue sonando.
func TestCambiarDeCalidadConservaPosicionYEstado(t *testing.T) {
	a, w, vaciar, s := verConEscalera(t)
	if a.visor.AlCalidad == nil || a.visor.AlPiP == nil {
		t.Fatal("con tres calidades el visor tiene menú de calidad e imagen dentro de imagen")
	}
	// El menú, colgado de su botón.
	a.menuCalidad(a.visor.Objeto())
	menu := textosEnCapa(w)
	for _, quiero := range []string{"Automática", "1080p", "720p", "360p"} {
		if !strings.Contains(menu, quiero) {
			t.Errorf("el menú no tiene «%s»: %q", quiero, menu)
		}
	}
	w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())

	viejo := a.reproductor
	viejo.Buscar(time.Second)
	vaciar("el segundo 1", func() bool { return viejo.Posicion() >= time.Second })
	a.elegirCalidad(360)
	vaciar("el WebM de 360p", func() bool { return a.reproductor != viejo && a.reproductor != nil })
	nuevo := a.reproductor
	if p := nuevo.Posicion(); p < time.Second-50*time.Millisecond || p > time.Second+500*time.Millisecond {
		t.Errorf("cambiar de calidad no conservó la posición: %v", p)
	}
	if !nuevo.Pausado() || a.ver.enUso != 360 || a.ver.calidad != 360 || a.visor.Mando != nuevo {
		t.Errorf("el nuevo: pausado=%v en uso=%d elegida=%d", nuevo.Pausado(), a.ver.enUso, a.ver.calidad)
	}
	s.mu.Lock()
	medios := slices.Clone(s.medios)
	s.mu.Unlock()
	if !slices.ContainsFunc(medios, func(m string) bool { return strings.HasSuffix(m, "/webm/360p.webm") }) {
		t.Errorf("no se pidió el WebM de 360p: %q", medios)
	}

	// Sonando: el nuevo sigue sonando desde donde iba.
	nuevo.Reproducir()
	a.elegirCalidad(1080)
	vaciar("el WebM de 1080p", func() bool { return a.reproductor != nuevo && a.reproductor != nil })
	if a.reproductor.Pausado() {
		t.Error("cambiar de calidad sonando lo dejó en pausa")
	}
	if p := a.reproductor.Posicion(); p < time.Second-50*time.Millisecond {
		t.Errorf("volvió atrás al cambiar: %v", p)
	}
	a.Ir(RutaInicio)
}

// TestAutomaticaEligePorElTamano: con «Automática», un cuadro pequeño pide 360p y a
// pantalla completa (un cuadro de 1080 de alto) 1080p.
func TestAutomaticaEligePorElTamano(t *testing.T) {
	a, _, vaciar, _ := verConEscalera(t)
	if a.ver.calidad != 0 {
		t.Fatal("se empieza en Automática")
	}
	a.visor.medido = fyne.NewSize(480, 270)
	if q := a.objetivoAuto(); q != 360 {
		t.Errorf("un cuadro de 270 de alto: %d", q)
	}
	a.visor.medido = fyne.NewSize(1920, 1080)
	if q := a.objetivoAuto(); q != 1080 {
		t.Errorf("a pantalla completa: %d", q)
	}
	viejo := a.reproductor
	a.ver.ultimoAuto = time.Time{}
	a.reevaluarAuto()
	vaciar("el WebM de 1080p", func() bool { return a.reproductor != viejo && a.reproductor != nil })
	if a.ver.enUso != 1080 || a.ver.calidad != 0 {
		t.Errorf("Automática cambió a %d (elegida %d)", a.ver.enUso, a.ver.calidad)
	}
	// Sin dejar pasar la pausa, no vuelve a cambiar aunque cambie el tamaño.
	a.visor.medido = fyne.NewSize(480, 270)
	antes := a.reproductor
	a.reevaluarAuto()
	if a.reproductor != antes || a.ver.cambiando {
		t.Error("Automática cambió dos veces seguidas")
	}
	a.Ir(RutaInicio)
}

// textosEnCapa junta los rótulos del menú abierto.
func textosEnCapa(w fyne.Window) string {
	var b strings.Builder
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.ElementoMenu:
			b.WriteString(x.Texto + "\n")
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	for _, c := range w.Canvas().Overlays().List() {
		mirar(c)
	}
	return b.String()
}
