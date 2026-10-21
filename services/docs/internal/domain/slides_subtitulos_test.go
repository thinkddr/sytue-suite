// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const vttPrueba = "WEBVTT\n\n00:00.000 --> 00:02.000\nHola\n"

func modeloConSubtitulos() slidesModel {
	return slidesModel{Slides: []slideT{{ID: "s1", Boxes: []slideBox{
		{
			ID: "v", X: 10, Y: 10, W: 60, H: 50, Kind: "media", MediaType: "video", Src: mp4Falso, Poster: onePxPNG,
			Tracks: []slideTrack{
				{Lang: "es", VTT: vttPrueba, Default: true},
				{Lang: "en-GB", Label: "English (UK)", VTT: "WEBVTT\n\n00:00.000 --> 00:02.000\nHello\n"},
			},
		},
		{
			ID: "a", X: 10, Y: 70, W: 60, H: 12, Kind: "media", MediaType: "audio", Src: "https://media.example.eu/voz.mp3",
			Tracks: []slideTrack{{Lang: "es", VTT: vttPrueba, Default: true}},
		},
		{ID: "t", X: 75, Y: 10, W: 20, H: 10, Kind: "text", HTML: "<p>x</p>"},
	}}}}
}

// comprobarSubtitulos es lo que debe conservar un formato al exportar el modelo con
// subtítulos y volver a importarlo.
func comprobarSubtitulos(t *testing.T, m slidesModel) {
	t.Helper()
	b := m.Slides[0].Boxes
	if len(b) != 3 {
		t.Fatalf("cajas = %d", len(b))
	}
	want := modeloConSubtitulos().Slides[0].Boxes
	if !reflect.DeepEqual(b[0].Tracks, want[0].Tracks) {
		t.Errorf("pistas del vídeo = %+v", b[0].Tracks)
	}
	if !reflect.DeepEqual(b[1].Tracks, want[1].Tracks) {
		t.Errorf("pistas del audio = %+v", b[1].Tracks)
	}
	if b[2].Tracks != nil {
		t.Errorf("un texto no tiene pistas: %+v", b[2].Tracks)
	}
}

func TestSubtitulosPptxIdaYVuelta(t *testing.T) {
	data := exportarPptx(t, jsonDe(t, modeloConSubtitulos()))
	parts := zipParts(t, data)
	if parts["ppt/media/subtitles1.vtt"] != vttPrueba {
		t.Errorf("subtitles1.vtt = %q", parts["ppt/media/subtitles1.vtt"])
	}
	mustContain(t, "tipo vtt", parts["[Content_Types].xml"], `<Default Extension="vtt" ContentType="text/vtt"/>`)
	mustContain(t, "relación", parts["ppt/slides/_rels/slide1.xml.rels"], `office/2011/relationships/subtitles" Target="../media/subtitles1.vtt"`)
	mustContain(t, "extensión", parts["ppt/slides/slide1.xml"], `<sytue:track xmlns:sytue="urn:sytue:names:ooxml:1.0" lang="es" default="1" r:id="`)
	comprobarSubtitulos(t, reimportar(t, "subs.pptx", data))
}

func TestSubtitulosOdpIdaYVuelta(t *testing.T) {
	data := exportarOdp(t, jsonDe(t, modeloConSubtitulos()))
	parts := zipParts(t, data)
	if parts["Media/subtitles1.vtt"] != vttPrueba {
		t.Errorf("subtitles1.vtt = %q", parts["Media/subtitles1.vtt"])
	}
	mustContain(t, "manifiesto", parts["META-INF/manifest.xml"], `manifest:full-path="Media/subtitles1.vtt" manifest:media-type="text/vtt"`)
	mustContain(t, "pista", parts["content.xml"], `<sytue:track xlink:href="Media/subtitles1.vtt" sytue:lang="es" sytue:default="true"/>`)
	comprobarSubtitulos(t, reimportar(t, "subs.odp", data))
}

// TestSubtitulosEntradaHostil: lo que no es WebVTT, el idioma que no lo es, los repetidos y
// el exceso de pistas no llegan al fichero ni salen de él.
func TestSubtitulosEntradaHostil(t *testing.T) {
	pistas := make([]slideTrack, 0, 10)
	pistas = append(pistas,
		slideTrack{Lang: "es", VTT: "1\n00:00:01,000 --> 00:00:02,000\nSubRip, no WebVTT"},
		slideTrack{Lang: "<b>", VTT: vttPrueba},
		slideTrack{Lang: "fr", VTT: vttPrueba, Default: true},
		slideTrack{Lang: "FR", VTT: vttPrueba},
	)
	for _, l := range []string{"de", "it", "pt", "nl", "pl", "sv"} {
		pistas = append(pistas, slideTrack{Lang: l, VTT: vttPrueba, Default: true})
	}
	m := slidesModel{Slides: []slideT{{ID: "s", Boxes: []slideBox{{ID: "v", X: 1, Y: 1, W: 50, H: 50, Kind: "media", MediaType: "video", Src: mp4Falso, Tracks: pistas}}}}}
	for nombre, data := range map[string][]byte{"pptx": exportarPptx(t, jsonDe(t, m)), "odp": exportarOdp(t, jsonDe(t, m))} {
		got := reimportar(t, "h."+nombre, data).Slides[0].Boxes[0].Tracks
		langs := make([]string, 0, len(got))
		defectos := 0
		for _, tr := range got {
			langs = append(langs, tr.Lang)
			if tr.Default {
				defectos++
			}
		}
		if want := []string{"fr", "de", "it", "pt", "nl", "pl"}; !reflect.DeepEqual(langs, want) || defectos != 1 || !got[0].Default {
			t.Errorf("%s: idiomas %v, por defecto %d", nombre, langs, defectos)
		}
	}
}

func TestLimpiarVTTEIdioma(t *testing.T) {
	if v, ok := limpiarVTT("\uFEFFWEBVTT\r\n\r\n00:00.000 --> 00:01.000\r\nHi"); !ok || v != "WEBVTT\n\n00:00.000 --> 00:01.000\nHi" {
		t.Errorf("BOM y saltos: %q %v", v, ok)
	}
	for _, malo := range []string{"", "WEBVTTX\n", "1\n00:00:01,000 --> 00:00:02,000\nx", "WEBVTT\n" + strings.Repeat("x", maxVTT)} {
		if _, ok := limpiarVTT(malo); ok {
			t.Errorf("%.20q no es WebVTT válido", malo)
		}
	}
	// Los mismos vectores que normalizeLang del editor (test/slides-subtitulos.test.ts).
	for _, c := range [][2]string{
		{"es", "es"},
		{" EN-gb ", "en-GB"},
		{"pt_br", "pt-BR"},
		{"zh-hant-tw", "zh-Hant-TW"},
		{"es-419", "es-419"},
		{"", ""},
		{"e", ""},
		{"espanol", ""},
		{"es-", ""},
		{"es--ES", ""},
		{"<b>", ""},
		{"es-XXXXXX", ""},
		{"a-b-c-d-e", ""},
	} {
		if got := normalizarIdioma(c[0]); got != c[1] {
			t.Errorf("normalizarIdioma(%q) = %q, quiero %q", c[0], got, c[1])
		}
	}
}

// TestSubtitulosSeAbrenEnLibreOffice: con subtítulos, LibreOffice abre y pinta el .pptx y el
// .odp (la extensión propia se ignora sin romper nada). Solo donde hay `soffice`.
func TestSubtitulosSeAbrenEnLibreOffice(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("sin soffice")
	}
	dir := t.TempDir()
	for nombre, data := range map[string][]byte{
		"subs.pptx": exportarPptx(t, jsonDe(t, modeloConSubtitulos())),
		"subs.odp":  exportarOdp(t, jsonDe(t, modeloConSubtitulos())),
	} {
		in := filepath.Join(dir, nombre)
		if err := os.WriteFile(in, data, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
			"--convert-to", "pdf", "--outdir", dir, in)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("soffice %s: %v\n%s", nombre, err, o)
		}
		pdf, err := os.ReadFile(filepath.Join(dir, strings.TrimSuffix(nombre, filepath.Ext(nombre))+".pdf"))
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
			t.Fatalf("LibreOffice no produjo un PDF válido de %s: %v", nombre, err)
		}
	}
}

// Una caja de medio se guarda con el origen que admiten los dos editores (http(s) o un
// data: de audio o vídeo) y con un identificador llano; lo demás no entra, esté la caja
// en una diapositiva o en el patrón. Las cajas que no son de medio no se miran.
func TestValidarMedios(t *testing.T) {
	set := func(modelo string) []Op {
		v, _ := json.Marshal(modelo)
		return []Op{{Kind: OpSet, Key: claveDiapos, Value: v}}
	}
	enDiapositiva := func(caja string) []Op { return set(`{"slides":[{"id":"s1","boxes":[` + caja + `]}]}`) }
	casos := []struct {
		nombre string
		ops    []Op
		vale   bool
	}{
		{"vídeo enlazado", enDiapositiva(`{"id":"s1b1","kind":"media","src":"https://media.example.eu/v.mp4"}`), true},
		{"audio incrustado", enDiapositiva(`{"id":"0b8e7c1a-5d2f-4a3b-9c6d-1e2f3a4b5c6d","kind":"media","src":"data:audio/mpeg;base64,AAAA"}`), true},
		{"medio aún sin origen", enDiapositiva(`{"id":"b1","kind":"media"}`), true},
		{"una caja que no es de medio", enDiapositiva(`{"id":"a/b","kind":"text","src":"lo que sea"}`), true},
		{"un registro que no es un modelo", []Op{{Kind: OpSet, Key: claveDiapos, Value: json.RawMessage(`7`)}}, true},
		{"otro registro", []Op{{Kind: OpSet, Key: "doc:html", Value: json.RawMessage(`"<p>x</p>"`)}}, true},
		{"incrustado que no es audio ni vídeo", enDiapositiva(`{"id":"b1","kind":"media","src":"data:text/plain;base64,AAAA"}`), false},
		{"otro esquema", enDiapositiva(`{"id":"b1","kind":"media","src":"file:///tmp/x.mp4"}`), false},
		{"origen que no es texto", enDiapositiva(`{"id":"b1","kind":"media","src":7}`), false},
		{"identificador con forma de ruta", enDiapositiva(`{"id":"../b1","kind":"media","src":"https://media.example.eu/v.mp4"}`), false},
		{"sin identificador", enDiapositiva(`{"kind":"media","src":"https://media.example.eu/v.mp4"}`), false},
		{"en el patrón", set(`{"slides":[],"master":{"boxes":[{"id":"b1","kind":"media","src":"file:///tmp/x.mp4"}]}}`), false},
	}
	for _, c := range casos {
		err := ValidarMedios(c.ops)
		if c.vale && err != nil {
			t.Errorf("%s: se rechazó: %v", c.nombre, err)
		}
		if !c.vale && !errors.Is(err, ErrInvalidOp) {
			t.Errorf("%s: salió %v, esperaba ErrInvalidOp", c.nombre, err)
		}
	}
}

// Lo que produce la importación de una presentación con medios pasa la regla: si no, abrir
// un .pptx o un .odp con vídeo dejaría un documento que no se puede volver a guardar.
func TestLasPresentacionesImportadasPasanValidarMedios(t *testing.T) {
	vistas := 0
	for _, m := range corpus {
		if m.ext != "pptx" && m.ext != "odp" {
			continue
		}
		res, err := Import("p."+m.ext, leerMuestra(t, m))
		if err != nil {
			t.Fatalf("%s: %v", m.ruta, err)
		}
		v, _ := json.Marshal(res.Content)
		if err := ValidarMedios([]Op{{Kind: OpSet, Key: claveDiapos, Value: v}}); err != nil {
			t.Errorf("%s: %v", m.ruta, err)
		}
		if strings.Contains(res.Content, `"kind":"media"`) {
			vistas++
		}
	}
	if vistas == 0 {
		t.Fatal("ninguna presentación del corpus trae un medio: la prueba no mira nada")
	}
}
