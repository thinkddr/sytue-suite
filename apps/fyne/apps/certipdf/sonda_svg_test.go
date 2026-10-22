// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"image"
	"testing"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// TestElSVGDeUnaPaginaPierdeSuTexto es LA MEDIDA de la que sale toda la arquitectura de
// este paquete, y es además un aviso: el día que se ponga en ROJO hay que volver aquí.
//
// # Qué se mide, y por qué por este camino
//
// El motor de la casa (`services/docs/internal/domain/pdfrender.go`) entrega cada
// página como SVG con `<text>` y las fuentes incrustadas en `@font-face`, y NO
// rasteriza a propósito (ADR-0033: «el motor de la casa traduce a SVG precisamente para
// que rasterice quien sabe»). En el navegador eso es gratis. En Fyne, TODO el SVG pasa
// por `github.com/fyne-io/oksvg`, que dibuja formas pero no implementa `<text>`.
//
// Se llama a oksvg DIRECTAMENTE y no a través de un `canvas.Image` en una ventana de
// prueba: es el mismo lector que usa Fyne —y el mismo que usa `iconos/pintar.go`—, y
// por ahí la medida no depende ni del tema, ni del fondo de la ventana, ni de cuándo
// Fyne decide materializar el rasterizado. La primera versión de esta sonda sí iba por
// la ventana y dio dos lecturas seguidas SIN SENTIDO —14 400 de 14 400 en los dos casos
// (que es 120×120: contaba el fondo entero), y luego 0 hasta en el control—, que es
// exactamente el modo de fallo de un guardián que no se ha visto en rojo a mano.
//
// # Por qué importa tanto
//
// Si se metiera el SVG de una página en un `canvas.Image` y ya está, una factura se
// vería con sus líneas y sus recuadros y SIN UNA SOLA LETRA, con aspecto de estar bien.
// Es el «sello verde falso» de esta app: peor que no enseñar nada. Por eso [Hoja] no lo
// intenta y dice en pantalla que la página no se puede dibujar aquí.
//
// # Cuándo se relevará
//
// Cuando el servicio ofrezca la página ya rasterizada (un `op: render` con `formato:
// png`), que es una petición al servidor y no una dependencia nueva en el cliente.
func TestElSVGDeUnaPaginaPierdeSuTexto(t *testing.T) {
	const lado = 120

	// Control: una forma que oksvg SÍ sabe. Sin él, un «0 píxeles» no distingue entre
	// «no dibuja texto» y «la sonda no dibuja nada».
	conRect := `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120" viewBox="0 0 120 120">` +
		`<rect x="10" y="10" width="100" height="100" fill="#000000"/></svg>`
	// Lo que manda el motor para una página con texto, reducido a lo esencial.
	conTexto := `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120" viewBox="0 0 120 120">` +
		`<text x="10" y="70" font-family="sans-serif" font-size="48" fill="#000000">Hola</text></svg>`

	rect := pintadosPorOksvg(t, conRect, lado)
	texto := pintadosPorOksvg(t, conTexto, lado)
	t.Logf("MEDIDO con oksvg a %dx%d: <rect> pinta %d píxeles; <text> pinta %d", lado, lado, rect, texto)

	if rect == 0 {
		t.Fatalf("el control no pintó nada: la sonda está rota, no oksvg")
	}
	if texto != 0 {
		t.Fatalf("oksvg YA dibuja <text> (%d píxeles): revisar la decisión de `pagina.go`, "+
			"puede que ahora sí se pueda rasterizar la página en el cliente", texto)
	}
}

// pintadosPorOksvg cuenta los píxeles con algo de tinta que oksvg deja sobre un lienzo
// transparente. Sobre transparente y no sobre un fondo: así «pintado» es simplemente
// «alfa > 0» y no hay que elegir umbral de color.
func pintadosPorOksvg(t *testing.T, svg string, lado int) int {
	t.Helper()
	icono, err := oksvg.ReadIconStream(bytes.NewReader([]byte(svg)))
	if err != nil {
		t.Fatalf("SVG ilegible: %v", err)
	}
	icono.SetTarget(0, 0, float64(lado), float64(lado))
	rgba := image.NewRGBA(image.Rect(0, 0, lado, lado))
	sc := rasterx.NewScannerGV(lado, lado, rgba, rgba.Bounds())
	icono.Draw(rasterx.NewDasher(lado, lado, sc), 1)

	n := 0
	for y := range lado {
		for x := range lado {
			if _, _, _, a := rgba.At(x, y).RGBA(); a > 0 {
				n++
			}
		}
	}
	return n
}
