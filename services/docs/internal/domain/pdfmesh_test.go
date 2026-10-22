// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de los sombreados de malla. El fallo que cubren no es un error: es un
// degradado que sale como un bloque de color plano, que es lo que pasaba cuando
// los tipos 4-7 se resolvían con el color del centro del dominio.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// escritorDeBits arma el flujo de una malla. Existe porque estos flujos son
// campos de anchura arbitraria sin alinear a byte, y escribirlos con literales
// haría los tests ilegibles.
type escritorDeBits struct {
	bytes []byte
	bits  int
}

func (w *escritorDeBits) poner(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.bits%8 == 0 {
			w.bytes = append(w.bytes, 0)
		}
		if (v>>uint(i))&1 == 1 {
			w.bytes[len(w.bytes)-1] |= 1 << uint(7-w.bits%8)
		}
		w.bits++
	}
}

func (w *escritorDeBits) alinear() {
	for w.bits%8 != 0 {
		w.poner(0, 1)
	}
}

// vertice8 escribe un vértice con coordenadas y color de 8 bits, que es lo que
// usan estos tests: con /Decode [0 255 0 255 0 1 0 1 0 1] el byte ES la
// coordenada, así que los números del test se leen directamente.
func (w *escritorDeBits) vertice8(x, y, r, g, b uint64) {
	w.poner(x, 8)
	w.poner(y, 8)
	w.poner(r, 8)
	w.poner(g, 8)
	w.poner(b, 8)
}

const decodeMalla8 = "[0 255 0 255 0 1 0 1 0 1]"

// pdfConMalla arma un PDF cuyo `sh` apunta a una malla con el flujo dado.
func pdfConMalla(t *testing.T, tipo int, extraSombreado string, flujo []byte) []byte {
	t.Helper()
	return pdfConContenido(t,
		"q 0 0 100 100 re W n /Sh0 sh Q",
		`/Resources << /Shading << /Sh0 5 0 R >> >>`,
		fmt.Sprintf("<< /ShadingType %d /ColorSpace /DeviceRGB "+
			"/BitsPerCoordinate 8 /BitsPerComponent 8 /BitsPerFlag 8 "+
			"/Decode %s %s /Length %d >>\nstream\n%s\nendstream",
			tipo, decodeMalla8, extraSombreado, len(flujo), string(flujo)))
}

// TestMallaLibreDibujaTriangulos es el check central del tipo 4: antes salía un
// solo rectángulo de color plano y ahora tienen que salir figuras.
func TestMallaLibreDibujaTriangulos(t *testing.T) {
	w := &escritorDeBits{}
	// Un triángulo: rojo abajo-izquierda, verde abajo-derecha, azul arriba.
	for _, v := range [][5]uint64{
		{0, 0, 255, 0, 0},
		{100, 0, 0, 255, 0},
		{50, 100, 0, 0, 255},
	} {
		w.poner(0, 8) // bandera 0: triángulo nuevo
		w.vertice8(v[0], v[1], v[2], v[3], v[4])
		w.alinear()
	}

	svg := renderizar(t, pdfConMalla(t, 4, "", w.bytes))
	if strings.Count(svg, "<path") < 4 {
		t.Fatalf("la malla no se trianguló: %s", primerasLineas(svg))
	}
	// Con subdivisión, los trocitos tienen colores intermedios: si saliera un
	// bloque plano habría un solo color.
	if colores := coloresDistintos(svg); colores < 4 {
		t.Errorf("solo %d colores: la malla salió plana", colores)
	}
}

// TestMallaLibreEncadenaTiras cubre las banderas 1 y 2, que reutilizan vértices
// del triángulo anterior. Confundirlas no da error: retuerce la tira.
func TestMallaLibreEncadenaTiras(t *testing.T) {
	w := &escritorDeBits{}
	for _, v := range [][5]uint64{{0, 0, 255, 0, 0}, {50, 0, 0, 255, 0}, {25, 50, 0, 0, 255}} {
		w.poner(0, 8)
		w.vertice8(v[0], v[1], v[2], v[3], v[4])
		w.alinear()
	}
	// Un cuarto vértice con bandera 1 debe formar un segundo triángulo.
	w.poner(1, 8)
	w.vertice8(75, 50, 255, 255, 0)
	w.alinear()

	d, err := LeerPDF(pdfConMalla(t, 4, "", w.bytes))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	st, _ := d.resolver(pdfRef{Num: 5}).(*pdfStream)
	if st == nil {
		t.Fatal("no se encontró el sombreado")
	}
	l := &lienzo{d: d, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}}
	tris, ok := l.trianguloDeMalla(st, 4)
	if !ok {
		t.Fatal("no se decodificó la malla")
	}
	if len(tris) != 2 {
		t.Fatalf("triángulos = %d, quería 2 (uno nuevo + uno encadenado)", len(tris))
	}
	// El encadenado con bandera 1 hereda el 2.º y 3.er vértice del anterior.
	if tris[1].a != tris[0].b || tris[1].b != tris[0].c {
		t.Error("la bandera 1 no encadenó por los vértices que dice la especificación")
	}
}

// TestMallaEnRejilla cubre el tipo 5: sin banderas, por filas.
func TestMallaEnRejilla(t *testing.T) {
	w := &escritorDeBits{}
	// Dos filas de dos vértices = un cuadrilátero = dos triángulos.
	w.vertice8(0, 0, 255, 0, 0)
	w.vertice8(100, 0, 0, 255, 0)
	w.vertice8(0, 100, 0, 0, 255)
	w.vertice8(100, 100, 255, 255, 0)

	d, err := LeerPDF(pdfConMalla(t, 5, "/VerticesPerRow 2", w.bytes))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	st, _ := d.resolver(pdfRef{Num: 5}).(*pdfStream)
	l := &lienzo{d: d, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}}
	tris, ok := l.trianguloDeMalla(st, 5)
	if !ok || len(tris) != 2 {
		t.Fatalf("triángulos = %d, quería 2", len(tris))
	}
}

// TestParcheCoonsSigueSusCurvas cubre el tipo 6. Un parche son cuatro curvas de
// Bézier: si se ignoraran los puntos de control, el degradado saldría recortado
// en recto en vez de seguir el borde.
func TestParcheCoonsSigueSusCurvas(t *testing.T) {
	w := &escritorDeBits{}
	w.poner(0, 8) // bandera 0: parche completo
	// Doce puntos de control dando la vuelta a un cuadrado con los lados
	// abombados hacia fuera.
	for _, p := range [][2]uint64{
		{0, 0},
		{0, 40},
		{0, 60},
		{0, 100},
		{40, 130},
		{60, 130},
		{100, 100},
		{130, 60},
		{130, 40},
		{100, 0},
		{60, -0 + 0},
		{40, 0},
	} {
		w.poner(p[0], 8)
		w.poner(p[1], 8)
	}
	for _, c := range [][3]uint64{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 0}} {
		w.poner(c[0], 8)
		w.poner(c[1], 8)
		w.poner(c[2], 8)
	}
	w.alinear()

	svg := renderizar(t, pdfConMalla(t, 6, "", w.bytes))
	if strings.Count(svg, "<path") < 50 {
		t.Fatalf("el parche no se trianguló en rejilla: %d caminos", strings.Count(svg, "<path"))
	}
	if colores := coloresDistintos(svg); colores < 8 {
		t.Errorf("solo %d colores: el parche salió plano", colores)
	}
}

// TestParcheTensorLeeSusDieciseisPuntos: el tipo 7 trae cuatro puntos interiores
// más. Si no se leyeran, el flujo se desalinearía y el parche siguiente saldría
// en cualquier sitio.
func TestParcheTensorLeeSusDieciseisPuntos(t *testing.T) {
	w := &escritorDeBits{}
	w.poner(0, 8)
	for i := 0; i < 16; i++ {
		w.poner(uint64(i*6), 8)
		w.poner(uint64(i*5), 8)
	}
	for i := 0; i < 4; i++ {
		w.poner(uint64(60*i), 8)
		w.poner(0, 8)
		w.poner(uint64(255-60*i), 8)
	}
	w.alinear()

	d, err := LeerPDF(pdfConMalla(t, 7, "", w.bytes))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	st, _ := d.resolver(pdfRef{Num: 5}).(*pdfStream)
	l := &lienzo{d: d, fuentes: map[string]*fuentePDF{}, caraCSS: map[string]bool{}}
	tris, ok := l.parchesDeMalla(st, 7)
	if !ok {
		t.Fatal("no se decodificó el parche tensor")
	}
	if len(tris) != pasosDelParche*pasosDelParche*2 {
		t.Errorf("triángulos = %d, quería %d", len(tris), pasosDelParche*pasosDelParche*2)
	}
}

// TestMallaComoRellenoSeEnvuelve: un patrón de sombreado necesita un servidor de
// pintura, y las figuras no lo son. Sin envolverlas en `<pattern>`, el relleno se
// quedaría sin nada.
func TestMallaComoRellenoSeEnvuelve(t *testing.T) {
	w := &escritorDeBits{}
	for _, v := range [][5]uint64{{0, 0, 255, 0, 0}, {100, 0, 0, 255, 0}, {50, 100, 0, 0, 255}} {
		w.poner(0, 8)
		w.vertice8(v[0], v[1], v[2], v[3], v[4])
		w.alinear()
	}
	svg := renderizar(t, pdfConContenido(t,
		"/Pattern cs /P0 scn 0 0 100 100 re f",
		`/Resources << /Pattern << /P0 5 0 R >> >>`,
		`<< /PatternType 2 /Shading 6 0 R >>`,
		fmt.Sprintf("<< /ShadingType 4 /ColorSpace /DeviceRGB "+
			"/BitsPerCoordinate 8 /BitsPerComponent 8 /BitsPerFlag 8 "+
			"/Decode %s /Length %d >>\nstream\n%s\nendstream",
			decodeMalla8, len(w.bytes), string(w.bytes))))

	id := regexp.MustCompile(`<pattern id="([^"]+)"`).FindStringSubmatch(svg)
	if len(id) != 2 || !strings.Contains(svg, `fill="url(#`+id[1]+`)"`) {
		t.Fatalf("el relleno no apunta a la malla envuelta: %s", primerasLineas(svg))
	}
	if !strings.Contains(svg, "<pattern ") {
		t.Errorf("falta el <pattern> que hace de servidor de pintura")
	}
}

// TestMallaNoSeAtascaConBasura: el flujo de una malla es binario y adverso.
func TestMallaNoSeAtascaConBasura(t *testing.T) {
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		for _, tipo := range []int{4, 5, 6, 7} {
			for _, flujo := range [][]byte{
				{},
				{0},
				{0xFF},
				make([]byte, 300),
				[]byte("esto no es una malla ni de lejos"),
			} {
				extra := ""
				if tipo == 5 {
					extra = "/VerticesPerRow 2"
				}
				svg := renderizar(t, pdfConMalla(t, tipo, extra, flujo))
				if !strings.HasSuffix(svg, "</svg>") {
					t.Errorf("tipo %d con %d bytes: SVG mal cerrado", tipo, len(flujo))
				}
			}
		}
	}()
	select {
	case <-hecho:
	case <-timeoutCorto():
		t.Fatal("la decodificación de una malla se colgó")
	}
}

// coloresDistintos cuenta los `fill="#rrggbb"` únicos del SVG, que es la forma
// barata de distinguir un degradado de un bloque plano.
func coloresDistintos(svg string) int {
	vistos := map[string]bool{}
	for _, trozo := range strings.Split(svg, `fill="#`)[1:] {
		if len(trozo) >= 6 {
			vistos[trozo[:6]] = true
		}
	}
	return len(vistos)
}
