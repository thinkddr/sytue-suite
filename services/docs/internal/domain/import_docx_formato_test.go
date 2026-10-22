// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"
)

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"`

// importarDocx empaqueta las partes dadas (document.xml solo con el cuerpo) y devuelve el HTML.
func importarDocx(t *testing.T, cuerpo string, partes map[string]string) string {
	t.Helper()
	files := map[string]string{"word/document.xml": `<w:document ` + wNS + `><w:body>` + cuerpo + `</w:body></w:document>`}
	for k, v := range partes {
		files[k] = v
	}
	data, err := zipPackage(files)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Import("prueba.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	// Lo que importa tiene que caber en el contrato del saneador sin perder un byte
	// (saneado_html.go): si no, el documento se guardaría distinto de como se importó.
	if limpio := SanearContenido("doc:html", res.Content); limpio != res.Content {
		t.Errorf("el HTML importado cambia al sanearlo, %s", difiere(res.Content, limpio))
	}
	return res.Content
}

func tieneTodo(t *testing.T, html string, quiero ...string) {
	t.Helper()
	for _, q := range quiero {
		if !strings.Contains(html, q) {
			t.Errorf("falta %q en\n%s", q, html)
		}
	}
}

func noTiene(t *testing.T, html string, no ...string) {
	t.Helper()
	for _, q := range no {
		if strings.Contains(html, q) {
			t.Errorf("sobra %q en\n%s", q, html)
		}
	}
}

// El patrón de Pages de Apple: cada tramo lleva w:u solo con color, interruptores a 0
// (outline, rtl, bidi), w14:textFill y el estilo de carácter «Ninguno»; el formato real está en
// el estilo de párrafo y en el sombreado de la tabla. Con el importador de antes el documento
// entero salía subrayado y sin tipografía, tamaño ni fondo.
func TestDocxPatronDePages(t *testing.T) {
	styles := `<w:styles ` + wNS + `>
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman"/><w:sz w:val="20"/><w:u w:val="none" w:color="auto"/><w:outline w:val="0"/><w:vanish w:val="0"/></w:rPr></w:rPrDefault>
<w:pPrDefault><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="0"/></w:numPr><w:pBdr><w:top w:val="nil"/><w:bottom w:val="nil"/></w:pBdr><w:shd w:val="clear" w:color="auto" w:fill="auto"/><w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:sz w:val="24"/></w:rPr></w:style>
<w:style w:type="character" w:styleId="Ninguno"><w:name w:val="Ninguno"/><w:rPr><w:lang w:val="es-ES_tradnl"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Default"><w:name w:val="Default"/><w:pPr><w:bidi w:val="0"/></w:pPr><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/><w:color w:val="000000"/><w:sz w:val="24"/><w:u w:val="none" w:color="000000"/><w:shd w:val="nil" w:color="auto" w:fill="auto"/></w:rPr></w:style>
</w:styles>`
	tramo := func(extra, texto string) string {
		return `<w:r><w:rPr><w:rStyle w:val="Ninguno"/>` + extra + `<w:outline w:val="0"/><w:u w:color="7f7f7f"/><w:rtl w:val="0"/><w:lang w:val="es-ES_tradnl"/>` +
			`<w14:textOutline><w14:noFill/></w14:textOutline><w14:textFill><w14:solidFill><w14:srgbClr w14:val="7F7F7F"/></w14:solidFill></w14:textFill></w:rPr><w:t>` + texto + `</w:t></w:r>`
	}
	celda := `<w:tc><w:tcPr><w:tcW w:type="dxa" w:w="4322"/><w:tcBorders><w:top w:val="single" w:color="000000" w:sz="4"/></w:tcBorders><w:shd w:val="clear" w:color="auto" w:fill="auto"/><w:tcMar><w:left w:type="dxa" w:w="80"/></w:tcMar></w:tcPr>` +
		`<w:p><w:pPr><w:pStyle w:val="Default"/><w:bidi w:val="0"/></w:pPr>` + tramo(`<w:b w:val="1"/>`, "ScriptRunner") + `</w:p></w:tc>`
	cuerpo := `<w:p><w:pPr><w:pStyle w:val="Default"/><w:bidi w:val="0"/></w:pPr>` + tramo(`<w:b w:val="1"/><w:color w:val="7f7f7f"/><w:sz w:val="32"/>`, "NOMBRE") + `</w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="Default"/></w:pPr>` + tramo(`<w:shd w:val="clear" w:color="auto" w:fill="ced7e7"/>`, "sombreado") + `</w:p>` +
		`<w:tbl><w:tblPr><w:tblW w:w="8644" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="ced7e7"/></w:tblPr><w:tblGrid><w:gridCol w:w="4322"/></w:tblGrid>` +
		`<w:tr><w:tblPrEx><w:shd w:val="clear" w:color="auto" w:fill="ced7e7"/></w:tblPrEx>` + celda + `</w:tr></w:tbl>`
	html := importarDocx(t, cuerpo, map[string]string{"word/styles.xml": styles})
	noTiene(t, html, "<u>", "display:none", "rtl")
	tieneTodo(t, html,
		"font-family:Calibri", // el estilo de párrafo, en el bloque (sus 12 pt son los del folio)
		`font-size:16pt"><strong><span style="color:#7F7F7F">NOMBRE</span></strong>`, // el tamaño, común a todo el párrafo, sube a él
		"background-color:#CED7E7\">sombreado",                                       // sombreado de carácter
		"background-color:#CED7E7;padding:0 0 0 5.33px;width:288.13px",               // el de la tabla, bajo una celda «auto»
		"border-top:1px solid #000000",
	)
}

// ST_OnOff (§17.17.4): true, on y 1 activan; false, off y 0 apagan; sin w:val, activa.
func TestDocxInterruptoresOnOff(t *testing.T) {
	var cuerpo strings.Builder
	for _, v := range []string{"", ` w:val="true"`, ` w:val="on"`, ` w:val="1"`, ` w:val="false"`, ` w:val="off"`, ` w:val="0"`} {
		cuerpo.WriteString(`<w:p><w:r><w:rPr><w:b` + v + `/><w:i` + v + `/><w:caps` + v + `/></w:rPr><w:t>x</w:t></w:r></w:p>`)
	}
	html := importarDocx(t, cuerpo.String(), nil)
	if n := strings.Count(html, "<strong><em><span style=\"text-transform:uppercase\">x"); n != 4 {
		t.Errorf("activados = %d, quiero 4\n%s", n, html)
	}
	if n := strings.Count(html, "<p>x</p>"); n != 3 {
		t.Errorf("apagados = %d, quiero 3\n%s", n, html)
	}
}

// La herencia (§17.7.2): docDefaults < estilo de tabla < de párrafo (con basedOn) < de carácter
// < lo directo. Los interruptores se alternan entre estilos de tipos distintos (§17.7.3).
func TestDocxHerenciaDeEstilos(t *testing.T) {
	styles := `<w:styles ` + wNS + `>
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:asciiTheme="minorHAnsi"/><w:sz w:val="22"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="259" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="Base"><w:name w:val="Base"/><w:pPr><w:jc w:val="center"/><w:ind w:left="720"/></w:pPr><w:rPr><w:b/><w:color w:val="1F4E79"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Hijo"><w:name w:val="Hijo"/><w:basedOn w:val="Base"/><w:pPr><w:spacing w:before="240"/></w:pPr><w:rPr><w:sz w:val="28"/></w:rPr></w:style>
<w:style w:type="character" w:styleId="Fuerte"><w:name w:val="Strong"/><w:rPr><w:b/><w:highlight w:val="yellow"/></w:rPr></w:style>
</w:styles>`
	tema := `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:themeElements><a:clrScheme name="x"><a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:accent1><a:srgbClr val="4472C4"/></a:accent1></a:clrScheme>` +
		`<a:fontScheme name="x"><a:majorFont><a:latin typeface="Calibri Light"/></a:majorFont><a:minorFont><a:latin typeface="Calibri"/></a:minorFont></a:fontScheme></a:themeElements></a:theme>`
	cuerpo := `<w:p><w:pPr><w:pStyle w:val="Hijo"/></w:pPr><w:r><w:t>hereda</w:t></w:r><w:r><w:rPr><w:rStyle w:val="Fuerte"/></w:rPr><w:t>alterna</w:t></w:r>` +
		`<w:r><w:rPr><w:b w:val="0"/><w:color w:val="4472C4" w:themeColor="accent1" w:themeShade="BF"/></w:rPr><w:t>directo</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>normal</w:t></w:r></w:p>`
	html := importarDocx(t, cuerpo, map[string]string{"word/styles.xml": styles, "word/theme/theme1.xml": tema})
	tieneTodo(t, html,
		// Hijo hereda de Base el centrado, la sangría, la negrita y el color; añade tamaño y espacio.
		`<p style="font-family:Calibri,Carlito,sans-serif;font-size:14pt;color:#1F4E79;margin-top:16px;margin-bottom:10.67px;line-height:1.3;margin-left:48px;text-align:center">`,
		`<strong>hereda</strong>`,
		// Negrita del estilo de párrafo y del de carácter: se anulan. El resaltado sí llega.
		`<span style="background-color:#FFFF00">alterna</span>`,
		// Lo directo manda; el color del tema con su sombra (BF) y no el val.
		`<span style="color:#2F5496">directo</span>`,
		`<p style="font-family:Calibri,Carlito,sans-serif;font-size:11pt;margin-bottom:10.67px;line-height:1.3">normal</p>`,
	)
}

// w:shd (§17.3.5): nil y fill «auto» no pintan nada; clear pinta el relleno, solid el color del
// patrón y un porcentaje mezcla los dos. El tema resuelve themeFill con su tinte.
func TestDocxSombreado(t *testing.T) {
	tema := `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:themeElements><a:clrScheme name="x"><a:accent1><a:srgbClr val="4472C4"/></a:accent1></a:clrScheme></a:themeElements></a:theme>`
	casos := map[string]string{
		`<w:shd w:val="nil" w:color="auto" w:fill="ced7e7"/>`:                                            "",
		`<w:shd w:val="clear" w:color="auto" w:fill="auto"/>`:                                            "",
		`<w:shd w:val="clear" w:color="auto" w:fill="ced7e7"/>`:                                          "CED7E7",
		`<w:shd w:val="solid" w:color="FF0000" w:fill="auto"/>`:                                          "FF0000",
		`<w:shd w:val="pct10" w:color="auto" w:fill="auto"/>`:                                            "E6E6E6",
		`<w:shd w:val="clear" w:color="auto" w:fill="auto" w:themeFill="accent1" w:themeFillTint="33"/>`: "DAE3F3",
	}
	for shd, quiero := range casos {
		html := importarDocx(t, `<w:p><w:r><w:rPr>`+shd+`</w:rPr><w:t>x</w:t></w:r></w:p>`, map[string]string{"word/theme/theme1.xml": tema})
		if quiero == "" {
			noTiene(t, html, "background")
			continue
		}
		tieneTodo(t, html, "background-color:#"+quiero)
	}
}

// Las listas de Pages y Word delegan en un estilo de numeración (numStyleLink) y cambian el
// formato de un nivel con lvlOverride; numId 0 quita la lista heredada.
func TestDocxNumeracionEnlazada(t *testing.T) {
	numeracion := `<w:numbering ` + wNS + `>
<w:abstractNum w:abstractNumId="0"><w:numStyleLink w:val="Importado"/></w:abstractNum>
<w:abstractNum w:abstractNumId="1"><w:styleLink w:val="Importado"/><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/><w:lvlText w:val="-"/></w:lvl><w:lvl w:ilvl="1"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num>
<w:num w:numId="2"><w:abstractNumId w:val="0"/></w:num>
<w:num w:numId="3"><w:abstractNumId w:val="0"/><w:lvlOverride w:ilvl="0"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl></w:lvlOverride></w:num>
</w:numbering>`
	styles := `<w:styles ` + wNS + `><w:style w:type="numbering" w:styleId="Importado"><w:name w:val="Importado"/><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Lista"><w:name w:val="Lista"/><w:pPr><w:numPr><w:numId w:val="2"/></w:numPr></w:pPr></w:style></w:styles>`
	p := func(num, texto string) string {
		return `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="` + num + `"/></w:numPr></w:pPr><w:r><w:t>` + texto + `</w:t></w:r></w:p>`
	}
	cuerpo := p("2", "guion") + `<w:p><w:pPr><w:pStyle w:val="Lista"/></w:pPr><w:r><w:t>del estilo</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>suelto</w:t></w:r></w:p>` + p("3", "numerada") +
		`<w:p><w:pPr><w:pStyle w:val="Lista"/><w:numPr><w:numId w:val="0"/></w:numPr></w:pPr><w:r><w:t>sin lista</w:t></w:r></w:p>`
	html := importarDocx(t, cuerpo, map[string]string{"word/numbering.xml": numeracion, "word/styles.xml": styles})
	assertBlocksEqual(t, "numeración", `<ul><li>guion</li><li>del estilo</li></ul><p>suelto</p><ol><li>numerada</li></ol><p>sin lista</p>`, html)
	// La viñeta «-» de Pages llega a la lista; el «·» de Symbol de Word se queda en el disco.
	tieneTodo(t, html, `<ul style="list-style-type:'- '"><li>guion</li>`)
}

// Párrafo: espaciado, interlineado exacto, sangría francesa, sombreado y borde inferior; un
// borde «nil» no es una regla; una línea vacía conserva su alto; dos párrafos de una celda no
// se pegan.
func TestDocxPropiedadesDeParrafo(t *testing.T) {
	cuerpo := `<w:p><w:pPr><w:spacing w:before="120" w:after="240" w:line="360" w:lineRule="exact"/><w:ind w:left="1440" w:hanging="360"/>` +
		`<w:shd w:val="clear" w:fill="FFF2CC"/><w:pBdr><w:bottom w:val="single" w:sz="6" w:color="auto"/></w:pBdr></w:pPr><w:r><w:t>uno</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pBdr><w:top w:val="nil"/><w:bottom w:val="nil"/></w:pBdr><w:spacing w:after="0"/></w:pPr></w:p>` +
		`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="4"/></w:pBdr></w:pPr></w:p>` +
		`<w:p><w:r><w:t>a</w:t></w:r><w:r><w:rPr><w:vertAlign w:val="superscript"/></w:rPr><w:t>2</w:t></w:r><w:r><w:rPr><w:vertAlign w:val="subscript"/></w:rPr><w:t>i</w:t></w:r><w:r><w:rPr><w:vanish/></w:rPr><w:t>oculto</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>primero</w:t></w:r></w:p><w:p><w:r><w:t>segundo</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	html := importarDocx(t, cuerpo, nil)
	tieneTodo(t, html,
		`<p style="margin-top:8px;margin-bottom:16px;line-height:18pt;margin-left:96px;text-indent:-24px;background-color:#FFF2CC;border-bottom:1px solid #000000">uno</p>`,
		`<p style="margin-bottom:0"><br></p><hr>`,
		`a<sup>2</sup><sub>i</sub><span style="display:none">oculto</span>`,
		`<p>primero</p><p>segundo</p>`,
	)
}

// Tabla: los bordes de la tabla van fuera y insideH/insideV entre celdas; tcBorders manda; los
// anchos salen de la rejilla.
func TestDocxBordesYAnchosDeTabla(t *testing.T) {
	b := func(v string) string { return `w:val="` + v + `" w:sz="8" w:color="FF0000"` }
	cuerpo := `<w:tbl><w:tblPr><w:tblBorders><w:top ` + b("single") + `/><w:left ` + b("single") + `/><w:bottom ` + b("single") + `/><w:right ` + b("single") + `/>` +
		`<w:insideH ` + b("dotted") + `/><w:insideV w:val="nil"/></w:tblBorders></w:tblPr><w:tblGrid><w:gridCol w:w="1500"/><w:gridCol w:w="3000"/></w:tblGrid>` +
		`<w:tr><w:tc><w:p/></w:tc><w:tc><w:tcPr><w:tcBorders><w:right w:val="double" w:sz="4"/></w:tcBorders><w:vAlign w:val="center"/></w:tcPr><w:p/></w:tc></w:tr>` +
		`<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`
	html := importarDocx(t, cuerpo, nil)
	tieneTodo(t, html,
		`<table style="width:300px">`,
		`<td style="border-top:1px solid #FF0000;border-right:none;border-bottom:1px dotted #FF0000;border-left:1px solid #FF0000;width:100px">`,
		`<td style="border-top:1px solid #FF0000;border-right:3px double #000000;border-bottom:1px dotted #FF0000;border-left:none;width:200px;vertical-align:middle">`,
		`<td colspan="2" style="border-top:1px dotted #FF0000;border-right:1px solid #FF0000;border-bottom:1px solid #FF0000;border-left:1px solid #FF0000;width:300px">`,
	)
}

// El estilo sale escrito como lo serializa el navegador: la comilla simple de una fuente no se
// escapa, o el editor reescribiría el documento entero al montarlo.
func TestDocxEstiloComoElNavegador(t *testing.T) {
	html := importarDocx(t, `<w:p><w:r><w:rPr><w:rFonts w:ascii="Times New Roman"/></w:rPr><w:t>x</w:t></w:r></w:p>`, nil)
	tieneTodo(t, html, `<p style="font-family:'Times New Roman','Liberation Serif',Tinos,serif">x</p>`)
	noTiene(t, html, "&#39;")
}

// Lo que el corpus de .docx reales perdía: el texto de los cuadros de texto (Pages maqueta así
// sus plantillas), el resultado de un campo simple, las filas dentro de un control de
// contenido y la parte principal con otro nombre (Word en la web). Y lo que lo hinchaba: el
// mismo formato directo repetido en cada tramo.
func TestDocxLoQuePerdiaElCorpus(t *testing.T) {
	cuadro := `<w:r><mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><mc:Choice Requires="wps"><w:drawing><wp:anchor xmlns:wp="x"><a:graphic xmlns:a="y"><a:graphicData><wps:wsp xmlns:wps="z"><wps:txbx>` +
		`<w:txbxContent><w:p><w:r><w:t>En el cuadro</w:t></w:r></w:p></w:txbxContent></wps:txbx></wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><v:shape xmlns:v="v"><v:textbox><w:txbxContent><w:p><w:r><w:t>En el cuadro</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback></mc:AlternateContent></w:r>`
	lista := `<w:pPr><w:pStyle w:val="Lista"/></w:pPr>`
	cuerpo := `<w:p><w:r><w:t>Ancla</w:t></w:r>` + cuadro + `</w:p>` +
		`<w:p><w:fldSimple w:instr=" MERGEFIELD Nombre "><w:r><w:t>«Nombre»</w:t></w:r></w:fldSimple></w:p>` +
		`<w:tbl><w:sdt><w:sdtContent><w:tr><w:tc><w:p><w:r><w:t>fila en sdt</w:t></w:r></w:p></w:tc></w:tr></w:sdtContent></w:sdt></w:tbl>` +
		`<w:p>` + lista + `<w:r><w:t>uno</w:t></w:r></w:p><w:p>` + lista + `<w:r><w:t>dos</w:t></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:rFonts w:ascii="Georgia"/><w:sz w:val="22"/></w:rPr><w:t>mismo </w:t></w:r><w:r><w:rPr><w:rFonts w:ascii="Georgia"/><w:sz w:val="22"/><w:b/></w:rPr><w:t>formato</w:t></w:r></w:p>`
	styles := `<w:styles ` + wNS + `><w:style w:type="paragraph" w:styleId="Lista"><w:name w:val="Lista"/><w:pPr><w:spacing w:before="120" w:after="240"/><w:contextualSpacing/></w:pPr></w:style></w:styles>`
	files := map[string]string{
		"word/document22.xml": `<w:document ` + wNS + `><w:body>` + cuerpo + `</w:body></w:document>`,
		"word/styles.xml":     styles,
		"_rels/.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="/word/document22.xml"/></Relationships>`,
	}
	data, err := zipPackage(files)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Import("web.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	html := res.Content
	tieneTodo(t, html,
		"<p>Ancla</p><p>En el cuadro</p>", // después de su ancla, y una sola vez (no el respaldo VML)
		"«Nombre»",
		"fila en sdt",
		`<p style="margin-top:8px;margin-bottom:0">uno</p><p style="margin-top:0;margin-bottom:16px">dos</p>`,
		`<p style="font-family:Georgia,serif;font-size:11pt">mismo <strong>formato</strong></p>`,
	)
	if n := strings.Count(html, "En el cuadro"); n != 1 {
		t.Errorf("el cuadro sale %d veces", n)
	}
}

func TestDocxVinetaPropia(t *testing.T) {
	for _, c := range []struct{ texto, fuente, quiero string }{
		{"-", "Calibri", "-"},
		{"o", "Courier New", "o"},
		{"\uf0b7", "Symbol", ""},
		{"§", "Wingdings", ""},
		{"•", "", ""},
		{"%1.", "", ""},
		{"'", "", ""},
	} {
		if got := vinetaPropia(c.texto, c.fuente); got != c.quiero {
			t.Errorf("%q en %q: %q, quiero %q", c.texto, c.fuente, got, c.quiero)
		}
	}
}

// El formato condicional de un estilo de tabla (tblStylePr): la fila de encabezado y las bandas
// de una «Tabla de cuadrícula 4» de Word. Antes la tabla salía con los bordes del estilo y nada
// más: ni el encabezado en blanco sobre color ni las filas alternas.
func TestDocxFormatoCondicionalDeTabla(t *testing.T) {
	styles := `<w:styles ` + wNS + `>
<w:style w:type="table" w:styleId="Rejilla4"><w:name w:val="Grid Table 4"/>
<w:tblPr><w:tblStyleRowBandSize w:val="1"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="8EAADB"/><w:left w:val="single" w:sz="4" w:color="8EAADB"/><w:bottom w:val="single" w:sz="4" w:color="8EAADB"/><w:right w:val="single" w:sz="4" w:color="8EAADB"/><w:insideH w:val="single" w:sz="4" w:color="8EAADB"/><w:insideV w:val="single" w:sz="4" w:color="8EAADB"/></w:tblBorders></w:tblPr>
<w:tblStylePr w:type="firstRow"><w:rPr><w:b/><w:color w:val="FFFFFF"/></w:rPr><w:tcPr><w:tcBorders><w:top w:val="single" w:sz="4" w:color="4472C4"/><w:insideV w:val="nil"/></w:tcBorders><w:shd w:val="clear" w:color="auto" w:fill="4472C4"/></w:tcPr></w:tblStylePr>
<w:tblStylePr w:type="lastRow"><w:rPr><w:b/></w:rPr><w:tcPr><w:tcBorders><w:top w:val="double" w:sz="4" w:color="4472C4"/></w:tcBorders></w:tcPr></w:tblStylePr>
<w:tblStylePr w:type="firstCol"><w:rPr><w:i/></w:rPr></w:tblStylePr>
<w:tblStylePr w:type="band1Horz"><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"/></w:tcPr></w:tblStylePr>
<w:tblStylePr w:type="band1Vert"><w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="FFC0FF"/></w:tcPr></w:tblStylePr>
</w:style></w:styles>`
	celda := func(txt string) string {
		return `<w:tc><w:tcPr><w:tcW w:w="2000" w:type="dxa"/></w:tcPr><w:p><w:r><w:t>` + txt + `</w:t></w:r></w:p></w:tc>`
	}
	tabla := func(look string) string {
		return `<w:tbl><w:tblPr><w:tblStyle w:val="Rejilla4"/>` + look + `</w:tblPr><w:tblGrid><w:gridCol w:w="2000"/><w:gridCol w:w="2000"/></w:tblGrid>` +
			`<w:tr>` + celda("Cab") + celda("Era") + `</w:tr><w:tr>` + celda("Uno") + celda("U2") + `</w:tr><w:tr>` + celda("Dos") + celda("D2") + `</w:tr><w:tr>` + celda("Tres") + celda("T2") + `</w:tr></w:tbl>`
	}
	for _, look := range []string{
		`<w:tblLook w:firstRow="1" w:lastRow="1" w:firstColumn="1" w:lastColumn="0" w:noHBand="0" w:noVBand="1"/>`,
		`<w:tblLook w:val="04E0"/>`, // lo mismo en la forma de Word 2007
	} {
		html := importarDocx(t, tabla(look), map[string]string{"word/styles.xml": styles})
		celdas := strings.Split(html, "<td")[1:]
		if len(celdas) != 8 {
			t.Fatalf("%s: %d celdas en\n%s", look, len(celdas), html)
		}
		for i, quiero := range [][]string{
			{"background-color:#4472C4", "<strong>", "color:#FFFFFF", "<em>"}, // esquina: encabezado y primera columna
			{"background-color:#4472C4", "<strong>", "color:#FFFFFF", "border-right:1px solid #8EAADB", "border-left:none"},
			{"background-color:#D9E2F3", "<em>Uno"}, // primera banda
			{"background-color:#D9E2F3"},
			{"<em>Dos"}, // segunda banda: sin sombreado
			{},
			{"border-top:3px double #4472C4"}, // última fila; su negrita no se sabe al leerla
			{"border-top:3px double #4472C4"},
		} {
			tieneTodo(t, celdas[i], quiero...)
		}
		noTiene(t, celdas[4], "background-color")
		noTiene(t, celdas[3], "<em>")
	}
	// Sin encabezado ni primera columna y con las dos bandas, la de columnas tapa a la de filas,
	// como en la referencia de LibreOffice.
	html := importarDocx(t, tabla(`<w:tblLook w:val="0000"/>`), map[string]string{"word/styles.xml": styles})
	celdas := strings.Split(html, "<td")[1:]
	tieneTodo(t, celdas[0], "background-color:#FFC0FF")
	tieneTodo(t, celdas[1], "background-color:#D9E2F3")
	tieneTodo(t, celdas[2], "background-color:#FFC0FF")
	noTiene(t, celdas[3], "background-color", "<strong>")
}

// Las paradas propias de un párrafo (w:tabs) se acumulan entre el estilo y el párrafo, una
// «clear» quita la heredada y se cuentan desde la sangría izquierda. Antes el tabulador
// avanzaba siempre a la parada por defecto: la fecha de un currículum, alineada a la derecha
// del renglón, quedaba pegada al cargo.
func TestDocxParadasDeTabulacion(t *testing.T) {
	styles := `<w:styles ` + wNS + `>
<w:style w:type="paragraph" w:styleId="Cargo"><w:name w:val="Cargo"/><w:pPr><w:tabs><w:tab w:val="left" w:pos="1440"/><w:tab w:val="right" w:leader="dot" w:pos="9360"/></w:tabs></w:pPr></w:style>
</w:styles>`
	cuerpo := `<w:p><w:pPr><w:pStyle w:val="Cargo"/><w:tabs><w:tab w:val="clear" w:pos="1440"/><w:tab w:val="center" w:pos="4680"/><w:tab w:val="num" w:pos="720"/></w:tabs><w:ind w:left="720"/></w:pPr>` +
		`<w:r><w:t>Cargo</w:t></w:r><w:r><w:tab/><w:t>Centro</w:t><w:tab/><w:t>2020</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:ind w:left="360" w:hanging="360"/></w:pPr><w:r><w:t>1.</w:t><w:tab/><w:t>Uno</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:tabs><w:tab w:val="right" w:pos="2840"/></w:tabs><w:ind w:left="3200" w:hanging="3200"/></w:pPr><w:r><w:tab/><w:t>Fax</w:t><w:tab/><w:t>011</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="Cargo"/></w:pPr><w:r><w:t>Sin tabulador</w:t></w:r></w:p>`
	html := importarDocx(t, cuerpo, map[string]string{"word/styles.xml": styles})
	tieneTodo(t, html,
		"--sytue-tabs:264px center, 576px right dot", // (4680-720)/15 y (9360-720)/15; la de 1440 se quitó
		"--sytue-tabs:0px left",                      // la sangría francesa es una parada
		"--sytue-tabs:-24px right, 0px left",         // a la izquierda de la sangría: para la primera línea
	)
	if strings.Count(html, "--sytue-tabs") != 3 {
		t.Errorf("un párrafo sin tabulador no necesita paradas:\n%s", html)
	}
}

// Las ecuaciones de Office (m:oMath) entran como texto en el formato lineal de Word, y una
// m:oMathPara va centrada. Antes se tiraban enteras: el texto de la ecuación desaparecía.
func TestDocxEcuaciones(t *testing.T) {
	mNS := `xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"`
	r := func(s string) string { return `<m:r><m:t>` + s + `</m:t></m:r>` }
	e := func(s string) string { return `<m:e>` + s + `</m:e>` }
	ecuacion := `<m:oMath>` +
		`<m:f><m:num>` + r("a+b") + `</m:num><m:den>` + r("2") + `</m:den></m:f>` + r("=") +
		`<m:sSup>` + e(r("x")) + `<m:sup>` + r("n+1") + `</m:sup></m:sSup>` + r("+") +
		`<m:rad><m:radPr><m:degHide/></m:radPr><m:deg/>` + e(r("y")) + `</m:rad>` + r("+") +
		`<m:nary><m:naryPr><m:chr m:val="∑"/></m:naryPr><m:sub>` + r("i=1") + `</m:sub><m:sup>` + r("n") + `</m:sup>` + e(`<m:sSub>`+e(r("a"))+`<m:sub>`+r("i")+`</m:sub></m:sSub>`) + `</m:nary>` +
		`<m:d><m:dPr><m:begChr m:val="["/><m:endChr m:val="]"/></m:dPr>` + e(r("1")) + e(r("2")) + `</m:d>` +
		`<m:d>` + e(`<m:f><m:fPr><m:type m:val="noBar"/></m:fPr><m:num>`+r("n")+`</m:num><m:den>`+r("k")+`</m:den></m:f>`) + `</m:d>` +
		`</m:oMath>`
	cuerpo := `<w:p ` + mNS + `><m:oMathPara>` + ecuacion + `</m:oMathPara></w:p>` +
		`<w:p ` + mNS + `><w:r><w:t xml:space="preserve">Si </w:t></w:r><m:oMath>` + r("E=m") + `<m:sSup>` + e(r("c")) + `<m:sup>` + r("2") + `</m:sup></m:sSup></m:oMath><w:r><w:t xml:space="preserve"> entonces</w:t></w:r></w:p>`
	html := importarDocx(t, cuerpo, nil)
	tieneTodo(t, html,
		`text-align:center`,
		`(a+b)/2=x^(n+1)+√y+∑_(i=1)^n a_i[1│2](n¦k)`,
		`Si <span style="font-family:'Cambria Math',serif">E=mc^2</span> entonces`,
	)
}

// Los marcos de texto (framePr) y los cuadros anclados (wp:anchor con wps:txbx) van en un
// div[data-marco] que flota a su lado con su ancho: la etiqueta de la columna izquierda de un
// currículum de Pages queda a la izquierda de su texto, no debajo y a la derecha. El marco neutro
// que Pages pone en pPrDefault no es un marco. Y la exportación los devuelve como framePr.
func TestDocxMarcosDeTexto(t *testing.T) {
	styles := `<w:styles ` + wNS + `><w:docDefaults><w:pPrDefault><w:pPr>` +
		`<w:framePr w:anchorLock="0" w:w="0" w:h="0" w:vSpace="0" w:hSpace="0" w:xAlign="left" w:y="0" w:hRule="exact" w:vAnchor="margin"/>` +
		`</w:pPr></w:pPrDefault></w:docDefaults></w:styles>`
	marco := `<w:framePr w:w="2400" w:wrap="around" w:hAnchor="margin" w:x="-300" w:y="0" w:hSpace="180" w:vSpace="60"/>`
	cuadro := `<w:r><w:drawing><wp:anchor distT="0" distB="0" distL="114300" distR="114300" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">` +
		`<wp:positionH relativeFrom="margin"><wp:posOffset>-6350</wp:posOffset></wp:positionH>` +
		`<wp:positionV relativeFrom="line"><wp:posOffset>259588</wp:posOffset></wp:positionV>` +
		`<wp:extent cx="1803400" cy="582367"/><wp:wrapNone/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData>` +
		`<wps:wsp xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:txbx><w:txbxContent>` +
		`<w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:t>APTITUDES</w:t></w:r></w:p></w:txbxContent></wps:txbx></wps:wsp>` +
		`</a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>`
	cuerpo := `<w:p><w:r><w:t>Antes</w:t></w:r>` + cuadro + `</w:p>` +
		`<w:p><w:pPr>` + marco + `</w:pPr><w:r><w:t>Uno</w:t></w:r></w:p>` +
		`<w:p><w:pPr>` + marco + `</w:pPr><w:r><w:t>Dos</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Después</w:t></w:r></w:p>`
	html := importarDocx(t, cuerpo, map[string]string{"word/styles.xml": styles})
	tieneTodo(t, html,
		`<p>Antes</p><div data-marco="" style="float:left;width:189.33px;margin-left:-0.67px"><p style="text-align:right">APTITUDES</p></div>`,
		`<div data-marco="" style="float:left;width:160px;margin-left:-20px;margin-right:12px;margin-bottom:4px"><p>Uno</p><p>Dos</p></div><p>Después</p>`,
	)
	if strings.Count(html, "data-marco") != 2 {
		t.Errorf("el marco neutro de Pages no es un marco:\n%s", html)
	}

	salida, _, _, err := Export(KindDocument, "", stateWithContent(t, "doc:html", html), FormatOOXML)
	if err != nil {
		t.Fatal(err)
	}
	partes, _ := readZip(salida)
	mustContain(t, "marco exportado", string(partes["word/document.xml"]),
		`<w:framePr w:w="2400" w:wrap="around" w:hAnchor="margin" w:x="-300" w:hSpace="180" w:vSpace="60" w:vAnchor="text" w:y="0"/>`)
	vuelta, err := Import("vuelta.docx", salida)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`<div data-marco="" style="float:left;width:160px;margin-left:-20px;margin-right:12px;margin-bottom:4px"><p>Uno</p><p>Dos</p></div>`,
		`<div data-marco="" style="float:left;width:189.33px;margin-left:-0.67px"><p style="text-align:right">APTITUDES</p></div>`,
	} {
		mustContain(t, "marco de vuelta", vuelta.Content, q)
	}
}
