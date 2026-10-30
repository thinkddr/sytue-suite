// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

// pages09 arma un .pages '09 mínimo con este índice: lo que hace falta para reconocerlo
// (index.xml + buildVersionHistory.plist) y nada más.
func pages09(t *testing.T, indice string, extra map[string][]byte) []byte {
	t.Helper()
	e := map[string][]byte{"index.xml": []byte(indice), "buildVersionHistory.plist": []byte("<plist/>")}
	for k, v := range extra {
		e[k] = v
	}
	return zipDe(t, e)
}

const cabecera09 = `<?xml version="1.0"?><sl:document xmlns:sf="http://developer.apple.com/namespaces/sf" ` +
	`xmlns:sfa="http://developer.apple.com/namespaces/sfa" xmlns:sl="http://developer.apple.com/namespaces/sl">`

// estilos09Prueba: un título, un párrafo centrado que hereda de él la alineación pero no
// el nombre, y estilos de carácter con negrita, cursiva, subrayado, tachado y color; uno
// de ellos define la negrita como nada (sf:null) y gana a la del párrafo.
const estilos09Prueba = `<sf:stylesheet>
<sf:paragraphstyle sf:ident="ps-titulo" sf:name="Heading 2"><sf:property-map><sf:bold><sf:number sfa:number="1"/></sf:bold><sf:alignment/></sf:property-map></sf:paragraphstyle>
<sf:paragraphstyle sf:ident="ps-centro" sf:name="Body"><sf:property-map><sf:alignment><sf:number sfa:number="2"/></sf:alignment><sf:bold><sf:number sfa:number="1"/></sf:bold></sf:property-map></sf:paragraphstyle>
<sf:paragraphstyle sfa:ID="ps-anon" sf:parent-ident="ps-centro"><sf:property-map><sf:alignment/></sf:property-map></sf:paragraphstyle>
<sf:characterstyle sfa:ID="cs-i"><sf:property-map><sf:italic><sf:number sfa:number="1"/></sf:italic><sf:bold><sf:null/></sf:bold></sf:property-map></sf:characterstyle>
<sf:characterstyle sfa:ID="cs-us"><sf:property-map><sf:underline><sf:number sfa:number="1"/></sf:underline><sf:strikethru><sf:number sfa:number="1"/></sf:strikethru><sf:fontColor><sf:color sfa:r="1" sfa:g="0" sfa:b="0.5"/></sf:fontColor></sf:property-map></sf:characterstyle>
</sf:stylesheet>`

func TestPages09ConvierteElTextoConSuFormato(t *testing.T) {
	indice := cabecera09 + estilos09Prueba +
		`<sl:section-prototypes><sl:prototype><sf:text-storage sf:kind="body"><sf:text-body><sf:p>MUESTRA DE PLANTILLA</sf:p></sf:text-body></sf:text-storage></sl:prototype></sl:section-prototypes>` +
		`<sf:headers><sf:header sf:name="SFWPDefaultOddHeaderIdentifier"><sf:text-storage sf:kind="header"><sf:text-body><sf:p>Cabecera<sf:tab/><sf:page-number sf:value="1"/></sf:p></sf:text-body></sf:text-storage></sf:header></sf:headers>` +
		`<sf:text-storage sf:kind="body"><sf:footnotes><sf:text-storage sf:kind="footnote"><sf:text-body>` +
		`<sf:p><sf:span><sf:footnote-mark sf:mark="1"/></sf:span> La nota<sf:footnotebr/></sf:p></sf:text-body></sf:text-storage></sf:footnotes>` +
		`<sf:text-body><sf:section><sf:layout>` +
		`<sf:p sf:style="ps-titulo">Capítulo &amp; uno<sf:br/></sf:p>` +
		`<sf:p sf:style="ps-anon">Centrado <sf:span sf:style="cs-i">cursiva</sf:span> y <sf:span sf:style="cs-us">raro</sf:span><sf:footnote sf:autonumber="1"/><sf:br/>segundo<sf:lnbr/>tras salto<sf:br/></sf:p>` +
		`<sf:p><sf:br/></sf:p>` +
		`<sf:p><sf:ghost-text>Muestra visible</sf:ghost-text> en <sf:link href="https://sytue.eu"><sf:span>enlace</sf:span></sf:link>.<sf:br/></sf:p>` +
		`<sf:p/>` +
		`</sf:layout></sf:section></sf:text-body></sf:text-storage></sl:document>`

	res, err := ImportLocalized("informe.pages", pages09(t, indice, nil), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindDocument {
		t.Fatalf("kind %q", res.Kind)
	}
	for _, quiero := range []string{
		`<h2>Capítulo &amp; uno</h2>`,
		// La alineación se hereda del padre; la negrita del párrafo, también, salvo donde
		// el estilo de carácter la define como nada.
		`<p style="text-align:center"><strong>Centrado </strong><em>cursiva</em>`,
		`<strong><u><s><span style="color:#FF0080">raro</span></s></u></strong>`,
		// Varios sf:br en un mismo sf:p son varios párrafos; sf:lnbr es un salto de línea.
		`<strong>segundo</strong><br><strong>tras salto</strong></p>`,
		`<p></p>`,
		`Muestra visible en <a href="https://sytue.eu">enlace</a>.`,
		`<sup data-footnote-ref="1"`,
		`<li id="fn-1" value="1">La nota</li>`,
		`<div data-doc-header`, `Cabecera` + "\t" + `<span data-page-field`,
	} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, res.Content)
		}
	}
	for _, no := range []string{"MUESTRA DE PLANTILLA", "<blockquote>", "<img"} {
		if strings.Contains(res.Content, no) {
			t.Errorf("sobra %q en:\n%s", no, res.Content)
		}
	}
}

func TestPages09ConvierteListasConNiveles(t *testing.T) {
	estilos := `<sf:stylesheet>` +
		`<sf:liststyle sfa:ID="ls-vinetas"><sf:property-map><sf:listLabelTypes><sf:array>` +
		`<sf:list-label-typeinfo sf:type="text"><sf:text-label sf:type="bullet"/></sf:list-label-typeinfo>` +
		`<sf:list-label-typeinfo sf:type="image"/>` +
		`</sf:array></sf:listLabelTypes></sf:property-map></sf:liststyle>` +
		`<sf:liststyle sfa:ID="ls-num"><sf:property-map><sf:listLabelTypes><sf:array>` +
		`<sf:list-label-typeinfo sf:type="text"><sf:text-label sf:type="decimal"/></sf:list-label-typeinfo>` +
		`</sf:array></sf:listLabelTypes></sf:property-map></sf:liststyle>` +
		`<sf:paragraphstyle sfa:ID="ps-v0"><sf:property-map><sf:listStyle><sf:liststyle-ref sfa:IDREF="ls-vinetas"/></sf:listStyle><sf:listLevel><sf:number sfa:number="0"/></sf:listLevel></sf:property-map></sf:paragraphstyle>` +
		`<sf:paragraphstyle sfa:ID="ps-v1" sf:parent-ident="ps-v0"><sf:property-map><sf:listLevel><sf:number sfa:number="1"/></sf:listLevel></sf:property-map></sf:paragraphstyle>` +
		`<sf:paragraphstyle sfa:ID="ps-ord"><sf:property-map><sf:listStyle><sf:liststyle-ref sfa:IDREF="ls-num"/></sf:listStyle><sf:listLevel><sf:number sfa:number="0"/></sf:listLevel></sf:property-map></sf:paragraphstyle>` +
		`</sf:stylesheet>`
	indice := cabecera09 + estilos +
		`<sf:text-storage sf:kind="body"><sf:text-body>` +
		`<sf:p sf:style="ps-v0">Punto 1<sf:br/></sf:p>` +
		`<sf:p sf:style="ps-v1">Subpunto 1.1<sf:br/></sf:p>` +
		`<sf:p sf:style="ps-ord">Paso A<sf:br/>Paso B<sf:br/></sf:p>` +
		`</sf:text-body></sf:text-storage></sl:document>`
	res, err := Import("listas.pages", pages09(t, indice, nil))
	if err != nil {
		t.Fatal(err)
	}
	const quiero = `<ul><li>Punto 1<ul><li>Subpunto 1.1</li></ul></li></ul><ol><li>Paso A</li><li>Paso B</li></ol>`
	if res.Content != quiero {
		t.Errorf("listas:\n%s\nquiero:\n%s", res.Content, quiero)
	}
}

// Lo que no se convierte (aquí, una tabla en línea) no desaparece en silencio: el aviso
// de conversión parcial va delante y la vista previa del fichero, al final.
func TestPages09ParcialLlevaAvisoYVistaPrevia(t *testing.T) {
	indice := cabecera09 + `<sf:text-storage sf:kind="body"><sf:text-body>` +
		`<sf:p>Antes<sf:br/></sf:p><sf:p><sf:attachment-ref sfa:IDREF="t-0" sf:kind="tabular-attachment"/><sf:br/></sf:p>` +
		`</sf:text-body></sf:text-storage></sl:document>`
	res, err := ImportLocalized("informe.pages", pages09(t, indice, map[string][]byte{"QuickLook/Thumbnail.jpg": jpegDe(t, 8, 8)}), false, "es")
	if err != nil {
		t.Fatal(err)
	}
	aviso := strings.Index(res.Content, "Conversión parcial · documento .pages")
	texto := strings.Index(res.Content, "<p>Antes</p>")
	img := strings.Index(res.Content, `<img src="data:image/jpeg;base64,`)
	if aviso < 0 || texto < aviso || img < texto {
		t.Errorf("quiero aviso, texto y vista previa en ese orden (%d, %d, %d):\n%.600s", aviso, texto, img, res.Content)
	}
}

// El fichero de Tika: el cuerpo entra y el texto de muestra de la plantilla, no.
func TestPages09DeTika(t *testing.T) {
	res, err := ImportLocalized("viejo.pages", leerIWork(t, "testPages.pages"), false, "en")
	if err != nil {
		t.Fatal(err)
	}
	// Su tabla entra como tabla, así que ya no hay aviso de conversión parcial.
	for _, quiero := range []string{"Sample pages document", "Some plain text to parse.", "A text box with text.", "<td>Column one</td>", "<td>Cell nine</td>"} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%.800s", quiero, res.Content)
		}
	}
	if strings.Contains(res.Content, "Partial conversion") {
		t.Errorf("no debería llevar el aviso de conversión parcial:\n%.400s", res.Content)
	}
}

// Los topes del índice: si salta uno, el fichero se abre por el escalón que no falla. Y
// un índice en gzip se lee igual.
func TestPages09TopesYGzip(t *testing.T) {
	hondo := cabecera09 + `<sf:text-storage sf:kind="body"><sf:text-body><sf:p>` +
		strings.Repeat("<sf:span>", maxProfundidad09) + "x" + strings.Repeat("</sf:span>", maxProfundidad09) +
		`</sf:p></sf:text-body></sf:text-storage></sl:document>`
	res, err := Import("hondo.pages", pages09(t, hondo, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "Conversion not available · .pages document") {
		t.Errorf("un índice más hondo que el tope debería abrirse con el aviso:\n%.300s", res.Content)
	}

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte(cabecera09 + `<sf:text-storage sf:kind="body"><sf:text-body><sf:p>comprimido</sf:p></sf:text-body></sf:text-storage></sl:document>`))
	_ = zw.Close()
	res, err = Import("gz.pages", zipDe(t, map[string][]byte{"index.xml.gz": gz.Bytes(), "buildVersionHistory.plist": nil}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "<p>comprimido</p>" {
		t.Errorf("index.xml.gz: %q", res.Content)
	}

	// Un índice sin texto (todo en tablas, o vacío) no es un documento vacío: es el aviso.
	res, err = Import("vacio.pages", pages09(t, cabecera09+`<sf:text-storage sf:kind="body"><sf:text-body><sf:p/></sf:text-body></sf:text-storage></sl:document>`, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "Conversion not available") {
		t.Errorf("sin texto debería abrirse con el aviso:\n%.300s", res.Content)
	}
}

func TestPages09ConvierteImagenesYComentarios(t *testing.T) {
	indice := cabecera09 +
		`<sf:binaries>` +
		`<sf:unfiltered sfa:ID="bin-1"><sf:size sfa:w="300" sfa:h="150"/><sf:data sf:path="Data/foto.jpg"/></sf:unfiltered>` +
		`<sf:unfiltered sfa:ID="bin-2"><sf:size sfa:w="1200" sfa:h="300"/><sf:data sf:path="ancha.jpg"/></sf:unfiltered>` +
		`</sf:binaries>` +
		`<sl:drawables><sl:page-group>` +
		`<sf:image><sf:geometry><sf:size sfa:w="1200" sfa:h="300"/></sf:geometry><sf:binary-ref sfa:IDREF="bin-2"/></sf:image>` +
		`<sf:sticky-note><sf:text-storage><sf:text-body><sf:p>Nota suelta</sf:p></sf:text-body></sf:text-storage></sf:sticky-note>` +
		`</sl:page-group></sl:drawables>` +
		`<sf:text-storage sf:kind="body">` +
		`<sf:attachments><sf:attachment sfa:ID="adj-1"><sf:media><sf:content><sf:image-media><sf:filtered-image>` +
		`<sf:unfiltered-ref sfa:IDREF="bin-1"/>` +
		`</sf:filtered-image></sf:image-media></sf:content></sf:media></sf:attachment></sf:attachments>` +
		`<sf:annotations><sf:annotation sf:target="c-1" sf:author="Ana" sf:date="2026:10:29:12:00:00">` +
		`<sf:text-storage><sf:text-body><sf:p>Revisar cifra</sf:p></sf:text-body></sf:text-storage>` +
		`</sf:annotation></sf:annotations>` +
		`<sf:text-body><sf:p>Total: <sf:annotation-field sfa:ID="c-1">42</sf:annotation-field> <sf:attachment-ref sfa:IDREF="adj-1"/><sf:br/></sf:p></sf:text-body>` +
		`</sf:text-storage></sl:document>`
	extra := map[string][]byte{
		"Data/foto.jpg":       jpegDe(t, 30, 15),
		"Data/foto-small.jpg": jpegDe(t, 4, 4),
		"Data/ancha.jpg":      jpegDe(t, 120, 30),
	}
	paq := pages09(t, indice, extra)

	res, err := ImportLocalized("informe.pages", paq, true, "es")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 2 {
		t.Fatalf("comentarios = %+v, quiero 2", res.Comments)
	}
	for _, quiero := range []string{
		`<mark data-comment-id="` + res.Comments[0].ID + `">42</mark>`,
		`data-w="300" data-h="150"`,
		`data-w="600" data-h="150"`,
	} {
		if !strings.Contains(res.Content, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, res.Content)
		}
	}
	if strings.Contains(res.Content, "Conversión parcial") {
		t.Errorf("no debería llevar aviso de conversión parcial:\n%s", res.Content)
	}
	if c := res.Comments[0]; c.Body != "Revisar cifra" || c.Quote != "42" || c.Author != "Ana" || c.Date != "2026-10-29T12:00:00Z" || !c.Anchored {
		t.Errorf("comentario anclado: %+v", c)
	}
	if c := res.Comments[1]; c.Body != "Nota suelta" || c.Anchored {
		t.Errorf("nota adhesiva: %+v", c)
	}

	sinCom, err := ImportLocalized("informe.pages", paq, false, "es")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sinCom.Content, "<mark") || len(sinCom.Comments) != 0 {
		t.Errorf("con comments=false no deben salir marcas ni comentarios: %s / %+v", sinCom.Content, sinCom.Comments)
	}
}
