// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"encoding/xml"
	"strings"
	"time"
)

func xmlEsc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// xmp es el paquete de metadatos XMP del catálogo. PDF/UA lo exige con dc:title y con la
// declaración pdfuaid:part=1 (ISO 14289-1, 5): sin ella el fichero no dice qué norma
// pretende cumplir. El esquema de extensión describe pdfuaid, como pide XMP para una
// propiedad que no es de los esquemas estándar.
func xmp(titulo, productor, idioma string, fecha time.Time) []byte {
	var b strings.Builder
	b.WriteString("<?xpacket begin=\"\xef\xbb\xbf\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")
	b.WriteString(`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">` + xmlEsc(titulo) + `</rdf:li></rdf:Alt></dc:title>` +
		`<dc:language><rdf:Bag><rdf:li>` + xmlEsc(idioma) + `</rdf:li></rdf:Bag></dc:language>` +
		`<dc:format>application/pdf</dc:format></rdf:Description>` + "\n")
	b.WriteString(`<rdf:Description rdf:about="" xmlns:pdf="http://ns.adobe.com/pdf/1.3/"><pdf:Producer>` +
		xmlEsc(productor) + `</pdf:Producer></rdf:Description>` + "\n")
	if !fecha.IsZero() {
		b.WriteString(`<rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/"><xmp:CreateDate>` +
			fecha.UTC().Format(time.RFC3339) + `</xmp:CreateDate></rdf:Description>` + "\n")
	}
	b.WriteString(`<rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/"><pdfuaid:part>1</pdfuaid:part></rdf:Description>` + "\n")
	b.WriteString(`<rdf:Description rdf:about="" xmlns:pdfaExtension="http://www.aiim.org/pdfa/ns/extension/" ` +
		`xmlns:pdfaSchema="http://www.aiim.org/pdfa/ns/schema#" xmlns:pdfaProperty="http://www.aiim.org/pdfa/ns/property#">` +
		`<pdfaExtension:schemas><rdf:Bag><rdf:li rdf:parseType="Resource">` +
		`<pdfaSchema:schema>PDF/UA identification schema</pdfaSchema:schema>` +
		`<pdfaSchema:namespaceURI>http://www.aiim.org/pdfua/ns/id/</pdfaSchema:namespaceURI>` +
		`<pdfaSchema:prefix>pdfuaid</pdfaSchema:prefix>` +
		`<pdfaSchema:property><rdf:Seq><rdf:li rdf:parseType="Resource">` +
		`<pdfaProperty:name>part</pdfaProperty:name><pdfaProperty:valueType>Integer</pdfaProperty:valueType>` +
		`<pdfaProperty:category>internal</pdfaProperty:category>` +
		`<pdfaProperty:description>Indicates, which part of ISO 14289 standard is followed</pdfaProperty:description>` +
		`</rdf:li></rdf:Seq></pdfaSchema:property></rdf:li></rdf:Bag></pdfaExtension:schemas></rdf:Description>` + "\n")
	b.WriteString("</rdf:RDF></x:xmpmeta>\n<?xpacket end=\"w\"?>")
	return []byte(b.String())
}
