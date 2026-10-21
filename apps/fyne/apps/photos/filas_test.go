// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLasFilasSonLasDeLaWeb ata los números de filas.go a los literales de la web: los
// topes de la proporción (lib/filas.ts) y el hueco, los escalones y el 3,6 de `.filas`
// (styles/app.css). Cambiar uno allí pone esto en rojo.
func TestLasFilasSonLasDeLaWeb(t *testing.T) {
	ts := webref.Read(t, "apps/frontend/apps/photos/src/lib/filas.ts")
	css := webref.Read(t, "apps/frontend/apps/photos/src/styles/app.css")
	for _, c := range []struct{ fuente, literal, que string }{
		{ts, "PROPORCION_MIN = 9 / 20", "el tope de abajo"},
		{ts, "PROPORCION_MAX = 5 / 2", "el tope de arriba"},
		{css, "gap: 0.25rem", "el hueco"},
		{css, "--fila-max: 9rem", "el escalón de base"},
		{css, "--fila-max: 11rem", "el escalón de sm"},
		{css, "--fila-max: 13rem", "el escalón de xl"},
		{css, fmt.Sprintf("(100cqw - 0.25rem) / %g", CabenEnFila), "lo que cabe en una fila"},
	} {
		if !strings.Contains(c.fuente, c.literal) {
			t.Errorf("%s: la web ya no dice %q", c.que, c.literal)
		}
	}
	if ProporcionMin != 9.0/20 || ProporcionMax != 5.0/2 || HuecoFilas != 0.25*tema.Rem {
		t.Errorf("los topes o el hueco de filas.go no son los de la web: %v %v %v", ProporcionMin, ProporcionMax, HuecoFilas)
	}
}

// TestLaProporcionSeAcotaSoloEnLasExtremas es `proporcion()` de lib/filas.ts, con sus
// mismos casos. VISTO EN ROJO intercambiando los dos topes.
func TestLaProporcionSeAcotaSoloEnLasExtremas(t *testing.T) {
	for _, c := range []struct {
		w, h int
		r    float32
	}{
		{4000, 3000, 4.0 / 3},
		{3000, 4000, 3.0 / 4},
		{1170, 2532, 1170.0 / 2532},
		{9000, 2250, ProporcionMax},
		{900, 3600, ProporcionMin},
		{0, 0, 1},
		{4000, 0, 1},
	} {
		if got := Proporcion(api.Photo{Width: c.w, Height: c.h}); math.Abs(float64(got-c.r)) > 1e-5 {
			t.Errorf("Proporcion(%d×%d) = %v y la web da %v", c.w, c.h, got, c.r)
		}
	}
}

// razonesDelArnes son las proporciones del primer mes del arnés de medidas de la web
// (27 fotos: corrientes, una blanca, una panorámica, una tira vertical y una sin medir).
func razonesDelArnes() []float32 {
	medidas := [][2]int{
		{4000, 3000},
		{3000, 4000},
		{6000, 4000},
		{4000, 3000},
		{1920, 1080},
		{3000, 4000},
		{4000, 3000},
		{2000, 2000},
		{4000, 6000},
		{1170, 2532},
		{4000, 3000},
		{6000, 4000},
		{3000, 4000},
		{1080, 1920},
		{4000, 3000},
		{5472, 3648},
		{3000, 4000},
		{4000, 3000},
	}
	var r []float32
	n := 0
	foto := func(w, h int) {
		if w < 0 {
			w, h = medidas[n%len(medidas)][0], medidas[n%len(medidas)][1]
		}
		n++
		r = append(r, Proporcion(api.Photo{Width: w, Height: h}))
	}
	foto(-1, 0)
	foto(-1, 0)
	foto(4000, 3000) // la blanca
	foto(-1, 0)
	foto(9000, 2250) // panorámica
	foto(-1, 0)
	foto(900, 3600) // tira vertical
	foto(-1, 0)
	foto(0, 0) // sin dimensiones
	foto(4000, 3000)
	for range 17 {
		foto(-1, 0)
	}
	return r
}

// TestLasFilasMidenLoQueChromium: la misma cuenta que flexbox. Los números son de
// Chromium con la hoja real (arnés de medidas, 2026-10-04): el primer mes, a 1280 de
// ventana (rejilla de 1216) y a 390 (rejilla de 358). Cuántas fotos por fila y cuánto
// mide cada fila; la última, sin estirar, mide `--fila`.
func TestLasFilasMidenLoQueChromium(t *testing.T) {
	type fila struct {
		n    int
		alto float32
	}
	for _, c := range []struct {
		ventana, ancho float32
		filas          []fila
	}{
		{1280, 1216, []fila{{4, 253.47}, {5, 210.53}, {5, 219.02}, {4, 244.89}, {4, 244.89}, {5, 208}}},
		{390, 358, []fila{
			{3, 102.45},
			{1, 268.5},
			{2, 108.92},
			{3, 142.86},
			{2, 132.75},
			{3, 124.44},
			{2, 124.94},
			{3, 102.45},
			{2, 157.33},
			{2, 113.78},
			{3, 113.52},
			{1, 98.31},
		}},
	} {
		f := Filas{Razones: razonesDelArnes(), Ventana: c.ventana}
		cajas, medidas, alto := f.repartir(c.ancho)
		var got []fila
		for i := range cajas {
			if i == 0 || cajas[i].Y != cajas[i-1].Y {
				got = append(got, fila{0, medidas[i].Height})
			}
			got[len(got)-1].n++
			if d := medidas[i].Height - got[len(got)-1].alto; d > 0.01 || d < -0.01 {
				t.Errorf("a %v, la foto %d mide %v de alto y su fila %v", c.ventana, i, medidas[i].Height, got[len(got)-1].alto)
			}
		}
		if len(got) != len(c.filas) {
			t.Fatalf("a %v salen %d filas y Chromium pinta %d: %v", c.ventana, len(got), len(c.filas), got)
		}
		suma := float32(0)
		for i, g := range got {
			// Tres sesentaicuatroavos: Chromium cuenta en esa rejilla y redondea la
			// última fila hacia abajo (98,31 por 98,33).
			if g.n != c.filas[i].n || math.Abs(float64(g.alto-c.filas[i].alto)) > 0.05 {
				t.Errorf("a %v, la fila %d tiene %d fotos de %v y Chromium, %d de %v", c.ventana, i, g.n, g.alto, c.filas[i].n, c.filas[i].alto)
			}
			suma += g.alto
		}
		// El alto total: las filas y un hueco entre cada dos.
		if quiero := suma + HuecoFilas*float32(len(got)-1); math.Abs(float64(alto-quiero)) > 0.01 {
			t.Errorf("a %v la rejilla mide %v de alto y sus filas suman %v", c.ventana, alto, quiero)
		}
		// Las filas llenas llegan al borde derecho; la última no se estira.
		for i := range cajas {
			ultimaDeFila := i == len(cajas)-1 || cajas[i+1].Y != cajas[i].Y
			enUltimaFila := cajas[i].Y == cajas[len(cajas)-1].Y
			if ultimaDeFila && !enUltimaFila {
				if d := c.ancho - (cajas[i].X + medidas[i].Width); d > 0.01 || d < -0.01 {
					t.Errorf("a %v, la fila que acaba en la foto %d se queda a %v del borde", c.ventana, i, d)
				}
			}
		}
	}
}

// TestUnaFotoSolaNoSeEstira: la última fila mide `--fila` aunque sea una sola foto, y
// una panorámica nunca se sale de la rejilla, por estrecha que sea.
func TestUnaFotoSolaNoSeEstira(t *testing.T) {
	f := Filas{Razones: []float32{ProporcionMax}, Ventana: 1280}
	_, medidas, _ := f.repartir(1216)
	if medidas[0].Height != 13*tema.Rem {
		t.Errorf("una foto sola en la última fila mide %v de alto: quiero %v", medidas[0].Height, 13*tema.Rem)
	}
	for _, ancho := range []float32{120, 288, 358, 736} {
		_, medidas, _ = f.repartir(ancho)
		if medidas[0].Width > ancho {
			t.Errorf("la panorámica mide %v en una rejilla de %v", medidas[0].Width, ancho)
		}
	}
}

// TestElTopeDeLaFilaMideLaVentanaNoLaCaja es la trampa de las media queries: a 1280 de
// ventana la rejilla mide 1216, y si el corte `xl` se comparase con la CAJA no se
// cumpliría y la fila bajaría a 11rem. Los escalones de `--fila-max` son de ventana; lo
// que mira a la caja es el otro brazo del `min()`, el de los teléfonos.
func TestElTopeDeLaFilaMideLaVentanaNoLaCaja(t *testing.T) {
	if got := AltoFila(1280, 1216); got != 13*tema.Rem {
		t.Errorf("a 1280 de ventana la fila mide %v y la web 13rem", got)
	}
	if got := AltoFila(1216, 1216); got != 11*tema.Rem {
		t.Errorf("a 1216 de ventana la fila mide %v y la web 11rem: el corte es de ventana", got)
	}
	if got, quiero := AltoFila(390, 358), float32(358-4)/3.6; got != quiero {
		t.Errorf("en un teléfono la fila mide %v y la web (100cqw − hueco)/3,6 = %v", got, quiero)
	}
}
