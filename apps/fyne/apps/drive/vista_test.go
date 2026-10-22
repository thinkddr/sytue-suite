// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/gen2brain/avif"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaVistaPreviaEsSoloParaLoQueNoTieneApp: el texto (hasta 1 MB), el WebP y el AVIF
// se ven aquí; un PDF, una foto o un documento van a su app, y lo demás —el SVG
// también, desde FYS-07— se descarga.
func TestLaVistaPreviaEsSoloParaLoQueNoTieneApp(t *testing.T) {
	for nombre, quiero := range map[string]string{
		"notas.md": "texto", "main.go": "texto", "foto.webp": "imagen", "logo.svg": "", "foto.avif": "imagen",
		"informe.pdf": "", "playa.jpg": "", "acta.docx": "", "copia.zip": "",
	} {
		if got := TipoVista(Elemento{Nombre: nombre, Tamano: 10}); got != quiero {
			t.Errorf("%s: %q y quiero %q", nombre, got, quiero)
		}
	}
	if TipoVista(Elemento{Nombre: "enorme.log", Tamano: TopeVistaTexto + 1}) != "" {
		t.Error("un texto de más de 1 MB se enseñaría: tiene que descargarse")
	}
}

// TestUnSVGNoSeDescodifica (FYS-07): un SVG de 90 bytes desbordaba la pila de oksvg
// y cerraba Drive. Ya no hay vista previa de SVG: ni el que cerraba la app pasa por el
// rasterizador.
func TestUnSVGNoSeDescodifica(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><rect width="200" height="100" fill="#c00"/></svg>`)
	if _, _, ok := ImagenVista("logo.svg", svg); ok {
		t.Fatal("un SVG tiene vista previa")
	}
}

// TestUnaImagenEnormeNoSeDescodifica (FYS-07): un WebP o un PNG de 49 MP (por encima
// del tope común) no reserva sus píxeles.
func TestUnaImagenEnormeNoSeDescodifica(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 7000, 7000), color.Palette{color.White})
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := ImagenVista("foto.webp", b.Bytes()); ok {
		t.Fatal("una imagen de 49 MP se descodificó")
	}
}

// TestUnAVIFSeVe: un AVIF se descodifica y se enseña con su proporción. El fichero se
// fabrica en el test con el codificador de la misma biblioteca, para no guardar binarios.
func TestUnAVIFSeVe(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var b bytes.Buffer
	if err := avif.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	c, proporcion, ok := ImagenVista("foto.avif", b.Bytes())
	if !ok || c.Image == nil || proporcion != 0.5 {
		t.Fatalf("AVIF: ok=%v proporción=%v", ok, proporcion)
	}
}

// TestElTextoSePartePorLetras es el `whitespace-pre-wrap break-words`: las líneas se
// respetan, y la que no cabe se parte por letras sin perder ninguna.
func TestElTextoSePartePorLetras(t *testing.T) {
	tv := &textoVista{lineas: []string{"hola", "", "0123456789abcdefghij"}}
	avance := fyne.MeasureText("M", tema.TextoSM.Tam, tv.estilo()).Width
	trozos := tv.partir(avance*8 + 1)
	if len(trozos) != 5 || trozos[1] != "" || trozos[2] != "01234567" || trozos[4] != "gh"+"ij" {
		t.Errorf("el reparto es %q", trozos)
	}
}

// TestLaFechaEsLaDeLaLista: el día a dos cifras, como mide producción («06 sept 2026»),
// y no el `dateStyle: 'medium'` («6 sept 2026»).
func TestLaFechaEsLaDeLaLista(t *testing.T) {
	if got := Fecha(traductor(t).Locale(), time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)); got != "06 sept 2026" {
		t.Errorf("la fecha sale «%s»", got)
	}
}
