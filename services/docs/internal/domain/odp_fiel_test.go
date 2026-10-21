// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Fidelidad de .odp al EXPORTAR: lo que el editor sabe de una presentación sale en ODF
// —tema, patrón con sus campos, tamaño, fondo, ocultar el patrón, transiciones,
// animaciones, medios y conectores— y LibreOffice lo abre y lo conserva.

func exportarOdp(t *testing.T, modelo string) []byte {
	t.Helper()
	data, _, ext, err := Export(KindPresentation, "Fiel", stateWithContent(t, "doc:slides", modelo), FormatODF)
	if err != nil || ext != "odp" {
		t.Fatalf("export odp: %v %s", err, ext)
	}
	return data
}

func TestOdpExportaTodoLoDelModelo(t *testing.T) {
	data := exportarOdp(t, modeloCompleto(t))
	parts := zipParts(t, data) // también comprueba que todo el XML está bien formado
	c, s := parts["content.xml"], parts["styles.xml"]

	// Tamaño 4:3 con el ancho del .pptx (13,333 in): la página lo dice.
	mustContain(t, "styles.xml", s, `fo:page-width="33.867cm" fo:page-height="25.4cm"`)
	// Tema carbón: fondo, tinta y fuente en la maestra y en «standard».
	mustContain(t, "styles.xml", s, `draw:fill-color="#0A0A0A"`)
	mustContain(t, "styles.xml", s, `fo:color="#FFFFFF"`)
	mustContain(t, "styles.xml", s, `style:font-name="Calibri"`)
	// Patrón: sus dos cajas y los campos de número y total.
	mustContain(t, "styles.xml", s, `<style:master-page style:name="Default"`)
	mustContain(t, "styles.xml", s, `<text:page-number`)
	mustContain(t, "styles.xml", s, `<text:page-count>3</text:page-count>`)
	mustContain(t, "styles.xml", s, `<draw:ellipse`)
	// Fondo propio, patrón oculto y las tres transiciones.
	mustContain(t, "content.xml", c, `presentation:background-objects-visible="false"`)
	mustContain(t, "content.xml", c, `draw:fill-color="#FED7AA"`)
	mustContain(t, "content.xml", c, `smil:type="fade" smil:subtype="crossfade" `)
	mustContain(t, "content.xml", c, `sytue:transition="morph"`)
	mustContain(t, "content.xml", c, `smil:type="pushWipe" smil:subtype="fromRight"`)
	// Animaciones: la secuencia, un preset de cada clase y la trayectoria.
	mustContain(t, "content.xml", c, `presentation:node-type="main-sequence"`)
	for _, want := range []string{
		`presentation:preset-id="ooo-entrance-fade-in"`, `presentation:preset-id="ooo-emphasis-flash-bulb"`,
		`presentation:preset-id="ooo-exit-fly-out"`, `presentation:preset-class="motion-path"`,
		`presentation:preset-class="media-call"`, `svg:path="M 0 0 L 0.3 0"`,
	} {
		mustContain(t, "content.xml", c, want)
	}
	// Medios: el vídeo incrustado, el audio enlazado y el póster.
	mustContain(t, "content.xml", c, `<draw:plugin xlink:href="Media/media1.mp4"`)
	mustContain(t, "content.xml", c, `<draw:param draw:name="Loop" draw:value="true"/>`)
	mustContain(t, "content.xml", c, `xlink:href="https://media.example.eu/voz.mp3"`)
	if _, ok := parts["Media/media1.mp4"]; !ok {
		t.Error("el vídeo no se incrustó")
	}
	mustContain(t, "manifest", parts["META-INF/manifest.xml"], `manifest:full-path="Media/media1.mp4" manifest:media-type="video/mp4"`)
	// Conectores: los cinco, los anclados con su forma y las flechas.
	if n := strings.Count(c, "<draw:connector "); n != 5 {
		t.Errorf("conectores = %d, quiero 5", n)
	}
	if !regexp.MustCompile(`draw:start-shape="id\d+" draw:start-glue-point="\d"`).MatchString(c) {
		t.Error("el conector anclado no lleva su forma y su punto de unión")
	}
	mustContain(t, "content.xml", c, `draw:marker-end="Sytue_20_flecha"`)
	mustContain(t, "styles.xml", s, `<draw:marker draw:name="Sytue_20_flecha"`)
	// Los títulos llevan el tamaño del editor.
	mustContain(t, "content.xml", c, `fo:font-size="44pt"`)
}

// TestOdpFormasListasYAnimaciones: todas las formas, las listas anidadas y cada efecto
// del editor salen con su geometría, su sangría y su preset.
func TestOdpFormasListasYAnimaciones(t *testing.T) {
	formas := []string{"rect", "ellipse", "line", "arrow", "triangle", "diamond", "pentagon", "hexagon", "star", "cylinder", "callout"}
	boxes := make([]slideBox, 0, len(formas)+2)
	for i, f := range formas {
		boxes = append(boxes, slideBox{ID: f, X: float64(2 + i*8), Y: 10, W: 7, H: 10, Kind: "shape", Shape: f, HTML: "<p>x</p>"})
	}
	tipos := []string{"appear", "fade", "fly", "zoom", "pulse", "spin", "grow-shrink", "disappear", "fade-out", "fly-out", "zoom-out", "motion-right", "motion-arc"}
	anims := make([]slideAnim, 0, len(tipos))
	for i, typ := range tipos {
		anims = append(anims, slideAnim{Type: typ, Order: i + 1})
	}
	boxes = append(boxes,
		slideBox{ID: "anim", X: 5, Y: 40, W: 20, H: 10, Kind: "text", HTML: "<p>x</p>", Anims: anims},
		slideBox{
			ID: "lista", X: 30, Y: 40, W: 40, H: 40, Kind: "text",
			HTML: "<ul><li>uno</li><li>dos<ul><li>anidado</li></ul></li></ul><ol><li>primero</li></ol>",
		})
	m := slidesModel{Slides: []slideT{{ID: "s", Boxes: boxes}}}
	data := exportarOdp(t, jsonDe(t, m))
	c := zipParts(t, data)["content.xml"]
	for _, want := range []string{
		"<draw:rect ", "<draw:ellipse ", "<draw:line ", `draw:type="right-arrow"`, `draw:type="isosceles-triangle"`,
		`draw:type="diamond"`, `draw:type="pentagon"`, `draw:type="hexagon"`, `draw:type="star5"`, `draw:type="non-primitive"`,
		`draw:type="rectangular-callout"`,
		`<text:list text:style-name="Lbullet"><text:list-item><text:p text:style-name="P1">uno</text:p></text:list-item>`,
		`<text:list text:style-name="Lbullet"><text:list-item><text:p text:style-name="P1">anidado`,
		`<text:list text:style-name="Lnumber">`, `text:list-level-style-number text:level="1"`,
	} {
		mustContain(t, "content.xml", c, want)
	}
	for _, typ := range tipos {
		if o, ok := odpFxOf[typ]; ok {
			mustContain(t, typ, c, `presentation:preset-id="`+o.id+`"`)
		}
	}
	if n := strings.Count(c, `presentation:preset-class="motion-path"`); n != 2 {
		t.Errorf("trayectorias = %d, quiero 2", n)
	}
	if soffice, err := exec.LookPath("soffice"); err == nil {
		dir := t.TempDir()
		in := filepath.Join(dir, "formas.odp")
		if err := os.WriteFile(in, data, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
			"--convert-to", "pdf", "--outdir", dir, in)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("soffice: %v\n%s", err, o)
		}
		if pdf, err := os.ReadFile(filepath.Join(dir, "formas.pdf")); err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
			t.Fatalf("LibreOffice no produjo un PDF válido: %v", err)
		}
	}
}

func jsonDe(t *testing.T, m slidesModel) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestOdpDiapositivaSencillaSinEstilo: una diapositiva sin nada especial no lleva estilo propio.
func TestOdpDiapositivaSencillaSinEstilo(t *testing.T) {
	data := exportarOdp(t, `{"slides":[{"id":"s1","boxes":[{"id":"a","x":10,"y":10,"w":50,"h":20,"kind":"text","html":"<p>Hola</p>"}]}]}`)
	c := zipParts(t, data)["content.xml"]
	mustContain(t, "content.xml", c, `<draw:page draw:name="Diapositiva 1" draw:master-page-name="Default">`)
	if strings.Contains(c, "anim:par") {
		t.Error("sin animaciones no debe haber árbol de tiempos")
	}
}

// TestOdpIdaYVuelta: lo que se exporta al .odp vuelve igual al importarlo, lo mismo que
// con el .pptx: el tema, el patrón y sus campos, el fondo, la transición, las
// animaciones, los conectores y los medios.
func TestOdpIdaYVuelta(t *testing.T) {
	comprobarIdaYVuelta(t, reimportar(t, "fiel.odp", exportarOdp(t, modeloCompleto(t))))
}

// TestOdpSeAbreEnLibreOffice: lo exportado lo abre LibreOffice, lo pinta (PDF) y lo
// conserva al guardarlo de nuevo como .odp: transiciones, animaciones, el medio, los
// conectores, los campos del patrón y el fondo del tema. Solo donde hay `soffice`.
func TestOdpSeAbreEnLibreOffice(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("sin soffice")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "fiel.odp")
	if err := os.WriteFile(in, exportarOdp(t, modeloCompleto(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	convertir := func(fmt, out string) {
		t.Helper()
		cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
			"--convert-to", fmt, "--outdir", filepath.Join(dir, out), in)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("soffice %s: %v\n%s", fmt, err, o)
		}
	}
	convertir("pdf", "pdf")
	if pdf, err := os.ReadFile(filepath.Join(dir, "pdf", "fiel.pdf")); err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("LibreOffice no produjo un PDF válido: %v", err)
	}
	convertir("odp", "odp")
	odp, err := os.ReadFile(filepath.Join(dir, "odp", "fiel.odp"))
	if err != nil {
		t.Fatalf("LibreOffice no guardó el .odp: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(odp), int64(len(odp)))
	if err != nil {
		t.Fatal(err)
	}
	var content, styles string
	for _, f := range zr.File {
		rc, _ := f.Open()
		var b bytes.Buffer
		_, _ = b.ReadFrom(rc)
		_ = rc.Close()
		switch f.Name {
		case "content.xml":
			content = b.String()
		case "styles.xml":
			styles = b.String()
		}
	}
	if os.Getenv("SYTUE_VOLCAR") != "" {
		_ = os.WriteFile("/tmp/odp-content.xml", []byte(content), 0o600)
		_ = os.WriteFile("/tmp/odp-styles.xml", []byte(styles), 0o600)
		_ = os.WriteFile("/tmp/odp-fiel.odp", exportarOdp(t, modeloCompleto(t)), 0o600)
	}
	for _, want := range []string{
		`smil:type="fade"`, `smil:type="pushWipe"`, // transiciones
		`presentation:node-type="main-sequence"`, `presentation:preset-id="ooo-entrance-fade-in"`, // animaciones
		`presentation:preset-class="motion-path"`, `presentation:preset-class="exit"`,
		`<draw:connector`, `<draw:plugin`, // conectores y vídeo
		`draw:fill-color="#fed7aa"`, // fondo propio
	} {
		if !strings.Contains(content, want) {
			t.Errorf("LibreOffice perdió %s", want)
		}
	}
	for _, want := range []string{`<text:page-number`, `<text:page-count`, `#0a0a0a`} {
		if !strings.Contains(styles, want) {
			t.Errorf("LibreOffice perdió %s de la maestra", want)
		}
	}
	// El tamaño 4:3 llega a la página.
	if !strings.Contains(styles, `fo:page-width="33.867cm"`) || !strings.Contains(styles, `fo:page-height="25.4cm"`) {
		t.Error("LibreOffice cambió el tamaño de la página")
	}
}
