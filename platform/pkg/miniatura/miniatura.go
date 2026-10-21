// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package miniatura reduce una imagen ráster a una miniatura codificada, con la
// guarda anti bomba de descompresión incluida.
//
// POR QUÉ ESTÁ AQUÍ Y NO EN CADA SERVICIO. Lo escribió Drive para las vistas
// previas de ficheros; Rooms necesita exactamente lo mismo para proxiar la
// imagen de una vista previa de enlace, y esa imagen viene de un host
// cualquiera de internet, o sea del peor origen posible. Copiar la función
// habría significado copiar TAMBIÉN la barrera anti-bomba, y una barrera
// duplicada es una barrera que un día se arregla en un sitio y no en el otro.
//
// Una sola dependencia, `golang.org/x/image/draw`, y entró con medida delante:
// el filtro de caja que había aquí tardaba 283 ms en bajar una foto de
// 4000×3000 a 512 px porque llamaba a `src.At()` por píxel —despacho de interfaz
// y conversión de color, doce millones de veces—. Con x/image, el mismo trabajo
// con MEJOR filtro cuesta 170 ms. Misma procedencia que `x/crypto`, que ya
// estaba en el módulo.
package miniatura

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // registra el decodificador GIF en image.Decode/DecodeConfig
	"image/jpeg"
	"image/png"
	"sort"

	xdraw "golang.org/x/image/draw"
)

// ErrMedioNoSoportado se devuelve cuando el origen no se puede decodificar, no
// es una imagen ráster conocida o declara unas dimensiones imposibles. Es un
// error de ENTRADA, nunca un fallo del servidor: quien lo reciba debe responder
// 415 o servir el hueco, no 500.
var ErrMedioNoSoportado = errors.New("miniatura: medio no soportado")

const (
	// MaxPixeles acota el ancho×alto DECLARADO en la cabecera del fichero. Es la
	// barrera anti bomba de descompresión: un PNG de 30 KB puede declarar
	// 50.000×50.000 y reventar la memoria del proceso al decodificarlo. Se
	// comprueba ANTES de decodificar, leyendo solo la cabecera.
	MaxPixeles = 40_000_000
	// MaxBytes acota la memoria que cuesta DECODIFICAR y reducir la imagen, que
	// depende del modelo de color y no solo de los píxeles (FILE-07). Medido en el
	// frontal (12 MP, 2026-10-15): decodificar cuesta 1,5 B/px un JPEG 4:2:0, 1 un
	// PNG gris, 4 uno RGBA y 8 uno de 16 bits; reducir con Render añade ~4,3 B/px.
	// Con solo MaxPixeles, un PNG de 16 bits de 40 MP —648 KB en disco— pedía
	// ~490 MB, más que el pod de Drive y el doble que el de Rooms. 192 MiB deja
	// pasar un JPEG de 32 MP o un PNG RGBA de 20 MP, de uno en uno (ver Turno).
	MaxBytes = 192 << 20
	// CalidadJPEG para las miniaturas de fotos: pequeña y nítida.
	CalidadJPEG = 82
)

// bytesPorPixel es lo que cuesta cada píxel ya decodificado, según el tipo de
// imagen que construye el decodificador para ese modelo de color.
func bytesPorPixel(m color.Model) float64 {
	switch m {
	case color.GrayModel, color.AlphaModel:
		return 1
	case color.Gray16Model, color.Alpha16Model:
		return 2
	case color.YCbCrModel, color.NYCbCrAModel:
		return 3 // el peor submuestreo (4:4:4); 4:2:0 cuesta la mitad
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	}
	if _, ok := m.(color.Palette); ok {
		return 1
	}
	return 4 // RGBA, NRGBA, CMYK
}

// Coste es la memoria que pide decodificar una imagen con esa cabecera y
// reducirla: los píxeles en su modelo de color más el lienzo de trabajo.
func Coste(cfg image.Config) int64 {
	return int64(float64(int64(cfg.Width)*int64(cfg.Height)) * (bytesPorPixel(cfg.ColorModel) + 4.5))
}

// Cabe dice si una imagen con esa cabecera se puede decodificar sin poner en
// peligro el proceso: dimensiones posibles, dentro de MaxPixeles y de MaxBytes.
func Cabe(cfg image.Config) bool {
	return cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= MaxPixeles && Coste(cfg) <= MaxBytes
}

// turno deja decodificar una imagen a la vez en cada proceso: MaxBytes es el
// coste de UNA, y dos o tres miniaturas a la vez lo multiplicaban.
var turno = make(chan struct{}, 1)

// Turno espera a que no haya otra imagen decodificándose y devuelve con qué
// soltar el turno. Lo usa Render; quien decodifique por su cuenta (Photos)
// lo toma alrededor de todo el trabajo con la imagen.
func Turno() func() {
	turno <- struct{}{}
	return func() { <-turno }
}

// Decodificar es DecodeConfig, Cabe y Decode, en ese orden: la cabecera decide
// si se gasta la memoria antes de gastarla.
func Decodificar(datos []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || !Cabe(cfg) {
		return nil, ErrMedioNoSoportado
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil, ErrMedioNoSoportado
	}
	return img, nil
}

// Render decodifica `datos` y devuelve una miniatura con lado máximo `maxLado`,
// codificada en `tipoSalida` ("image/png" o, por defecto, "image/jpeg").
//
// El orden importa y es la razón de ser de esta función: primero DecodeConfig
// —que solo lee la cabecera—, luego el rechazo por tamaño, y solo entonces el
// decodificado completo. Invertirlo deja el proceso a merced del fichero.
func Render(datos []byte, maxLado int, tipoSalida string) ([]byte, error) {
	liberar := Turno()
	defer liberar()
	img, err := Decodificar(datos)
	if err != nil {
		return nil, err
	}
	thumb := Reducir(img, maxLado)

	var buf bytes.Buffer
	switch tipoSalida {
	case "image/png":
		if err := png.Encode(&buf, thumb); err != nil {
			return nil, fmt.Errorf("miniatura: codificar png: %w", err)
		}
	default: // image/jpeg
		if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: CalidadJPEG}); err != nil {
			return nil, fmt.Errorf("miniatura: codificar jpeg: %w", err)
		}
	}
	return buf.Bytes(), nil
}

// Reducir baja `src` a un lado máximo de `maxLado` manteniendo el aspecto y sin
// AMPLIAR nunca. Trabaja en NRGBA (alfa recto) para evitar la ambigüedad del
// premultiplicado.
//
// CATMULLROM Y NO ApproxBiLinear, aunque este último sea treinta veces más
// rápido: lo es porque NO HACE EL TRABAJO. Muestrea 2×2 píxeles de origen por
// píxel de destino, así que en una reducción de 7,8× ignora la mayoría de la
// imagen y el resultado tiene aliasing visible. La velocidad de un filtro que
// no filtra no es una ventaja.
func Reducir(src image.Image, maxLado int) *image.NRGBA {
	return escalar(src, maxLado, xdraw.CatmullRom)
}

// escalar es el motor común de Reducir y Cascada.
func escalar(src image.Image, maxLado int, k xdraw.Interpolator) *image.NRGBA {
	b := src.Bounds()
	dw, dh := Dimensiones(b.Dx(), b.Dy(), maxLado)
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	k.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// Cascada reduce a VARIOS tamaños de una sola imagen, cada uno a partir del
// anterior en vez de a partir del original.
//
// POR QUÉ EN CASCADA, medido sobre una foto de 4000×3000 y los tres tamaños de
// una galería (3200, 2048, 512):
//
//	tres veces desde el original ......... 852 ms
//	en cascada ........................... 602 ms
//	en cascada, filtro corto en el salto
//	    que casi no reduce ............... 509 ms
//
// La razón es que el coste manda el TAMAÑO DE ORIGEN, no el de destino: reducir
// 2048→512 mira cuatro veces menos píxeles que 4000→512, y el resultado es el
// mismo porque la información que se pierde ya se había perdido.
//
// El primer salto suele reducir poco (4000→3200 es 1,25×), y a esa proporción
// un filtro bilineal no introduce aliasing apreciable: ahí se gana tiempo sin
// pagarlo en calidad. Los saltos que reducen de verdad van con CatmullRom.
//
// `lados` se ordena de mayor a menor: pasarlos al revés reduciría y volvería a
// ampliar, que es la forma de destrozar una imagen sin darse cuenta.
func Cascada(src image.Image, lados []int) map[int]*image.NRGBA {
	ordenados := append([]int(nil), lados...)
	sort.Sort(sort.Reverse(sort.IntSlice(ordenados)))

	out := make(map[int]*image.NRGBA, len(ordenados))
	actual := src
	anchoActual := src.Bounds().Dx()
	altoActual := src.Bounds().Dy()
	for _, lado := range ordenados {
		mayor := anchoActual
		if altoActual > mayor {
			mayor = altoActual
		}
		// Umbral 2×: por debajo de esa proporción el bilineal ya mira suficiente
		// vecindario. Por encima, empieza a saltarse píxeles y aparece el
		// aliasing que CatmullRom evita.
		k := xdraw.Interpolator(xdraw.CatmullRom)
		if mayor < lado*2 {
			k = xdraw.BiLinear
		}
		img := escalar(actual, lado, k)
		out[lado] = img
		actual = img
		anchoActual, altoActual = img.Bounds().Dx(), img.Bounds().Dy()
	}
	return out
}

// Dimensiones calcula el destino con lado máximo `maxLado`, manteniendo el
// aspecto y sin ampliar: una imagen ya pequeña conserva su tamaño. Cada lado es
// como mínimo 1 px.
func Dimensiones(sw, sh, maxLado int) (int, int) {
	if sw <= maxLado && sh <= maxLado {
		return sw, sh
	}
	if sw >= sh {
		return maxLado, max(1, sh*maxLado/sw)
	}
	return max(1, sw*maxLado/sh), maxLado
}
