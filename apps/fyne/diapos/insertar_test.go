// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// TestLaCintaDeDiaposEsLaDeLaTaxonomia: la cinta de una presentación es Inicio · Insertar ·
// Diseño · Presentar · Revisar · Vista, las de la taxonomía común con la web
// (`office.PestanasDe`), sin «Archivo», que es del cromo.
func TestLaCintaDeDiaposEsLaDeLaTaxonomia(t *testing.T) {
	e, _, _ := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	visibles := Registro(e).Visibles(editor.Presentacion)
	ids := make([]string, 0, len(visibles))
	for _, p := range visibles {
		ids = append(ids, p.ID)
	}
	if got := strings.Join(ids, " "); got != "inicio insertar diseno presentar revisar vista" {
		t.Errorf("pestañas = %q", got)
	}
	comun := office.PestanasDe(office.Presentacion)
	taxonomia := make([]string, 0, len(comun))
	for _, p := range comun {
		taxonomia = append(taxonomia, p.ID)
	}
	if strings.Join(ids, " ") != strings.Join(taxonomia, " ") {
		t.Errorf("la cinta de Diapos pinta %v y la taxonomía común dice %v", ids, taxonomia)
	}
}

// TestLoInsertadoNaceElegidoYDondeLaWeb: una forma nace centrada (las lineales finas),
// elegida y con los colores por defecto; un conector con dos cajas elegidas las une con
// punta al final, y sin ellas nace suelto; un diagrama abre su panel.
func TestLoInsertadoNaceElegidoYDondeLaWeb(t *testing.T) {
	e, _, _ := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	e.NuevaForma("star")
	e.NuevaForma("arrow")
	cs := e.m.Diapos[0].Cajas
	if len(cs) != 2 || cs[0].Y != 32 || cs[0].H != 36 || cs[1].Y != 47 || cs[1].H != 6 || cs[1].Relleno != rellenoForma {
		t.Fatalf("formas = %+v", cs)
	}
	if e.sel != cs[1].ID {
		t.Error("lo insertado nace elegido")
	}
	e.SeleccionarSolo(cs[0].ID)
	e.Alternar(cs[1].ID)
	e.NuevoConector("elbow")
	k := e.m.Diapos[0].Conectores[0]
	if k.Desde.Caja != cs[0].ID || k.Hasta.Caja != cs[1].ID || k.PuntaFin == nil || !*k.PuntaFin {
		t.Errorf("el conector entre dos elegidas = %+v", k)
	}
	e.NuevoConector("straight")
	if k := e.m.Diapos[0].Conectores[1]; k.Desde.Caja != "" || *k.Desde.X != 28 || *k.Hasta.Y != 62 {
		t.Errorf("sin elegidas el conector nace suelto: %+v", k)
	}
	e.NuevoDiagrama("hierarchy")
	if e.abajo != abajoSmart || len(e.Primaria().Diagrama.Elementos) != 3 {
		t.Error("un diagrama nuevo abre sus elementos, con raíz y dos hijos")
	}
	if !e.m.Fiel([]byte(e.Serializar())) {
		t.Error("lo insertado no se vuelve a leer igual")
	}
}

// TestInsertarDeDiaposTieneElGrafico: «Gráfico» está en Insertar, tras Conectores y
// antes de SmartArt como en la web, y su botón inserta lo mismo que el de la mini-barra:
// una caja de barras elegida y con el panel de sus datos abierto. Antes solo vivía en la
// mini-barra, y quien lo buscaba donde está en Docus no lo encontraba.
func TestInsertarDeDiaposTieneElGrafico(t *testing.T) {
	e, ctx, _ := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	banda := e.cuerpoInsertar(ctx)
	rotulos := strings.Join(editor.RotulosDe(banda), " · ")
	conectores, grafico, smart := strings.Index(rotulos, "Conectores"), strings.Index(rotulos, "Gráfico"), strings.Index(rotulos, "SmartArt")
	if grafico < 0 || conectores > grafico || grafico > smart {
		t.Fatalf("Insertar no tiene «Gráfico» entre Conectores y SmartArt: %s", rotulos)
	}
	var boton *editor.BotonCinta
	for _, b := range buscar[*editor.BotonCinta](banda) {
		if b.Rotulo == "Insertar gráfico" {
			boton = b
		}
	}
	if boton == nil || boton.Inerte {
		t.Fatal("«Insertar gráfico» está en Insertar y se puede pulsar")
	}
	test.Tap(boton)
	cs := e.m.Diapos[0].Cajas
	if len(cs) != 1 || cs[0].Tipo != "chart" || cs[0].Grafico == nil || cs[0].Grafico.Tipo != "bar" {
		t.Fatalf("el botón no insertó un gráfico de barras: %+v", cs)
	}
	if e.sel != cs[0].ID || e.abajo != abajoGrafico {
		t.Error("el gráfico nace elegido y con el panel de sus datos abierto")
	}
}

// TestElEncuadreDeUnaImagenEsElDeLaWeb son los casos de `slides-elements.test.ts`.
func TestElEncuadreDeUnaImagenEsElDeLaWeb(t *testing.T) {
	cerca := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	x, y, w, h := RectImagen(1600, 900, 540)
	if w != 40 || !cerca(h, 40) || !cerca(x, 30) || !cerca(y, 30) {
		t.Errorf("16:9 = %v %v %v %v", x, y, w, h)
	}
	_, _, w, h = RectImagen(500, 500, 540)
	if !cerca(h*540/(w*960), 1) {
		t.Error("una cuadrada tiene que salir cuadrada en píxeles")
	}
	if _, _, w, h = RectImagen(900, 3000, 540); h != 70 || !cerca(w, 40*70/(40*(3000.0/900)*960/540)) {
		t.Errorf("vertical extrema = %v × %v", w, h)
	}
	if _, _, _, h = RectImagen(4000, 40, 540); h != 4 {
		t.Errorf("panorámica extrema: alto %v y el mínimo es 4", h)
	}
}

// TestUnaImagenGrandeSeReduceHastaCaber: un PNG de ruido de 2400 × 1800 (no se comprime)
// sale como JPEG por debajo de los 700 KiB, a 1600 de lado como mucho; y uno con
// transparencia sigue siendo PNG.
func TestUnaImagenGrandeSeReduceHastaCaber(t *testing.T) {
	ruido := image.NewNRGBA(image.Rect(0, 0, 2400, 1800))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range ruido.Pix {
		ruido.Pix[i] = uint8(r.IntN(256))
		if i%4 == 3 {
			ruido.Pix[i] = 255
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, ruido); err != nil {
		t.Fatal(err)
	}
	url, err := CodificarImagen(b.Bytes(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") || len(url) > maxCaracteresImagen {
		t.Errorf("sale %q… de %d caracteres", url[:30], len(url))
	}
	img, ok := imagenDe(url)
	if !ok || max(img.Bounds().Dx(), img.Bounds().Dy()) > maxLadoImagen {
		t.Errorf("el lado mayor es %v y el tope 1600", img.Bounds())
	}
	vidrio := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	vidrio.Set(1, 1, color.NRGBA{R: 255, A: 128})
	b.Reset()
	_ = png.Encode(&b, vidrio)
	if url, _ := CodificarImagen(b.Bytes(), false); !strings.HasPrefix(url, "data:image/png;") {
		t.Error("con transparencia tiene que seguir siendo PNG")
	}
}
