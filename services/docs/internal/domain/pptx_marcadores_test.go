// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// reParrafoHTML es un párrafo del HTML de una caja, con su etiqueta y su estilo.
var reParrafoHTML = regexp.MustCompile(`<(p|h[1-6]|li)\b[^>]*>.*?</(?:p|h[1-6]|li)>`)

// pptxConMarcadores es una presentación de `ancho` EMU con un patrón (`master`: lo que va
// dentro de <p:spTree> y, detrás de </p:cSld>, sus `p:txStyles`), un diseño (`diseno`: su
// spTree) y una diapositiva (su spTree) que lo usa.
func pptxConMarcadores(t *testing.T, ancho int, master, txStyles, diseno, diapo string) *slidesModel {
	t.Helper()
	tipo := func(n string) string { return tipoR + n }
	files := map[string]string{
		"ppt/theme/theme1.xml": temaSin,
		"ppt/slideMasters/slideMaster1.xml": `<p:sldMaster ` + nsPres + `><p:cSld><p:spTree>` + master + `</p:spTree></p:cSld>` +
			`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
			`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rL"/></p:sldLayoutIdLst>` + txStyles + `</p:sldMaster>`,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rT" Type="` + tipo("theme") + `" Target="../theme/theme1.xml"/>` +
			`<Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`,
		"ppt/slideLayouts/slideLayout1.xml":            `<p:sldLayout ` + nsPres + `><p:cSld name="Contenido"><p:spTree>` + diseno + `</p:spTree></p:cSld></p:sldLayout>`,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="../slideMasters/slideMaster1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":                        `<p:sld ` + nsPres + `><p:cSld><p:spTree>` + diapo + `</p:spTree></p:cSld></p:sld>`,
		"ppt/slides/_rels/slide1.xml.rels":             `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout1.xml"/></Relationships>`,
		"ppt/presentation.xml": `<p:presentation ` + nsPres + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rM"/></p:sldMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rS"/></p:sldIdLst><p:sldSz cx="` + strconv.Itoa(ancho) + `" cy="6858000"/>` +
			`<p:defaultTextStyle><a:lvl1pPr><a:defRPr sz="1800"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill></a:defRPr></a:lvl1pPr></p:defaultTextStyle></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="slideMasters/slideMaster1.xml"/>` +
			`<Relationship Id="rS" Type="` + tipo("slide") + `" Target="slides/slide1.xml"/></Relationships>`,
	}
	return parseSlidesModelJSON(t, importar1(t, "marcadores.pptx", files))
}

// marcador es un <p:sp> marcador de posición (`ph`: los atributos de p:ph), con su geometría
// si la lleva y lo que va en su txBody tras el bodyPr.
func marcador(id, ph, xfrm, cuerpo string) string {
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + id + `" name="M` + id + `"/><p:cNvSpPr/><p:nvPr><p:ph ` + ph + `/></p:nvPr></p:nvSpPr>` +
		`<p:spPr>` + xfrm + `</p:spPr><p:txBody><a:bodyPr/>` + cuerpo + `</p:txBody></p:sp>`
}

const xfrmMarcador = `<a:xfrm><a:off x="838200" y="365125"/><a:ext cx="10515600" cy="1325563"/></a:xfrm>`

// El texto de los marcadores de posición hereda su formato: el título, sin tamaño, alineación
// ni color propios, los toma de la lista del título del patrón (`p:titleStyle`); el cuerpo, de
// la lista del marcador en el diseño y, en el segundo nivel, de la del cuerpo del patrón; lo que
// el texto dice manda. Antes salía todo a 20 px, a la izquierda y con la tinta del tema.
func TestPptxMarcadoresHeredanFormato(t *testing.T) {
	txStyles := `<p:txStyles>` +
		`<p:titleStyle><a:lvl1pPr algn="ctr"><a:defRPr sz="4000"><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:defRPr></a:lvl1pPr></p:titleStyle>` +
		`<p:bodyStyle><a:lvl1pPr><a:defRPr sz="2800"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill></a:defRPr></a:lvl1pPr>` +
		`<a:lvl2pPr algn="just"><a:defRPr sz="2400"/></a:lvl2pPr></p:bodyStyle>` +
		`<p:otherStyle><a:lvl1pPr><a:defRPr sz="1800"/></a:lvl1pPr></p:otherStyle></p:txStyles>`
	master := marcador("2", `type="title"`, xfrmMarcador, `<a:lstStyle/><a:p><a:r><a:rPr lang="es-ES"/><a:t>Título del patrón</a:t></a:r></a:p>`) +
		marcador("3", `type="body" idx="1"`, `<a:xfrm><a:off x="838200" y="1825625"/><a:ext cx="10515600" cy="4351338"/></a:xfrm>`, `<a:lstStyle/><a:p><a:r><a:t>Texto</a:t></a:r></a:p>`)
	diseno := marcador("2", `type="title"`, "", `<a:lstStyle/><a:p><a:endParaRPr/></a:p>`) +
		marcador("3", `idx="1"`, "", `<a:lstStyle><a:lvl1pPr algn="r"><a:defRPr sz="3200"><a:solidFill><a:srgbClr val="00B050"/></a:solidFill></a:defRPr></a:lvl1pPr></a:lstStyle><a:p><a:endParaRPr/></a:p>`)
	diapo := marcador("2", `type="title"`, "", `<a:lstStyle/><a:p><a:r><a:rPr lang="es-ES"/><a:t>Mi título</a:t></a:r></a:p>`) +
		marcador("3", `idx="1"`, "", `<a:lstStyle/>`+
			`<a:p><a:r><a:rPr lang="es-ES"/><a:t>Primer nivel</a:t></a:r></a:p>`+
			`<a:p><a:pPr lvl="1"/><a:r><a:rPr lang="es-ES"/><a:t>Segundo nivel</a:t></a:r></a:p>`+
			`<a:p><a:pPr algn="l"/><a:r><a:rPr lang="es-ES" sz="1800"><a:solidFill><a:srgbClr val="C00000"/></a:solidFill></a:rPr><a:t>Lo suyo</a:t></a:r></a:p>`)
	m := pptxConMarcadores(t, 12192000, master, txStyles, diseno, diapo)
	if len(m.Slides) != 1 || len(m.Slides[0].Boxes) != 2 {
		t.Fatalf("diapositivas = %+v", m.Slides)
	}
	parrafo := func(texto string) string {
		for _, b := range m.Slides[0].Boxes {
			for _, h := range reParrafoHTML.FindAllString(b.HTML, -1) {
				if strings.Contains(h, texto) {
					return h
				}
			}
		}
		t.Fatalf("no está %q", texto)
		return ""
	}
	for _, c := range []struct{ texto, quiero, noQuiero string }{
		{"Mi título", "text-align:center|font-size:40px|color:#4472C4", ""},
		{"Primer nivel", "text-align:right|font-size:32px|color:#00B050", ""},
		// El 24 del segundo nivel es heredado: no lo convierte en un título del editor (h3).
		{"Segundo nivel", "text-align:justify|font-size:24px", "<h3"},
		// La tinta del tema no se escribe: el editor ya la pinta.
		{"Segundo nivel", "", "color:"},
		{"Lo suyo", "font-size:18px|color:#C00000", "text-align"},
	} {
		h := parrafo(c.texto)
		for _, q := range strings.Split(c.quiero, "|") {
			if q != "" && !strings.Contains(h, q) {
				t.Errorf("%s: falta %q en %s", c.texto, q, h)
			}
		}
		if c.noQuiero != "" && strings.Contains(h, c.noQuiero) {
			t.Errorf("%s: sobra %q en %s", c.texto, c.noQuiero, h)
		}
	}
}

// En una diapositiva de 10 pulgadas (la 4:3 y la de Google Presentaciones, 720 pt de ancho) el
// texto se escala al escenario del editor como la geometría: 18 pt son 24 px de los 960.
func TestPptxTamanoDeTextoAEscalaDelEscenario(t *testing.T) {
	diapo := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="T"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="4572000" cy="1000000"/></a:xfrm></p:spPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="es-ES"/><a:t>Por defecto</a:t></a:r></a:p>` +
		`<a:p><a:r><a:rPr lang="es-ES" sz="3300"/><a:t>Título</a:t></a:r></a:p></p:txBody></p:sp>`
	m := pptxConMarcadores(t, 9144000, "", "", "", diapo)
	h := m.Slides[0].Boxes[0].HTML
	if !strings.Contains(h, "font-size:24px") {
		t.Errorf("los 18 pt de la presentación, en 720 pt de ancho, son 24 px: %s", h)
	}
	// 33 pt en 720 pt de ancho son los 44 px de un título del editor: el que escribe el
	// exportador al guardar en 4:3 un título suyo.
	if !strings.Contains(h, "<h1>Título</h1>") {
		t.Errorf("33 pt en 720 pt de ancho = h1: %s", h)
	}
}

// El tamaño propio de un tramo sale al .pptx en puntos y vuelve igual: sin esto, «Guardar en
// Drive» devolvía todo el texto heredado a 20 pt.
func TestPptxTamanoDeTramoIdaYVuelta(t *testing.T) {
	m := slidesModel{Slides: []slideT{{ID: "s1", Boxes: []slideBox{
		{ID: "a", X: 5, Y: 5, W: 90, H: 30, Kind: "text", HTML: `<p><span style="font-size:32px">Grande</span> y normal</p>`},
	}}}}
	data := exportarPptx(t, jsonDe(t, m))
	partes := zipParts(t, data)
	mustContain(t, "tramo con tamaño", partes["ppt/slides/slide1.xml"], `sz="3200"`)
	mustContain(t, "tramo sin tamaño", partes["ppt/slides/slide1.xml"], `sz="2000"`)
	vuelta := reimportar(t, "ida.pptx", data)
	h := vuelta.Slides[0].Boxes[0].HTML
	if !strings.Contains(h, "font-size:32px") || strings.Count(h, "font-size") != 1 {
		t.Errorf("a la vuelta: %s", h)
	}
}

// Una presentación con dos patrones: cada diapositiva hereda del suyo (el de su diseño), no del
// primero. Antes, las de los demás patrones tomaban el texto, la geometría y los colores del
// primero, y pintaban sus cajas.
func TestPptxDiapositivaHeredaDeSuPatron(t *testing.T) {
	tipo := func(n string) string { return tipoR + n }
	patron := func(tit, fondo string) string {
		return `<p:sldMaster ` + nsPres + `><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="` + fondo + `"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>` +
			marcador("2", `type="title"`, xfrmMarcador, `<a:lstStyle/><a:p><a:r><a:t>T</a:t></a:r></a:p>`) +
			`<p:sp><p:nvSpPr><p:cNvPr id="9" name="Logo"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="500000" cy="500000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></p:spPr></p:sp>` +
			`</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
			`<p:txStyles><p:titleStyle><a:lvl1pPr algn="ctr"><a:defRPr sz="` + tit + `"><a:solidFill><a:schemeClr val="accent2"/></a:solidFill></a:defRPr></a:lvl1pPr></p:titleStyle></p:txStyles></p:sldMaster>`
	}
	files := map[string]string{
		"ppt/theme/theme1.xml":                         temaSin,
		"ppt/slideMasters/slideMaster1.xml":            patron("4000", "FFFFFF"),
		"ppt/slideMasters/slideMaster2.xml":            patron("3000", "1F3864"),
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rT" Type="` + tipo("theme") + `" Target="../theme/theme1.xml"/></Relationships>`,
		"ppt/slideMasters/_rels/slideMaster2.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rT" Type="` + tipo("theme") + `" Target="../theme/theme1.xml"/></Relationships>`,
		"ppt/presentation.xml": `<p:presentation ` + nsPres + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rM1"/><p:sldMasterId id="2147483650" r:id="rM2"/></p:sldMasterIdLst>` +
			`<p:sldIdLst><p:sldId id="256" r:id="rS1"/><p:sldId id="257" r:id="rS2"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rM1" Type="` + tipo("slideMaster") + `" Target="slideMasters/slideMaster1.xml"/>` +
			`<Relationship Id="rM2" Type="` + tipo("slideMaster") + `" Target="slideMasters/slideMaster2.xml"/>` +
			`<Relationship Id="rS1" Type="` + tipo("slide") + `" Target="slides/slide1.xml"/><Relationship Id="rS2" Type="` + tipo("slide") + `" Target="slides/slide2.xml"/></Relationships>`,
	}
	for _, n := range []string{"1", "2"} {
		files["ppt/slideLayouts/slideLayout"+n+".xml"] = `<p:sldLayout ` + nsPres + `><p:cSld><p:spTree/></p:cSld></p:sldLayout>`
		files["ppt/slideLayouts/_rels/slideLayout"+n+".xml.rels"] = `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="../slideMasters/slideMaster` + n + `.xml"/></Relationships>`
		files["ppt/slides/slide"+n+".xml"] = `<p:sld ` + nsPres + `><p:cSld><p:spTree>` + marcador("2", `type="title"`, "", `<a:lstStyle/><a:p><a:r><a:rPr lang="es-ES"/><a:t>Título `+n+`</a:t></a:r></a:p>`) + `</p:spTree></p:cSld></p:sld>`
		files["ppt/slides/_rels/slide"+n+".xml.rels"] = `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout` + n + `.xml"/></Relationships>`
	}
	m := parseSlidesModelJSON(t, importar1(t, "patrones.pptx", files))
	if len(m.Slides) != 2 {
		t.Fatalf("diapositivas = %d", len(m.Slides))
	}
	uno, dos := m.Slides[0], m.Slides[1]
	if h := uno.Boxes[0].HTML; !strings.Contains(h, "font-size:40px") || !strings.Contains(h, "color:#ED7D31") {
		t.Errorf("la del primer patrón: %s", h)
	}
	if h := dos.Boxes[len(dos.Boxes)-1].HTML; !strings.Contains(h, "font-size:30px") || !strings.Contains(h, "text-align:center") {
		t.Errorf("la del segundo patrón hereda su título: %s", h)
	}
	// La segunda no tapa ella el patrón del modelo: lo tapa su diseño, que trae el logo de su patrón.
	var diseno *slideLayout
	for i := range m.Master.Layouts {
		if m.Master.Layouts[i].ID == dos.Layout {
			diseno = &m.Master.Layouts[i]
		}
	}
	if dos.Bg != "#1F3864" || dos.HideMaster || uno.HideMaster || diseno == nil || !diseno.HideMaster || len(diseno.Decor) != 1 {
		t.Errorf("fondo y patrón de la segunda: bg %q, hideMaster %v (la primera %v), diseño %+v", dos.Bg, dos.HideMaster, uno.HideMaster, diseno)
	}
}
