// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"strings"
	"testing"
)

const pngMarca = "data:image/png;base64,iVBORw0KGgo="

// TestLaMarcaDeAguaEsLaDeLaWeb son los vectores de `pagedecor.test.ts`: el saneado, el SVG
// con su giro alrededor de (397, 561.5), el texto escapado, y el estilo sin comillas dobles
// ni «<» crudos. Se vio en ROJO con el centro en enteros: salía `rotate(-45 397 561)`.
func TestLaMarcaDeAguaEsLaDeLaWeb(t *testing.T) {
	if m := SanearMarcaAgua(false, "X", "", "rojo", "", "", ""); m.Color != ColorMarcaAgua || m.Opacidad != 0.15 || m.Tamano != 120 {
		t.Errorf("saneado del texto: %+v", m)
	}
	if m := SanearMarcaAgua(true, "", pngMarca, "", "", "500", "1"); !m.Imagen || m.Tamano != 100 || m.Src != pngMarca || !m.Diagonal {
		t.Errorf("saneado de la imagen: %+v", m)
	}
	if SanearMarcaAgua(false, "  ", "", "", "", "", "") != nil || ImagenMarcaSegura("data:image/svg+xml;base64,PHN2Zz4=") {
		t.Error("sin texto no hay marca, y un SVG no es una imagen segura")
	}
	m := SanearMarcaAgua(false, "CONFIDENCIAL<script>", "", "", "", "", "1")
	svg := svgMarca(m)
	for _, quiero := range []string{`viewBox="0 0 794 1123"`, `transform="rotate(-45 397 561.5)"`, "&lt;script&gt;", `font-size="120"`} {
		if !strings.Contains(svg, quiero) {
			t.Errorf("el SVG no lleva %q: %s", quiero, svg)
		}
	}
	st := estiloMarca(m)
	if !strings.Contains(st, "opacity:0.15;") || !strings.Contains(st, "background-image:url('data:image/svg+xml,%3Csvg") ||
		!strings.Contains(st, "background-size:100% 1123px;") || strings.ContainsAny(st, `<"`) {
		t.Errorf("el estilo: %s", st)
	}
}

// TestLaDecoracionVaYVuelve: la marca y el borde son capas al principio del folio, con sus
// `data-*`, que se entienden, vuelven igual y se releen; quitarlas no deja nada. No son
// bloques: el documento sigue teniendo solo su texto.
func TestLaDecoracionVaYVuelve(t *testing.T) {
	d := Parse(`<p>a</p>`)
	d.PonerBorde(SanearBorde("dashed", "2", "#475569"))
	d.PonerMarcaAgua(SanearMarcaAgua(false, "Borrador", "", "", "0.25", "80", "0"))
	h := d.HTML()
	if !strings.HasPrefix(h, `<div data-sytue-watermark="" data-kind="text" data-color="#94A3B8" data-opacity="0.25" data-size="80" `+
		`data-diagonal="0" data-text="Borrador" contenteditable="false" aria-hidden="true" style="position:absolute;inset:0;z-index:-1;`) ||
		!strings.Contains(h, `<div data-sytue-page-border="" data-style="dashed" data-width="2" data-color="#475569" contenteditable="false" `+
			`aria-hidden="true" style="position:absolute;pointer-events:none;z-index:0;top:calc(var(--doc-page-margin, 96px) / 2);`) {
		t.Fatalf("las capas: %s", h)
	}
	otra := Parse(h)
	if !otra.Editable() || otra.HTML() != h || len(otra.Bloques()) != 1 {
		t.Fatalf("no vuelve igual o no se entiende (%v)", otra.NoEntendido())
	}
	if m := otra.MarcaAguaDe(); m == nil || m.Texto != "Borrador" || m.Opacidad != 0.25 || m.Diagonal {
		t.Errorf("la marca releída: %+v", m)
	}
	if b := otra.BordeDe(); b == nil || b.Estilo != "dashed" || b.Grosor != 2 {
		t.Errorf("el borde releído: %+v", b)
	}
	otra.PonerMarcaAgua(nil)
	otra.PonerBorde(nil)
	despues(t, "quitarlas", otra, `<p>a</p>`)
	if d := Parse(`<div data-sytue-watermark="" data-x="1"></div><p>a</p>`); d.Editable() {
		t.Error("una capa con un atributo que no es suyo no se edita")
	}
}

// TestLasColumnasComoLaWeb es `setColumns`: envuelve los bloques del folio que toca la
// selección, se ajusta si ya es un envoltorio y con 1 se deshace; los bloques de dentro lo
// saben. Una tabla dentro no se edita.
func TestLasColumnasComoLaWeb(t *testing.T) {
	d := Parse(`<p>a</p><p>b</p><p>c</p>`)
	if _, ok := d.Columnas(d.Bloques(), 0, 1, 2); !ok {
		t.Fatal("no se han puesto las columnas")
	}
	despues(t, "dos columnas", d, `<div class="sytue-cols" data-cols="2" style="column-count: 2; column-gap: 2em;"><p>a</p><p>b</p></div><p>c</p>`)
	if b := d.Bloques(); b[0].Columnas == nil || b[0].Columnas.N != 2 || b[0].Columnas != b[1].Columnas || b[2].Columnas != nil {
		t.Errorf("los bloques no saben de sus columnas: %+v", b)
	}
	d.Columnas(d.Bloques(), 0, 0, 3)
	despues(t, "tres", d, `<div class="sytue-cols" data-cols="3" style="column-count: 3; column-gap: 2em;"><p>a</p><p>b</p></div><p>c</p>`)
	d.Columnas(d.Bloques(), 1, 1, 1)
	despues(t, "deshacer", d, `<p>a</p><p>b</p><p>c</p>`)
	if d := Parse(`<div class="sytue-cols" data-cols="2" style="column-count: 2; column-gap: 2em;"><table><tbody><tr><td>x</td></tr></tbody></table></div>`); d.Editable() {
		t.Error("una tabla en columnas no se edita")
	}
}
