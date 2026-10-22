// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestImagenDentroDeImagen: P saca el vídeo a su ventana con el MISMO reproductor (la
// posición y el sonido no se cortan) y la página lo dice; navegar no lo para; «volver»
// desde otra pantalla abre la del vídeo y se lo devuelve sonando; cerrar con la X lo
// devuelve a su sitio en pausa.
func TestImagenDentroDeImagen(t *testing.T) {
	a, _, vaciar, _ := verConEscalera(t)
	r := a.reproductor
	r.Reproducir()
	a.tecla(&fyne.KeyEvent{Name: fyne.KeyP})
	if a.pip == nil || a.pip.r != r || a.reproductor != nil {
		t.Fatalf("P no sacó el vídeo a su ventana con el mismo reproductor: pip=%v", a.pip)
	}
	if r.Pausado() {
		t.Fatal("sacar el vídeo lo paró")
	}
	if a.visor.aviso == nil || a.visor.Mando != r {
		t.Error("la página no dice que suena en imagen dentro de imagen o perdió sus mandos")
	}
	// Los fotogramas van a la ventana.
	a.pip.imagen.Image = nil
	vaciar("un fotograma en la ventana", func() bool { return a.pip.imagen.Image != nil })

	// Navegar no lo corta: sigue sonando y avanzando.
	a.Ir(RutaInicio)
	if a.pip == nil || a.pip.r != r {
		t.Fatal("navegar cerró la ventana")
	}
	// (El reloj de pared avanza aunque se cierre: lo que prueba que sigue vivo son los
	// fotogramas que llegan a la ventana después de navegar.)
	r.Buscar(0)
	a.pip.imagen.Image = nil
	vaciar("fotogramas tras navegar", func() bool { return a.pip.imagen.Image != nil })
	if r.Err() != nil || r.Pausado() {
		t.Fatalf("salir de Ver paró el reproductor de la ventana: %v", r.Err())
	}

	// «Volver» desde Inicio: abre la página del vídeo y se lo queda, sonando.
	r.Buscar(0)
	a.cerrarPiP(true)
	vaciar("la página del vídeo", func() bool { return a.ruta == RutaVer && a.reproductor != nil })
	if a.pip != nil || a.reproductor != r || a.visor.Mando != r {
		t.Fatalf("volver no devolvió el mismo reproductor: %p %p", a.reproductor, r)
	}
	if r.Pausado() {
		t.Error("volver lo dejó en pausa")
	}
	if a.visor.aviso != nil {
		t.Error("de vuelta, la página sigue diciendo que suena en otra ventana")
	}

	// Otra vez fuera, y cerrar con la X en su página: vuelve a su sitio, en pausa.
	a.alternarPiP()
	if a.pip == nil {
		t.Fatal("el botón no abrió la ventana")
	}
	a.cerrarPiP(false)
	if a.pip != nil || a.reproductor != r || !r.Pausado() || a.visor.aviso != nil {
		t.Errorf("cerrar: pip=%v mismo=%v pausado=%v", a.pip, a.reproductor == r, r.Pausado())
	}

	// Cerrar la app cierra la ventana y suelta el reproductor.
	a.alternarPiP()
	fyne.DoAndWait(a.Cerrar)
	if a.pip != nil || a.reproductor != nil {
		t.Error("cerrar la app dejó la ventana o el reproductor")
	}
}

// TestCapturasPiPYMenuDeCalidad pinta, en claro y en oscuro, la página con el menú de
// calidad abierto y la ventana de imagen dentro de imagen. Con SYTUE_CAPTURA=/ruta
// deja las capturas para MIRARLAS.
func TestCapturasPiPYMenuDeCalidad(t *testing.T) {
	for _, variante := range []tema.Variante{tema.Claro, tema.Oscuro} {
		nombre := map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[variante]
		t.Run(nombre, func(t *testing.T) {
			conVariante(t, variante)
			a, w, vaciar, _ := verConEscalera(t)
			a.v.Variante = variante
			a.visor.v.Variante = variante
			a.visor.Rehacer()
			engranaje := a.visor.Objeto()
			for _, o := range a.visor.barra.Objects {
				if b, ok := o.(*ui.BotonIcono); ok && b.Icono == iconos.Ajustes {
					engranaje = b
				}
			}
			a.menuCalidad(engranaje)
			vaciar("el menú", func() bool { return w.Canvas().Overlays().Top() != nil })
			guardar(t, "menu-calidad-"+nombre, w.Canvas().Capture())
			w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())

			a.reproductor.Buscar(time.Second)
			a.alternarPiP()
			if a.pip == nil {
				t.Fatal("no se abrió la ventana")
			}
			a.pip.imagen.Image = nil
			vaciar("un fotograma en la ventana", func() bool { return a.pip.imagen.Image != nil })
			img := a.pip.w.Canvas().Capture()
			guardar(t, "pip-ventana-"+nombre, img)
			if b := img.Bounds(); b.Dx() < 400 || b.Dy() < 250 {
				t.Errorf("la ventana mide %v", b)
			}
			guardar(t, "pip-pagina-"+nombre, w.Canvas().Capture())
			fyne.DoAndWait(a.Cerrar)
		})
	}
}

// TestImagenDentroDeImagenSoloBajoDemanda: en directo no se ofrece (como en la web).
func TestImagenDentroDeImagenSoloBajoDemanda(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.AbrirVideo(s.directos[0].ID)
	if a.visor.AlPiP != nil {
		t.Error("el directo ofrece imagen dentro de imagen")
	}
	a.alternarPiP()
	if a.pip != nil {
		t.Error("P abrió la ventana en un directo")
	}
}
