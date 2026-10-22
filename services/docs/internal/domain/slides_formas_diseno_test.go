// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

// pptxConDiseno es una presentación con un diseño (`atributos` de su p:sldLayout y lo que va
// en su spTree) que usan dos diapositivas; `retocar` cambia cada parte antes de importarla.
func pptxConDiseno(t *testing.T, atributos, diseno string, retocar ...func(string) string) *slidesModel {
	t.Helper()
	tipo := func(n string) string { return tipoR + n }
	diapo := `<p:sld ` + nsPres + `><p:cSld><p:spTree>` + marcador("2", `type="title"`, xfrmMarcador, `<a:lstStyle/><a:p><a:r><a:t>Hola</a:t></a:r></a:p>`) + `</p:spTree></p:cSld></p:sld>`
	files := map[string]string{
		"ppt/theme/theme1.xml": temaSin,
		"ppt/slideMasters/slideMaster1.xml": `<p:sldMaster ` + nsPres + `><p:cSld><p:spTree/></p:cSld>` +
			`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
			`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rL"/></p:sldLayoutIdLst></p:sldMaster>`,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rT" Type="` + tipo("theme") + `" Target="../theme/theme1.xml"/>` +
			`<Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`,
		"ppt/slideLayouts/slideLayout1.xml": `<p:sldLayout ` + nsPres + atributos + `><p:cSld name="Portada"><p:spTree>` + diseno + `</p:spTree></p:cSld></p:sldLayout>`,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="../slideMasters/slideMaster1.xml"/>` +
			`<Relationship Id="rImg" Type="` + tipo("image") + `" Target="../media/logo.png"/></Relationships>`,
		"ppt/media/logo.png":               string(pngBytes(t)),
		"ppt/slides/slide1.xml":            diapo,
		"ppt/slides/slide2.xml":            diapo,
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`,
		"ppt/slides/_rels/slide2.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`,
		"ppt/presentation.xml": `<p:presentation ` + nsPres + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rM"/></p:sldMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rS1"/><p:sldId id="257" r:id="rS2"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="slideMasters/slideMaster1.xml"/>` +
			`<Relationship Id="rS1" Type="` + tipo("slide") + `" Target="slides/slide1.xml"/><Relationship Id="rS2" Type="` + tipo("slide") + `" Target="slides/slide2.xml"/></Relationships>`,
	}
	for _, r := range retocar {
		for k, v := range files {
			files[k] = r(v)
		}
	}
	return parseSlidesModelJSON(t, importar1(t, "diseno.pptx", files))
}

// Las formas de un diseño que no son marcadores (una franja, un logo, una línea) se perdían:
// solo se leían sus marcadores. Llegan UNA vez al diseño —no a cada diapositiva, que repetiría
// sus imágenes—, con sus líneas y con que el diseño oculta el patrón, y vuelven a .pptx y a
// .odp en el diseño.
func TestFormasDelDiseno(t *testing.T) {
	franja := `<p:sp><p:nvSpPr><p:cNvPr id="5" name="Franja"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="12192000" cy="685800"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="1F3864"/></a:solidFill></p:spPr></p:sp>`
	logo := `<p:pic><p:nvPicPr><p:cNvPr id="6" name="Logo"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="rImg"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>` +
		`<p:spPr><a:xfrm><a:off x="11000000" y="6000000"/><a:ext cx="609600" cy="609600"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`
	linea := `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="7" name="Línea"/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr><p:spPr><a:xfrm><a:off x="609600" y="1371600"/><a:ext cx="10972800" cy="0"/></a:xfrm>` +
		`<a:prstGeom prst="line"><a:avLst/></a:prstGeom><a:ln w="12700"><a:solidFill><a:srgbClr val="C00000"/></a:solidFill></a:ln></p:spPr></p:cxnSp>`
	titulo := marcador("2", `type="title"`, xfrmMarcador, `<a:lstStyle/><a:p><a:r><a:t>Título</a:t></a:r></a:p>`)
	// Una forma oculta (`hidden`, como las de think-cell) no se pinta: PowerPoint tampoco.
	oculta := strings.Replace(franja, `name="Franja"`, `name="Oculta" hidden="1"`, 1)
	m := pptxConDiseno(t, ` showMasterSp="0"`, franja+logo+linea+titulo+oculta)
	comprobar := func(donde string, m *slidesModel) {
		t.Helper()
		if m.Master == nil || len(m.Master.Layouts) != 1 {
			t.Fatalf("%s: diseños = %+v", donde, m.Master)
		}
		l := m.Master.Layouts[0]
		if len(l.Boxes) != 1 || len(l.Decor) != 2 || len(l.Connectors) != 1 || !l.HideMaster {
			t.Fatalf("%s: diseño = %d marcadores, %d formas, %d líneas, oculta %v", donde, len(l.Boxes), len(l.Decor), len(l.Connectors), l.HideMaster)
		}
		if f := l.Decor[0]; f.Kind != "shape" || !strings.EqualFold(f.Fill, "#1F3864") || f.W < 99 {
			t.Errorf("%s: la franja = %+v", donde, f)
		}
		if g := l.Decor[1]; g.Kind != "image" || !strings.HasPrefix(g.Src, "data:image/") {
			t.Errorf("%s: el logo = %+v", donde, g)
		}
		if c := l.Connectors[0]; c.From.X == nil || c.To.X == nil || !strings.EqualFold(c.Stroke, "#C00000") {
			t.Errorf("%s: la línea = %+v", donde, c)
		}
		for _, s := range m.Slides {
			if len(s.Boxes) != 1 || s.Layout != l.ID {
				t.Errorf("%s: la diapositiva %s lleva %d cajas y el diseño %q: las formas son del diseño, no suyas", donde, s.ID, len(s.Boxes), s.Layout)
			}
		}
	}
	comprobar("pptx", m)
	for _, formato := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, ext, err := Export(KindPresentation, "x", stateWithContent(t, "doc:slides", jsonDe(t, *m)), formato)
		if err != nil {
			t.Fatal(err)
		}
		v := reimportar(t, "x."+ext, data)
		comprobar(ext+" a la vuelta", &v)
	}
	// Un diseño sin marcadores pero con formas también es un diseño (el «en blanco» con logo).
	if m := pptxConDiseno(t, "", logo); m.Master == nil || len(m.Master.Layouts) != 1 || len(m.Master.Layouts[0].Decor) != 1 {
		t.Errorf("el diseño en blanco con logo no llega")
	}
	// En OOXML estricto (ISO 29500 Strict) el patrón cita sus diseños con otro espacio de
	// nombres de relación: no llegaba ninguno, ni sus marcadores ni sus formas.
	estricto := strings.NewReplacer(nsRelaciones, nsRelacionesEstricto).Replace
	if m := pptxConDiseno(t, "", logo+titulo, estricto); m.Master == nil || len(m.Master.Layouts) != 1 || len(m.Master.Layouts[0].Decor) != 1 {
		t.Errorf("el diseño de un .pptx estricto no llega: %+v", m.Master)
	}
}

// Una diapositiva de otro patrón (el modelo tiene uno) trae su diseño a la lista, con las
// formas de su patrón debajo de las suyas y tapando el del modelo; si un diseño del primero
// ya se llama igual, el nombre lleva el del patrón. Decisión del dueño (2026-10-16).
func TestDisenosDeOtroPatron(t *testing.T) {
	tipo := func(n string) string { return tipoR + n }
	rels := func(r ...string) string {
		return `<Relationships ` + nsRels + `>` + strings.Join(r, "") + `</Relationships>`
	}
	rel := func(id, t, destino string) string {
		return `<Relationship Id="` + id + `" Type="` + tipo(t) + `" Target="` + destino + `"/>`
	}
	forma := func(id, nombre, color string) string {
		return `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name="` + nombre + `"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/>` +
			`<a:ext cx="12192000" cy="685800"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill></p:spPr></p:sp>`
	}
	titulo := marcador("2", `type="title"`, xfrmMarcador, `<a:lstStyle/><a:p><a:r><a:t>T</a:t></a:r></a:p>`)
	patron := func(idDiseno, formas string) string {
		return `<p:sldMaster ` + nsPres + `><p:cSld><p:spTree>` + formas + `</p:spTree></p:cSld>` +
			`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
			`<p:sldLayoutIdLst><p:sldLayoutId id="` + idDiseno + `" r:id="rL"/></p:sldLayoutIdLst></p:sldMaster>`
	}
	diseno := func(formas string) string {
		return `<p:sldLayout ` + nsPres + `><p:cSld name="Título"><p:spTree>` + titulo + formas + `</p:spTree></p:cSld></p:sldLayout>`
	}
	diapo := `<p:sld ` + nsPres + `><p:cSld><p:spTree>` + titulo + `</p:spTree></p:cSld></p:sld>`
	files := map[string]string{
		"ppt/theme/theme1.xml":                         temaSin,
		"ppt/theme/theme2.xml":                         strings.Replace(temaSin, `name="Prueba"`, `name="Oscuro"`, 1),
		"ppt/slideMasters/slideMaster1.xml":            patron("2147483649", ""),
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": rels(rel("rT", "theme", "../theme/theme1.xml"), rel("rL", "slideLayout", "../slideLayouts/slideLayout1.xml")),
		"ppt/slideMasters/slideMaster2.xml":            patron("2147483651", forma("3", "Banda", "1F3864")),
		"ppt/slideMasters/_rels/slideMaster2.xml.rels": rels(rel("rT", "theme", "../theme/theme2.xml"), rel("rL", "slideLayout", "../slideLayouts/slideLayout2.xml")),
		"ppt/slideLayouts/slideLayout1.xml":            diseno(""),
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": rels(rel("rM", "slideMaster", "../slideMasters/slideMaster1.xml")),
		"ppt/slideLayouts/slideLayout2.xml":            diseno(forma("4", "Logo", "C00000")),
		"ppt/slideLayouts/_rels/slideLayout2.xml.rels": rels(rel("rM", "slideMaster", "../slideMasters/slideMaster2.xml")),
		"ppt/slides/slide1.xml":                        diapo,
		"ppt/slides/slide2.xml":                        diapo,
		"ppt/slides/_rels/slide1.xml.rels":             rels(rel("rL", "slideLayout", "../slideLayouts/slideLayout1.xml")),
		"ppt/slides/_rels/slide2.xml.rels":             rels(rel("rL", "slideLayout", "../slideLayouts/slideLayout2.xml")),
		"ppt/presentation.xml": `<p:presentation ` + nsPres + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rM1"/><p:sldMasterId id="2147483650" r:id="rM2"/></p:sldMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rS1"/><p:sldId id="257" r:id="rS2"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": rels(rel("rM1", "slideMaster", "slideMasters/slideMaster1.xml"), rel("rM2", "slideMaster", "slideMasters/slideMaster2.xml"),
			rel("rS1", "slide", "slides/slide1.xml"), rel("rS2", "slide", "slides/slide2.xml")),
	}
	comprobar := func(donde string, m *slidesModel) {
		t.Helper()
		if m.Master == nil || len(m.Master.Layouts) != 2 || len(m.Slides) != 2 {
			t.Fatalf("%s: diseños %+v", donde, m.Master)
		}
		uno, otro := m.Master.Layouts[0], m.Master.Layouts[1]
		if uno.Name != "Título" || otro.Name != "Título · Oscuro" {
			t.Errorf("%s: nombres %q y %q", donde, uno.Name, otro.Name)
		}
		if !otro.HideMaster || len(otro.Decor) != 2 || !strings.EqualFold(otro.Decor[0].Fill, "#1F3864") || !strings.EqualFold(otro.Decor[1].Fill, "#C00000") {
			t.Errorf("%s: el diseño del otro patrón tapa %v y lleva %+v (la banda de su patrón debajo del logo)", donde, otro.HideMaster, otro.Decor)
		}
		if s := m.Slides[0]; s.Layout != uno.ID || s.HideMaster {
			t.Errorf("%s: la 1 lleva %q y tapa %v", donde, s.Layout, s.HideMaster)
		}
		if s := m.Slides[1]; s.Layout != otro.ID || s.HideMaster {
			t.Errorf("%s: la 2 lleva %q y tapa %v: el que tapa el patrón es su diseño", donde, s.Layout, s.HideMaster)
		}
	}
	m := parseSlidesModelJSON(t, importar1(t, "patrones.pptx", files))
	comprobar("pptx", m)
	data, _, _, err := Export(KindPresentation, "x", stateWithContent(t, "doc:slides", jsonDe(t, *m)), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	v := reimportar(t, "x.pptx", data)
	comprobar("pptx a la vuelta", &v)
}
