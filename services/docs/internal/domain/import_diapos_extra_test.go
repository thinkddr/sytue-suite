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
