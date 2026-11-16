// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// pngRojo es un PNG de w × h de un rojo puro, como data: URL.
func pngRojo(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: 0xff, A: 0xff})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

// dibujoCon es el bloque «Dibujo» de la web con una escena de 400 × 200 y ese respaldo.
func dibujoCon(respaldo string) string {
	img := ""
	if respaldo != "" {
		img = `<img data-dibujo-respaldo="" hidden="" alt="Plano" width="400" height="200" data-w="400" data-h="200" src="` + respaldo + `">`
	}
	return `<figure data-dibujo="" data-escena="{&quot;v&quot;:1,&quot;ancho&quot;:400,&quot;alto&quot;:200,&quot;titulo&quot;:&quot;Plano&quot;,&quot;capas&quot;:[]}" ` +
		`contenteditable="false" tabindex="0" style="margin:1em auto;width:100%"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 200" width="100%">` +
		`<rect width="400" height="200" fill="#FF0000"></rect></svg>` + img + `</figure>`
}

const embedPowerBI = `<div data-sytue-embed="" data-provider="powerbi" data-instance="pbi-1" data-report="ventas-q3" data-group="" ` +
	`contenteditable="false"><div class="sytue-embed-fallback"><span class="sytue-embed-fallback-title">Informe de Power BI</span>` +
	`<span class="sytue-embed-fallback-ref">ventas-q3</span></div></div>`

// TestElDibujoYElEmbedNoBloqueanElDocumento: con un dibujo o un informe de Power BI en el
// cuerpo, el folio se edita; escribir antes y después del bloque no lo toca (vuelve byte a
// byte), Supr al final del párrafo de antes se lo lleva entero y deshacer lo devuelve. Se
// vio en ROJO antes de tratarlos como bloques atómicos: Cargar decía que no los entendía.
func TestElDibujoYElEmbedNoBloqueanElDocumento(t *testing.T) {
	for nombre, bloque := range map[string]string{"dibujo": dibujoCon(pngRojo(t, 8, 4)), "embed": embedPowerBI} {
		doc := `<p>Antes</p>` + bloque + `<p>Después</p>`
		f := folioCon(t, doc)
		if !f.doc.Editable() || f.ctx.MotivoSoloLectura != "" {
			t.Fatalf("%s: el documento no se edita: %v", nombre, f.doc.NoEntendido())
		}
		if got := f.Serializar(); got != doc {
			t.Fatalf("%s: sin tocar no vuelve igual:\n%s", nombre, got)
		}
		f.mover(punto{0, len("Antes")}, false)
		f.escribir("!")
		f.mover(punto{2, 0}, false)
		f.escribir("¡")
		if got, quiero := f.Serializar(), `<p>Antes!</p>`+bloque+`<p>¡Después</p>`; got != quiero {
			t.Fatalf("%s: escribir alrededor lo toca:\n%s", nombre, got)
		}
		f.mover(punto{0, len("Antes!")}, false)
		f.suprimir()
		if got := f.Serializar(); got != `<p>Antes!</p><p>¡Después</p>` {
			t.Fatalf("%s: Supr al final del párrafo de antes deja %s", nombre, got)
		}
		f.deshacer()
		if got, quiero := f.Serializar(), `<p>Antes!</p>`+bloque+`<p>¡Después</p>`; got != quiero {
			t.Errorf("%s: deshacer no lo devuelve igual:\n%s", nombre, got)
		}
	}
}

// TestElDibujoSePintaConSuRespaldo: el PNG de respaldo, estirado a lo ancho del folio con la
// proporción de la escena (400 × 200: medio ancho de alto), es lo que se ve en su centro.
func TestElDibujoSePintaConSuRespaldo(t *testing.T) {
	f := folioCon(t, `<p>Antes</p>`+dibujoCon(pngRojo(t, 8, 4))+`<p>Después</p>`)
	c := f.cajas[1]
	if c.b.Tipo != rico.Figura || abs(c.alto-c.ancho/2) > 0.01 {
		t.Fatalf("la caja del dibujo mide %v × %v y es de medio ancho de alto", c.ancho, c.alto)
	}
	img, en := capturaFolio(t, f, "SYTUE_CAPTURA_DIBUJO")
	x, y := en(c.izq+c.ancho/2, c.y+c.alto/2)
	if r, g, b, _ := img.At(x, y).RGBA(); r>>8 < 0xe0 || g>>8 > 0x30 || b>>8 > 0x30 {
		t.Errorf("en el centro del dibujo hay %v y es el rojo del respaldo", img.At(x, y))
	}
}

// TestSinRespaldoSePintaLaTarjeta: el informe de Power BI (460 de alto, como `--embed-h`) y
// un dibujo sin respaldo, o con uno que pasa del tope de la web, son una tarjeta con su
// título y «se edita en la web»; el dibujo, del tamaño que tendría.
func TestSinRespaldoSePintaLaTarjeta(t *testing.T) {
	enorme := "data:image/png;base64," + strings.Repeat("A", MaxImagen)
	for _, c := range []struct {
		nombre, bloque string
		textos         []string
		alto           func(ancho float32) float32
	}{
		{"embed", embedPowerBI, []string{"embed.powerbi.title", "ventas-q3", "embed.editOnWeb"}, func(float32) float32 { return altoEmbed }},
		{"dibujo sin respaldo", dibujoCon(""), []string{"Plano", "embed.editOnWeb"}, func(a float32) float32 { return a / 2 }},
		{"dibujo con respaldo enorme", dibujoCon(enorme), []string{"Plano", "embed.editOnWeb"}, func(a float32) float32 { return a / 2 }},
	} {
		f := folioCon(t, `<p>x</p>`+c.bloque)
		caja := f.cajas[1]
		if abs(caja.alto-c.alto(caja.ancho)) > 0.01 {
			t.Errorf("%s: mide %v de alto en %v de ancho", c.nombre, caja.alto, caja.ancho)
		}
		var textos []string
		for _, o := range f.pintarContenido(nil, fyne.Position{}, 0, 0, 1) {
			if tx, ok := o.(*canvas.Text); ok {
				textos = append(textos, tx.Text)
			}
		}
		for _, q := range c.textos {
			if !slices.Contains(textos, q) {
				t.Errorf("%s: falta %q en la tarjeta: %v", c.nombre, q, textos)
			}
		}
	}
	_, _ = capturaFolio(t, folioCon(t, `<p>x</p>`+embedPowerBI), "SYTUE_CAPTURA_EMBED")
}

// TestLaProporcionDelRespaldoSeAcota: sin medidas en la escena, la caja toma la proporción
// del PNG de respaldo, pero acotada como la de la escena: un respaldo de 2 × 200 no hace un
// bloque de cien folios de alto. Se vio en ROJO con la proporción del PNG sin acotar.
func TestLaProporcionDelRespaldoSeAcota(t *testing.T) {
	bloque := `<figure data-dibujo="" data-escena="{}" contenteditable="false">` +
		`<img data-dibujo-respaldo="" hidden="" src="` + pngRojo(t, 2, 200) + `"></figure>`
	f := folioCon(t, `<p>x</p>`+bloque)
	if c := f.cajas[1]; abs(c.alto-4*c.ancho) > 0.01 {
		t.Errorf("el dibujo mide %v de alto en %v de ancho; quiero cuatro anchos, el tope", c.alto, c.ancho)
	}
}
