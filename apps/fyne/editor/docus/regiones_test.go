// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image/png"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// TestLasRegionesVanEnElMargen: el encabezado se asoma 30 px al margen de arriba con su
// caja de 20 + 6 + la raya, y el texto abajo (`flex-end`); el pie, 30 px por encima del
// borde de abajo del papel, con el texto arriba; los dos fuera de la corriente, y cada
// hijo separado por el `gap` de 8. Se vio en ROJO dejando el encabezado en la corriente
// (el cuerpo bajaba 18 px) y sin los huecos (el número iba pegado al texto).
func TestLasRegionesVanEnElMargen(t *testing.T) {
	cab := `<div data-doc-header="" style="` + rico.EstiloEncabezado + `">Acme<span data-page-field="" style="white-space:nowrap">Página 1 de 1</span></div>`
	pie := `<div data-doc-footer="" style="` + rico.EstiloPie + `"></div>`
	f := folioCon(t, cab+`<p>cuerpo</p>`+pie)
	h, cuerpo, p := f.cajas[0], f.cajas[1], f.cajas[2]
	if h.y != -30 || h.alto != 27 || h.arribaTexto != 2 {
		t.Errorf("el encabezado: y %g, alto %g, texto a %g; la web: -30, 27, 2", h.y, h.alto, h.arribaTexto)
	}
	if cuerpo.y != 0 {
		t.Errorf("el cuerpo empieza en %g y tiene que empezar arriba del contenido", cuerpo.y)
	}
	if quiero := AltoPagina - 2*MargenPagina + 30 - 27; p.y != quiero || p.arribaTexto != 7 {
		t.Errorf("el pie: y %g (quiero %g), texto a %g (quiero 7)", p.y, quiero, p.arribaTexto)
	}
	piezas := h.lineas[0].piezas
	if n := len(piezas); n != 3 || !piezas[1].atomo || piezas[2].texto != "status.page" {
		t.Fatalf("la línea del encabezado: %+v", piezas)
	}
	if hueco := piezas[2].x - (piezas[0].x + piezas[0].ancho); hueco != 8 {
		t.Errorf("entre «Acme» y el número de página hay %g y el `gap` es 8", hueco)
	}
	// Un clic en el margen de arriba cae en el encabezado; uno en el pie, en el pie.
	if q := f.enMaqueta(40, -10); q.b != 0 {
		t.Errorf("el clic en el encabezado cae en el bloque %d", q.b)
	}
	if q := f.enMaqueta(40, p.y+10); q.b != 2 {
		t.Errorf("el clic en el pie cae en el bloque %d", q.b)
	}
	// La raya: debajo del encabezado y encima del pie.
	for i, quiero := range map[int]float32{0: -4, 2: p.y} {
		objs := f.pintarRegion(nil, f.cajas[i], 0, 0, 1)
		if r, ok := objs[0].(*canvas.Rectangle); !ok || r.Position().Y != quiero || r.Size().Height != 1 {
			t.Errorf("la raya de la región %d: %v en %v", i, objs[0], quiero)
		}
	}
	// El pie vacío enseña su nombre a media tinta; el encabezado con texto, no.
	if n := len(f.pintarRegion(nil, f.cajas[2], 0, 0, 1)); n != 2 {
		t.Errorf("el pie vacío pinta %d objetos: la raya y su nombre", n)
	}
}

// TestEscribirEnElEncabezado: «Encabezado» lo crea y lleva el cursor dentro; ahí Intro no
// parte nada y un título no se aplica; «Número de página» deja el campo; y «Primera página
// distinta» lo esconde y saca el cursor de él.
func TestEscribirEnElEncabezado(t *testing.T) {
	f := folioCon(t, `<p>cuerpo</p>`)
	f.Mandar(editor.InsertarEncabezado, "")
	if f.cur.b != 0 || !f.enRegion() {
		t.Fatalf("el cursor no está en el encabezado: %v", f.cur)
	}
	f.escribir("Acme")
	f.intro()
	f.Mandar(editor.Titulo, "h1")
	f.Mandar(editor.InsertarRegla, "")
	f.Mandar(editor.InsertarNumeroPagina, "Página 1 de 1")
	quiero := `<div data-doc-header="" style="` + rico.EstiloEncabezado + `">Acme` +
		`<span data-page-field="" style="white-space:nowrap">Página 1 de 1</span></div><p>cuerpo</p>`
	if got := f.Serializar(); got != quiero {
		t.Errorf("el encabezado:\n  hay     %s\n  quería  %s", got, quiero)
	}
	f.Mandar(editor.PrimeraPaginaDistinta, "")
	if !f.bloques[0].Oculta || f.deTexto(0) || f.enRegion() {
		t.Errorf("escondido, el encabezado no admite el cursor: %v", f.cur)
	}
	if !strings.Contains(f.Serializar(), `data-first-page-different=""`) {
		t.Errorf("«Primera página distinta» no se guarda: %s", f.Serializar())
	}
}

// TestLasRegionesSePintan pinta el folio con encabezado y pie: un token inventado entraría
// en pánico al pintar. Con SYTUE_CAPTURA_REGIONES deja el PNG.
func TestLasRegionesSePintan(t *testing.T) {
	cab := `<div data-doc-header="" style="` + rico.EstiloEncabezado + `">Acme, S. L.<span data-page-field="" style="white-space:nowrap">Página 1 de 1</span></div>`
	pie := `<div data-doc-footer="" style="` + rico.EstiloPie + `"></div>`
	f := folioCon(t, cab+`<h1>Propuesta</h1><p>El cuerpo del documento.</p>`+pie)
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(container.NewStack(f))
	c.Resize(fyne.NewSize(900, 1300))
	img := c.Capture()
	if ruta := os.Getenv("SYTUE_CAPTURA_REGIONES"); ruta != "" {
		fh, err := os.Create(ruta)
		if err != nil {
			t.Fatal(err)
		}
		defer fh.Close()
		_ = png.Encode(fh, img)
	}
}
