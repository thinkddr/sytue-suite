// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Fidelidad de .pptx: lo que el editor sabe de una presentación sale al exportar y
// vuelve al reimportar con nuestro importador —tema, patrón con sus campos, tamaño,
// fondo, ocultar el patrón, transiciones, animaciones con sus disparadores y
// trayectorias, vídeo y audio, y conectores anclados o libres—.

// Un MP4 de juguete: para el paquete basta con los bytes y su tipo.
var mp4Falso = "data:video/mp4;base64," + base64.StdEncoding.EncodeToString([]byte("\x00\x00\x00\x18ftypmp42sytue-prueba"))

func modeloCompleto(t *testing.T) string {
	t.Helper()
	f := func(v float64) *float64 { return &v }
	m := slidesModel{
		Theme: "carbon",
		Size:  &outSize{Preset: "4:3"},
		Master: &slideMaster{Boxes: []slideBox{
			{ID: "pie", X: 80, Y: 92, W: 18, H: 6, Kind: "text", HTML: "<p>{n} / {total}</p>"},
			{ID: "logo", X: 2, Y: 92, W: 6, H: 6, Kind: "shape", Shape: "ellipse", Fill: "#2563EB", Stroke: "transparent", HTML: ""},
		}},
		Slides: []slideT{
			{
				ID: "s1", HideMaster: true, Transition: "fade",
				Boxes: []slideBox{
					{
						ID: "a", X: 10, Y: 10, W: 30, H: 20, Kind: "shape", Shape: "rect", Fill: "#16A34A", Stroke: "#0A0A0A", HTML: "<p>Origen</p>",
						Anims: []slideAnim{{Type: "fade", Order: 1}, {Type: "pulse", Order: 3, Trigger: "after"}},
					},
					{
						ID: "b", X: 60, Y: 60, W: 30, H: 20, Kind: "shape", Shape: "ellipse", Fill: "#DC2626", Stroke: "#0A0A0A", HTML: "",
						Anims: []slideAnim{{Type: "zoom", Order: 2, Trigger: "with"}, {Type: "motion-right", Order: 4}, {Type: "fly-out", Order: 6, Trigger: "after"}},
					},
					{
						ID: "c", X: 40, Y: 5, W: 20, H: 10, Kind: "text", HTML: "<h1>Título</h1>",
						Anims: []slideAnim{{Type: "motion-custom", Order: 5, Trigger: "with", Points: []motionPoint{{}, {DX: 10, DY: 5}, {DX: 20, DY: -3}}}},
					},
					{ID: "d", X: 75, Y: 30, W: 15, H: 15, Kind: "shape", Shape: "hexagon", HTML: ""},
				},
				Connectors: []slideConnector{
					{ID: "k1", Kind: "straight", From: connectorEnd{BoxID: "a"}, To: connectorEnd{BoxID: "b"}, EndArrow: true},
					{ID: "k2", Kind: "elbow", From: connectorEnd{BoxID: "a", Anchor: "bottom"}, To: connectorEnd{BoxID: "b", Anchor: "left"}, Stroke: "#DC2626"},
					{ID: "k3", Kind: "curved", From: connectorEnd{X: f(5), Y: f(90)}, To: connectorEnd{BoxID: "b", Anchor: "top"}, StartArrow: true},
					{ID: "k4", Kind: "elbow", From: connectorEnd{X: f(95), Y: f(5)}, To: connectorEnd{X: f(70), Y: f(50)}},
					// A un hexágono no se ancla (sus sitios no están en los lados): vuelve
					// como punto libre, en su sitio.
					{ID: "k5", Kind: "straight", From: connectorEnd{BoxID: "a", Anchor: "right"}, To: connectorEnd{BoxID: "d", Anchor: "left"}},
				},
			},
			{
				ID: "s2", Bg: "#FED7AA", Transition: "morph",
				Boxes: []slideBox{
					{ID: "v", X: 10, Y: 10, W: 50, H: 50, Kind: "media", MediaType: "video", Src: mp4Falso, Poster: onePxPNG, Autoplay: true, Loop: true, HTML: ""},
					{ID: "w", X: 65, Y: 10, W: 30, H: 10, Kind: "media", MediaType: "audio", Src: "https://media.example.eu/voz.mp3", HTML: ""},
					{ID: "x", X: 10, Y: 70, W: 30, H: 10, Kind: "text", HTML: "<p>texto</p>", Anims: []slideAnim{{Type: "appear", Order: 1}}},
				},
			},
			{ID: "s3", Transition: "slide", Boxes: []slideBox{{ID: "z", X: 5, Y: 5, W: 90, H: 20, Kind: "text", HTML: "<p>Fin</p>"}}},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exportarPptx(t *testing.T, modelo string) []byte {
	t.Helper()
	data, _, ext, err := Export(KindPresentation, "Fiel", stateWithContent(t, "doc:slides", modelo), FormatOOXML)
	if err != nil || ext != "pptx" {
		t.Fatalf("export pptx: %v %s", err, ext)
	}
	return data
}

func reimportar(t *testing.T, nombre string, data []byte) slidesModel {
	t.Helper()
	res, err := Import(nombre, data)
	if err != nil {
		t.Fatalf("import %s: %v", nombre, err)
	}
	var m slidesModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// animsDe resume las animaciones de una caja como «tipo/disparador».
func animsDe(b slideBox) []string {
	out := make([]string, 0, len(b.Anims))
	for _, a := range b.Anims {
		tr := a.Trigger
		if tr == "" {
			tr = "click"
		}
		out = append(out, a.Type+"/"+tr)
	}
	return out
}

func TestPptxIdaYVuelta(t *testing.T) {
	comprobarIdaYVuelta(t, reimportar(t, "fiel.pptx", exportarPptx(t, modeloCompleto(t))))
}

// comprobarIdaYVuelta es lo que debe conservar un formato al exportar el modelo completo
// y volver a importarlo: lo mismo en .pptx y en .odp.
func comprobarIdaYVuelta(t *testing.T, m slidesModel) {
	t.Helper()
	if m.Theme != "carbon" {
		t.Errorf("tema = %q, quiero carbon", m.Theme)
	}
	if m.Size == nil || m.Size.Preset != "4:3" {
		t.Errorf("tamaño = %+v, quiero 4:3", m.Size)
	}
	if m.Master == nil || len(m.Master.Boxes) != 2 {
		t.Fatalf("patrón = %+v, quiero sus dos cajas", m.Master)
	}
	assertBlocksEqual(t, "campos del patrón", "<p>{n} / {total}</p>", m.Master.Boxes[0].HTML)
	if b := m.Master.Boxes[1]; b.Kind != "shape" || b.Shape != "ellipse" || b.Fill != "#2563EB" || b.Stroke != "transparent" {
		t.Errorf("forma del patrón = %+v", b)
	}
	if len(m.Slides) != 3 {
		t.Fatalf("diapositivas = %d", len(m.Slides))
	}
	s1, s2, s3 := m.Slides[0], m.Slides[1], m.Slides[2]
	if !s1.HideMaster || s2.HideMaster {
		t.Errorf("ocultar patrón: %v %v", s1.HideMaster, s2.HideMaster)
	}
	if s1.Transition != "fade" || s2.Transition != "morph" || s3.Transition != "slide" {
		t.Errorf("transiciones = %q %q %q", s1.Transition, s2.Transition, s3.Transition)
	}
	if s2.Bg != "#FED7AA" || s1.Bg != "" {
		t.Errorf("fondos = %q %q", s1.Bg, s2.Bg)
	}

	// Animaciones: tipos, disparadores y orden global.
	quiero := [][]string{{"fade/click", "pulse/after"}, {"zoom/with", "motion-right/click", "fly-out/after"}, {"motion-custom/with"}}
	for i, q := range quiero {
		if got := animsDe(s1.Boxes[i]); !reflect.DeepEqual(got, q) {
			t.Errorf("caja %d: animaciones %v, quiero %v", i, got, q)
		}
	}
	orden := []int{s1.Boxes[0].Anims[0].Order, s1.Boxes[1].Anims[0].Order, s1.Boxes[0].Anims[1].Order, s1.Boxes[1].Anims[1].Order, s1.Boxes[2].Anims[0].Order, s1.Boxes[1].Anims[2].Order}
	if !reflect.DeepEqual(orden, []int{1, 2, 3, 4, 5, 6}) {
		t.Errorf("orden = %v", orden)
	}
	if pts := s1.Boxes[2].Anims[0].Points; !reflect.DeepEqual(pts, []motionPoint{{}, {DX: 10, DY: 5}, {DX: 20, DY: -3}}) {
		t.Errorf("trayectoria propia = %+v", pts)
	}
	if s1.Boxes[3].Shape != "hexagon" || s1.Boxes[3].Fill != "#475569" {
		t.Errorf("hexágono = %+v", s1.Boxes[3])
	}

	// Conectores: anclados por id, con lado o automáticos; libres por su punto.
	if len(s1.Connectors) != 5 {
		t.Fatalf("conectores = %+v", s1.Connectors)
	}
	a, b := s1.Boxes[0].ID, s1.Boxes[1].ID
	k := s1.Connectors
	if k[0].Kind != "straight" || k[0].From.BoxID != a || k[0].To.BoxID != b || k[0].From.Anchor != "" || !k[0].EndArrow || k[0].StartArrow || k[0].Stroke != "" {
		t.Errorf("conector recto = %+v", k[0])
	}
	if k[1].Kind != "elbow" || k[1].From.Anchor != "bottom" || k[1].To.Anchor != "left" || k[1].Stroke != "#DC2626" {
		t.Errorf("conector en codo = %+v", k[1])
	}
	if k[2].Kind != "curved" || k[2].From.BoxID != "" || !casi(k[2].From.X, 5) || !casi(k[2].From.Y, 90) || k[2].To.Anchor != "top" || !k[2].StartArrow {
		t.Errorf("conector curvo = %+v (desde %v,%v)", k[2], deref(k[2].From.X), deref(k[2].From.Y))
	}
	if !casi(k[3].From.X, 95) || !casi(k[3].From.Y, 5) || !casi(k[3].To.X, 70) || !casi(k[3].To.Y, 50) {
		t.Errorf("codo libre en vertical = %v,%v → %v,%v", deref(k[3].From.X), deref(k[3].From.Y), deref(k[3].To.X), deref(k[3].To.Y))
	}
	if k[4].From.Anchor != "right" || k[4].To.BoxID != "" || !casi(k[4].To.X, 75) || !casi(k[4].To.Y, 37.5) {
		t.Errorf("conector al hexágono = %+v → %v,%v", k[4], deref(k[4].To.X), deref(k[4].To.Y))
	}

	// Medios: el vídeo incrustado con su póster, en bucle y automático; el audio,
	// enlazado.
	v, w := s2.Boxes[0], s2.Boxes[1]
	if v.Kind != "media" || v.MediaType != "video" || v.Src != mp4Falso || v.Poster != onePxPNG || !v.Autoplay || !v.Loop {
		t.Errorf("vídeo = %+v", v)
	}
	if w.Kind != "media" || w.MediaType != "audio" || w.Src != "https://media.example.eu/voz.mp3" || w.Poster != "" || w.Autoplay || w.Loop {
		t.Errorf("audio = %+v", w)
	}
	if got := animsDe(s2.Boxes[2]); !reflect.DeepEqual(got, []string{"appear/click"}) {
		t.Errorf("tras el paso automático, la animación es %v", got)
	}
	assertBlocksEqual(t, "título", "<h1>Título</h1>", s1.Boxes[2].HTML)
}

func casi(v *float64, want float64) bool { return v != nil && *v > want-0.05 && *v < want+0.05 }

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// TestPptxPresentacionValida: el orden de los hijos de p:presentation es el del
// esquema (PowerPoint repara el fichero si no), el tema oscuro invierte el mapa de
// colores, y cada medio incrustado tiene su tipo de contenido.
func TestPptxPresentacionValida(t *testing.T) {
	parts := zipParts(t, exportarPptx(t, modeloCompleto(t)))
	pres := parts["ppt/presentation.xml"]
	orden := []string{"<p:sldMasterIdLst>", "<p:sldIdLst>", "<p:sldSz ", "<p:notesSz ", "<p:defaultTextStyle>"}
	last := -1
	for _, o := range orden {
		i := strings.Index(pres, o)
		if i < last {
			t.Fatalf("%s fuera de orden en %s", o, pres)
		}
		last = i
	}
	mustContain(t, "4:3", pres, `<p:sldSz cx="12192000" cy="9144000"/>`)
	mustContain(t, "mapa oscuro", parts["ppt/slideMasters/slideMaster1.xml"], `bg1="dk1" tx1="lt1"`)
	mustContain(t, "tipo mp4", parts["[Content_Types].xml"], `<Default Extension="mp4" ContentType="video/mp4"/>`)
	if _, ok := parts["ppt/media/media1.mp4"]; !ok {
		t.Error("el vídeo no se incrustó")
	}
	for i := 1; i <= 4; i++ {
		if _, ok := parts["ppt/slideLayouts/slideLayout"+string(rune('0'+i))+".xml"]; !ok {
			t.Errorf("falta el diseño %d", i)
		}
	}
}

// TestPptxSeAbreEnLibreOffice: lo exportado lo abre LibreOffice y conserva lo que
// importa al convertirlo a .odp: las transiciones, las animaciones, el medio y los
// conectores. Solo donde hay `soffice` (el contenedor de los agentes); en el resto
// se salta.
func TestPptxSeAbreEnLibreOffice(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("sin soffice")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "fiel.pptx")
	if err := os.WriteFile(in, exportarPptx(t, modeloCompleto(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
		"--convert-to", "odp", "--outdir", dir, in)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice: %v\n%s", err, out)
	}
	// Además de .odp, debe poder pintarse: la conversión a PDF es lo que hace LibreOffice al imprimir.
	pdfCmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
		"--convert-to", "pdf", "--outdir", dir, in)
	if out, err := pdfCmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice (pdf): %v\n%s", err, out)
	}
	if pdf, err := os.ReadFile(filepath.Join(dir, "fiel.pdf")); err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("LibreOffice no produjo un PDF válido: %v", err)
	}
	odp, err := os.ReadFile(filepath.Join(dir, "fiel.odp"))
	if err != nil {
		t.Fatalf("LibreOffice no produjo el .odp: %v", err)
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
		_ = os.WriteFile("/tmp/fiel-content.xml", []byte(content), 0o600)
		_ = os.WriteFile("/tmp/fiel-styles.xml", []byte(styles), 0o600)
		_ = os.WriteFile("/tmp/fiel.pptx", exportarPptx(t, modeloCompleto(t)), 0o600)
	}
	for _, want := range []string{
		`smil:type="fade"`,                       // transición de fundido
		`presentation:node-type="main-sequence"`, // animaciones
		`presentation:preset-class="motion-path"`,
		`<draw:connector`, // conectores
		`<draw:plugin`,    // vídeo
	} {
		if !strings.Contains(content, want) {
			t.Errorf("LibreOffice perdió %s", want)
		}
	}
	if !strings.Contains(styles, "0a0a0a") && !strings.Contains(styles, "0A0A0A") {
		t.Error("el fondo del tema no llegó al patrón")
	}
}

// TestPptxTodasLasAnimaciones: cada efecto del editor —entradas, énfasis, salidas y
// todas las trayectorias— vuelve como el mismo efecto.
func TestPptxTodasLasAnimaciones(t *testing.T) {
	tipos := make([]string, 0, len(animFxOf)+len(motionPresetPoints))
	tipos = append(tipos, "appear", "fade", "fly", "zoom", "pulse", "spin", "grow-shrink", "disappear", "fade-out", "fly-out", "zoom-out")
	for typ := range motionPresetPoints {
		tipos = append(tipos, typ)
	}
	anims := make([]slideAnim, 0, len(tipos))
	for i, typ := range tipos {
		anims = append(anims, slideAnim{Type: typ, Order: i + 1})
	}
	b, _ := json.Marshal(slidesModel{Slides: []slideT{{ID: "s", Boxes: []slideBox{{ID: "x", X: 10, Y: 10, W: 20, H: 20, Kind: "text", HTML: "<p>x</p>", Anims: anims}}}}})
	m := reimportar(t, "anims.pptx", exportarPptx(t, string(b)))
	got := make([]string, 0, len(tipos))
	for _, a := range m.Slides[0].Boxes[0].Anims {
		got = append(got, a.Type)
	}
	if !reflect.DeepEqual(got, tipos) {
		t.Errorf("animaciones\n got %v\nwant %v", got, tipos)
	}
}
