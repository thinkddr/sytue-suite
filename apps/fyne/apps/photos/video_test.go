// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestDuracionCorta(t *testing.T) {
	for ms, want := range map[int64]string{0: "", 400: "0:00", 7000: "0:07", 754_000: "12:34", 3_723_000: "1:02:03"} {
		if got := DuracionCorta(ms); got != want {
			t.Errorf("DuracionCorta(%d) = %q; quiero %q", ms, got, want)
		}
	}
}

// Los textos propios de una edición sustituyen a los del catálogo, con sus variables;
// sin ellos, el catálogo, como siempre.
func TestLosTextosPropiosSustituyenAlCatalogo(t *testing.T) {
	v := vistaDePrueba(t, nil)
	catalogo := v.Traduce("gallery.empty.description")
	if catalogo == "" || catalogo == "gallery.empty.description" {
		t.Fatal("la prueba necesita el texto del catálogo:", catalogo)
	}
	v.Textos = map[string]string{"gallery.empty.description": "Aquí, tus fotos.", "viewer.counter": "{index} de {total}"}
	if got := v.Traduce("gallery.empty.description"); got != "Aquí, tus fotos." {
		t.Fatal("el texto propio no sustituye:", got)
	}
	if got := v.Traduce("viewer.counter", i18n.Vars{"index": 3, "total": 9}); got != "3 de 9" {
		t.Fatal("las variables del texto propio:", got)
	}
	if got := v.Traduce("gallery.empty.title"); got == "" || got == "gallery.empty.title" {
		t.Fatal("lo que no se sustituye sale del catálogo:", got)
	}
}

// reproductorFalso cuenta cuántas veces se monta y se para, y si el Espacio llega.
type reproductorFalso struct {
	*canvas.Rectangle
	alternados int
}

func (r *reproductorFalso) Alternar() { r.alternados++ }

// Con App.Reproductor, un vídeo se ve en su reproductor (montado una vez por vídeo
// aunque el visor se rehaga), el Espacio va al reproductor y no al pase, pasar a otra
// foto o cerrar el visor lo para, y una foto sigue siendo una foto.
func TestElVisorMontaElReproductorDeUnVideo(t *testing.T) {
	fotos := fotosDelServicio(3)
	fotos[1].ContentType, fotos[1].DurationMs = "video/mp4", 83_000
	s := &servicioFalso{fotos: fotos, porPagina: 100}
	a, _ := appDePrueba(t, s)
	montados, parados := 0, 0
	var ultimo *reproductorFalso
	a.Reproductor = func(f api.Photo) (fyne.CanvasObject, func()) {
		montados++
		ultimo = &reproductorFalso{Rectangle: canvas.NewRectangle(nil)}
		return ultimo, func() { parados++ }
	}
	a.AvisoDeFoto = func(f api.Photo) string {
		if f.ID == fotos[1].ID {
			return "falta el original"
		}
		return ""
	}
	a.Arrancar()
	a.abrirVisor(desdeGaleria, posicion(a.g.fotos, fotos[0].ID))
	if montados != 0 || a.vis.medio != nil {
		t.Fatal("una foto no monta reproductor")
	}
	a.abrirVisor(desdeGaleria, posicion(a.g.fotos, fotos[1].ID))
	a.pintarVisor() // se rehace: el mismo reproductor
	if montados != 1 || a.vis.medio != ultimo {
		t.Fatalf("el vídeo debe montar UN reproductor: %d", montados)
	}
	a.runaVisor(' ')
	if ultimo.alternados != 1 || a.vis.pase {
		t.Fatal("el Espacio en un vídeo es reproducir o pausar, no el pase")
	}
	a.abrirVisor(desdeGaleria, posicion(a.g.fotos, fotos[2].ID))
	if parados != 1 || a.vis.medio != nil {
		t.Fatal("pasar a otra foto para el reproductor:", parados)
	}
	a.abrirVisor(desdeGaleria, posicion(a.g.fotos, fotos[1].ID))
	a.cerrarVisor()
	if montados != 2 || parados != 2 {
		t.Fatal("cerrar el visor para el reproductor:", montados, parados)
	}
}
