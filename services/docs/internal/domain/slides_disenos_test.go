// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func modeloConDisenos() slidesModel {
	return slidesModel{
		Theme:  "custom-1",
		Themes: []customTheme{{ID: "custom-1", Name: "Cooperativa", Bg: "#F4F1EA", Ink: "#1B2A41", Font: "Georgia"}},
		Master: &slideMaster{
			Boxes: []slideBox{{ID: "pie", X: 5, Y: 92, W: 30, H: 6, Kind: "text", HTML: "<p>Cooperativa</p>"}},
			Layouts: []slideLayout{
				{ID: "portada", Name: "Portada", Boxes: []slideBox{
					{ID: "t", X: 10, Y: 35, W: 80, H: 20, Kind: "text", HTML: "<h1>Título de la portada</h1>"},
				}},
				{ID: "dos", Name: "Dos columnas & más", Boxes: []slideBox{
					{ID: "a", X: 8, Y: 8, W: 84, H: 16, Kind: "text", HTML: "<h1>Título</h1>"},
					{ID: "b", X: 8, Y: 30, W: 40, H: 60, Kind: "text", HTML: "<p>Izquierda</p>"},
					{ID: "c", X: 52, Y: 30, W: 40, H: 60, Kind: "text", HTML: "<p>Derecha</p>"},
				}},
			},
		},
		Slides: []slideT{
			{ID: "s1", Layout: "portada", Boxes: []slideBox{{ID: "x", X: 10, Y: 35, W: 80, H: 20, Kind: "text", HTML: "<h1>Hola</h1>"}}},
			{ID: "s2", Layout: "dos", Boxes: []slideBox{{ID: "y", X: 8, Y: 30, W: 40, H: 60, Kind: "text", HTML: "<p>Cuerpo</p>"}}},
			{ID: "s3", Boxes: []slideBox{{ID: "z", X: 8, Y: 8, W: 84, H: 16, Kind: "text", HTML: "<h1>Sin diseño</h1>"}}},
		},
	}
}

func layoutDe(m slidesModel, s slideT) *slideLayout {
	if m.Master == nil {
		return nil
	}
	for i, l := range m.Master.Layouts {
		if l.ID == s.Layout {
			return &m.Master.Layouts[i]
		}
	}
	return nil
}

func cerca(a, b float64) bool { return math.Abs(a-b) < 0.06 }

// comprobarDisenos es lo que debe conservar un formato al exportar el modelo con tema propio y
// diseños y volver a importarlo.
func comprobarDisenos(t *testing.T, m slidesModel) {
	t.Helper()
	want := modeloConDisenos()
	// Tema propio: colores, fuente y nombre (el id lo pone el importador).
	if len(m.Themes) != 1 || m.Theme != m.Themes[0].ID {
		t.Fatalf("tema = %q %+v", m.Theme, m.Themes)
	}
	if th := m.Themes[0]; !strings.EqualFold(th.Bg, "#F4F1EA") || !strings.EqualFold(th.Ink, "#1B2A41") || th.Font != "Georgia" || th.Name != "Cooperativa" {
		t.Errorf("tema propio = %+v", th)
	}
	// Diseños: nombre, marcadores con su sitio y su muestra, y la diapositiva que lleva cada uno.
	if m.Master == nil || len(m.Master.Layouts) != 2 {
		t.Fatalf("diseños = %+v", m.Master)
	}
	for i, wl := range want.Master.Layouts {
		gl := m.Master.Layouts[i]
		if gl.Name != wl.Name || len(gl.Boxes) != len(wl.Boxes) {
			t.Errorf("diseño %d = %q con %d marcadores, quiero %q con %d", i, gl.Name, len(gl.Boxes), wl.Name, len(wl.Boxes))
			continue
		}
		for j, wb := range wl.Boxes {
			gb := gl.Boxes[j]
			if !cerca(gb.X, wb.X) || !cerca(gb.Y, wb.Y) || !cerca(gb.W, wb.W) || !cerca(gb.H, wb.H) || gb.HTML != wb.HTML {
				t.Errorf("marcador %d.%d = %+v, quiero %+v", i, j, gb, wb)
			}
		}
	}
	for i, s := range m.Slides {
		l := layoutDe(m, s)
		switch {
		case want.Slides[i].Layout == "" && l != nil:
			t.Errorf("diapositiva %d sin diseño lleva %q", i+1, l.Name)
		case want.Slides[i].Layout != "" && (l == nil || l.Name != layoutDe(want, want.Slides[i]).Name):
			t.Errorf("diapositiva %d: diseño %+v", i+1, l)
		}
	}
	if m.Master.Boxes == nil || len(m.Master.Boxes) != 1 {
		t.Errorf("cajas del patrón = %+v", m.Master.Boxes)
	}
}

func TestDisenosYTemaPropioPptxIdaYVuelta(t *testing.T) {
	data := exportarPptx(t, jsonDe(t, modeloConDisenos()))
	parts := zipParts(t, data)
	mustContain(t, "tema", parts["ppt/theme/theme1.xml"], `name="Cooperativa"`)
	mustContain(t, "fuente", parts["ppt/theme/theme1.xml"], `<a:latin typeface="Georgia"/>`)
	mustContain(t, "diseño", parts["ppt/slideLayouts/slideLayout3.xml"], `<p:cSld name="Dos columnas &amp; más">`)
	mustContain(t, "diseño de la 2.ª", parts["ppt/slides/_rels/slide2.xml.rels"], `slideLayouts/slideLayout3.xml`)
	mustContain(t, "sin diseño", parts["ppt/slides/_rels/slide3.xml.rels"], `slideLayouts/slideLayout1.xml`)
	comprobarDisenos(t, reimportar(t, "d.pptx", data))
}

func TestDisenosYTemaPropioOdpIdaYVuelta(t *testing.T) {
	data := exportarOdp(t, jsonDe(t, modeloConDisenos()))
	parts := zipParts(t, data)
	mustContain(t, "tema", parts["styles.xml"], `style:display-name="Cooperativa"`)
	mustContain(t, "fuente", parts["styles.xml"], `style:font-name="Georgia"`)
	mustContain(t, "diseño", parts["styles.xml"], `<style:presentation-page-layout style:name="AL_dos" sytue:id="dos" sytue:name="Dos columnas &amp; más">`)
	mustContain(t, "diapositiva", parts["content.xml"], `presentation:presentation-page-layout-name="AL_dos"`)
	comprobarDisenos(t, reimportar(t, "d.odp", data))
}

// TestDisenosYTemaHostiles: lo que no es un id, un color ni una fuente de verdad no llega al
// fichero, y el tema con colores rotos es el papel de siempre.
func TestDisenosYTemaHostiles(t *testing.T) {
	m := modeloConDisenos()
	m.Themes[0].Font = `Georgia"/><script>alert(1)</script>`
	m.Master.Layouts = append(m.Master.Layouts,
		slideLayout{ID: `x" y="`, Name: "id roto", Boxes: []slideBox{{Kind: "text", HTML: "<p>x</p>"}}},
		slideLayout{ID: "dos", Name: "repetido", Boxes: nil},
		slideLayout{ID: "imagenes", Name: "solo texto", Boxes: []slideBox{{Kind: "image", Src: onePxPNG}, {Kind: "text", X: -5, Y: 120, W: 500, H: 10, HTML: "<p>ok</p>"}}},
	)
	for nombre, data := range map[string][]byte{"pptx": exportarPptx(t, jsonDe(t, m)), "odp": exportarOdp(t, jsonDe(t, m))} {
		for fichero, contenido := range zipParts(t, data) {
			if strings.Contains(contenido, "<script") || strings.Contains(contenido, `x" y="`) {
				t.Errorf("%s %s: entrada hostil sin sanear", nombre, fichero)
			}
		}
		got := reimportar(t, "h."+nombre, data)
		if len(got.Master.Layouts) != 3 {
			t.Errorf("%s: diseños = %d, quiero los tres válidos", nombre, len(got.Master.Layouts))
		}
		if th := got.Themes; len(th) != 1 || th[0].Font != "" {
			t.Errorf("%s: la fuente rota debía quedar sin fuente: %+v", nombre, th)
		}
	}
	// Los colores rotos: el papel, sin colores propios.
	m.Themes[0].Bg = "rojo"
	if th := m.theme(); th.ID != "" || th.Bg != temaPapel.Bg {
		t.Errorf("tema con fondo roto = %+v", th)
	}
	// Los techos.
	muchos := make([]slideLayout, 0, 40)
	for i := range 40 {
		muchos = append(muchos, slideLayout{ID: "l" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Name: strings.Repeat("n", 100)})
	}
	if got := disenosValidos(muchos); len(got) != maxDisenos || len([]rune(got[0].Name)) != maxNombreDiseno {
		t.Errorf("techos: %d diseños, nombre de %d", len(got), len([]rune(got[0].Name)))
	}
}

// TestDisenosDePowerPoint: los diseños de un .pptx de PowerPoint entran con su nombre y sus
// marcadores (donde diga el diseño o el patrón), y la diapositiva que usa uno lo lleva.
func TestDisenosDePowerPoint(t *testing.T) {
	m := reimportar(t, "powerpoint.pptx", leerMuestra(t, muestra{ruta: "office/presentacion-powerpoint"}))
	if m.Master == nil || len(m.Master.Layouts) != 1 {
		t.Fatalf("diseños = %+v", m.Master)
	}
	l := m.Master.Layouts[0]
	if l.Name != "Título y objetos" || len(l.Boxes) != 2 {
		t.Fatalf("diseño = %+v", l)
	}
	if !strings.HasPrefix(l.Boxes[0].HTML, "<h1>") || l.Boxes[0].W < 80 || l.Boxes[1].Y <= l.Boxes[0].Y {
		t.Errorf("marcadores = %+v", l.Boxes)
	}
	if m.Slides[0].Layout != l.ID {
		t.Errorf("diseño de la diapositiva = %q, quiero %q", m.Slides[0].Layout, l.ID)
	}
}

// TestDisenosYTemaSeAbrenEnLibreOffice: LibreOffice abre y pinta el .pptx y el .odp con tema
// propio y diseños, y al guardar el .odp no se inventa marcadores en las diapositivas.
func TestDisenosYTemaSeAbrenEnLibreOffice(t *testing.T) {
	soffice, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("sin soffice")
	}
	dir := t.TempDir()
	in := map[string][]byte{
		"d.pptx": exportarPptx(t, jsonDe(t, modeloConDisenos())),
		"d.odp":  exportarOdp(t, jsonDe(t, modeloConDisenos())),
	}
	for nombre, data := range in {
		f := filepath.Join(dir, nombre)
		if err := os.WriteFile(f, data, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
			"--convert-to", "pdf", "--outdir", dir, f)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("soffice %s: %v\n%s", nombre, err, o)
		}
		pdf, err := os.ReadFile(filepath.Join(dir, strings.TrimSuffix(nombre, filepath.Ext(nombre))+".pdf"))
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
			t.Fatalf("LibreOffice no produjo un PDF válido de %s: %v", nombre, err)
		}
	}
	// Al guardar el .odp de nuevo, LibreOffice conserva las cajas de cada diapositiva y no
	// añade marcadores de presentación (título, esquema) por el diseño.
	cmd := exec.Command(soffice, "--headless", "-env:UserInstallation=file://"+filepath.Join(dir, "perfil"),
		"--convert-to", "odp", "--outdir", filepath.Join(dir, "salida"), filepath.Join(dir, "d.odp"))
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice odp: %v\n%s", err, o)
	}
	guardado, err := os.ReadFile(filepath.Join(dir, "salida", "d.odp"))
	if err != nil {
		t.Fatal(err)
	}
	content := zipParts(t, guardado)["content.xml"]
	if n := strings.Count(content, `presentation:class="title"`) + strings.Count(content, `presentation:class="outline"`); n != 0 {
		t.Errorf("LibreOffice añadió %d marcadores de presentación", n)
	}
	// (Cada página lleva además el marco de sus notas, que LibreOffice escribe siempre.)
	if n := strings.Count(content, "<draw:frame ") - strings.Count(content, `presentation:class="notes"`); n != 3 {
		t.Errorf("cajas tras guardar con LibreOffice = %d, quiero 3", n)
	}
}
