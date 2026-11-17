// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"testing"
)

// dibujoDeLaWeb es un «Dibujo» como lo deja el navegador tras `dibujoFigureHtml`: la escena
// en `data-escena`, su SVG y el PNG de respaldo oculto.
const dibujoDeLaWeb = `<figure data-dibujo="" data-dibujo-id="d1" data-escena="{&quot;v&quot;:1,&quot;ancho&quot;:400,&quot;alto&quot;:200,` +
	`&quot;titulo&quot;:&quot;Plano de la nave&quot;,&quot;capas&quot;:[]}" contenteditable="false" tabindex="0" style="margin:1em auto;width:100%">` +
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 200" width="100%" role="img" aria-label="Plano de la nave">` +
	`<title>Plano de la nave</title><rect x="10" y="10" width="100" height="50" fill="#2563EB"></rect><text x="20" y="40">Muelle</text></svg>` +
	`<img data-dibujo-respaldo="" hidden="" alt="Plano de la nave" width="400" height="200" data-w="400" data-h="200" ` +
	`src="data:image/png;base64,iVBORw0KGgo="></figure>`

// embedDeLaWeb es el bloque del informe de Power BI de `buildEmbedHtml`, ya sin la capa viva.
const embedDeLaWeb = `<div data-sytue-embed="" data-provider="powerbi" data-instance="pbi-1" data-report="ventas-q3" data-group="" ` +
	`contenteditable="false"><div class="sytue-embed-fallback"><span class="sytue-embed-fallback-title">Informe de Power BI</span>` +
	`<span class="sytue-embed-fallback-ref">ventas-q3</span><span class="sytue-embed-fallback-note">El contenido interactivo se muestra en el editor.</span>` +
	`</div></div>`

// TestElDibujoYElEmbedSonBloquesAtomicos: un dibujo y un informe de Power BI ya no dejan el
// documento en solo lectura; vuelven byte a byte sin leer su interior, y cada uno es UN
// bloque que no es de texto, con lo que hace falta para pintarlo. Se vio en ROJO sin
// reconocerlos en esFigura: «<figure>», «<svg>», «div[data-sytue-embed]»… no se entendían.
func TestElDibujoYElEmbedSonBloquesAtomicos(t *testing.T) {
	for _, c := range []struct {
		nombre, html string
		quiero       Atomico
	}{
		{"dibujo", dibujoDeLaWeb, Atomico{Clase: ClaseDibujo, Titulo: "Plano de la nave", Respaldo: "data:image/png;base64,iVBORw0KGgo=", Proporcion: 0.5}},
		{"embed", embedDeLaWeb, Atomico{Clase: ClaseEmbed, Titulo: "ventas-q3", Proveedor: "powerbi"}},
	} {
		doc := `<p>Antes</p>` + c.html + `<p>Después</p>`
		d := Parse(doc)
		if !d.Editable() || !d.Guardable() || d.HTML() != doc {
			t.Fatalf("%s: no se entiende (%v) o no vuelve igual:\n%s", c.nombre, d.NoEntendido(), d.HTML())
		}
		bs := d.Bloques()
		if len(bs) != 3 || bs[1].Tipo != Figura || bs[1].Tipo.EsTexto() || bs[1].Ilustracion.Atomico == nil {
			t.Fatalf("%s: bloques %+v", c.nombre, bs)
		}
		if got := *bs[1].Ilustracion.Atomico; got != c.quiero {
			t.Errorf("%s: leído %+v; quiero %+v", c.nombre, got, c.quiero)
		}
		if got := bs[1].Ilustracion.Clase(); got != c.quiero.Clase {
			t.Errorf("%s: clase %q", c.nombre, got)
		}
		// Retroceso desde el párrafo de detrás se lo lleva entero, como una forma.
		if _, _, ok := d.Fundir(bs, 2); !ok || d.HTML() != `<p>Antes</p><p>Después</p>` {
			t.Errorf("%s: Retroceso tras el bloque deja %s", c.nombre, d.HTML())
		}
	}
}

// TestElDibujoFueraDeSuSitioDiceQueEsUnDibujo: dentro de una lista no se pintaría como en la
// web, así que no se entiende; el motivo lleva la marca para que la franja diga «dibujos».
func TestElDibujoFueraDeSuSitioDiceQueEsUnDibujo(t *testing.T) {
	for _, c := range []struct{ html, motivo string }{
		{`<ul><li>` + dibujoDeLaWeb + `</li></ul>`, "figure[data-dibujo] dentro de otro"},
		{`<ul><li>` + embedDeLaWeb + `</li></ul>`, "div[data-sytue-embed] dentro de otro"},
	} {
		if d := Parse(c.html); d.Editable() || len(d.NoEntendido()) != 1 || d.NoEntendido()[0] != c.motivo {
			t.Errorf("motivos %v; quiero [%s]", d.NoEntendido(), c.motivo)
		}
	}
}

// TestLaProporcionDelDibujoSeAcota: de la escena; sin escena legible, del respaldo; y
// acotada, que una escena de 1 × 10 000 no haga un bloque de un kilómetro.
func TestLaProporcionDelDibujoSeAcota(t *testing.T) {
	for html, quiero := range map[string]float64{
		`<figure data-dibujo data-escena="{&quot;ancho&quot;:1,&quot;alto&quot;:10000}"></figure>`:                          proporcionMax,
		`<figure data-dibujo data-escena="{&quot;ancho&quot;:10000,&quot;alto&quot;:1}"></figure>`:                          proporcionMin,
		`<figure data-dibujo data-escena="no es json"><img data-dibujo-respaldo hidden data-w="300" data-h="150"></figure>`: 0.5,
		`<figure data-dibujo data-escena="{}"></figure>`:                                                                    0,
	} {
		bs := Parse(html).Bloques()
		if len(bs) != 1 || bs[0].Ilustracion == nil || bs[0].Ilustracion.Atomico == nil || bs[0].Ilustracion.Atomico.Proporcion != quiero {
			t.Errorf("%s: %+v; quiero proporción %v", html, bs, quiero)
		}
	}
}
