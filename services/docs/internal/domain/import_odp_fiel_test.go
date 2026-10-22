// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Importación fiel de una presentación hecha por LibreOffice de verdad: los ficheros de
// testdata/fidelidad/libreoffice/diapos-avanzada.* salen de fuentes/diapos-avanzada.fodp
// con generar.sh, no de nuestro exportador.

func abrirDiapos(t *testing.T, nombre string) slidesModel {
	t.Helper()
	datos, err := os.ReadFile(filepath.Join("testdata", "fidelidad", "libreoffice", nombre))
	if err != nil {
		t.Fatal(err)
	}
	return reimportar(t, nombre, datos)
}

// comprobarDiapositivasAvanzadas es lo común del .odp y del .pptx de LibreOffice: el fondo
// del patrón, el fondo propio, el patrón oculto, las transiciones, el conector con su
// punta y las animaciones.
func comprobarDiapositivasAvanzadas(t *testing.T, m slidesModel) {
	t.Helper()
	if len(m.Slides) != 3 {
		t.Fatalf("diapositivas = %d, quiero 3", len(m.Slides))
	}
	s1, s2, s3 := m.Slides[0], m.Slides[1], m.Slides[2]
	// El fondo del patrón no es el de ningún tema del editor: la presentación lleva un tema
	// propio con él, y las diapositivas sin fondo propio no repiten nada.
	if m.Theme != "custom-importado" || len(m.Themes) != 1 || !strings.EqualFold(m.Themes[0].Bg, "#1B2A41") {
		t.Errorf("tema = %q %+v, quiero uno propio con el fondo #1B2A41", m.Theme, m.Themes)
	}
	if s1.Bg != "" || !strings.EqualFold(s2.Bg, "#E8EFE0") || !strings.EqualFold(s3.Bg, "#0F1B2D") {
		t.Errorf("fondos = %q %q %q", s1.Bg, s2.Bg, s3.Bg)
	}
	if s1.HideMaster || s2.HideMaster || !s3.HideMaster {
		t.Errorf("patrón oculto = %v %v %v", s1.HideMaster, s2.HideMaster, s3.HideMaster)
	}
	if s1.Transition != "fade" || s2.Transition != "slide" || s3.Transition != "" {
		t.Errorf("transiciones = %q %q %q", s1.Transition, s2.Transition, s3.Transition)
	}
	if m.Master == nil || !strings.Contains(boxesHTML(m.Master.Boxes), "Cooperativa La Vega") || !strings.Contains(boxesHTML(m.Master.Boxes), "{n}") {
		t.Errorf("patrón = %+v, quiero el pie y los campos de número y total", m.Master)
	}

	// Formas y conector de la diapositiva 2.
	deposito, pozo := -1, -1
	for i, b := range s2.Boxes {
		switch {
		case strings.Contains(b.HTML, "Depósito"):
			deposito = i
		case strings.Contains(b.HTML, "Pozo"):
			pozo = i
		}
	}
	if deposito < 0 || pozo < 0 {
		t.Fatalf("faltan las formas: %+v", s2.Boxes)
	}
	if b := s2.Boxes[deposito]; b.Kind != "shape" || b.Shape != "rect" || !strings.EqualFold(b.Fill, "#2E86AB") {
		t.Errorf("depósito = %+v", b)
	}
	if b := s2.Boxes[pozo]; b.Kind != "shape" || b.Shape != "ellipse" || !strings.EqualFold(b.Fill, "#8C6239") {
		t.Errorf("pozo = %+v", b)
	}
	if len(s2.Connectors) != 1 {
		t.Fatalf("conectores = %+v", s2.Connectors)
	}
	if k := s2.Connectors[0]; k.From.BoxID != s2.Boxes[deposito].ID || k.To.BoxID != s2.Boxes[pozo].ID || !k.EndArrow || k.StartArrow {
		t.Errorf("conector = %+v", k)
	}

	// Animaciones: el depósito entra al hacer clic, el pozo gira a la vez y sale en otro clic.
	if got := animsDe(s2.Boxes[deposito]); !reflect.DeepEqual(got, []string{"fade/click"}) {
		t.Errorf("animaciones del depósito = %v", got)
	}
	if got := animsDe(s2.Boxes[pozo]); !reflect.DeepEqual(got, []string{"spin/with", "fade-out/click"}) {
		t.Errorf("animaciones del pozo = %v", got)
	}
}

func boxesHTML(boxes []slideBox) string {
	var b strings.Builder
	for _, x := range boxes {
		b.WriteString(x.HTML)
	}
	return b.String()
}

func TestImportOdpDeLibreOfficeEsFiel(t *testing.T) {
	comprobarDiapositivasAvanzadas(t, abrirDiapos(t, "diapos-avanzada.odp"))
}

func TestImportPptxDeLibreOfficeEsFiel(t *testing.T) {
	comprobarDiapositivasAvanzadas(t, abrirDiapos(t, "diapos-avanzada.pptx"))
}

// odpMinimo empaqueta un .odp con ese content.xml y ese styles.xml, más los ficheros
// extra: lo que escribe LibreOffice para un vídeo, sin tener que generarlo con él.
func odpMinimo(t *testing.T, content, styles string, extra map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	put := func(name string, data []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	}
	put("mimetype", []byte("application/vnd.oasis.opendocument.presentation"))
	put("content.xml", []byte(content))
	put("styles.xml", []byte(styles))
	for n, d := range extra {
		put(n, d)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const odpNS = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
	`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
	`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:xlink="http://www.w3.org/1999/xlink" ` +
	`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" ` +
	`xmlns:smil="urn:oasis:names:tc:opendocument:xmlns:smil-compatible:1.0" xmlns:anim="urn:oasis:names:tc:opendocument:xmlns:animation:1.0"`

const odpEstilosMinimos = `<?xml version="1.0"?><office:document-styles ` + odpNS + `><office:automatic-styles>` +
	`<style:page-layout style:name="PM1"><style:page-layout-properties fo:page-width="28cm" fo:page-height="15.75cm"/></style:page-layout>` +
	`</office:automatic-styles><office:master-styles><style:master-page style:name="M" style:page-layout-name="PM1"/></office:master-styles></office:document-styles>`

// TestImportOdpMedioDeLibreOffice: un vídeo como lo guarda Impress —`draw:plugin` con su
// fichero en Media/, el bucle como parámetro y el póster— y el audio de una URL: vuelven
// como medios, con el póster solo si es del usuario, y el que arranca solo, automático.
func TestImportOdpMedioDeLibreOffice(t *testing.T) {
	video := []byte("\x00\x00\x00\x18ftypmp42sytue-prueba")
	png, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(onePxPNG, "data:image/png;base64,"))
	content := `<?xml version="1.0"?><office:document-content ` + odpNS + `><office:body><office:presentation>` +
		`<draw:page draw:name="p1" draw:master-page-name="M">` +
		`<draw:frame draw:id="id1" svg:x="2cm" svg:y="2cm" svg:width="14cm" svg:height="8cm">` +
		`<draw:plugin xlink:href="Media/clip.mp4" xlink:type="simple" draw:mime-type="application/vnd.sun.star.media">` +
		`<draw:param draw:name="Loop" draw:value="true"/><draw:param draw:name="Mute" draw:value="false"/></draw:plugin>` +
		`<draw:image xlink:href="Pictures/poster.png"/></draw:frame>` +
		`<draw:frame draw:id="id2" svg:x="18cm" svg:y="2cm" svg:width="8cm" svg:height="2cm">` +
		`<draw:plugin xlink:href="https://media.example.eu/voz.mp3" draw:mime-type="application/vnd.sun.star.media"/></draw:frame>` +
		`<anim:par presentation:node-type="timing-root"><anim:seq presentation:node-type="main-sequence">` +
		`<anim:par smil:begin="0s"><anim:par smil:begin="0s"><anim:par smil:begin="0s" presentation:node-type="after-previous" presentation:preset-class="media-call" presentation:preset-id="">` +
		`<anim:command smil:dur="0.001s" smil:targetElement="id1" anim:command="play"/></anim:par></anim:par></anim:par>` +
		`</anim:seq></anim:par></draw:page></office:presentation></office:body></office:document-content>`
	m := reimportar(t, "m.odp", odpMinimo(t, content, odpEstilosMinimos, map[string][]byte{"Media/clip.mp4": video, "Pictures/poster.png": png}))
	if len(m.Slides) != 1 || len(m.Slides[0].Boxes) != 2 {
		t.Fatalf("modelo = %+v", m)
	}
	v, a := m.Slides[0].Boxes[0], m.Slides[0].Boxes[1]
	if v.Kind != "media" || v.MediaType != "video" || v.Src != mp4Falso || v.Poster != onePxPNG || !v.Loop || !v.Autoplay {
		t.Errorf("vídeo = %+v", v)
	}
	if a.Kind != "media" || a.MediaType != "audio" || a.Src != "https://media.example.eu/voz.mp3" || a.Autoplay || a.Loop {
		t.Errorf("audio = %+v", a)
	}
}

// TestImportOdpEstilosHostiles: una herencia de estilos en círculo o sin hueco no cuelga
// ni rompe la importación, y sin `styles.xml` hay diapositivas igual.
func TestImportOdpEstilosHostiles(t *testing.T) {
	content := `<?xml version="1.0"?><office:document-content ` + odpNS + `><office:automatic-styles>` +
		`<style:style style:name="a" style:family="graphic" style:parent-style-name="b"/>` +
		`<style:style style:name="b" style:family="graphic" style:parent-style-name="a"><style:graphic-properties draw:fill="solid" draw:fill-color="#112233"/></style:style>` +
		`</office:automatic-styles><office:body><office:presentation><draw:page draw:style-name="a">` +
		`<draw:rect draw:style-name="a" svg:x="1cm" svg:y="1cm" svg:width="4cm" svg:height="2cm"><text:p>x</text:p></draw:rect>` +
		`<draw:connector draw:start-shape="no-existe" svg:x1="1cm" svg:y1="1cm" svg:x2="2cm" svg:y2="2cm"/>` +
		`</draw:page></office:presentation></office:body></office:document-content>`
	m := reimportar(t, "h.odp", odpMinimo(t, content, odpEstilosMinimos, nil))
	if len(m.Slides) != 1 || len(m.Slides[0].Boxes) != 1 || len(m.Slides[0].Connectors) != 1 {
		t.Fatalf("modelo = %+v", m)
	}
	if k := m.Slides[0].Connectors[0]; k.From.BoxID != "" || k.From.X == nil || k.Kind != "elbow" {
		t.Errorf("conector sin forma = %+v", k)
	}
	if got := importarSinEstilos(t, content); got != 1 {
		t.Errorf("sin styles.xml: %d diapositivas", got)
	}
}

func importarSinEstilos(t *testing.T, content string) int {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("content.xml")
	_, _ = w.Write([]byte(content))
	_ = zw.Close()
	return len(reimportar(t, "s.odp", buf.Bytes()).Slides)
}

// TestPptxMarcadoresHeredanSuGeometria: el título y el cuerpo de una diapositiva de
// PowerPoint, con `<p:spPr/>` vacío, se colocan donde dice su diseño y su patrón; y un
// párrafo de 32 pt con un solo tramo en negrita sigue siendo texto con su negrita, no un
// título.
func TestPptxMarcadoresHeredanSuGeometria(t *testing.T) {
	datos := leerMuestra(t, muestra{ruta: "office/presentacion-powerpoint"})
	m := reimportar(t, "powerpoint.pptx", datos)
	var titulo, cuerpo *slideBox
	for i, b := range m.Slides[0].Boxes {
		switch {
		case strings.Contains(b.HTML, "Plan de riego 2027"):
			titulo = &m.Slides[0].Boxes[i]
		case strings.Contains(b.HTML, "reducir un 20 %"):
			cuerpo = &m.Slides[0].Boxes[i]
		}
	}
	if titulo == nil || cuerpo == nil {
		t.Fatalf("faltan cajas: %+v", m.Slides[0].Boxes)
	}
	if titulo.X < 6 || titulo.X > 8.5 || titulo.Y < 8 || titulo.Y > 11 || titulo.W < 80 {
		t.Errorf("título en %.1f,%.1f %.1fx%.1f: no hereda la geometría del patrón", titulo.X, titulo.Y, titulo.W, titulo.H)
	}
	if cuerpo.Y <= titulo.Y || cuerpo.W < 80 {
		t.Errorf("cuerpo en %.1f,%.1f %.1fx%.1f: debería ir bajo el título", cuerpo.X, cuerpo.Y, cuerpo.W, cuerpo.H)
	}
	if !strings.Contains(cuerpo.HTML, "<strong>reducir un 20 %") && !strings.Contains(cuerpo.HTML, "<b>reducir un 20 %") {
		t.Errorf("el cuerpo perdió la negrita: %s", cuerpo.HTML)
	}
}

// TestPptxTituloPorTamano: los tamaños de título del exportador siguen dando título; los
// que usaban versiones anteriores (32 y 28 pt) solo si todo el párrafo va en negrita.
func TestPptxTituloPorTamano(t *testing.T) {
	casos := []struct {
		sz        int
		negritas  []bool
		quiereTit bool
	}{
		{4400, []bool{false}, true},
		{3000, []bool{true}, true},
		{2400, []bool{true, false}, true},
		{3200, []bool{true}, true},
		{2800, []bool{true, true}, true},
		{3200, []bool{false, true, false}, false},
		{2800, []bool{false}, false},
	}
	for _, c := range casos {
		p := &xpara{}
		for _, b := range c.negritas {
			p.runs = append(p.runs, xrun{text: "x", b: b})
		}
		headingFromSize(p, c.sz)
		if got := strings.HasPrefix(p.style, "Heading"); got != c.quiereTit {
			t.Errorf("%d pt con negritas %v: título = %v, quiero %v", c.sz, c.negritas, got, c.quiereTit)
		}
	}
}
