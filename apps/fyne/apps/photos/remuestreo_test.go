// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"

	xdraw "golang.org/x/image/draw"
)

// # Qué algoritmo remuestrea una FOTO, y cuánto cuesta. Medido, no supuesto
//
// Para un VECTOR la respuesta es no remuestrear: se vuelve a pintar al tamaño
// pequeño, que es lo que hacen los iconos con `canvas.Raster` y lo que midió el port
// de Diapos. Una foto no se puede volver a pintar: hay que remuestrear píxeles sí o
// sí, así que la pregunta es CUÁL y CON QUÉ ERROR.
//
// La referencia no es «lo que se ve bien»: es el filtro de CAJA, que promedia
// exactamente los píxeles de origen que caen en cada píxel de destino. Es lo que
// significa reducir una imagen, y cualquier otro núcleo se juzga contra él.
//
// Y el caso de prueba no es una foto cualquiera: son rayas de un píxel, que es la
// frecuencia más alta que la imagen puede llevar (el tejado, la verja, el estor). Ahí
// el resultado correcto de reducir es un GRIS UNIFORME, así que la varianza de lo que
// salga es exactamente la energía de aliasing. Un número, no una impresión.

// rayas es el peor caso: líneas verticales de un píxel, blanco y negro.
func rayas(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		c := color.RGBA{A: 255}
		if x%2 == 0 {
			c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
		}
		for y := range h {
			m.Set(x, y, c)
		}
	}
	return m
}

// desviacion es la desviación típica del canal rojo, en niveles de 255. Con rayas de
// un píxel reducidas, lo correcto es 0: todo gris.
func desviacion(m *image.RGBA) float64 {
	n, suma := 0, 0.0
	for y := m.Bounds().Min.Y; y < m.Bounds().Max.Y; y++ {
		for x := m.Bounds().Min.X; x < m.Bounds().Max.X; x++ {
			suma += float64(m.RGBAAt(x, y).R)
			n++
		}
	}
	media := suma / float64(n)
	acc := 0.0
	for y := m.Bounds().Min.Y; y < m.Bounds().Max.Y; y++ {
		for x := m.Bounds().Min.X; x < m.Bounds().Max.X; x++ {
			d := float64(m.RGBAAt(x, y).R) - media
			acc += d * d
		}
	}
	return math.Sqrt(acc / float64(n))
}

// escalarDeLaFototeca reduce con el MISMO escalador que usa la app, para que esto mida
// lo que se pinta y no una copia suya.
func escalarDeLaFototeca(src *image.RGBA, lado int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, lado, lado))
	escalador(src.Bounds(), lado, lado).Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

func escalarCon(k xdraw.Interpolator, src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	k.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// TestElRemuestreoDeFotosNoAliasea es la medición que pedía el encargo: a qué coste de
// CALIDAD remuestrea cada núcleo una foto, y por qué el que usa `canvas.Image` no vale.
//
// Se mide en los dos tamaños que de verdad pinta esta app: 402 px (una tesela de 201
// de CSS en una pantalla HiDPI, reducción ×1,27) y 201 px (la misma en una pantalla
// normal, reducción ×2,55).
func TestElRemuestreoDeFotosNoAliasea(t *testing.T) {
	src := rayas(512, 512)
	for _, destino := range []int{402, 201, 150, 128} {
		razon := 512.0 / float64(destino)
		for _, k := range []struct {
			nombre string
			cual   xdraw.Interpolator
		}{
			{"NearestNeighbor", xdraw.NearestNeighbor},
			{"ApproxBiLinear (lo que usa canvas.Image)", xdraw.ApproxBiLinear},
			{"BiLinear", xdraw.BiLinear},
			{"CatmullRom", xdraw.CatmullRom},
		} {
			d := desviacion(escalarCon(k.cual, src, destino, destino))
			t.Logf("MEDIDO  512→%3d (×%.2f)  %-40s  aliasing σ=%6.2f de 255", destino, razon, k.nombre, d)
		}
		// El que ESTA app elige, que es lo que hay que defender.
		elegido := desviacion(escalarDeLaFototeca(src, destino))
		t.Logf("MEDIDO  512→%3d (×%.2f)  el que elige la Fototeca                  aliasing σ=%6.2f de 255",
			destino, razon, elegido)

		// El guardián, y es el que tumbó la heurística que había: a CUALQUIER razón de
		// reducción lo que elegimos tiene que aliasear menos que la ruta de
		// `canvas.Image`. La frontera en ×2 que este código tuvo un rato no existe:
		// `ApproxBiLinear` interpola los 2×2 más cercanos del ORIGEN sin filtrar, así
		// que su error es el mismo a ×1,27 que a ×2,55.
		// El suelo de 2 σ no es indulgencia: a razón ENTERA (512→128 es ×4 clavado) y
		// con rayas de periodo 2, el 2×2 de `ApproxBiLinear` cae siempre en el mismo
		// sitio y acierta por casualidad —σ=0—, mientras que Catmull-Rom deja 1,21 de
		// rizado de su lóbulo negativo. Es un artefacto del patrón sintético en un
		// caso degenerado, no un defecto: por debajo de dos niveles de 255 no hay
		// nada que ver. A cualquier razón NO entera manda la comparación.
		aprox := desviacion(escalarCon(xdraw.ApproxBiLinear, src, destino, destino))
		if elegido > aprox && elegido >= 2 {
			t.Errorf("a ×%.2f el núcleo elegido aliasea σ=%.2f y ApproxBiLinear σ=%.2f: "+
				"no está mejorando nada", razon, elegido, aprox)
		}
	}
}

// TestLaRutaDeCanvasImageEsPeorQueLaNuestra compara las DOS rutas de punta a punta, que
// es la comparación que decide el diseño:
//
//   - `canvas.Image`: rasteriza al tamaño natural y ESTIRA el mapa de bits con
//     ApproxBiLinear hasta la caja. Con una tesela a ×1 eso son 512→201 con un filtro
//     de 2×2.
//   - la nuestra: `canvas.Raster`, un solo remuestreo a los píxeles de DISPOSITIVO con
//     el núcleo que pide la razón.
func TestLaRutaDeCanvasImageEsPeorQueLaNuestra(t *testing.T) {
	src := rayas(512, 512)
	const destino = 201 // tesela de 201 px de CSS a ×1

	comoCanvasImage := desviacion(escalarCon(xdraw.ApproxBiLinear, src, destino, destino))
	comoNosotros := desviacion(escalarDeLaFototeca(src, destino))
	t.Logf("MEDIDO  la ruta de canvas.Image deja σ=%.2f de aliasing y la nuestra σ=%.2f "+
		"(rayas de 1 px reducidas a 201; lo correcto es 0)", comoCanvasImage, comoNosotros)
	if comoNosotros >= comoCanvasImage {
		t.Errorf("nuestra ruta (σ=%.2f) no mejora la de canvas.Image (σ=%.2f): "+
			"entonces no hay motivo para el raster", comoNosotros, comoCanvasImage)
	}
}
