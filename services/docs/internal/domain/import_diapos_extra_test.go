// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Lo que otras herramientas escriben y el exportador de Sytue no: los esquemas de página
// de LibreOffice y los subtítulos que no llevan la extensión de Sytue.

// TestEsquemasDeLibreOfficeSeMapeanADisenos: los AL1T0 y AL2T1 de un .odp de LibreOffice
// de verdad entran como diseños de Sytue, con la geometría del esquema y el nombre de su
// composición, y las diapositivas dicen cuál llevan. Los de folleto (AL0T26) no.
func TestEsquemasDeLibreOfficeSeMapeanADisenos(t *testing.T) {
	datos, err := os.ReadFile("testdata/odf/impress-7-1.odp")
	if err != nil {
		t.Fatal(err)
	}
	m := reimportar(t, "impress-7-1.odp", datos)
	if m.Master == nil {
		t.Fatal("sin patrón")
	}
	por := map[string]slideLayout{}
	for _, l := range m.Master.Layouts {
		por[l.ID] = l
	}
	if len(por) != 2 {
		t.Fatalf("diseños = %+v", m.Master.Layouts)
	}
	titulo, cuerpo := por["lo-AL1T0"], por["lo-AL2T1"]
	if titulo.Name != "Título y subtítulo" || cuerpo.Name != "Título y contenido" {
		t.Errorf("nombres %q y %q", titulo.Name, cuerpo.Name)
	}
	if len(cuerpo.Boxes) != 2 || !strings.HasPrefix(cuerpo.Boxes[0].HTML, "<h1>") || cuerpo.Boxes[1].Y < cuerpo.Boxes[0].Y {
		t.Errorf("marcadores del cuerpo = %+v", cuerpo.Boxes)
	}
	if b := cuerpo.Boxes[1]; b.W < 80 || b.W > 90 || b.H < 50 {
		t.Errorf("geometría del contenido (28 × 15,75 cm) = %+v", b)
	}
	usados := map[string]int{}
	for _, s := range m.Slides {
		usados[s.Layout]++
	}
	if usados["lo-AL1T0"] != 1 || usados["lo-AL2T1"] != 9 || len(usados) != 2 {
		t.Errorf("diseños de las diapositivas = %v", usados)
	}
}

// TestUnDisenoNuncaRepiteNombre: dos esquemas de la misma composición (el contenido es
// de otro tipo en cada uno) no salen con el mismo nombre.
func TestUnDisenoNuncaRepiteNombre(t *testing.T) {
	styles := `<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"><office:styles>` +
		`<style:presentation-page-layout style:name="AL2T1"><presentation:placeholder presentation:object="title" svg:x="1cm" svg:y="1cm" svg:width="20cm" svg:height="3cm"/><presentation:placeholder presentation:object="outline" svg:x="1cm" svg:y="5cm" svg:width="20cm" svg:height="10cm"/></style:presentation-page-layout>` +
		`<style:presentation-page-layout style:name="AL2T2"><presentation:placeholder presentation:object="title" svg:x="1cm" svg:y="1cm" svg:width="20cm" svg:height="3cm"/><presentation:placeholder presentation:object="chart" svg:x="1cm" svg:y="5cm" svg:width="20cm" svg:height="10cm"/></style:presentation-page-layout>` +
		`<style:presentation-page-layout style:name="AL0T26"><presentation:placeholder presentation:object="handout" svg:x="1cm" svg:y="1cm" svg:width="5cm" svg:height="-1cm"/></style:presentation-page-layout>` +
		`</office:styles></office:document-styles>`
	model := &slidesModel{}
	ids := readODPLayouts([]byte(styles), 28, 15.75, model)
	if len(ids) != 2 || ids["AL0T26"] != "" {
		t.Fatalf("ids = %v", ids)
	}
	if a, b := model.Master.Layouts[0].Name, model.Master.Layouts[1].Name; a != "Título y contenido" || b != "Título y contenido 2" {
		t.Errorf("nombres %q y %q", a, b)
	}
}

var reExtSytuePista = regexp.MustCompile(`(?s)<p:ext uri="\{8C3F7A52-2B1E-4D6A-9F40-5E7B1C0D3A19\}">.*?</p:ext>`)

func modeloUnVideo() slidesModel {
	return slidesModel{Slides: []slideT{{ID: "s1", Boxes: []slideBox{{
		ID: "v", X: 10, Y: 10, W: 60, H: 50, Kind: "media", MediaType: "video", Src: mp4Falso, Poster: onePxPNG,
		Tracks: []slideTrack{{Lang: "es", VTT: vttPrueba, Default: true}},
	}}}}}
}

// comoPowerPoint deja un .pptx exportado como lo dejaría otra herramienta: la pista sigue
// citada por la diapositiva (relación de subtítulos) pero sin la extensión de Sytue.
func comoPowerPoint(t *testing.T, data []byte, nombreVTT string) []byte {
	t.Helper()
	parts := zipParts(t, data)
	parts["ppt/slides/slide1.xml"] = reExtSytuePista.ReplaceAllString(parts["ppt/slides/slide1.xml"], "")
	if strings.Contains(parts["ppt/slides/slide1.xml"], "sytue:track") {
		t.Fatal("la extensión de Sytue sigue en la diapositiva")
	}
	if nombreVTT != "subtitles1.vtt" {
		parts["ppt/media/"+nombreVTT] = parts["ppt/media/subtitles1.vtt"]
		delete(parts, "ppt/media/subtitles1.vtt")
		parts["ppt/slides/_rels/slide1.xml.rels"] = strings.ReplaceAll(parts["ppt/slides/_rels/slide1.xml.rels"], "subtitles1.vtt", nombreVTT)
	}
	return zipStore(t, parts)
}

// TestSubtitulosDePowerPointSinIdioma: una pista que la diapositiva cita por relación se
// importa aunque no lleve la extensión de Sytue. El idioma sale del nombre del fichero,
// o del texto de la diapositiva y, si no hay, queda «und».
func TestSubtitulosDePowerPointSinIdioma(t *testing.T) {
	base := exportarPptx(t, jsonDe(t, modeloUnVideo()))
	for nombre, c := range map[string]struct {
		vtt, lang string
	}{
		"por el nombre del fichero": {"captions_fr-CA.vtt", "fr-CA"},
		"sin idioma que decir":      {"media2.vtt", "und"},
	} {
		got := reimportar(t, "ppt.pptx", comoPowerPoint(t, base, c.vtt)).Slides[0].Boxes[0].Tracks
		if len(got) != 1 || got[0].Lang != c.lang || got[0].VTT != vttPrueba {
			t.Errorf("%s: pistas = %+v", nombre, got)
		}
	}
	// Con texto en la diapositiva, su idioma.
	m := modeloUnVideo()
	m.Slides[0].Boxes = append(m.Slides[0].Boxes, slideBox{ID: "t", X: 10, Y: 70, W: 50, H: 10, Kind: "text", HTML: "<p>Hola</p>"})
	data := comoPowerPoint(t, exportarPptx(t, jsonDe(t, m)), "media2.vtt")
	if got := reimportar(t, "ppt.pptx", data).Slides[0].Boxes[0].Tracks; len(got) != 1 || got[0].Lang == "und" || got[0].Lang == "" {
		t.Errorf("con texto: pistas = %+v", got)
	}
}

// TestSubtitulosJuntoAlMedio: el paquete que lleva `clip.es.vtt` junto a `clip.mp4`
// (ODF no tiene pistas de texto) las da al medio.
func TestSubtitulosJuntoAlMedio(t *testing.T) {
	data := exportarOdp(t, jsonDe(t, modeloUnVideo()))
	parts := zipParts(t, data)
	parts["content.xml"] = regexp.MustCompile(`<sytue:track [^>]*/>`).ReplaceAllString(parts["content.xml"], "")
	var medio string
	for n := range parts {
		if strings.HasPrefix(n, "Media/") && strings.HasSuffix(n, ".mp4") {
			medio = n
		}
	}
	if medio == "" {
		t.Fatal("el .odp no lleva el vídeo")
	}
	delete(parts, "Media/subtitles1.vtt")
	base := strings.TrimSuffix(medio, ".mp4")
	parts[base+".es.vtt"] = vttPrueba
	parts[base+".vtt"] = "WEBVTT\n\n00:00.000 --> 00:01.000\nSin idioma\n"
	got := reimportar(t, "j.odp", zipStore(t, parts)).Slides[0].Boxes[0].Tracks
	if len(got) != 2 || got[0].Lang != "es" || got[0].VTT != vttPrueba || got[1].Lang != "und" {
		t.Errorf("pistas = %+v", got)
	}
}

// TestViajePorLibreOfficeConservaMedioYDisenos: lo que Sytue exporta pasa por Impress de
// verdad (convertido a .odp, que es guardarlo con LibreOffice) y vuelve a Sytue con su
// vídeo y las diapositivas; los diseños vuelven como esquemas de página de LibreOffice,
// ya mapeados. Solo donde hay `soffice` (el contenedor de los agentes).
func TestViajePorLibreOfficeConservaMedioYDisenos(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("sin soffice")
	}
	m := modeloConDisenos()
	m.Slides[0].Boxes = append(m.Slides[0].Boxes, slideBox{
		ID: "v", X: 10, Y: 60, W: 40, H: 30, Kind: "media", MediaType: "video", Src: mp4Falso, Poster: onePxPNG,
		Tracks: []slideTrack{{Lang: "es", VTT: vttPrueba, Default: true}},
	})
	dir := t.TempDir()
	for nombre, data := range map[string][]byte{
		"v.pptx": exportarPptx(t, jsonDe(t, m)),
		"v.odp":  exportarOdp(t, jsonDe(t, m)),
	} {
		f := filepath.Join(dir, nombre)
		if err := os.WriteFile(f, data, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
			"--convert-to", "odp", "--outdir", filepath.Join(dir, "salida-"+nombre), f)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("soffice %s: %v\n%s", nombre, err, o)
		}
		guardado, err := os.ReadFile(filepath.Join(dir, "salida-"+nombre, "v.odp"))
		if err != nil {
			t.Fatal(err)
		}
		got := reimportar(t, "v.odp", guardado)
		if len(got.Slides) != 3 {
			t.Fatalf("%s: diapositivas = %d", nombre, len(got.Slides))
		}
		hayVideo := false
		for _, b := range got.Slides[0].Boxes {
			hayVideo = hayVideo || b.Kind == "media" && strings.HasPrefix(b.Src, "data:video/mp4")
		}
		if !hayVideo {
			t.Errorf("%s: el vídeo no sobrevive a LibreOffice: %+v", nombre, got.Slides[0].Boxes)
		}
		// El .pptx pierde los diseños al pasar a .odp por LibreOffice (no los convierte); el
		// .odp los devuelve como esquemas de página de LibreOffice, ya mapeados.
		if nombre == "v.odp" && (got.Slides[0].Layout == "" || got.Slides[1].Layout == "" || got.Master == nil || len(got.Master.Layouts) != 2) {
			t.Errorf("%s: los diseños no vuelven de LibreOffice: %+v, %q, %q", nombre, got.Master, got.Slides[0].Layout, got.Slides[1].Layout)
		}
	}
}

// TestDiaposImportaGraficosYObjetosIncrustados comprueba que un gráfico y un objeto OLE
// en .pptx y en .odp se importan como tabla de datos e imagen/caja en la diapositiva.
func TestDiaposImportaGraficosYObjetosIncrustados(t *testing.T) {
	basePptx := exportarPptx(t, jsonDe(t, slidesModel{Slides: []slideT{{ID: "s1"}}}))
	partsPptx := zipParts(t, basePptx)
	partsPptx["ppt/charts/chart1.xml"] = `<?xml version="1.0" encoding="UTF-8"?>` +
		`<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
		`<c:chart><c:title><c:tx><a:rich><a:p><a:r><a:t>Ventas trimestrales</a:t></a:r></a:p></a:rich></c:tx></c:title>` +
		`<c:plotArea><c:barChart>` +
		`<c:ser><c:tx><c:v>2025</c:v></c:tx>` +
		`<c:cat><c:strRef><c:strCache><c:pt idx="0"><c:v>T1</c:v></c:pt><c:pt idx="1"><c:v>T2</c:v></c:pt></c:strCache></c:strRef></c:cat>` +
		`<c:val><c:numRef><c:numCache><c:pt idx="0"><c:v>120</c:v></c:pt><c:pt idx="1"><c:v>180</c:v></c:pt></c:numCache></c:numRef></c:val></c:ser>` +
		`<c:ser><c:tx><c:v>2026</c:v></c:tx>` +
		`<c:cat><c:strRef><c:strCache><c:pt idx="0"><c:v>T1</c:v></c:pt><c:pt idx="1"><c:v>T2</c:v></c:pt></c:strCache></c:strRef></c:cat>` +
		`<c:val><c:numRef><c:numCache><c:pt idx="0"><c:v>150</c:v></c:pt><c:pt idx="1"><c:v>210</c:v></c:pt></c:numCache></c:numRef></c:val></c:ser>` +
		`</c:barChart></c:plotArea></c:chart></c:chartSpace>`
	partsPptx["ppt/media/ole1.jpg"] = string(jpegDe(t, 32, 24))
	partsPptx["ppt/slides/_rels/slide1.xml.rels"] = `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rIdChart1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart1.xml"/>` +
		`<Relationship Id="rIdImg1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/ole1.jpg"/>` +
		`</Relationships>`
	partsPptx["ppt/slides/slide1.xml"] = `<?xml version="1.0" encoding="UTF-8"?>` +
		`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart">` +
		`<p:cSld><p:spTree>` +
		`<p:graphicFrame><p:xfrm><a:off x="914400" y="685800"/><a:ext cx="4572000" cy="3429000"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart r:id="rIdChart1"/></a:graphicData></a:graphic></p:graphicFrame>` +
		`<p:graphicFrame><p:xfrm><a:off x="5486400" y="685800"/><a:ext cx="2743200" cy="2057400"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/presentationml/2006/ole">` +
		`<p:oleObj name="Hoja incrustada"><p:pic><p:blipFill><a:blip r:embed="rIdImg1"/></p:blipFill></p:pic></p:oleObj>` +
		`</a:graphicData></a:graphic></p:graphicFrame>` +
		`<p:graphicFrame><p:xfrm><a:off x="914400" y="4572000"/><a:ext cx="2743200" cy="1371600"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/presentationml/2006/ole">` +
		`<p:oleObj name="Presupuesto.xlsx" progId="Excel.Sheet.12"/>` +
		`</a:graphicData></a:graphic></p:graphicFrame>` +
		`</p:spTree></p:cSld></p:sld>`

	gotPptx := reimportar(t, "graficos.pptx", zipStore(t, partsPptx))
	if len(gotPptx.Slides) != 1 || len(gotPptx.Slides[0].Boxes) != 3 {
		t.Fatalf("pptx cajas = %+v", gotPptx.Slides)
	}
	b0, b1, b2 := gotPptx.Slides[0].Boxes[0], gotPptx.Slides[0].Boxes[1], gotPptx.Slides[0].Boxes[2]
	if !strings.Contains(b0.HTML, "<table>") || !strings.Contains(b0.HTML, "Ventas trimestrales") ||
		!strings.Contains(b0.HTML, "<td>T1</td>") || !strings.Contains(b0.HTML, "<td>210</td>") {
		t.Errorf("pptx gráfico = %+v", b0)
	}
	if b1.Kind != "image" || !strings.HasPrefix(b1.Src, "data:image/jpeg;base64,") || b1.W <= 0 {
		t.Errorf("pptx objeto OLE con imagen = %+v", b1)
	}
	if b2.Kind != "text" || !strings.Contains(b2.HTML, "Presupuesto.xlsx") {
		t.Errorf("pptx objeto OLE sin imagen = %+v", b2)
	}

	// En .odp: un marco con <draw:object xlink:href="./Object 1"/> (gráfico ODF con <table:table>)
	// y dos marcos con <draw:object-ole> (uno con <draw:image> de reemplazo y otro sin ella).
	baseOdp := exportarOdp(t, jsonDe(t, slidesModel{Slides: []slideT{{ID: "s1"}}}))
	partsOdp := zipParts(t, baseOdp)
	partsOdp["Object 1/content.xml"] = `<?xml version="1.0" encoding="UTF-8"?>` +
		`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:chart="urn:oasis:names:tc:opendocument:xmlns:chart:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">` +
		`<office:body><office:chart><chart:chart chart:class="chart:bar">` +
		`<chart:title><text:p>Cuota por región</text:p></chart:title>` +
		`<table:table table:name="local-table">` +
		`<table:table-header-rows><table:table-row><table:table-cell><text:p/></table:table-cell><table:table-cell><text:p>2025</text:p></table:table-cell><table:table-cell><text:p>2026</text:p></table:table-cell></table:table-row></table:table-header-rows>` +
		`<table:table-rows>` +
		`<table:table-row><table:table-cell><text:p>Norte</text:p></table:table-cell><table:table-cell><text:p>40</text:p></table:table-cell><table:table-cell><text:p>55</text:p></table:table-cell></table:table-row>` +
		`<table:table-row><table:table-cell><text:p>Sur</text:p></table:table-cell><table:table-cell><text:p>60</text:p></table:table-cell><table:table-cell><text:p>75</text:p></table:table-cell></table:table-row>` +
		`</table:table-rows></table:table></chart:chart></office:chart></office:body></office:document-content>`
	partsOdp["Pictures/ole.jpg"] = string(jpegDe(t, 32, 24))
	partsOdp["content.xml"] = `<?xml version="1.0" encoding="UTF-8"?>` +
		`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<office:body><office:presentation><draw:page draw:name="page1">` +
		`<draw:frame svg:x="2cm" svg:y="2cm" svg:width="12cm" svg:height="8cm"><draw:object xlink:href="./Object 1"/><draw:image xlink:href="Pictures/ole.jpg"/></draw:frame>` +
		`<draw:frame draw:name="Tabla OLE" svg:x="15cm" svg:y="2cm" svg:width="8cm" svg:height="6cm"><draw:object-ole xlink:href="./Object 2"/><draw:image xlink:href="Pictures/ole.jpg"/></draw:frame>` +
		`<draw:frame draw:name="Fórmula OLE" svg:x="15cm" svg:y="9cm" svg:width="8cm" svg:height="4cm"><draw:object-ole xlink:href="./Object 3"/></draw:frame>` +
		`</draw:page></office:presentation></office:body></office:document-content>`

	gotOdp := reimportar(t, "graficos.odp", zipStore(t, partsOdp))
	if len(gotOdp.Slides) != 1 || len(gotOdp.Slides[0].Boxes) != 3 {
		t.Fatalf("odp cajas = %+v", gotOdp.Slides)
	}
	o0, o1, o2 := gotOdp.Slides[0].Boxes[0], gotOdp.Slides[0].Boxes[1], gotOdp.Slides[0].Boxes[2]
	if !strings.Contains(o0.HTML, "<table>") || !strings.Contains(o0.HTML, "Cuota por región") ||
		!strings.Contains(o0.HTML, "<td>Norte</td>") || !strings.Contains(o0.HTML, "<td>75</td>") {
		t.Errorf("odp gráfico = %+v", o0)
	}
	if o1.Kind != "image" || !strings.HasPrefix(o1.Src, "data:image/jpeg;base64,") {
		t.Errorf("odp objeto OLE con imagen = %+v", o1)
	}
	if o2.Kind != "text" || !strings.Contains(o2.HTML, "Fórmula OLE") {
		t.Errorf("odp objeto OLE sin imagen = %+v", o2)
	}
}
