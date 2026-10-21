// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Los estilos en línea son los de Insertar.tsx: se leen de allí para que un cambio en la
// web ponga esto en rojo en vez de dejar dos encabezados distintos.
func TestLosEstilosDeLasRegionesSonLosDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/office/src/editor/ribbon/tabs/Insertar.tsx")
	for nombre, quiero := range map[string]string{"HEADER_STYLE": EstiloEncabezado, "FOOTER_STYLE": EstiloPie} {
		_, resto, ok := strings.Cut(fuente, "const "+nombre+" =")
		trozo, _, ok2 := strings.Cut(resto, ";\n")
		if !ok || !ok2 {
			t.Fatalf("Insertar.tsx ya no declara %s", nombre)
		}
		var got strings.Builder
		for _, m := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(trozo, -1) {
			got.WriteString(m[1])
		}
		if got.String() != quiero {
			t.Errorf("%s:\n  web    %q\n  aquí   %q", nombre, got.String(), quiero)
		}
	}
	if !strings.Contains(fuente, `<span data-page-field style="`+EstiloCampoPagina+`">`) {
		t.Error("el número de página de Insertar.tsx ya no es el de aquí")
	}
}

// TestElEncabezadoYElPieDeLaWeb: lo que dejan `ensureRegion`, `insertPageField` y
// `toggleFirstPageDifferent` se entiende, vuelve igual y se describe como región, con la
// letra de 12 y la tinta de las regiones; cada hijo es un elemento del flex. Se vio en
// ROJO sin `data-first-page-different` entre los atributos entendidos: el documento se
// abría en solo lectura.
func TestElEncabezadoYElPieDeLaWeb(t *testing.T) {
	cab := `<div data-doc-header="" style="` + EstiloEncabezado + `" data-first-page-different="">Mi empresa` +
		`<span data-page-field="" style="white-space:nowrap">Página 1 de 1</span></div>`
	pie := `<div data-doc-footer="" style="` + EstiloPie + `"></div>`
	web := cab + `<p>cuerpo</p>` + pie
	d := Parse(web)
	if !d.Editable() || !d.Guardable() || d.HTML() != web {
		t.Fatalf("la web no vuelve igual o no se edita (%v): %s", d.NoEntendido(), d.HTML())
	}
	bs := d.Bloques()
	if len(bs) != 3 || bs[0].Region != Encabezado || !bs[0].Oculta || bs[1].Region != "" || bs[2].Region != Pie || !bs[2].Vacio() {
		t.Fatalf("las regiones: %+v", bs)
	}
	if e := bs[0].Tramos[0].Estilo; e.Tamano != 12 || e.Color != tintaRegion {
		t.Errorf("la letra de la región: %+v", e)
	}
	if tr := bs[0].Tramos[1]; !tr.CampoPagina || bs[0].Texto() != "Mi empresa\uFFFC" {
		t.Errorf("el número de página es UNA runa: %+v, %q", tr, bs[0].Texto())
	}
	if got := bs[0].CortesFlex(); !slices.Equal(got, []int{10}) {
		t.Errorf("los hijos del flex empiezan en %v", got)
	}
	for fuente, motivo := range map[string]string{
		`<div data-doc-header="" style="x"><p>a</p></div>`:            "div[data-doc-header] con bloques",
		`<div><div data-doc-footer="" style="x">a</div></div>`:        "div[data-doc-footer] dentro de otro",
		`<p><span data-page-field="" style="color: red">x</span></p>`: "style: color",
	} {
		if d := Parse(fuente); d.Editable() || !slices.Contains(d.NoEntendido(), motivo) {
			t.Errorf("%s: quería %q entre los motivos, hay %v", fuente, motivo, d.NoEntendido())
		}
	}
}

// TestCrearLasRegiones: el encabezado va lo primero, el pie delante de las notas al pie, y
// pedir uno que ya está no crea otro. En una región Intro no parte nada ni hay títulos.
func TestCrearLasRegiones(t *testing.T) {
	notas := `<section data-footnotes=""><hr><ol><li id="fn-1" value="1">n</li></ol></section>`
	d := Parse(`<p>a</p>` + notas)
	if _, ok := d.Region(Pie); !ok {
		t.Fatal("no se ha creado el pie")
	}
	l, ok := d.Region(Encabezado)
	if !ok || indice(d.Bloques(), l) != 0 {
		t.Fatal("el encabezado es el primer bloque")
	}
	d.Region(Encabezado)
	despues(t, "crear las dos", d, `<div data-doc-header="" style="`+EstiloEncabezado+`"></div><p>a</p>`+
		`<div data-doc-footer="" style="`+EstiloPie+`"></div>`+notas)
	e := abrir(t, d, 0)
	e.Sustituir(0, 0, "Acme ")
	abrir(t, d, 0).InsertarCampoPagina(5, "Página 1 de 1")
	if _, nuevo := abrir(t, d, 0).Partir(2); nuevo || abrir(t, d, 0).CambiarTipo("h1") {
		t.Error("en una región Intro no parte y no hay títulos")
	}
	if !d.AlternarPrimeraDistinta() || !d.Bloques()[0].Oculta {
		t.Error("«Primera página distinta» esconde el encabezado")
	}
	despues(t, "escribir en el encabezado", d, `<div data-doc-header="" style="`+EstiloEncabezado+`" data-first-page-different="">`+
		`Acme&nbsp;<span data-page-field="" style="white-space:nowrap">Página 1 de 1</span></div><p>a</p>`+
		`<div data-doc-footer="" style="`+EstiloPie+`"></div>`+notas)
	d.AlternarPrimeraDistinta()
	if d.Bloques()[0].Oculta {
		t.Error("y otra vez lo enseña")
	}
	if Parse(`<p>a</p>`).AlternarPrimeraDistinta() {
		t.Error("sin encabezado no hay nada que esconder")
	}
}
