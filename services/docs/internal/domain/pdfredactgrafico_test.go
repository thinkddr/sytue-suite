// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Censura de IMÁGENES y VECTORES: la prueba que importa es que el contenido
// original ya NO ESTÁ en el fichero.
//
// Igual que con el texto (ver pdfredact_test.go), cada caso mira DOS cosas: lo que
// ve quien abre el PDF y lo que ve quien lo abre con un editor hexadecimal. La
// segunda es la que distingue censurar de tapar, y aquí se hace sobre TODO el
// contenido del fichero: los bytes tal cual Y los de cada flujo descomprimido,
// porque un PDF casi siempre comprime lo que importa y un `grep` sobre el fichero
// crudo no encontraría nada aunque estuviera todo.
//
// Los PDF se arman a mano, objeto a objeto, con píxeles y coordenadas
// reconocibles a propósito (una secuencia de bytes que no se parece a ninguna
// otra, un número con decimales raros), para poder buscarlos después sin falsos
// positivos.
package domain

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// flujoPDF escribe un objeto con cuerpo. Los datos pueden ser binarios.
func flujoPDF(dict string, datos []byte) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(datos), datos)
}

// pdfDe200 arma un PDF de una página de 200×200 con el contenido, los recursos y
// los objetos extra (que empiezan en el 5) dados.
func pdfDe200(t *testing.T, contenido, recursos, extraPagina string, extra ...string) []byte {
	t.Helper()
	objs := append([]string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R ` +
			`/Resources << ` + recursos + ` >> ` + extraPagina + ` >>`,
		flujoPDF("", []byte(contenido)),
	}, extra...)
	return pdfMinimo(t, fmt.Sprintf("<< /Size %d /Root 1 0 R >>", len(objs)+1), objs)
}

// pixelesRGB8 devuelve una imagen RGB de 8×8 en la que ningún píxel se parece a otro.
func pixelesRGB8() []byte {
	out := make([]byte, 0, 8*8*3)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			out = append(out, byte(0x10+x*8), byte(0x20+y*8), byte(0xE0-x-y))
		}
	}
	return out
}

// todoElFichero junta los bytes del fichero y los de cada flujo descomprimido: es
// lo que habría que mirar para afirmar que algo «no está».
func todoElFichero(t *testing.T, datos []byte) []byte {
	t.Helper()
	todo := append([]byte(nil), datos...)
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("el PDF de salida no abre: %v", err)
	}
	nums := make([]int, 0, len(d.xref))
	for n := range d.xref {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		o, err := d.objeto(n)
		if err != nil {
			continue
		}
		if st, ok := o.(*pdfStream); ok {
			if b, _, err := d.datosDeFlujoHasta(st); err == nil {
				todo = append(todo, '\n')
				todo = append(todo, b...)
			}
		}
	}
	return todo
}

// renderPagina dibuja la página 0 a 72 ppp.
func renderPagina(t *testing.T, datos []byte) image.Image {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("abrir para dibujar: %v", err)
	}
	b, err := d.RenderizarPaginaPNG(0, OpcionesRender{Escala: 1})
	if err != nil {
		t.Fatalf("dibujar: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("PNG ilegible: %v", err)
	}
	return img
}

// imagenesDelFichero devuelve los flujos con /Subtype /Image del PDF, en claro.
func imagenesDelFichero(t *testing.T, datos []byte) []struct {
	dict  pdfDict
	datos []byte
} {
	t.Helper()
	d, err := LeerPDF(datos)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	nums := make([]int, 0, len(d.xref))
	for n := range d.xref {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	var out []struct {
		dict  pdfDict
		datos []byte
	}
	for _, n := range nums {
		o, err := d.objeto(n)
		if err != nil {
			continue
		}
		st, ok := o.(*pdfStream)
		if !ok {
			continue
		}
		if sub, _ := d.resolver(st.Dict["Subtype"]).(pdfName); sub != "Image" {
			continue
		}
		b, _, _ := d.datosDeFlujoHasta(st)
		out = append(out, struct {
			dict  pdfDict
			datos []byte
		}{st.Dict, b})
	}
	return out
}

func redactarOK(t *testing.T, pdf []byte, areas ...AreaRedaccion) []byte {
	t.Helper()
	doc, err := LeerPDF(pdf)
	if err != nil {
		t.Fatalf("abrir el original: %v", err)
	}
	out, err := RedactarPDF(doc, areas)
	if err != nil {
		t.Fatalf("RedactarPDF: %v", err)
	}
	return out
}

// --- Imágenes ------------------------------------------------------------------

const recursosConImagen = `/XObject << /Im0 5 0 R >>`

func pdfConImagen(t *testing.T, imagenExtra string) []byte {
	t.Helper()
	img := flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB `+
		`/BitsPerComponent 8 `+imagenExtra, pixelesRGB8())
	return pdfDe200(t, "q 80 0 0 80 20 100 cm /Im0 Do Q\n", recursosConImagen, "", img)
}

// zonaDeNueve cubre exactamente los píxeles (0..2, 0..2) de la imagen de arriba:
// cada píxel mide 10×10 pt y la fila 0 es la de más arriba (y 170..180).
var zonaDeNueve = AreaRedaccion{Pagina: 0, X1: 20, Y1: 150, X2: 50, Y2: 180}

func TestRedactarImagenReescribeLosPixelesDeLaZona(t *testing.T) {
	orig := pdfConImagen(t, "")
	out := redactarOK(t, orig, zonaDeNueve)

	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 {
		t.Fatalf("imágenes en la salida = %d, quería 1 (la original NO debe viajar)", len(imgs))
	}
	got, want := imgs[0].datos, pixelesRGB8()
	if len(got) != len(want) {
		t.Fatalf("la imagen nueva mide %d bytes, quería %d", len(got), len(want))
	}
	dentro := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			o := (y*8 + x) * 3
			px := got[o : o+3]
			if x <= 2 && y <= 2 {
				dentro++
				if px[0] != 0 || px[1] != 0 || px[2] != 0 {
					t.Errorf("píxel (%d,%d) de la zona = %v, quería negro", x, y, px)
				}
				continue
			}
			if !bytes.Equal(px, want[o:o+3]) {
				t.Errorf("píxel (%d,%d) de FUERA de la zona cambió: %v → %v", x, y, want[o:o+3], px)
			}
		}
	}
	if dentro != 9 {
		t.Fatalf("la prueba está mal armada: %d píxeles dentro", dentro)
	}

	// La garantía: la fila de arriba de la zona —nueve bytes que no se parecen a
	// nada— no está en ninguna parte del fichero, ni cruda ni descomprimida.
	todo := todoElFichero(t, out)
	if bytes.Contains(todo, want[:9]) {
		t.Error("los píxeles originales de la zona SIGUEN en el fichero")
	}
	if bytes.Contains(todo, want[3*8*2:3*8*2+9]) {
		t.Error("los píxeles originales de la tercera fila de la zona SIGUEN en el fichero")
	}
	// Y como control, lo que quedó fuera sí está (si no, el test buscaría en vano).
	if !bytes.Contains(todo, want[9:18]) {
		t.Error("el control falla: los píxeles de fuera de la zona deberían seguir en el fichero")
	}
}

func TestRedactarImagenDejaLoDeFueraComoEstaba(t *testing.T) {
	orig := pdfConImagen(t, "")
	out := redactarOK(t, orig, zonaDeNueve)
	a, b := renderPagina(t, orig), renderPagina(t, out)
	// El render interpola con un núcleo de dos píxeles de imagen (10 pt cada uno).
	zona := image.Rect(20-25, 200-180-25, 50+25, 200-150+25)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			if (image.Point{x, y}).In(zona) {
				continue
			}
			if a.At(x, y) != b.At(x, y) {
				t.Fatalf("el pixel (%d,%d) de fuera de la zona cambió: %v → %v", x, y, a.At(x, y), b.At(x, y))
			}
		}
	}
	// Dentro, el recuadro negro.
	for _, p := range []image.Point{{25, 25}, {45, 45}, {35, 35}} {
		if c := color.NRGBAModel.Convert(b.At(p.X, p.Y)).(color.NRGBA); c.R+c.G+c.B != 0 {
			t.Errorf("el pixel (%d,%d) de la zona no es negro: %v", p.X, p.Y, c)
		}
	}
}

func TestRedactarImagenSinTocarLaZonaNoLaReescribe(t *testing.T) {
	// Una zona lejos de la imagen: la imagen original viaja intacta, sin
	// recodificar (ni un píxel ni un byte de más).
	orig := pdfConImagen(t, "")
	out := redactarOK(t, orig, AreaRedaccion{Pagina: 0, X1: 150, Y1: 10, X2: 190, Y2: 50})
	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 || !bytes.Equal(imgs[0].datos, pixelesRGB8()) {
		t.Fatalf("una imagen fuera de la zona no debe cambiar (imágenes: %d)", len(imgs))
	}
}

func TestRedactarImagenCompartidaConOtraPaginaNoLaTocaAhi(t *testing.T) {
	img := flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8`,
		pixelesRGB8())
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 5 0 R /Resources << /XObject << /Im0 6 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 5 0 R /Resources << /XObject << /Im0 6 0 R >> >> >>`,
		flujoPDF("", []byte("q 80 0 0 80 20 100 cm /Im0 Do Q\n")),
		img,
	}
	pdf := pdfMinimo(t, fmt.Sprintf("<< /Size %d /Root 1 0 R >>", len(objs)+1), objs)
	out := redactarOK(t, pdf, zonaDeNueve) // solo la página 1

	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 2 {
		t.Fatalf("debería haber la original (página 2) y la censurada (página 1): %d imágenes", len(imgs))
	}
	var intactas, negras int
	for _, im := range imgs {
		switch {
		case bytes.Equal(im.datos, pixelesRGB8()):
			intactas++
		case bytes.Equal(im.datos[:9], make([]byte, 9)):
			negras++
		}
	}
	if intactas != 1 || negras != 1 {
		t.Errorf("intactas=%d censuradas=%d; quería una de cada", intactas, negras)
	}
	// La página 2 se ve exactamente igual que antes.
	d0, _ := LeerPDF(pdf)
	d1, _ := LeerPDF(out)
	a, err := d0.RenderizarPaginaPNG(1, OpcionesRender{Escala: 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := d1.RenderizarPaginaPNG(1, OpcionesRender{Escala: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("la página 2, que no se censuró, cambió al censurar la 1")
	}
}

func TestRedactarImagenConMascaraSuaveTambienSeBorra(t *testing.T) {
	// La máscara suave puede ser una silueta con información: el alfa de la zona
	// también se reescribe (a opaco).
	alfa := make([]byte, 64)
	for i := range alfa {
		alfa[i] = byte(0x40 + i) // 0x40..0x7F: distinguible del 255 que queda
	}
	orig := pdfDe200(t, "q 80 0 0 80 20 100 cm /Im0 Do Q\n", recursosConImagen, "",
		flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB `+
			`/BitsPerComponent 8 /SMask 6 0 R`, pixelesRGB8()),
		flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceGray /BitsPerComponent 8`, alfa))
	out := redactarOK(t, orig, zonaDeNueve)

	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 2 {
		t.Fatalf("imágenes = %d, quería la imagen y su máscara", len(imgs))
	}
	for _, im := range imgs {
		if len(im.datos) != 64 {
			continue
		}
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				v, want := im.datos[y*8+x], alfa[y*8+x]
				if x <= 2 && y <= 2 {
					want = 255
				}
				if v != want {
					t.Errorf("alfa (%d,%d) = %d, quería %d", x, y, v, want)
				}
			}
		}
	}
	todo := todoElFichero(t, out)
	if bytes.Contains(todo, alfa[:3]) {
		t.Error("el alfa original de la zona SIGUE en el fichero")
	}
}

func TestRedactarImagenJPEG(t *testing.T) {
	const n = 64
	src := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: byte(x * 4), G: byte(y * 4), B: 200, A: 255})
		}
	}
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	orig := pdfDe200(t, "q 128 0 0 128 20 20 cm /Im0 Do Q\n", recursosConImagen, "",
		flujoPDF(fmt.Sprintf(`/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB `+
			`/BitsPerComponent 8 /Filter /DCTDecode`, n, n), jb.Bytes()))
	// Mitad izquierda de la imagen (x 20..84), mitad superior (y 84..148).
	out := redactarOK(t, orig, AreaRedaccion{Pagina: 0, X1: 20, Y1: 84, X2: 84, Y2: 148})

	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 {
		t.Fatalf("imágenes = %d", len(imgs))
	}
	if f, _ := imgs[0].dict["Filter"].(pdfName); f != "DCTDecode" {
		t.Errorf("una foto debe seguir siendo JPEG (si no, pesa diez veces más): %v", imgs[0].dict["Filter"])
	}
	if bytes.Contains(todoElFichero(t, out), jb.Bytes()[100:400]) {
		t.Error("los bytes del JPEG original SIGUEN en el fichero")
	}
	d, _ := LeerPDF(out)
	var raw []byte
	for n := range d.xref {
		if o, err := d.objeto(n); err == nil {
			if st, ok := o.(*pdfStream); ok {
				if sub, _ := d.resolver(st.Dict["Subtype"]).(pdfName); sub == "Image" {
					raw, _, _ = d.datosDeFlujoHasta(st)
				}
			}
		}
	}
	dec, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("el JPEG de salida no se decodifica: %v", err)
	}
	if r, g, b, _ := dec.At(5, 5).RGBA(); r>>8 > 40 || g>>8 > 40 || b>>8 > 40 {
		t.Errorf("el pixel (5,5), de la zona, no es negro: %d %d %d", r>>8, g>>8, b>>8)
	}
	// Fuera de la zona (cuarto inferior derecho) se conserva lo que había.
	want := src.NRGBAAt(50, 50)
	r, g, b, _ := dec.At(50, 50).RGBA()
	for i, par := range [][2]int{{int(r >> 8), int(want.R)}, {int(g >> 8), int(want.G)}, {int(b >> 8), int(want.B)}} {
		if d := par[0] - par[1]; d > 12 || d < -12 {
			t.Errorf("canal %d del pixel (50,50) de fuera de la zona: %d, había %d", i, par[0], par[1])
		}
	}
}

func TestRedactarImagenMascaraDeEstarcido(t *testing.T) {
	// 16×16, un bit por pixel, todo pintado (0): tras censurar, la zona queda sin
	// pintar (1) y lo demás sigue igual.
	bits := make([]byte, 2*16)
	orig := pdfDe200(t, "q 0 0 1 rg 160 0 0 160 20 20 cm /Im0 Do Q\n", recursosConImagen, "",
		flujoPDF(`/Type /XObject /Subtype /Image /Width 16 /Height 16 /ImageMask true /BitsPerComponent 1`, bits))
	// Pixel = 10 pt; zona = columnas 0..3, filas 0..1 (arriba a la izquierda).
	out := redactarOK(t, orig, AreaRedaccion{Pagina: 0, X1: 20, Y1: 160, X2: 60, Y2: 180})
	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 || imgs[0].dict["ImageMask"] != true {
		t.Fatalf("debería quedar una imagen-máscara: %+v", imgs)
	}
	got := imgs[0].datos
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			bit := got[y*2+x/8] >> uint(7-x%8) & 1
			wantSinPintar := x < 4 && y < 2
			if (bit == 1) != wantSinPintar {
				t.Errorf("bit (%d,%d) = %d, sinPintar esperado %v", x, y, bit, wantSinPintar)
			}
		}
	}
}

func TestRedactarImagenEnLinea(t *testing.T) {
	pix := pixelesRGB8()
	var c bytes.Buffer
	c.WriteString("q 80 0 0 80 20 100 cm\nBI /W 8 /H 8 /CS /RGB /BPC 8 ID ")
	c.Write(pix)
	c.WriteString("\nEI Q\n0 0 1 rg 150 20 20 20 re f\n")
	orig := pdfDe200(t, c.String(), "", "")
	out := redactarOK(t, orig, zonaDeNueve)

	if bytes.Contains(todoElFichero(t, out), pix[:9]) {
		t.Error("los píxeles de la imagen en línea SIGUEN en el fichero")
	}
	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 || !bytes.Equal(imgs[0].datos[:9], make([]byte, 9)) ||
		!bytes.Equal(imgs[0].datos[9:18], pix[9:18]) {
		t.Fatalf("la imagen en línea debe quedar como imagen censurada (imágenes: %d)", len(imgs))
	}
	// El resto del contenido sigue ahí (el rectángulo azul de abajo).
	img := renderPagina(t, out)
	if r, g, b, _ := img.At(160, 200-30).RGBA(); r != 0 || g != 0 || b>>8 < 200 {
		t.Errorf("el resto de la página cambió: %d %d %d", r>>8, g>>8, b>>8)
	}
}

// --- Vectores ------------------------------------------------------------------

const contenidoVectorial = `0.9 g 0 0 200 200 re f
1 0 0 RG 2 w 10 150.123 m 190 150.123 l S
0 0 1 rg 70.777 120.555 20 20 re f
0 1 0 rg 77.31 133.71 m 91.2 140.3 l 85.5 128.9 l f
0 0 0 RG 5 5 m 195 5 l S
`

var zonaVectorial = AreaRedaccion{Pagina: 0, X1: 60, Y1: 110, X2: 120, Y2: 160}

func TestRedactarVectoresQuitaLaGeometriaDeLaZona(t *testing.T) {
	orig := pdfDe200(t, contenidoVectorial, "", "")
	out := redactarOK(t, orig, zonaVectorial)
	todo := todoElFichero(t, out)
	for _, secreto := range []string{"70.777", "120.555", "77.31", "133.71", "91.2", "140.3", "85.5", "128.9"} {
		if bytes.Contains(todo, []byte(secreto)) {
			t.Errorf("la coordenada %s de un vector de la zona SIGUE en el fichero", secreto)
		}
	}
	// La raya roja que cruza la zona no desaparece entera: se corta.
	img := renderPagina(t, out)
	if c := color.NRGBAModel.Convert(img.At(30, 200-150)).(color.NRGBA); c.R < 200 || c.G > 80 {
		t.Errorf("la parte de la raya fuera de la zona (izquierda) se perdió: %+v", c)
	}
	if c := color.NRGBAModel.Convert(img.At(170, 200-150)).(color.NRGBA); c.R < 200 || c.G > 80 {
		t.Errorf("la parte de la raya fuera de la zona (derecha) se perdió: %+v", c)
	}
	// El fondo sigue siendo un fondo, con el agujero negro.
	if c := color.NRGBAModel.Convert(img.At(10, 100)).(color.NRGBA); c.R < 220 || c.R > 240 {
		t.Errorf("el fondo gris se perdió: %+v", c)
	}
	if c := color.NRGBAModel.Convert(img.At(90, 200-135)).(color.NRGBA); c.R+c.G+c.B != 0 {
		t.Errorf("la zona no es negra: %+v", c)
	}
}

func TestRedactarVectoresDejaLoDeFueraIdentico(t *testing.T) {
	orig := pdfDe200(t, contenidoVectorial, "", "")
	out := redactarOK(t, orig, zonaVectorial)
	a, b := renderPagina(t, orig), renderPagina(t, out)
	zona := image.Rect(60-4, 200-160-4, 120+4, 200-110+4)
	diferentes := 0
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			if (image.Point{x, y}).In(zona) {
				continue
			}
			if a.At(x, y) != b.At(x, y) {
				diferentes++
			}
		}
	}
	if diferentes != 0 {
		t.Errorf("%d píxeles de fuera de la zona cambiaron", diferentes)
	}
}

func TestRedactarTrazadoDeRecorteSeConserva(t *testing.T) {
	// Un `W n` no pinta: quitarlo mostraría lo que estaba recortado. Se conserva, y
	// lo que se pinte con él después se sigue viendo recortado.
	c := "q 10 10 180 180 re W n 0 1 0 rg 0 0 200 200 re f Q\n"
	out := redactarOK(t, pdfDe200(t, c, "", ""), AreaRedaccion{Pagina: 0, X1: 80, Y1: 80, X2: 120, Y2: 120})
	img := renderPagina(t, out)
	if c := color.NRGBAModel.Convert(img.At(5, 195)).(color.NRGBA); c.R != 255 || c.G != 255 {
		t.Errorf("fuera del recorte debería verse el blanco del papel: %+v", c)
	}
	if c := color.NRGBAModel.Convert(img.At(30, 170)).(color.NRGBA); c.G < 200 || c.R > 40 {
		t.Errorf("dentro del recorte debería verse el verde: %+v", c)
	}
}

func TestRedactarVectorConCTMGirada(t *testing.T) {
	// El mismo rectángulo con la matriz girada 90°: se censura el sitio correcto.
	c := "q 0 1 -1 0 150 0 cm 0 0 1 rg 20 20 60 60 re f Q\n"
	// En página: x = 150 - y(usuario), y = x(usuario) → rectángulo x 70..130, y 20..80.
	out := redactarOK(t, pdfDe200(t, c, "", ""), AreaRedaccion{Pagina: 0, X1: 70, Y1: 20, X2: 100, Y2: 80})
	todo := todoElFichero(t, out)
	if bytes.Contains(todo, []byte("20 20 60 60 re")) {
		t.Error("el rectángulo original SIGUE en el fichero")
	}
	img := renderPagina(t, out)
	if c := color.NRGBAModel.Convert(img.At(115, 200-50)).(color.NRGBA); c.B < 200 || c.R > 40 {
		t.Errorf("la parte del rectángulo fuera de la zona debería seguir azul: %+v", c)
	}
}

// --- Formularios ---------------------------------------------------------------

func pdfConFormularioXObj(t *testing.T, formExtra string) []byte {
	t.Helper()
	form := flujoPDF(`/Type /XObject /Subtype /Form /BBox [0 0 100 100] `+
		`/Resources << /XObject << /Im1 7 0 R >> /Font << /F1 6 0 R >> >> `+formExtra,
		[]byte("q 50 0 0 50 10 10 cm /Im1 Do Q\n"+
			"BT /F1 12 Tf 10 80 Td (SECRETO) Tj ET\n"+
			"0 0 1 RG 5 60.4242 m 95 60.4242 l S\n"))
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 65 /LastChar 90 /Widths [` +
		strings.Repeat("600 ", 26) + `] >>`
	img := flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8`,
		pixelesRGB8())
	return pdfDe200(t, "q 1 0 0 1 50 50 cm /Fm0 Do Q\n", `/XObject << /Fm0 5 0 R >>`, "", form, fuente, img)
}

func TestRedactarFormularioCensuraSuContenidoEnUnaCopia(t *testing.T) {
	orig := pdfConFormularioXObj(t, "")
	// La zona cubre todo el formulario menos nada: x 50..150, y 50..150.
	out := redactarOK(t, orig, AreaRedaccion{Pagina: 0, X1: 50, Y1: 50, X2: 150, Y2: 150})
	todo := todoElFichero(t, out)
	for _, s := range []string{"SECRETO", "60.4242"} {
		if bytes.Contains(todo, []byte(s)) {
			t.Errorf("%q de dentro del formulario SIGUE en el fichero", s)
		}
	}
	if bytes.Contains(todo, pixelesRGB8()[:9]) {
		t.Error("los píxeles de la imagen dentro del formulario SIGUEN en el fichero")
	}
	d, err := LeerPDF(out)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Pagina(0)
	res, _ := d.resolver(p.dict["Resources"]).(pdfDict)
	xo, _ := d.resolver(res["XObject"]).(pdfDict)
	if _, hay := xo["Fm0"]; hay {
		t.Error("el formulario original sigue en los recursos de la página")
	}
	if len(xo) != 1 {
		t.Errorf("recursos XObject de la página = %d, quería solo la copia", len(xo))
	}
}

func TestRedactarFormularioFueraDeLaZonaViajaIntacto(t *testing.T) {
	orig := pdfConFormularioXObj(t, "")
	out := redactarOK(t, orig, AreaRedaccion{Pagina: 0, X1: 0, Y1: 180, X2: 20, Y2: 200})
	if !bytes.Contains(todoElFichero(t, out), []byte("SECRETO")) {
		t.Error("un formulario que no toca la zona no debe cambiar")
	}
}

func TestRedactarFormularioSinRecursosPropiosSeNiegaSiNoPuedeQuitarElOriginal(t *testing.T) {
	// Sin /Resources propios usa los de la página: ahí se le cambia la imagen y
	// quitar el original podría romper otros usos que no se cuentan. Se niega.
	form := flujoPDF(`/Type /XObject /Subtype /Form /BBox [0 0 100 100]`,
		[]byte("q 50 0 0 50 10 10 cm /Im1 Do Q\n"))
	img := flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8`,
		pixelesRGB8())
	pdf := pdfDe200(t, "q 1 0 0 1 50 50 cm /Fm0 Do Q\n", `/XObject << /Fm0 5 0 R /Im1 6 0 R >>`, "", form, img)
	doc, _ := LeerPDF(pdf)
	out, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 50, Y1: 50, X2: 150, Y2: 150}})
	if err == nil {
		// Si sale bien, el original NO puede seguir en el fichero.
		if bytes.Contains(todoElFichero(t, out), pixelesRGB8()[:9]) {
			t.Fatal("salió sin error pero la imagen original sigue en el fichero")
		}
		return
	}
	if !errors.Is(err, ErrRedaccionNoGarantizada) {
		t.Errorf("el error debería ser ErrRedaccionNoGarantizada: %v", err)
	}
}

// --- Miniatura ------------------------------------------------------------------

func TestRedactarNoDejaLaMiniaturaDeLaPagina(t *testing.T) {
	pdf := pdfDe200(t, "BT 0 0 Td ET\n", "", "/Thumb 5 0 R",
		flujoPDF(`/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`,
			[]byte("MINIATURA-SECRETA")))
	out := redactarOK(t, pdf, AreaRedaccion{Pagina: 0, X1: 0, Y1: 0, X2: 50, Y2: 50})
	if bytes.Contains(todoElFichero(t, out), []byte("MINIATURA-SECRETA")) {
		t.Error("la miniatura de la página, una foto SIN censurar, sigue en el fichero")
	}
}

// --- Límites y entrada hostil ---------------------------------------------------

func TestRedactarImagenGiganteSeNiegaSinGastarMemoria(t *testing.T) {
	pdf := pdfDe200(t, "q 80 0 0 80 20 100 cm /Im0 Do Q\n", recursosConImagen, "",
		flujoPDF(`/Type /XObject /Subtype /Image /Width 60000 /Height 60000 /ColorSpace /DeviceRGB /BitsPerComponent 8`,
			[]byte("x")))
	doc, _ := LeerPDF(pdf)
	inicio := time.Now()
	_, err := RedactarPDF(doc, []AreaRedaccion{zonaDeNueve})
	if !errors.Is(err, ErrRedaccionNoGarantizada) {
		t.Fatalf("una imagen de 3600 Mpx debe fallar con ErrRedaccionNoGarantizada: %v", err)
	}
	if time.Since(inicio) > 2*time.Second {
		t.Errorf("tardó %v en negarse", time.Since(inicio))
	}
}

func TestRedactarPideMuchasZonasSobreUnaImagenGrande(t *testing.T) {
	// Mil zonas sobre una imagen de 16 Mpx serían 16 000 Mpx de trabajo: el
	// presupuesto de píxeles recorridos lo corta.
	pdf := pdfDe200(t, "q 200 0 0 200 0 0 cm /Im0 Do Q\n", recursosConImagen, "",
		flujoPDF(`/Type /XObject /Subtype /Image /Width 4000 /Height 4000 /ColorSpace /DeviceGray /BitsPerComponent 8`,
			[]byte("x")))
	doc, _ := LeerPDF(pdf)
	zonas := make([]AreaRedaccion, 500)
	for i := range zonas {
		zonas[i] = AreaRedaccion{Pagina: 0, X1: 0, Y1: 0, X2: 200, Y2: 200}
	}
	inicio := time.Now()
	_, err := RedactarPDF(doc, zonas)
	if !errors.Is(err, ErrRedaccionNoGarantizada) {
		t.Fatalf("debería negarse con ErrRedaccionNoGarantizada: %v", err)
	}
	if d := time.Since(inicio); d > 20*time.Second {
		t.Errorf("tardó %v en negarse", d)
	}
}

func TestRedactarZonasDeCoordenadasExtremasNoRompenNada(t *testing.T) {
	doc, _ := LeerPDF(pdfConImagen(t, ""))
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: v, Y1: 0, X2: 10, Y2: 10}}); err == nil {
			t.Errorf("una zona con coordenada %v debe fallar", v)
		}
	}
	// Una zona gigante pero finita cubre toda la imagen: todo negro, sin pánico.
	out := redactarOK(t, pdfConImagen(t, ""), AreaRedaccion{Pagina: 0, X1: -1e308, Y1: -1e308, X2: 1e308, Y2: 1e308})
	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 || !bytes.Equal(imgs[0].datos, make([]byte, 8*8*3)) {
		t.Errorf("una zona que lo cubre todo debe dejar la imagen negra (imágenes: %d)", len(imgs))
	}
}

func TestRedactarImagenConMatrizQueNoEsUnNumeroSeQuita(t *testing.T) {
	// 1e999 no cabe en un float64: el CTM sale infinito y la imagen no se dibuja en
	// ninguna parte, pero sus píxeles seguirían en el fichero. Se quita.
	pdf := pdfDe200(t, "q 1e999 0 0 1e999 0 0 cm /Im0 Do Q\n", recursosConImagen, "",
		flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8`,
			pixelesRGB8()))
	out := redactarOK(t, pdf, AreaRedaccion{Pagina: 0, X1: 0, Y1: 0, X2: 10, Y2: 10})
	if bytes.Contains(todoElFichero(t, out), pixelesRGB8()[:9]) {
		t.Error("una imagen con matriz infinita sigue en el fichero")
	}
}

func TestRedactarNoAceptaZonasSinFin(t *testing.T) {
	doc := docDePrueba(t, "hola")
	zonas := make([]AreaRedaccion, maxAreasRedaccion+1)
	for i := range zonas {
		zonas[i] = AreaRedaccion{Pagina: 0, X1: 0, Y1: 0, X2: 1, Y2: 1}
	}
	if _, err := RedactarPDF(doc, zonas); err == nil {
		t.Error("más zonas que el máximo debe fallar")
	}
}

func TestRedactarFormularioQueSeLlamaASiMismoTermina(t *testing.T) {
	form := flujoPDF(`/Type /XObject /Subtype /Form /BBox [0 0 200 200] /Resources << /XObject << /Fm0 5 0 R >> >>`,
		[]byte("/Fm0 Do /Fm0 Do 0 g 10.5 10.5 5 5 re f\n"))
	pdf := pdfDe200(t, "/Fm0 Do\n", `/XObject << /Fm0 5 0 R >>`, "", form)
	doc, _ := LeerPDF(pdf)
	inicio := time.Now()
	_, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 0, X2: 200, Y2: 200}})
	if err == nil || !errors.Is(err, ErrRedaccionNoGarantizada) {
		t.Fatalf("un formulario recursivo debe fallar con ErrRedaccionNoGarantizada: %v", err)
	}
	if time.Since(inicio) > 3*time.Second {
		t.Errorf("la recursión tardó %v", time.Since(inicio))
	}
}

func TestRedactarDegradadoDeMallaSeNiega(t *testing.T) {
	sh := flujoPDF(`/ShadingType 4 /ColorSpace /DeviceRGB /BitsPerCoordinate 8 /BitsPerComponent 8 `+
		`/BitsPerFlag 8 /Decode [0 200 0 200 0 1 0 1 0 1]`, make([]byte, 18))
	pdf := pdfDe200(t, "/Sh0 sh\n", `/Shading << /Sh0 5 0 R >>`, "", sh)
	doc, _ := LeerPDF(pdf)
	if _, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 0, X2: 50, Y2: 50}}); !errors.Is(err, ErrRedaccionNoGarantizada) {
		t.Errorf("una malla de degradado en la página debe negarse: %v", err)
	}
}

func TestRedactarDegradadoAxialNoSeNiega(t *testing.T) {
	pdf := pdfDe200(t, "/Sh0 sh\n", `/Shading << /Sh0 << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 200 0] `+
		`/Function << /FunctionType 2 /Domain [0 1] /C0 [1 0 0] /C1 [0 0 1] /N 1 >> >> >>`, "")
	doc, _ := LeerPDF(pdf)
	if _, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 0, X2: 50, Y2: 50}}); err != nil {
		t.Errorf("un degradado axial no puede esconder datos y no debe negarse: %v", err)
	}
}

func TestRedactarTrazadoGiganteNoGastaMemoriaSinFin(t *testing.T) {
	var c bytes.Buffer
	c.WriteString("0 0 m\n")
	for i := 0; i < 300_000; i++ {
		fmt.Fprintf(&c, "%d %d l\n", i%190, (i/7)%190)
	}
	c.WriteString("S\n")
	doc, _ := LeerPDF(pdfDe200(t, c.String(), "", ""))
	out, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 90, Y1: 90, X2: 110, Y2: 110}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LeerPDF(out); err != nil {
		t.Errorf("la salida no abre: %v", err)
	}
}

// TestRedactarNoSePanicaConUnContenidoMutado muta a ciegas un contenido que usa de
// todo —texto, trazos, imagen en línea, formularios— y exige que la censura
// termine, sin pánico y con un PDF que abre o con error.
func TestRedactarNoSePanicaConUnContenidoMutado(t *testing.T) {
	var base bytes.Buffer
	base.WriteString("q 1 0 0 1 50 50 cm /Fm0 Do Q\n")
	base.WriteString(contenidoVectorial)
	base.WriteString("q 80 0 0 80 20 100 cm BI /W 8 /H 8 /CS /RGB /BPC 8 ID ")
	base.Write(pixelesRGB8())
	base.WriteString("\nEI Q\nBT /F1 12 Tf 10 80 Td (SECRETO) Tj ET\n[ [ [ 1 2 ] ] ] 0 d\n")
	fuente := `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 65 /LastChar 90 /Widths [` +
		strings.Repeat("600 ", 26) + `] >>`
	form := flujoPDF(`/Type /XObject /Subtype /Form /BBox [0 0 100 100] /Resources << /XObject << /Im1 7 0 R >> >>`,
		[]byte("q 50 0 0 50 10 10 cm /Im1 Do Q 5 60.4242 m 95 60.4242 l S\n"))
	img := flujoPDF(`/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8`,
		pixelesRGB8())
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 400; i++ {
		mut := append([]byte(nil), base.Bytes()...)
		for k := 0; k < 1+rng.Intn(6); k++ {
			mut[rng.Intn(len(mut))] = byte(rng.Intn(256))
		}
		pdf := pdfDe200(t, string(mut), `/XObject << /Fm0 5 0 R >> /Font << /F1 6 0 R >>`, "", form, fuente, img)
		doc, err := LeerPDF(pdf)
		if err != nil {
			continue
		}
		hecho := make(chan struct{})
		go func() {
			defer close(hecho)
			out, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 40, Y1: 90, X2: 130, Y2: 170}})
			if err == nil {
				if _, err := LeerPDF(out); err != nil {
					t.Errorf("mutación %d: la salida no abre: %v", i, err)
				}
			}
		}()
		select {
		case <-hecho:
		case <-time.After(20 * time.Second):
			t.Fatalf("mutación %d: la censura no termina", i)
		}
	}
}

// TestMedirRedaccionDeImagenes es una SONDA de coste, no una comprobación: sin
// SYTUE_PDF_MEDIR_REDACCION no hace nada. Mide tiempo y memoria reservada al
// censurar un cuarto de una foto grande, que es lo que decide si los topes de
// píxeles están bien puestos.
//
//	SYTUE_PDF_MEDIR_REDACCION=1 go test ./services/docs/internal/domain -run TestMedirRedaccionDeImagenes -v
func TestMedirRedaccionDeImagenes(t *testing.T) {
	if os.Getenv("SYTUE_PDF_MEDIR_REDACCION") == "" {
		t.Skip("sin SYTUE_PDF_MEDIR_REDACCION: no se mide nada")
	}
	type caso struct {
		nombre string
		w, h   int
		jpeg   bool
		gris   bool
	}
	for _, c := range []caso{
		{"RGB Flate 4000x3000 (12 Mpx)", 4000, 3000, false, false},
		{"JPEG 4000x3000 (12 Mpx)", 4000, 3000, true, false},
		{"gris 4960x3508 (17 Mpx, medio A4 a 600 ppp)", 4960, 3508, false, true},
		{"RGB Flate 6000x4000 (24 Mpx, el tope)", 6000, 4000, false, false},
	} {
		img := image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				v := byte((x*7 ^ y*13) & 0xFF)
				img.SetNRGBA(x, y, color.NRGBA{R: v, G: byte(x), B: byte(y), A: 255})
			}
		}
		var dict string
		var cuerpo []byte
		switch {
		case c.jpeg:
			var jb bytes.Buffer
			_ = jpeg.Encode(&jb, img, &jpeg.Options{Quality: 90})
			cuerpo = jb.Bytes()
			dict = fmt.Sprintf(`/ColorSpace /DeviceRGB /Filter /DCTDecode /Width %d /Height %d`, c.w, c.h)
		default:
			cuerpo = muestrasFlate(img, c.gris)
			cs := "/DeviceRGB"
			if c.gris {
				cs = "/DeviceGray"
			}
			dict = fmt.Sprintf(`/ColorSpace %s /Filter /FlateDecode /Width %d /Height %d`, cs, c.w, c.h)
		}
		pdf := pdfDe200(t, "q 200 0 0 150 0 25 cm /Im0 Do Q\n", recursosConImagen, "",
			flujoPDF(`/Type /XObject /Subtype /Image /BitsPerComponent 8 `+dict, cuerpo))
		doc, err := LeerPDF(pdf)
		if err != nil {
			t.Fatal(err)
		}
		var antes, despues runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&antes)
		inicio := time.Now()
		out, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: 100, X2: 100, Y2: 175}})
		dur := time.Since(inicio)
		runtime.ReadMemStats(&despues)
		fmt.Printf("%-48s entrada %6.1f MiB  salida %6.1f MiB  %6.2f s  reservado %7.0f MiB  err=%v\n",
			c.nombre, float64(len(pdf))/(1<<20), float64(len(out))/(1<<20), dur.Seconds(),
			float64(despues.TotalAlloc-antes.TotalAlloc)/(1<<20), err)
	}
}

// Un fondo dibujado como POLÍGONO (m/l/h, como lo escriben Quartz y Chrome) o una
// curva que asoma a la zona se quitaban enteros: la página se quedaba en blanco
// fuera de la zona. Ahora se recortan, y de dentro de la zona no queda ni un
// vértice.
func TestRedactarRecortaPoligonosYCurvas(t *testing.T) {
	const contenido = `0 0 1 rg 0 0 m 200 0 l 200 200 l 0 200 l h 100 100.4321 m 100.1234 100 l 100 99.98 l h f
1 0 0 RG 3 w 100 180 m 155 180 180 155 180 100 c S
`
	orig := pdfDe200(t, contenido, "", "")
	zona := AreaRedaccion{Pagina: 0, X1: 80, Y1: 80, X2: 120, Y2: 120}
	out := redactarOK(t, orig, zona)
	todo := todoElFichero(t, out)
	for _, secreto := range []string{"100.4321", "100.1234", "99.98"} {
		if bytes.Contains(todo, []byte(secreto)) {
			t.Errorf("el vértice %s, de dentro de la zona, sigue en el fichero", secreto)
		}
	}
	img := renderPagina(t, out)
	for _, p := range []image.Point{{10, 10}, {190, 190}, {100, 200 - 60}, {100, 200 - 140}} {
		if c := color.NRGBAModel.Convert(img.At(p.X, p.Y)).(color.NRGBA); c.B < 200 || c.R > 60 {
			t.Errorf("el fondo azul se perdió en %v: %+v", p, c)
		}
	}
	if c := color.NRGBAModel.Convert(img.At(100, 100)).(color.NRGBA); c.R+c.G+c.B != 0 {
		t.Errorf("la zona no es negra: %+v", c)
	}
	// La curva roja está entera fuera de la zona: tiene que seguir viéndose.
	if c := color.NRGBAModel.Convert(img.At(161, 200-161)).(color.NRGBA); c.R < 200 || c.B > 80 {
		t.Errorf("la curva de fuera de la zona se perdió: %+v", c)
	}
}

// Una imagen en un espacio calibrado (CalRGB, ICCBased) sale de la censura en el
// MISMO espacio: pasarla a DeviceRGB cambiaba el color y, en un PDF/A con
// intención de salida CMYK, rompía la conformidad (6.2.3.3 de PDF/A-1). Y si
// una imagen nueva queda en un espacio de dispositivo que la intención no
// cubre, la comprobación de PDF/A lo ve.
func TestRedactarImagenConservaElEspacioCalibrado(t *testing.T) {
	out := redactarOK(t, pdfConImagen(t, "/ColorSpace [/CalRGB << /WhitePoint [0.9505 1 1.089] >>]"), zonaDeNueve)
	imgs := imagenesDelFichero(t, out)
	if len(imgs) != 1 {
		t.Fatalf("imágenes = %d", len(imgs))
	}
	cs, ok := imgs[0].dict["ColorSpace"].(pdfArray)
	if !ok || len(cs) == 0 || cs[0] != pdfName("CalRGB") {
		t.Errorf("espacio de la imagen censurada = %v, quería el CalRGB del original", imgs[0].dict["ColorSpace"])
	}
	if imagenSinIntencion(pdfName("DeviceRGB"), 4) == "" || imagenSinIntencion(pdfName("DeviceRGB"), 3) != "" {
		t.Error("una imagen DeviceRGB solo cumple con una intención de salida RGB")
	}
}
