// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// adjuntoDe es la ficha de un adjunto tal como la manda el servicio.
func adjuntoDe(id, registro, mime string, miniatura bool) m {
	return m{"id": id, "record_id": registro, "field_id": "fot", "name": id + ".bin", "mime": mime, "size": 10, "has_thumb": miniatura}
}

// La portada es la primera imagen CON miniatura, por el orden de los campos: ni un PDF
// ni una imagen de la que el servicio no sacó miniatura (la tarjeta nunca baja el original).
func TestLaPortadaEsLaPrimeraImagenConMiniatura(t *testing.T) {
	docs := api.BasesField{ID: "d", Kind: api.BasesKindAttachment}
	fotos := api.BasesField{ID: "f", Kind: api.BasesKindAttachment}
	r := api.BasesRecord{Attachments: map[string][]api.BasesAttachment{
		"d": {{ID: "pdf", Mime: "application/pdf", HasThumb: true}, {ID: "grande", Mime: "image/png"}},
		"f": {{ID: "a", Mime: "image/jpeg", HasThumb: true}},
	}}
	campos := []api.BasesField{{ID: "n", Kind: api.BasesKindText}, docs, fotos}
	if a, hay := portadaDe(campos, r); !hay || a.ID != "a" {
		t.Errorf("portada: %+v %v", a, hay)
	}
	r.Attachments["d"] = append(r.Attachments["d"], api.BasesAttachment{ID: "c", Mime: "image/gif", HasThumb: true})
	if a, _ := portadaDe(campos, r); a.ID != "c" {
		t.Errorf("manda el orden de los campos: %q", a.ID)
	}
	if _, hay := portadaDe(campos, api.BasesRecord{}); hay {
		t.Error("un registro sin adjuntos no tiene portada")
	}
}

// Una vista de galería pinta una tarjeta por registro con la MINIATURA de su primera
// imagen (o el hueco), y la tarjeta entera abre la ficha.
func TestLaGaleriaEnsenaPortadasYAbreLaFicha(t *testing.T) {
	f := nuevoFalso(3)
	t1 := f.tabla("t1")
	t1["fields"] = append(t1["fields"].([]m), campoDe("fot", "t1", "Fotos", "attachment", nil))
	f.registros["t1"][0]["attachments"] = m{"fot": []m{adjuntoDe("pdf", "r001", "application/pdf", false), adjuntoDe("a1", "r001", "image/jpeg", true)}}
	f.registros["t1"][1]["attachments"] = m{"fot": []m{adjuntoDe("a2", "r002", "image/png", false)}}
	f.vistas["t1"] = []m{{"id": "vg", "table_id": "t1", "name": "Fotos", "kind": "gallery", "config": m{}}}
	p, w := abrir(t, f)
	elegir(t, w, "Vistas", "Fotos · Galería")
	if !f.vio("GET /tables/t1/records?limit=100&view=vg") {
		t.Fatalf("la galería no pide los registros de su vista: %q", f.peticiones("GET /tables/t1/records"))
	}
	// La segunda vuelta de pintar ya tiene la miniatura (la primera la pidió).
	p.Repintar()
	capturar(t, w, "galeria")
	if got := f.peticiones("GET /records/"); len(got) != 1 || got[0] != "GET /records/r001/attachments/a1/thumb" {
		t.Errorf("solo se pide la miniatura de la portada, una vez: %q", got)
	}
	imagenes, tarjetas := 0, map[string]*ui.FilaPulsable{}
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if im, ok := o.(*canvas.Image); ok && im.Image != nil && im.FillMode == canvas.ImageFillCover {
			imagenes++
			// La tarjeta es `rounded-lg overflow-hidden`: la portada no asoma por sus
			// esquinas de arriba.
			b := im.Image.Bounds()
			for _, x := range []int{b.Min.X, b.Max.X - 1} {
				if _, _, _, a := im.Image.At(x, b.Min.Y).RGBA(); a != 0 {
					t.Errorf("la esquina de arriba de la portada (x=%d) asoma con alfa %d", x, a)
				}
			}
		}
		if r, ok := o.(*canvas.Rectangle); ok && r.FillColor == tema.Color("bg-subtle", tema.Claro) &&
			(r.TopLeftCornerRadius != tema.RadioLG || r.TopRightCornerRadius != tema.RadioLG) {
			t.Errorf("el hueco de la portada no lleva las esquinas de la tarjeta: %v %v", r.TopLeftCornerRadius, r.TopRightCornerRadius)
		}
		if b, ok := o.(*ui.FilaPulsable); ok {
			tarjetas[b.Nombre] = b
		}
	})
	if imagenes != 1 {
		t.Errorf("una portada y dos huecos; hay %d imágenes", imagenes)
	}
	for _, n := range []string{"Persona 1", "Persona 2", "Persona 3"} {
		if tarjetas["Abrir "+n] == nil {
			t.Errorf("falta la tarjeta de %s: %v", n, tarjetas)
		}
	}
	got := textos(w)
	if !strings.Contains(got, "Edad: 22") || strings.Contains(got, "Intro o F2 edita una celda") {
		t.Errorf("la galería no enseña sus tarjetas o pinta la rejilla:\n%s", got)
	}

	test.Tap(tarjetas["Abrir Persona 2"])
	abierta := false
	enCapas(w, func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.Entrada); ok && e.Rotulo == "Nombre" && e.Texto() == "Persona 2" {
			abierta = true
		}
	})
	if !abierta {
		t.Error("pulsar la tarjeta no abre la ficha del registro")
	}
}

// Crear una galería desde el diálogo: el tipo se ofrece, dice qué es la portada y se
// guarda con su filtro y su orden, como la rejilla.
func TestCrearUnaGaleria(t *testing.T) {
	f := nuevoFalso(2)
	_, w := abrir(t, f)
	pulsarEnMenu(t, w, "Personas", "Añadir vista")
	campoDeDialogo(t, w, "Nombre de la vista").PonerTexto("Caras")
	elegir(t, w, "Tipo", "Galería")
	if !strings.Contains(textos(w), "primera imagen adjunta") {
		t.Error("el diálogo no dice qué sale de portada")
	}
	elegir(t, w, "Campo por el que ordenar", "Nombre")
	pulsar(t, w, "Crear vista")
	if !f.vio(`POST /tables/t1/views {"config":{"sort":[{"field":"nom","dir":"asc"}]},"kind":"gallery","name":"Caras"}`) {
		t.Fatalf("la galería no se ha creado como se pidió: %q", f.peticiones("POST /tables/t1/views"))
	}
}
