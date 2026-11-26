// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func botonesDe(o fyne.CanvasObject) []string {
	var out []string
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			out = append(out, botonesDe(h)...)
		}
	case *ui.Boton:
		out = append(out, x.Texto)
	}
	return out
}

// TestSinServidorLaNavYElAlbumNoOfrecenLoDelServicio: en la edición local la nav no
// lleva «Compartido conmigo» (sí «Repetidas», que se calculan por huella en el
// dispositivo), y un álbum no ofrece enlace público. Con el valor de siempre, la
// conectada sigue como estaba.
func TestSinServidorLaNavYElAlbumNoOfrecenLoDelServicio(t *testing.T) {
	v := vistaDePrueba(t, nil)
	if n := Nav(v, SecFotos, nil); len(n.Objects) != 6 {
		t.Fatalf("la conectada tiene %d pestañas y son 6", len(n.Objects))
	}
	v.SinServidor = true
	n := Nav(v, SecFotos, nil)
	if len(n.Objects) != 5 {
		t.Fatalf("sin servidor la nav tiene %d pestañas y son 5", len(n.Objects))
	}
	repetidas := false
	for _, o := range n.Objects {
		r := o.(*PestanaNav).Rotulo
		if r == v.Traduce("sharedWithMe.title") {
			t.Errorf("sin servidor se ofrece %q", r)
		}
		repetidas = repetidas || r == v.Traduce("nav.duplicates")
	}
	if !repetidas {
		t.Error("sin servidor no se ofrecen las repetidas")
	}

	d := AlbumAbierto{Album: api.Album{ID: "a1", Title: "Verano"}, Fotos: []api.Photo{}, AlCompartir: func() {}}
	enlace := v.Traduce(compartirO(d.Album))
	conectada := vistaDePrueba(t, nil)
	if !tieneRotulo(botonesDe(PantallaAlbum(conectada, d)), enlace) {
		t.Fatalf("la conectada ha dejado de ofrecer %q", enlace)
	}
	if tieneRotulo(botonesDe(PantallaAlbum(v, d)), enlace) {
		t.Errorf("sin servidor un álbum ofrece %q", enlace)
	}
	if !tieneRotulo(botonesDe(PantallaAlbumes(conectada, Albumes{})), v.Traduce("albums.suggestionsFind")) {
		t.Fatal("la conectada ha dejado de ofrecer sugerencias de álbum")
	}
	if tieneRotulo(botonesDe(PantallaAlbumes(v, Albumes{})), v.Traduce("albums.suggestionsFind")) {
		t.Error("sin servidor se ofrecen sugerencias de álbum")
	}
}

// TestLaGaleriaOfreceExportarSoloSiSeLePide: la acción extra de la barra (Sytue Office la
// usa para «Exportar la fototeca») no existe con el valor cero.
func TestLaGaleriaOfreceExportarSoloSiSeLePide(t *testing.T) {
	v := vistaDePrueba(t, nil)
	fotos := []GrupoMes{{Titulo: "Julio", Fotos: []api.Photo{{ID: "f1", Name: "a.jpg", Width: 10, Height: 10}}}}
	if tieneRotulo(botonesDe(PantallaGaleria(v, Galeria{Grupos: fotos})), "Exportar") {
		t.Fatal("sin acción extra se ofrece exportar")
	}
	pulsado := false
	d := Galeria{Grupos: fotos, Exportar: &AccionExtra{Rotulo: "Exportar", Al: func() { pulsado = true }}}
	if !tieneRotulo(botonesDe(PantallaGaleria(v, d)), "Exportar") {
		t.Fatal("con acción extra no se ofrece")
	}
	d.Exportar.Al()
	if !pulsado {
		t.Fatal("no llama a su función")
	}
}

func tieneRotulo(l []string, s string) bool { return slices.Contains(l, s) }
