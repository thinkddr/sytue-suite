// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de comprimir, imágenes↔PDF y recortar. Todos van de ida y vuelta: se
// produce el fichero, se vuelve a abrir y se comprueba que lo que tenía que
// quedar dentro está — que es la única forma de saber que una optimización
// optimizó y no rompió.
package domain

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// pngDePrueba genera un PNG con un degradado, para que comprimir tenga algo que
// hacer y no sea un bloque de color que cabe en nada.
func pngDePrueba(t *testing.T, ancho, alto int, conAlfa bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, ancho, alto))
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			a := byte(255)
			if conAlfa && x < ancho/2 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: byte(x * 255 / max(ancho-1, 1)),
				G: byte(y * 255 / max(alto-1, 1)),
				B: 128, A: a,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func jpegDePrueba(t *testing.T, ancho, alto int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			img.SetRGBA(x, y, color.RGBA{R: byte(x), G: byte(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// --- Imágenes a PDF ----------------------------------------------------------

func TestImagenesAPDFUnaPaginaPorImagen(t *testing.T) {
	salida, err := ImagenesAPDF([]ImagenAPagina{
		{Datos: jpegDePrueba(t, 40, 30), Nombre: "uno.jpg"},
		{Datos: pngDePrueba(t, 20, 60, false), Nombre: "dos.png"},
	})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF generado no abre: %v", err)
	}
	if d.NumPaginas() != 2 {
		t.Fatalf("NumPaginas = %d, quería 2", d.NumPaginas())
	}
	// Cada página mide lo que la imagen a 72 ppp.
	p0, _ := d.Pagina(0)
	if p0.Ancho != 40 || p0.Alto != 30 {
		t.Errorf("página 1 = %.0f×%.0f, quería 40×30", p0.Ancho, p0.Alto)
	}
	p1, _ := d.Pagina(1)
	if p1.Ancho != 20 || p1.Alto != 60 {
		t.Errorf("página 2 = %.0f×%.0f, quería 20×60", p1.Ancho, p1.Alto)
	}
	// Y se tienen que VER: una página con la imagen mal referenciada abre igual.
	svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "<image ") {
		t.Errorf("la imagen no llegó a la página: %s", primerasLineas(svg))
	}
}

// TestJPEGEntraSinRecomprimir comprueba lo que separa esta herramienta de una
// mala: un JPEG se guarda TAL CUAL. Pasarlo por PNG lo engordaría y volver a
// pasarlo por JPEG lo degradaría, y las dos cosas son gratis de evitar.
func TestJPEGEntraSinRecomprimir(t *testing.T) {
	origen := jpegDePrueba(t, 64, 64)
	salida, err := ImagenesAPDF([]ImagenAPagina{{Datos: origen, Nombre: "foto.jpg"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	if !bytes.Contains(salida, origen) {
		t.Error("el JPEG no viajó intacto dentro del PDF")
	}
	if !bytes.Contains(salida, []byte("/DCTDecode")) {
		t.Error("no se declaró como DCTDecode")
	}
}

// TestPNGConTransparenciaConservaElRecorte: sin /SMask, un logotipo con fondo
// recortado sale sobre un rectángulo, que es el fallo más visible de esta
// conversión.
func TestPNGConTransparenciaConservaElRecorte(t *testing.T) {
	salida, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 32, 32, true), Nombre: "logo.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	if !bytes.Contains(salida, []byte("/SMask")) {
		t.Error("se perdió la transparencia del PNG")
	}
}

func TestImagenesAPDFRechazaLoQueNoEsImagen(t *testing.T) {
	_, err := ImagenesAPDF([]ImagenAPagina{{Datos: []byte("esto no es una imagen"), Nombre: "x.txt"}})
	if err == nil {
		t.Error("un fichero que no es imagen debe fallar con un mensaje, no colarse")
	}
	if _, err := ImagenesAPDF(nil); err == nil {
		t.Error("sin imágenes no hay PDF")
	}
}

// --- Comprimir ---------------------------------------------------------------

func TestComprimirRecomprimeLosFlujosSueltos(t *testing.T) {
	// El escritor de la casa emite el contenido SIN comprimir, así que este
	// documento es justo el caso que la compresión sin pérdida resuelve.
	d := docDePrueba(t, strings.Repeat("una linea de texto que se repite mucho. ", 40))
	salida, res, err := ComprimirPDF(d, CompresionSinPerdida)
	if err != nil {
		t.Fatalf("ComprimirPDF: %v", err)
	}
	if res.FlujosRecomprimidos == 0 {
		t.Error("no se recomprimió ningún flujo y había contenido sin comprimir")
	}
	if res.Despues >= res.Antes {
		t.Errorf("no bajó de tamaño: %d → %d", res.Antes, res.Despues)
	}
	// Sin pérdida quiere decir SIN PÉRDIDA: el texto tiene que seguir ahí.
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF comprimido no abre: %v", err)
	}
	texto, err := d2.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "una linea de texto") {
		t.Errorf("comprimir se llevó el contenido: %q", texto)
	}
}

// TestComprimirNoDevuelveAlgoMasGordo es la garantía que hace usable la
// herramienta: sobre un documento ya optimizado, «comprimir» no puede empeorarlo.
func TestComprimirNoDevuelveAlgoMasGordo(t *testing.T) {
	d := docDePrueba(t, "x")
	salida, res, err := ComprimirPDF(d, CompresionEquilibrada)
	if err != nil {
		t.Fatalf("ComprimirPDF: %v", err)
	}
	if len(salida) > res.Antes {
		t.Errorf("el resultado (%d) es mayor que el original (%d)", len(salida), res.Antes)
	}
	if res.Ahorro() < 0 {
		t.Errorf("ahorro negativo: %d %%", res.Ahorro())
	}
}

func TestComprimirRecodificaLasImagenes(t *testing.T) {
	// Un PDF con una imagen PNG grande dentro: al comprimir se recodifica a JPEG.
	origen, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 300, 300, false), Nombre: "f.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	d, err := LeerPDF(origen)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, res, err := ComprimirPDF(d, CompresionEquilibrada)
	if err != nil {
		t.Fatalf("ComprimirPDF: %v", err)
	}
	if res.ImagenesTocadas == 0 {
		t.Fatal("no se recodificó ninguna imagen")
	}
	if res.Despues >= res.Antes {
		t.Errorf("no bajó: %d → %d", res.Antes, res.Despues)
	}
	// Y la página tiene que seguir enseñando la foto.
	d2, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el comprimido no abre: %v", err)
	}
	svg, err := d2.RenderizarPaginaSVG(0, OpcionesRender{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(svg, "<image ") {
		t.Errorf("la imagen desapareció al comprimir: %s", primerasLineas(svg))
	}
}

func TestAhorroRedondeado(t *testing.T) {
	if got := (ResultadoCompresion{Antes: 1000, Despues: 250}).Ahorro(); got != 75 {
		t.Errorf("ahorro = %d, quería 75", got)
	}
	if got := (ResultadoCompresion{}).Ahorro(); got != 0 {
		t.Errorf("sin datos el ahorro es 0, dio %d", got)
	}
}

// --- Recortar ----------------------------------------------------------------

func TestRecortarFijaLaCajaVisible(t *testing.T) {
	salida, err := RecortarPDF(docDePrueba(t, "con margenes"), [4]float64{20, 30, 20, 30})
	if err != nil {
		t.Fatalf("RecortarPDF: %v", err)
	}
	if !bytes.Contains(salida, []byte("/CropBox")) {
		t.Fatal("no se escribió /CropBox")
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el recortado no abre: %v", err)
	}
	// El contenido NO se toca: recortar es decir qué se enseña, no destruir.
	texto, err := d.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "con margenes") {
		t.Errorf("recortar se llevó el texto: %q", texto)
	}
}

func TestRecortarRechazaMargenesImposibles(t *testing.T) {
	d := docDePrueba(t, "x")
	if _, err := RecortarPDF(d, [4]float64{5000, 0, 5000, 0}); err == nil {
		t.Error("unos márgenes que dejan la página sin contenido deben fallar")
	}
	if _, err := RecortarPDF(d, [4]float64{-5, 0, 0, 0}); err == nil {
		t.Error("un margen negativo no tiene sentido")
	}
}
