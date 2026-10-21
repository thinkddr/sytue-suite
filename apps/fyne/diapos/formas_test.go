// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
)

// diapoDe arma un modelo de una diapositiva con esas cajas.
func diapoDe(t *testing.T, cajas string) *Modelo {
	t.Helper()
	m, err := Leer([]byte(`{"slides":[{"id":"s","boxes":[` + cajas + `]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// pintaDiapo pinta la diapositiva a 960×540 (escala 1 del escenario) y la guarda.
func pintaDiapo(t *testing.T, m *Modelo, nombre string) func(x, y float32) color.NRGBA {
	t.Helper()
	l := NuevoLienzo(m, 0)
	l.Resize(fyne.NewSize(AnchoEscenario, m.AltoEscenario()))
	img := pinta(t, AnchoEscenario, m.AltoEscenario(), l)
	guarda(t, nombre, img)
	return func(x, y float32) color.NRGBA { return px(img, int(x*escala), int(y*escala)) }
}

var (
	pizarra = color.NRGBA{R: 0x47, G: 0x55, B: 0x69, A: 255}
	azul    = color.NRGBA{R: 0x25, G: 0x63, B: 0xEB, A: 255}
)

// TestLasFormasSonElSVGDeLaWeb: las once formas se rasterizan (un SVG que `oksvg` no
// entiende entraría en pánico), la estrella lleva su relleno por dentro y su pico
// fuera, y la línea va por el MEDIO de la caja de lado a lado, no de esquina a esquina.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con la línea de esquina a esquina que
// pintaba el visor, el arranque a media altura queda en papel y la diagonal, entintada.
func TestLasFormasSonElSVGDeLaWeb(t *testing.T) {
	formas := []string{
		"rect", "ellipse", "line", "arrow", "triangle", "diamond", "pentagon", "hexagon",
		"star", "cylinder", "callout",
	}
	for _, f := range formas {
		svg := svgForma(Caja{Forma: f}, 200, 100)
		if !strings.HasPrefix(string(svg), "<svg") {
			t.Fatalf("%s: SVG raro: %s", f, svg)
		}
	}
	m := diapoDe(t, `{"id":"e","x":10,"y":10,"w":20,"h":40,"kind":"shape","shape":"star","html":""},`+
		`{"id":"l","x":50,"y":10,"w":40,"h":20,"kind":"shape","shape":"line","html":""}`)
	en := pintaDiapo(t, m, "formas.png")
	// La estrella ocupa 192×216 desde (96, 54): su centro es relleno y la esquina de
	// arriba a la izquierda de la caja, fuera de las puntas, es papel.
	if c := en(96+96, 54+108); !cerca(c, pizarra) {
		t.Errorf("el centro de la estrella pinta %v y es el relleno pizarra", c)
	}
	if c := en(96+4, 54+4); !cerca(c, papel) {
		t.Errorf("la esquina de la caja de la estrella pinta %v: la estrella no es un rectángulo", c)
	}
	// La línea: caja de 384×108 desde (480, 54); la raya va a y = 54+54, de x = 482 a
	// 862. Cerca de su arranque, en la altura media, hay tinta; donde pasaría la
	// diagonal de esquina a esquina (y = 54 + 20·108/384), papel.
	if c := en(480+20, 54+54); !cerca(c, tinta) {
		t.Errorf("el arranque de la línea pinta %v y es la tinta", c)
	}
	if c := en(480+20, 54+20*108.0/384); !cerca(c, papel) {
		t.Errorf("la diagonal de la caja de la línea pinta %v: la línea va de esquina a esquina", c)
	}
}

// TestLasMarcasBonitasSonLasDeLaHoja: `niceTicks` con sus casos del borde.
func TestLasMarcasBonitasSonLasDeLaHoja(t *testing.T) {
	casos := []struct {
		lo, hi float64
		quiero []float64
	}{
		{0, 15, []float64{0, 5, 10, 15}},
		{0, 12, []float64{0, 5, 10, 15}},
		{-3, 7, []float64{-5, 0, 5, 10}},
		{0, 0, []float64{0, 0.5, 1}},
		{5, 5, []float64{0, 2, 4, 6}},
	}
	for _, c := range casos {
		if got := marcasBonitas(c.lo, c.hi).Marcas; !slices.Equal(got, c.quiero) {
			t.Errorf("marcasBonitas(%v, %v) = %v, quiero %v", c.lo, c.hi, got, c.quiero)
		}
	}
	if n, _, ok := figuras.Celda("1.234,5"); !ok || n != 1234.5 {
		t.Errorf("«1.234,5» se lee %v (%v) y es 1234,5", n, ok)
	}
	if _, s, ok := figuras.Celda("Ene"); ok || s != "Ene" {
		t.Errorf("«Ene» es texto y sale %v %q", ok, s)
	}
}

// TestElGraficoDeArranqueEsElDeLaWeb: la rejilla de `DEFAULT_CHART_GRID` da cuatro
// categorías y una serie con cabecera, y pintada deja barras azules y rótulos.
func TestElGraficoDeArranqueEsElDeLaWeb(t *testing.T) {
	g := &Grafico{Tipo: "bar", Rejilla: [][]string{
		{"", "Serie 1"}, {"Ene", "10"}, {"Feb", "15"}, {"Mar", "7"}, {"Abr", "12"},
	}}
	rotulos, series := datosGrafico(g)
	if !slices.Equal(rotulos, []string{"Ene", "Feb", "Mar", "Abr"}) || len(series) != 1 ||
		series[0].Nombre != "Serie 1" || !slices.Equal(series[0].Valores, []float64{10, 15, 7, 12}) {
		t.Fatalf("datos = %v %+v", rotulos, series)
	}
	gc := componerGrafico(g, 576, 345.6)
	textos := make([]string, 0, len(gc.rotulos))
	for _, r := range gc.rotulos {
		textos = append(textos, r.Texto)
	}
	if !slices.Equal(textos, []string{"0", "5", "10", "15", "Ene", "Feb", "Mar", "Abr"}) {
		t.Errorf("rótulos = %v", textos)
	}
	m := diapoDe(t, `{"id":"g","x":20,"y":18,"w":60,"h":64,"kind":"chart","html":"",`+
		`"chart":{"type":"bar","grid":[["","Serie 1"],["Ene","10"],["Feb","15"],["Mar","7"],["Abr","12"]]}}`)
	en := pintaDiapo(t, m, "grafico.png")
	// La segunda barra (Feb, 15) llega arriba del todo: el centro de su columna, a
	// media altura del área de trazado, es azul.
	plotL, plotW := math.Min(38, 576*0.16), 576-math.Min(38, 576*0.16)-6
	x := 192 + float32(plotL+plotW/4*1.5)
	if c := en(x, 97+172); !cerca(c, azul) {
		t.Errorf("la barra de febrero pinta %v y es azul", c)
	}
}

// TestElDiagramaRepartelosNodos: la geometría de las cuatro disposiciones.
func TestElDiagramaRepartelosNodos(t *testing.T) {
	uno := 1.0
	elems := []Elemento{{Texto: "A"}, {Texto: "B", Nivel: &uno}, {Texto: "C", Nivel: &uno}, {Texto: "D"}}
	for _, c := range []struct {
		disp           string
		nodos, flechas int
	}{{"list", 4, 0}, {"process", 4, 3}, {"cycle", 4, 4}, {"hierarchy", 4, 2}} {
		n, f := geometriaSmart(c.disp, elems, 600, 300)
		if len(n) != c.nodos || len(f) != c.flechas {
			t.Errorf("%s: %d nodos y %d conectores, quiero %d y %d", c.disp, len(n), len(f), c.nodos, c.flechas)
		}
	}
	// En la jerarquía A tiene dos hijos y se centra sobre ellos; D es otra raíz.
	n, _ := geometriaSmart("hierarchy", elems, 600, 300)
	a, b, c := n[0], n[1], n[2]
	if math.Abs((a.X+a.W/2)-((b.X+b.W/2+c.X+c.W/2)/2)) > 0.01 || b.Y <= a.Y {
		t.Errorf("A no está centrado encima de sus hijos: %+v %+v %+v", a, b, c)
	}
}

// TestElConectorSaleDelBordeQueMira: un recto entre dos cajas en fila sale por el
// borde derecho de la primera y entra por el izquierdo de la segunda.
func TestElConectorSaleDelBordeQueMira(t *testing.T) {
	a := rectC{0, 0, 100, 50}
	b := rectC{300, 0, 100, 50}
	r := rutaConector(extremoC{rect: &a}, extremoC{rect: &b}, "straight")
	if r.d != "M 100 25 L 300 25" {
		t.Errorf("ruta = %q", r.d)
	}
	if r.final.ang != 0 || math.Abs(r.inicio.ang-math.Pi) > 1e-9 {
		t.Errorf("ángulos de las puntas: inicio %v, final %v", r.inicio.ang, r.final.ang)
	}
}
