// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"testing"
)

func TestDOCXEstilosLocalizadosHerenciaYCiclos(t *testing.T) {
	const styles = `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:style w:type="paragraph" w:styleId="Ttulo1"><w:name w:val="heading 1"/></w:style>
<w:style w:type="paragraph" w:styleId="Cita"><w:name w:val="Quote"/></w:style>
<w:style w:type="paragraph" w:styleId="Informe"><w:basedOn w:val="Ttulo1"/></w:style>
<w:style w:type="paragraph" w:styleId="Seccion"><w:pPr><w:outlineLvl w:val="2"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:basedOn w:val="Ttulo1"/><w:pPr><w:outlineLvl w:val="9"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="CicloA"><w:basedOn w:val="CicloB"/></w:style>
<w:style w:type="paragraph" w:styleId="CicloB"><w:basedOn w:val="CicloA"/></w:style>
<w:style w:type="character" w:styleId="Caracter"><w:name w:val="heading 1"/></w:style>
</w:styles>`
	m := parseDocxStyles([]byte(styles))
	for id, want := range map[string]string{"Ttulo1": "Heading1", "Cita": "Quote", "Informe": "Heading1", "Seccion": "Heading3", "Heading2": "", "CicloA": "", "CicloB": ""} {
		if got, ok := m[id]; !ok || got != want {
			t.Errorf("%s = %q (declarado %v), esperado %q", id, got, ok, want)
		}
	}
	if _, ok := m["Caracter"]; ok {
		t.Fatal("un estilo de carácter se aplicó a un párrafo")
	}
	data, err := zipPackage(map[string]string{
		"word/styles.xml": styles,
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
			`<w:p><w:pPr><w:pStyle w:val="Informe"/></w:pPr><w:r><w:t>Informe</w:t></w:r></w:p>` +
			`<w:p><w:pPr><w:pStyle w:val="Cita"/></w:pPr><w:r><w:t>Cita</w:t></w:r></w:p>` +
			`<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Normal</w:t></w:r></w:p>` +
			`</w:body></w:document>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Import("estilos.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	assertBlocksEqual(t, "estilos semánticos", "<h1>Informe</h1><blockquote>Cita</blockquote><p>Normal</p>", res.Content)
}
