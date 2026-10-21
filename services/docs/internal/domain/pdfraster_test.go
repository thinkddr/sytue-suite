// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la salida PNG. Lo que hay que demostrar aquí es UNA cosa y es la
// razón de existir del fichero: que el TEXTO sale pintado. Todo lo demás —el
// papel, las rayas, los recuadros— ya salía en SVG; lo que no salía en un cliente
// sin motor de SVG con texto eran las letras.
//
// Se cuentan píxeles, no cadenas: un PNG no se puede comprobar con `Contains`, y
// describir de memoria lo que uno cree que pintó es como se da por bueno un
// lienzo en blanco.
package domain

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// pintados cuenta los píxeles que NO son el blanco del papel.
func pintados(t *testing.T, datos []byte) (int, image.Rectangle) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("el PNG no se decodifica: %v", err)
	}
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0xf000 || g < 0xf000 || bl < 0xf000 {
				n++
			}
		}
	}
	return n, b
}

func rasterizar(t *testing.T, datos []byte, opts OpcionesRender) []byte {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, err := d.RenderizarPaginaPNG(0, opts)
	if err != nil {
		t.Fatalf("RenderizarPaginaPNG: %v", err)
	}
	return salida
}

// TestElPNGDeUnaPaginaSiTraeSuTexto es el guardián del encargo: el SVG de esta
// misma página lleva el texto en un `<text>`, que en Fyne no pinta ni un píxel.
func TestElPNGDeUnaPaginaSiTraeSuTexto(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("Hola mundo", "F1", 24)
	conTexto, _ := pintados(t, rasterizar(t, w.Bytes(), OpcionesRender{}))

	vacio := pdfsimple.Nuevo()
	sinTexto, _ := pintados(t, rasterizar(t, vacio.Bytes(), OpcionesRender{}))

	if sinTexto != 0 {
		t.Fatalf("una página vacía pinta %d píxeles y debería pintar 0", sinTexto)
	}
	// Diez letras a 24 pt son varios cientos de píxeles con tinta; el número
	// exacto depende de la cara de reserva, así que se exige el orden de magnitud.
	if conTexto < 200 {
		t.Fatalf("el texto pintó %d píxeles: eso no es una línea de texto", conTexto)
	}
}

// TestElPNGMideLaPaginaEnPixeles comprueba el encuadre: la escala es la única
// forma que tiene el cliente de pedir nitidez, y equivocarla desplaza todo.
func TestElPNGMideLaPaginaEnPixeles(t *testing.T) {
	doc := pdfConContenido(t, "", "") // MediaBox 100×100
	_, b1 := pintados(t, rasterizar(t, doc, OpcionesRender{Escala: 1}))
	_, b2 := pintados(t, rasterizar(t, doc, OpcionesRender{Escala: 2}))
	if b1.Dx() != 100 || b1.Dy() != 100 {
		t.Errorf("a ×1 la página mide %v y debería medir 100×100", b1.Size())
	}
	if b2.Dx() != 200 || b2.Dy() != 200 {
		t.Errorf("a ×2 la página mide %v y debería medir 200×200", b2.Size())
	}
}

// TestElPNGRellenaYRecorta cubre las dos piezas de las que cuelga todo lo demás:
// el relleno de un camino y el recorte que lo acota.
func TestElPNGRellenaYRecorta(t *testing.T) {
	// Un cuadrado de 40×40 en negro: 1600 píxeles exactos a ×1.
	lleno, _ := pintados(t, rasterizar(t,
		pdfConContenido(t, "0 0 0 rg 10 10 40 40 re f", ""), OpcionesRender{}))
	if lleno < 1500 || lleno > 1700 {
		t.Errorf("un cuadrado de 40×40 pintó %d píxeles, se esperaban ~1600", lleno)
	}

	// El mismo cuadrado recortado a la mitad izquierda: la mitad de píxeles.
	medio, _ := pintados(t, rasterizar(t,
		pdfConContenido(t, "0 0 0 rg 10 10 20 40 re W n 10 10 40 40 re f", ""), OpcionesRender{}))
	if medio < 700 || medio > 900 {
		t.Errorf("recortado pintó %d píxeles, se esperaban ~800", medio)
	}
	if medio >= lleno {
		t.Errorf("el recorte no recortó nada: %d contra %d", medio, lleno)
	}
}

// TestElPNGPintaDesdeLaEsquina: una figura que empieza en (0,0) hace que `vector`
// tome un camino rápido que copia la máscara sin mirar su stride. Con una máscara
// que no era contigua, la figura salía cizallada por toda la anchura de la hoja.
func TestElPNGPintaDesdeLaEsquina(t *testing.T) {
	datos := rasterizar(t, pdfConContenido(t, "0 0 0 rg 0 50 50 50 re f", ""), OpcionesRender{})
	img, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	oscuro := func(x, y int) bool { r, _, _, _ := img.At(x, y).RGBA(); return r < 0x8000 }
	if !oscuro(40, 40) {
		t.Error("el interior del cuadrado de la esquina salió en blanco")
	}
	if oscuro(90, 10) || oscuro(10, 90) {
		t.Error("el cuadrado de la esquina pintó fuera de sí mismo")
	}
}

// TestElPNGTrazaLineas: un trazo no es un relleno, y una tabla sin bordes es
// exactamente el aspecto que tiene un rasterizador al que le falta el trazado.
func TestElPNGTrazaLineas(t *testing.T) {
	n, _ := pintados(t, rasterizar(t,
		pdfConContenido(t, "0 0 0 RG 2 w 10 50 m 90 50 l S", ""), OpcionesRender{}))
	// 80 puntos de largo por 2 de grosor = ~160 píxeles.
	if n < 120 || n > 260 {
		t.Errorf("la línea pintó %d píxeles, se esperaban ~160", n)
	}
}

// TestTieneCmapUnicode distingue las dos familias de tablas `cmap`: por carácter
// y por código. Equivocarse deja sin letras un subconjunto que renumera los
// códigos, que es lo que hacía un formulario de CUPS.
func TestTieneCmapUnicode(t *testing.T) {
	if !tieneCmapUnicode(goregular.TTF) {
		t.Error("Go Regular trae (3,1) y no se reconoce")
	}
	// Una tabla sola, «cmap», con una única subtabla (3,0) de símbolos.
	f := make([]byte, 12+16+4+8)
	f[5] = 1 // numTables
	copy(f[12:], "cmap")
	f[12+8+3] = 28 // offset de la tabla
	f[28+3] = 1    // una subtabla
	f[28+4+1] = 3  // plataforma Windows, codificación 0 = símbolos
	if tieneCmapUnicode(f) {
		t.Error("una (3,0) de símbolos no es una tabla por carácter")
	}
	if tieneCmapMacRoman(f) {
		t.Error("una (3,0) de símbolos no es Macintosh Roman")
	}
	// La (1,0) no es Unicode, pero sfnt la consulta por carácter. Es la que
	// deja Word al incrustar un subconjunto de Calibri en un PDF simple.
	f[28+4], f[28+4+1] = 0, 1
	if !tieneCmapMacRoman(f) {
		t.Error("una (1,0) de Macintosh Roman no se reconoce")
	}
}

// TestElPNGNoSeComeLaMemoria: el tope se comprueba ANTES de reservar nada.
func TestElPNGNoSeComeLaMemoria(t *testing.T) {
	d, err := LeerPDF(pdfConContenido(t, "", ""))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	// 100×100 puntos a ×16 son 1600×1600: cabe. A ×16 sobre una página enorme, no.
	if _, err := d.RenderizarPaginaPNG(0, OpcionesRender{Escala: 16}); err != nil {
		t.Errorf("una página de 1600×1600 debería caber: %v", err)
	}
	if _, err := nuevoRasterizador(5000, 5000, 4); err == nil {
		t.Error("una página de 20000×20000 debería rechazarse antes de reservar")
	}
}
