// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

// Fixtures sintéticos de las clases de fallo que encontró la medida contra ficheros reales de
// terceros (corpus_terceros_test.go): cada uno reproduce, escrito a mano, lo que escribe un
// programa de verdad, y fallaba antes del arreglo.

const (
	nsHoja = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsRels = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
	tipoR  = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
)

// Los paquetes que escribe System.IO.Packaging (.NET: Open XML SDK, ClosedXML, informes de
// empresa) llevan los destinos de relación absolutos, desde la raíz del paquete. El libro se
// abría vacío.
func TestXlsxConDestinosAbsolutos(t *testing.T) {
	files := map[string]string{
		"_rels/.rels":                `<Relationships ` + nsRels + `><Relationship Id="R1" Type="` + tipoR + `officeDocument" Target="/xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<x:workbook xmlns:x="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><x:sheets><x:sheet name="Datos" sheetId="1" r:id="Rf1c" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></x:sheets></x:workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRels + `><Relationship Type="` + tipoR + `worksheet" Target="/xl/worksheets/sheet1.xml" Id="Rf1c"/><Relationship Type="` + tipoR + `sharedStrings" Target="/xl/sharedStrings.xml" Id="Rbb3"/><Relationship Type="` + tipoR + `hyperlink" Target="/ruta/externa" TargetMode="External" Id="Rx"/></Relationships>`,
		"xl/sharedStrings.xml":       `<x:sst xmlns:x="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><x:si><x:t>Licitación</x:t></x:si></x:sst>`,
		"xl/worksheets/sheet1.xml":   `<x:worksheet xmlns:x="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><x:sheetData><x:row r="1"><x:c r="A1" t="s"><x:v>0</x:v></x:c><x:c r="B1"><x:v>12.5</x:v></x:c></x:row></x:sheetData></x:worksheet>`,
	}
	got := parseSheetModelJSON(t, importar1(t, "informe.xlsx", files))
	if c := got.Sheets[0].Cells; got.Sheets[0].Name != "Datos" || c["A1"] != "Licitación" || c["B1"] != "12.5" {
		t.Fatalf("hoja = %q, celdas = %#v", got.Sheets[0].Name, c)
	}
}

// `r` es opcional en las filas y en las celdas (ECMA-376 §18.3.1.73 y §18.3.1.4): sin él,
// cada una va detrás de la anterior.
func TestXlsxSinReferenciasDeFilaNiCelda(t *testing.T) {
	files := map[string]string{
		"xl/workbook.xml":            `<workbook ` + nsHoja + `><sheets><sheet name="H" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rId1" Type="` + tipoR + `worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": `<worksheet ` + nsHoja + `><sheetData>` +
			`<row><c t="inlineStr"><is><t>Nombre</t></is></c><c t="inlineStr"><is><t>Edad</t></is></c></row>` +
			`<row><c t="inlineStr"><is><t>Ana</t></is></c><c><v>31</v></c></row>` +
			`<row r="5"><c r="C5"><v>7</v></c><c><v>8</v></c></row>` +
			`</sheetData></worksheet>`,
	}
	c := parseSheetModelJSON(t, importar1(t, "flujo.xlsx", files)).Sheets[0].Cells
	for ref, want := range map[string]string{"A1": "Nombre", "B1": "Edad", "A2": "Ana", "B2": "31", "C5": "7", "D5": "8"} {
		if c[ref] != want {
			t.Errorf("%s = %q, quiero %q (celdas %#v)", ref, c[ref], want, c)
		}
	}
}

// Un minigráfico sin celda (sqref vacío) hacía entrar en pánico al importador.
func TestXlsxMinigraficoSinCeldaNoRompe(t *testing.T) {
	files := map[string]string{
		"xl/workbook.xml":            `<workbook ` + nsHoja + `><sheets><sheet name="H" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rId1" Type="` + tipoR + `worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": `<worksheet ` + nsHoja + ` xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main" xmlns:xm="http://schemas.microsoft.com/office/excel/2006/main">` +
			`<sheetData><row r="1"><c r="A1"><v>1</v></c></row></sheetData><extLst><ext uri="{05C60535-1F16-4fd2-B633-F4F36F0B64E0}">` +
			`<x14:sparklineGroups><x14:sparklineGroup><x14:sparklines><x14:sparkline><xm:f>H!A1:A1</xm:f><xm:sqref></xm:sqref></x14:sparkline></x14:sparklines></x14:sparklineGroup></x14:sparklineGroups>` +
			`</ext></extLst></worksheet>`,
	}
	if c := parseSheetModelJSON(t, importar1(t, "mini.xlsx", files)).Sheets[0].Cells; c["A1"] != "1" {
		t.Fatalf("celdas = %#v", c)
	}
}

// Herramientas de Windows que escriben las entradas del ZIP con «\», y partes en UTF-16 (con
// marca de orden de bytes o sin ella): el libro no se abría («falta xl/workbook.xml»).
func TestXlsxConBarrasInvertidasYUTF16(t *testing.T) {
	utf16be := func(s string) string { return aUTF16(s, true, false) }
	utf16le := func(s string) string { return aUTF16(s, false, true) }
	files := map[string]string{
		`xl\workbook.xml`:            utf16be(`<?xml version="1.0" encoding="UTF-16"?><workbook ` + nsHoja + `><sheets><sheet name="Año" sheetId="1" r:id="rId1"/></sheets></workbook>`),
		`xl\_rels\workbook.xml.rels`: `<Relationships ` + nsRels + `><Relationship Id="rId1" Type="` + tipoR + `worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		`xl\worksheets\sheet1.xml`:   utf16le(`<?xml version="1.0" encoding="utf-16"?><worksheet ` + nsHoja + `><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Señal ✓</t></is></c></row></sheetData></worksheet>`),
	}
	got := parseSheetModelJSON(t, importar1(t, "windows.xlsx", files))
	if got.Sheets[0].Name != "Año" || got.Sheets[0].Cells["A1"] != "Señal ✓" {
		t.Fatalf("hoja %q, celdas %#v", got.Sheets[0].Name, got.Sheets[0].Cells)
	}
}

func TestPptxConDestinosAbsolutos(t *testing.T) {
	files := map[string]string{
		"ppt/presentation.xml": `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<p:sldIdLst><p:sldId id="256" r:id="R7"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships ` + nsRels + `><Relationship Type="` + tipoR + `slide" Target="/ppt/slides/slide1.xml" Id="R7"/></Relationships>`,
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:cSld><p:spTree>` +
			`<p:sp><p:nvSpPr><p:cNvPr id="2" name="T"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="6096000" cy="1000000"/></a:xfrm></p:spPr>` +
			`<p:txBody><a:bodyPr/><a:p><a:r><a:t>Resultados del trimestre</a:t></a:r></a:p></p:txBody></p:sp>` +
			`<p:pic><p:nvPicPr><p:cNvPr id="3" name="F"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="R9"/></p:blipFill><p:spPr><a:xfrm><a:off x="0" y="2000000"/><a:ext cx="1000000" cy="1000000"/></a:xfrm></p:spPr></p:pic>` +
			`</p:spTree></p:cSld></p:sld>`,
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships ` + nsRels + `><Relationship Type="` + tipoR + `image" Target="/ppt/media/foto.png" Id="R9"/></Relationships>`,
		"ppt/media/foto.png":               string(pngBytes(t)),
	}
	got := parseSlidesModelJSON(t, importar1(t, "informe.pptx", files))
	if len(got.Slides) != 1 || len(got.Slides[0].Boxes) != 2 {
		t.Fatalf("modelo = %#v", got)
	}
	if b := got.Slides[0].Boxes; !strings.Contains(b[0].HTML, "Resultados del trimestre") || b[1].Kind != "image" || !strings.HasPrefix(b[1].Src, "data:image/png") {
		t.Fatalf("cajas = %#v", b)
	}
}

func TestRelativizarDestinos(t *testing.T) {
	casos := []struct{ rels, entra, sale string }{
		{"xl/_rels/workbook.xml.rels", `<Relationship Id="a" Target="/xl/styles.xml"/>`, `<Relationship Id="a" Target="../xl/styles.xml"/>`},
		{"ppt/slides/_rels/slide1.xml.rels", `<Relationship Id="a" Target='/ppt/media/x.png'/>`, `<Relationship Id="a" Target='../../ppt/media/x.png'/>`},
		{"_rels/.rels", `<Relationship Id="a" Target="/word/document.xml"/>`, `<Relationship Id="a" Target="word/document.xml"/>`},
		{"xl/_rels/workbook.xml.rels", `<Relationship Id="a" Target="worksheets/sheet1.xml"/>`, `<Relationship Id="a" Target="worksheets/sheet1.xml"/>`},
		{"xl/_rels/workbook.xml.rels", `<Relationship Id="a" TargetMode="External" Target="/srv/datos.xlsx"/>`, `<Relationship Id="a" TargetMode="External" Target="/srv/datos.xlsx"/>`},
	}
	for _, c := range casos {
		if got := string(relativizarDestinos(c.rels, []byte(c.entra))); got != c.sale {
			t.Errorf("%s: %s\n  sale %s\nquiero %s", c.rels, c.entra, got, c.sale)
		}
	}
}

// aUTF16 escribe s en UTF-16, en el orden de bytes pedido y con su marca o sin ella.
func aUTF16(s string, be, bom bool) string {
	u := utf16.Encode([]rune("\uFEFF" + s))
	if !bom {
		u = u[1:]
	}
	b := make([]byte, 0, 2*len(u))
	for _, x := range u {
		if be {
			b = append(b, byte(x>>8), byte(x))
		} else {
			b = append(b, byte(x), byte(x>>8))
		}
	}
	return string(b)
}

// importar1 empaqueta los ficheros y los importa; falla el test si la importación falla.
func importar1(t *testing.T, nombre string, files map[string]string) string {
	t.Helper()
	res, err := Import(nombre, zipStore(t, files))
	if err != nil {
		t.Fatalf("importar %s: %v", nombre, err)
	}
	return res.Content
}

// --- Colores de Diapos ---

const (
	nsPres  = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	temaSin = `<a:theme ` + nsPres + ` name="Prueba"><a:themeElements><a:clrScheme name="P">` +
		`<a:dk1><a:sysClr val="windowText" lastClr="1A1A1A"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="44546A"/></a:dk2><a:lt2><a:srgbClr val="E7E6E6"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="4472C4"/></a:accent1><a:accent2><a:srgbClr val="ED7D31"/></a:accent2><a:accent3><a:srgbClr val="A5A5A5"/></a:accent3>` +
		`<a:accent4><a:srgbClr val="FFC000"/></a:accent4><a:accent5><a:srgbClr val="5B9BD5"/></a:accent5><a:accent6><a:srgbClr val="70AD47"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="0563C1"/></a:hlink><a:folHlink><a:srgbClr val="954F72"/></a:folHlink></a:clrScheme>` +
		`<a:fontScheme name="P"><a:majorFont><a:latin typeface="Arial"/></a:majorFont><a:minorFont><a:latin typeface="Arial"/></a:minorFont></a:fontScheme>` +
		`<a:fmtScheme name="P"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:tint val="50000"/></a:schemeClr></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
		`<a:lnStyleLst><a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
		`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
		`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:gradFill><a:gsLst><a:gs pos="0"><a:schemeClr val="phClr"/></a:gs><a:gs pos="100000"><a:srgbClr val="FFFFFF"/></a:gs></a:gsLst></a:gradFill></a:bgFillStyleLst></a:fmtScheme>` +
		`</a:themeElements></a:theme>`
)

// pptxDePrueba arma una presentación con su tema, su patrón (fondo bg1), un diseño por
// diapositiva (`disenos[i]`, el XML que va dentro de <p:cSld> antes de spTree, más lo que va
// detrás de </p:cSld>) y las diapositivas (lo que va dentro de <p:cSld>).
func pptxDePrueba(t *testing.T, disenos [][2]string, diapos []string, extra ...string) string {
	t.Helper()
	tipo := func(n string) string { return tipoR + n }
	files := map[string]string{
		"ppt/theme/theme1.xml": temaSin,
		"ppt/slideMasters/slideMaster1.xml": `<p:sldMaster ` + nsPres + `><p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg><p:spTree/></p:cSld>` +
			`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/><p:sldLayoutIdLst/></p:sldMaster>`,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="rT" Type="` + tipo("theme") + `" Target="../theme/theme1.xml"/></Relationships>`,
		"ppt/media/foto.png":                           string(pngBytes(t)),
	}
	pres := `<p:presentation ` + nsPres + `><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rM"/></p:sldMasterIdLst><p:sldIdLst>`
	rels := `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="slideMasters/slideMaster1.xml"/>`
	for i, d := range diapos {
		n := strconv.Itoa(i + 1)
		pres += `<p:sldId id="` + strconv.Itoa(256+i) + `" r:id="rS` + n + `"/>`
		rels += `<Relationship Id="rS` + n + `" Type="` + tipo("slide") + `" Target="slides/slide` + n + `.xml"/>`
		files["ppt/slides/slide"+n+".xml"] = `<p:sld ` + nsPres + `><p:cSld>` + d + `</p:cSld></p:sld>`
		files["ppt/slides/_rels/slide"+n+".xml.rels"] = `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipo("slideLayout") + `" Target="../slideLayouts/slideLayout` + n + `.xml"/>` +
			`<Relationship Id="rImg" Type="` + tipo("image") + `" Target="../media/foto.png"/></Relationships>`
		var dis [2]string
		if i < len(disenos) {
			dis = disenos[i]
		}
		files["ppt/slideLayouts/slideLayout"+n+".xml"] = `<p:sldLayout ` + nsPres + `><p:cSld>` + dis[0] + `<p:spTree/></p:cSld>` + dis[1] + `</p:sldLayout>`
		files["ppt/slideLayouts/_rels/slideLayout"+n+".xml.rels"] = `<Relationships ` + nsRels + `><Relationship Id="rM" Type="` + tipo("slideMaster") + `" Target="../slideMasters/slideMaster1.xml"/></Relationships>`
	}
	for i := 0; i+1 < len(extra); i += 2 {
		files[extra[i]] = extra[i+1]
	}
	files["ppt/presentation.xml"] = pres + `</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`
	files["ppt/_rels/presentation.xml.rels"] = rels + `</Relationships>`
	return importar1(t, "colores.pptx", files)
}

// forma es una <p:sp> con su spPr (sin la geometría), su p:style y un párrafo de texto.
func forma(spPr, estilo, texto string) string {
	return `<p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="F"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="6096000" cy="3429000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
		spPr + `</p:spPr>` + estilo + `<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="es-ES"/><a:t>` + texto + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree>`
}

// Los colores de Diapos que se perdían: el relleno y la letra que da el estilo de una forma
// (PowerPoint pinta así toda forma insertada sin tocarla), las variantes de un color del tema
// (lumMod/lumOff, el «Énfasis 1, más claro 40 %»), la transparencia, el fondo que se hereda
// del diseño, el que apunta al tema (bgRef), el degradado, el de imagen, el mapa de colores
// de un diseño oscuro y la negrita escrita «true».
func TestPptxColoresDeFormasYFondos(t *testing.T) {
	estiloAccent := `<p:style><a:lnRef idx="1"><a:schemeClr val="accent1"><a:shade val="50000"/></a:schemeClr></a:lnRef><a:fillRef idx="1"><a:schemeClr val="accent1"/></a:fillRef>` +
		`<a:effectRef idx="0"><a:schemeClr val="accent1"/></a:effectRef><a:fontRef idx="minor"><a:schemeClr val="lt1"/></a:fontRef></p:style>`
	m := parseSlidesModelJSON(t, pptxDePrueba(t,
		[][2]string{
			{},
			{},
			{},
			{`<p:bg><p:bgPr><a:solidFill><a:srgbClr val="1F3864"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>`, ""},
			{},
			{},
			{`<p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg>`, `<p:clrMapOvr><a:overrideClrMapping bg1="dk1" tx1="lt1" bg2="dk2" tx2="lt2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/></p:clrMapOvr>`},
		},
		[]string{
			forma("", estiloAccent, "Forma de serie"),
			forma(`<a:solidFill><a:schemeClr val="accent1"><a:lumMod val="60000"/><a:lumOff val="40000"/></a:schemeClr></a:solidFill>`, "", "Más claro"),
			forma(`<a:solidFill><a:srgbClr val="FF0000"><a:alpha val="50000"/></a:srgbClr></a:solidFill>`, "", "Translúcida"),
			forma("", "", "Fondo del diseño"),
			`<p:bg><p:bgRef idx="1003"><a:schemeClr val="accent2"/></p:bgRef></p:bg>` + forma("", "", "Fondo del tema"),
			`<p:bg><p:bgPr><a:blipFill><a:blip r:embed="rImg"/><a:stretch><a:fillRect/></a:stretch></a:blipFill><a:effectLst/></p:bgPr></p:bg>` + forma("", "", "Fondo de imagen"),
			`<p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="T"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="6096000" cy="1000000"/></a:xfrm></p:spPr>` +
				`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="es-ES" b="true"/><a:t>Oscura</a:t></a:r></a:p></p:txBody></p:sp></p:spTree>`,
		}))
	if len(m.Slides) != 7 {
		t.Fatalf("diapositivas = %d", len(m.Slides))
	}
	caja := func(i int) slideBox { return m.Slides[i].Boxes[len(m.Slides[i].Boxes)-1] }
	if b := caja(0); b.Kind != "shape" || b.Fill != "#4472C4" || b.Stroke != "#2F528F" || !strings.Contains(b.HTML, "color:#FFFFFF") {
		t.Errorf("forma de serie: %+v", b)
	}
	if b := caja(1); b.Fill != "#8FAADC" {
		t.Errorf("accent1 más claro 40 %%: relleno %q, quiero #8FAADC", b.Fill)
	}
	if b := caja(2); b.Fill != "#FF8080" {
		t.Errorf("rojo al 50 %% sobre blanco: relleno %q, quiero #FF8080", b.Fill)
	}
	if bg := m.Slides[3].Bg; bg != "#1F3864" {
		t.Errorf("fondo heredado del diseño = %q", bg)
	}
	if bg := m.Slides[4].Bg; bg != "#F6BE98" {
		t.Errorf("fondo del tema (degradado accent2 → blanco, su media) = %q", bg)
	}
	if b := m.Slides[5].Boxes[0]; b.Kind != "image" || b.X != 0 || b.Y != 0 || b.W != 100 || b.H != 100 || !strings.HasPrefix(b.Src, "data:image/png") {
		t.Errorf("fondo de imagen: %+v", b)
	}
	if s := m.Slides[6]; s.Bg != "#1A1A1A" || !strings.Contains(caja(6).HTML, "color:#FFFFFF") || !strings.Contains(caja(6).HTML, "<strong>") {
		t.Errorf("diseño oscuro (bg1 → dk1, tx1 → lt1) y negrita «true»: fondo %q, caja %+v", s.Bg, caja(6))
	}
}

// Lo agrupado y movido o escalado después de agruparlo (chOff/chExt distintos de off/ext),
// las coordenadas negativas (una forma que asoma por el borde) y una caja de texto con relleno,
// que es una forma con texto: salían en su sitio de antes de agrupar, pegadas al borde y sin
// relleno.
func TestPptxGruposYCajasConRelleno(t *testing.T) {
	grupo := `<p:spTree><p:grpSp><p:nvGrpSpPr><p:cNvPr id="5" name="G"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="6096000" y="0"/><a:ext cx="3048000" cy="1714500"/><a:chOff x="0" y="0"/><a:chExt cx="6096000" cy="3429000"/></a:xfrm></p:grpSpPr>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="6" name="T"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="6096000" cy="3429000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom>` +
		`<a:solidFill><a:srgbClr val="FF99CC"/></a:solidFill></p:spPr><p:txBody><a:bodyPr/><a:p><a:r><a:t>R</a:t></a:r></a:p></p:txBody></p:sp></p:grpSp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="7" name="N"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="-1219200" y="0"/><a:ext cx="2438400" cy="685800"/></a:xfrm></p:spPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>Asoma</a:t></a:r></a:p></p:txBody></p:sp></p:spTree>`
	b := parseSlidesModelJSON(t, pptxDePrueba(t, nil, []string{grupo})).Slides[0].Boxes
	if len(b) != 2 {
		t.Fatalf("cajas = %+v", b)
	}
	if g := b[0]; g.Kind != "shape" || g.Fill != "#FF99CC" || g.X != 50 || g.Y != 0 || g.W != 25 || g.H != 25 {
		t.Errorf("caja de texto rellena dentro de un grupo escalado a la mitad y movido: %+v", g)
	}
	if n := b[1]; n.X != -10 {
		t.Errorf("x negativa: %+v", n)
	}
}

// Las tablas y los SmartArt se perdían enteros (un `p:graphicFrame` no se leía): el texto, el
// relleno del estilo de tabla por partes (cabecera, bandas), el relleno y el borde propios de
// una celda, las combinadas, y el resaltado de un tramo.
func TestPptxTablasSmartArtYResaltado(t *testing.T) {
	celda := func(texto, tcPr string, attrs ...string) string {
		return `<a:tc` + strings.Join(attrs, "") + `><a:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="es-ES"/><a:t>` + texto + `</a:t></a:r></a:p></a:txBody><a:tcPr>` + tcPr + `</a:tcPr></a:tc>`
	}
	tabla := `<p:spTree><p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="4" name="Tabla"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="1219200" y="685800"/><a:ext cx="6096000" cy="2286000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table">` +
		`<a:tbl><a:tblPr firstRow="1" bandRow="1"><a:tableStyleId>{EST}</a:tableStyleId></a:tblPr><a:tblGrid><a:gridCol w="3048000"/><a:gridCol w="3048000"/></a:tblGrid>` +
		`<a:tr h="370840">` + celda("Mes", "") + celda("Ventas", "") + `</a:tr>` +
		`<a:tr h="370840">` + celda("Enero", "") + celda("120", `<a:lnB w="12700"><a:solidFill><a:srgbClr val="FF0000"/></a:solidFill></a:lnB><a:solidFill><a:srgbClr val="FFFF00"/></a:solidFill>`) + `</a:tr>` +
		`<a:tr h="370840">` + celda("Total del año", "", ` gridSpan="2"`) + celda("", "", ` hMerge="1"`) + `</a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>` +
		`<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="5" name="SmartArt"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="6096000" y="3429000"/><a:ext cx="3048000" cy="1714500"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/diagram">` +
		`<dgm:relIds xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" r:dm="rDm" r:lo="rLo" r:qs="rQs" r:cs="rCs"/></a:graphicData></a:graphic></p:graphicFrame>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="6" name="T"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="6000000"/><a:ext cx="6096000" cy="500000"/></a:xfrm></p:spPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="es-ES"><a:highlight><a:srgbClr val="FFFF00"/></a:highlight></a:rPr><a:t>Ojo</a:t></a:r></a:p></p:txBody></p:sp></p:spTree>`
	estilos := `<a:tblStyleLst ` + nsPres + ` def="{EST}"><a:tblStyle styleId="{EST}" styleName="Medio 2 - Énfasis 1">` +
		`<a:wholeTbl><a:tcTxStyle><a:schemeClr val="dk1"/></a:tcTxStyle><a:tcStyle><a:tcBdr><a:insideH><a:ln w="12700"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:insideH></a:tcBdr>` +
		`<a:fill><a:solidFill><a:schemeClr val="accent1"><a:tint val="20000"/></a:schemeClr></a:solidFill></a:fill></a:tcStyle></a:wholeTbl>` +
		`<a:band1H><a:tcStyle><a:tcBdr/><a:fill><a:solidFill><a:schemeClr val="accent1"><a:tint val="40000"/></a:schemeClr></a:solidFill></a:fill></a:tcStyle></a:band1H>` +
		`<a:firstRow><a:tcTxStyle b="on"><a:schemeClr val="lt1"/></a:tcTxStyle><a:tcStyle><a:tcBdr/><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill></a:tcStyle></a:firstRow>` +
		`</a:tblStyle></a:tblStyleLst>`
	dibujo := `<dsp:drawing xmlns:dsp="http://schemas.microsoft.com/office/drawing/2008/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dsp:spTree><dsp:nvGrpSpPr><dsp:cNvPr id="0" name=""/><dsp:cNvGrpSpPr/></dsp:nvGrpSpPr><dsp:grpSpPr/>` +
		`<dsp:sp modelId="{1}"><dsp:nvSpPr><dsp:cNvPr id="0" name=""/><dsp:cNvSpPr/></dsp:nvSpPr><dsp:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1524000" cy="857250"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></dsp:spPr>` +
		`<dsp:txBody><a:bodyPr/><a:p><a:r><a:t>Paso uno</a:t></a:r></a:p></dsp:txBody></dsp:sp></dsp:spTree></dsp:drawing>`
	datos := `<dgm:dataModel xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dgm:ptLst/><dgm:extLst><a:ext uri="http://schemas.microsoft.com/office/drawing/2008/diagram"><dsp:dataModelExt xmlns:dsp="http://schemas.microsoft.com/office/drawing/2008/diagram" relId="rDib" minVer="http://schemas.openxmlformats.org/drawingml/2006/diagram"/></a:ext></dgm:extLst></dgm:dataModel>`
	rels := `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipoR + `slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
		`<Relationship Id="rDm" Type="` + tipoR + `diagramData" Target="../diagrams/data1.xml"/><Relationship Id="rDib" Type="http://schemas.microsoft.com/office/2007/relationships/diagramDrawing" Target="../diagrams/drawing1.xml"/></Relationships>`
	m := parseSlidesModelJSON(t, pptxDePrueba(t, nil, []string{tabla},
		"ppt/tableStyles.xml", estilos, "ppt/diagrams/drawing1.xml", dibujo, "ppt/diagrams/data1.xml", datos, "ppt/slides/_rels/slide1.xml.rels", rels))
	cajas := m.Slides[0].Boxes
	if len(cajas) != 3 {
		t.Fatalf("cajas = %+v", cajas)
	}
	tab := cajas[0].HTML
	for _, quiero := range []string{
		// Cabecera: accent1 y letra lt1 en negrita; banda 1 y resto de la tabla, accent1 con tinte
		// del 40 % y del 20 % (los #CFD5EA y #E9EBF5 de la tabla de serie de PowerPoint).
		`background:#4472C4;`, `<strong><span style="color:#FFFFFF">Mes</span></strong>`,
		`<td style="background:#CFD5EA;`, `<td colspan="2" style="background:#E9EBF5;`,
		// El relleno y el borde propios de una celda, y el borde interior del estilo.
		`<td style="background:#FFFF00;border-top:1px solid #FFFFFF;border-bottom:1px solid #FF0000;`,
		`Total del año`,
	} {
		if !strings.Contains(tab, quiero) {
			t.Errorf("la tabla no lleva %s:\n%s", quiero, tab)
		}
	}
	if strings.Count(tab, "<td") != 5 {
		t.Errorf("celdas: la combinada no se pinta aparte:\n%s", tab)
	}
	if b := cajas[0]; b.X != 10 || b.Y != 10 || b.W != 50 {
		t.Errorf("la tabla en su sitio: %+v", b)
	}
	if b := cajas[1]; !strings.Contains(b.HTML, "Paso uno") || b.Fill != "#4472C4" || b.X != 50 || b.Y != 50 {
		t.Errorf("la forma del SmartArt, en el sitio del marco: %+v", b)
	}
	if !strings.Contains(cajas[2].HTML, "background-color:#FFFF00") {
		t.Errorf("resaltado: %s", cajas[2].HTML)
	}
}

// Una tabla importada vive en el HTML de una caja de texto, y al guardar como .pptx o .odp su
// texto se perdía (el escritor solo sabía de párrafos): sale celda a celda.
func TestDiaposTablaImportadaSeGuardaComoTexto(t *testing.T) {
	modelo := `{"slides":[{"id":"s1","boxes":[{"x":10,"y":10,"w":80,"h":40,"kind":"text","html":"<table><tbody><tr><td>Mes</td><td>Ventas</td></tr></tbody></table>"}]}]}`
	s := stateWithContent(t, "doc:slides", modelo)
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _, ext, err := Export(KindPresentation, "Tabla", s, f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Import("tabla."+ext, data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Content, "Mes") || !strings.Contains(res.Content, "Ventas") {
			t.Errorf("%s: el texto de la tabla no sobrevive al guardar: %s", ext, res.Content)
		}
	}
}

// Calcus: el color de pestaña (sheetPr/tabColor, por rgb, tema con tinte o indexado) se perdía
// —y Excel no lo guardaba de vuelta—, y del relleno de celda solo se leía el liso: una trama
// salía sin color, un degradado también, y un dxf «none» pintaba.
func TestXlsxPestanasYRellenos(t *testing.T) {
	hojaCon := func(pr, celdas string) string {
		return `<worksheet ` + nsHoja + `>` + pr + `<sheetData><row r="1">` + celdas + `</row></sheetData></worksheet>`
	}
	files := map[string]string{
		"xl/workbook.xml": `<workbook ` + nsHoja + `><sheets><sheet name="A" sheetId="1" r:id="r1"/><sheet name="B" sheetId="2" r:id="r2"/><sheet name="C" sheetId="3" r:id="r3"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRels + `><Relationship Id="r1" Type="` + tipoR + `worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="r2" Type="` + tipoR + `worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="r3" Type="` + tipoR + `worksheet" Target="worksheets/sheet3.xml"/>` +
			`<Relationship Id="r4" Type="` + tipoR + `styles" Target="styles.xml"/></Relationships>`,
		"xl/styles.xml": `<styleSheet ` + nsHoja + `><fonts count="1"><font><sz val="11"/></font></fonts>` +
			`<fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill>` +
			`<fill><patternFill patternType="mediumGray"><fgColor rgb="FFFF0000"/><bgColor rgb="FFFFFFFF"/></patternFill></fill>` +
			`<fill><gradientFill degree="90"><stop position="0"><color rgb="FF000000"/></stop><stop position="1"><color rgb="FFFFFFFF"/></stop></gradientFill></fill></fills>` +
			`<cellXfs count="3"><xf fontId="0" fillId="0"/><xf fontId="0" fillId="2"/><xf fontId="0" fillId="3"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": hojaCon(`<sheetPr><tabColor rgb="FF00B050"/></sheetPr>`, `<c r="A1" s="1"><v>1</v></c><c r="B1" s="2"><v>2</v></c>`),
		"xl/worksheets/sheet2.xml": hojaCon(`<sheetPr><tabColor theme="4" tint="0.39997558519241921"/></sheetPr>`, `<c r="A1"><v>1</v></c>`),
		"xl/worksheets/sheet3.xml": hojaCon(`<sheetPr><tabColor indexed="10"/></sheetPr>`, `<c r="A1"><v>1</v></c>`),
	}
	m := parseSheetModelJSON(t, importar1(t, "pestanas.xlsx", files))
	for i, quiero := range []string{"#00B050", "#8FAADC", "#FF0000"} {
		if got := m.Sheets[i].TabColor; got != quiero {
			t.Errorf("pestaña de %s = %q, quiero %s", m.Sheets[i].Name, got, quiero)
		}
	}
	if f := m.Sheets[0].Styles["A1"].Fill; f != "#FF8080" {
		t.Errorf("trama mediumGray roja sobre blanco = %q, quiero #FF8080", f)
	}
	if f := m.Sheets[0].Styles["B1"].Fill; f != "#808080" {
		t.Errorf("degradado de negro a blanco = %q, quiero #808080", f)
	}
	// Y la pestaña vuelve a Excel al guardar.
	data, _, _, err := Export(KindSpreadsheet, "x", stateWithContent(t, "doc:sheet", marshalSheetModel(m)), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := Import("x.xlsx", data); err != nil || parseSheetModelJSON(t, res.Content).Sheets[1].TabColor != "#8FAADC" {
		t.Errorf("la pestaña no vuelve de Excel: %v", err)
	}
}

// Un SmartArt sin dibujo compuesto (el de PowerPoint 2007 y otras herramientas) se perdía:
// llega su texto, nodo a nodo, como lista en el sitio del marco.
func TestPptxSmartArtSinDibujo(t *testing.T) {
	marco := `<p:spTree><p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="4" name="S"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="1219200" y="685800"/><a:ext cx="6096000" cy="2286000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/diagram">` +
		`<dgm:relIds xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" r:dm="rDm" r:lo="rLo" r:qs="rQs" r:cs="rCs"/></a:graphicData></a:graphic></p:graphicFrame></p:spTree>`
	pt := func(tipo, texto string) string {
		return `<dgm:pt modelId="{` + texto + `}"` + tipo + `><dgm:t><a:bodyPr/><a:p><a:r><a:t>` + texto + `</a:t></a:r></a:p></dgm:t></dgm:pt>`
	}
	datos := `<dgm:dataModel xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dgm:ptLst>` +
		pt(` type="doc"`, "Documento") + pt("", "Arriba") + pt(` type="node"`, "Abajo") + pt(` type="parTrans"`, "Conexión") + `</dgm:ptLst></dgm:dataModel>`
	rels := `<Relationships ` + nsRels + `><Relationship Id="rL" Type="` + tipoR + `slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
		`<Relationship Id="rDm" Type="` + tipoR + `diagramData" Target="../diagrams/data1.xml"/></Relationships>`
	b := parseSlidesModelJSON(t, pptxDePrueba(t, nil, []string{marco}, "ppt/diagrams/data1.xml", datos, "ppt/slides/_rels/slide1.xml.rels", rels)).Slides[0].Boxes
	if len(b) != 1 || b[0].HTML != "<ul><li>Arriba</li><li>Abajo</li></ul>" || b[0].X != 10 {
		t.Fatalf("cajas = %+v", b)
	}
}
