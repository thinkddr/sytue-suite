// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"slices"
)

// Las calidades de un vídeo: qué alturas (el lado corto, «720» es 720p) se
// generan, en las DOS escaleras a la vez —HLS H.264 para la web y WebM VP9 para
// la app de escritorio—, para que las dos ofrezcan lo mismo.
//
// Quien sube elige cuáles (al menos una) viendo cuánto ocupará cada una; lo
// recomendado es todas las que el original permite, porque así cada espectador
// recibe la que conviene a su pantalla y a su conexión. Nunca se escala hacia
// arriba: 1080p solo si el original llega.

// Peldano es una calidad con sus tasas de bits. Es la ÚNICA tabla de tasas: el
// transcodificador codifica con ella y la estimación de espacio calcula con ella,
// así que lo que se promete al elegir es lo que después se produce.
type Peldano struct {
	// Lado es el lado corto en píxeles: el alto en horizontal, el ancho en vertical.
	Lado int
	// HLSKbps es la tasa del vídeo H.264 (x264 -b:v); el tope y el búfer se
	// derivan de ella (×1,07 y ×1,5), como en la escalera de Apple.
	HLSKbps int
	// WebMKbps es la del vídeo VP9. VP9 da la misma calidad con bastante menos que
	// H.264, de ahí que sea ~0,6 de la de HLS.
	WebMKbps int
}

// Tasas del audio: una pista por variante en HLS (AAC) y en cada WebM (Opus).
const (
	AudioHLSKbps  = 128
	AudioWebMKbps = 96
)

// Peldanos son las tres calidades de la decisión del dueño, de mayor a menor
// (CAST-ENCARGO §7: 1080p 5 Mbps, 720p 2,5, 360p 0,8 en H.264).
var Peldanos = []Peldano{
	{Lado: 1080, HLSKbps: 5000, WebMKbps: 3000},
	{Lado: 720, HLSKbps: 2500, WebMKbps: 1800},
	{Lado: 360, HLSKbps: 800, WebMKbps: 500},
}

// CalidadPorDefecto es la que se sirve como `webm_url` si está: la que ya daba
// Cast antes de la escalera, y la que abre la app de escritorio mientras mide.
const CalidadPorDefecto = 720

// PeldanoDe es el peldaño de esa altura. Una altura que no es de la tabla (la de
// un original más pequeño que 360p) usa las tasas del menor.
func PeldanoDe(lado int) Peldano {
	for _, p := range Peldanos {
		if p.Lado == lado {
			return p
		}
	}
	menor := Peldanos[len(Peldanos)-1]
	return Peldano{Lado: lado, HLSKbps: menor.HLSKbps, WebMKbps: menor.WebMKbps}
}

// Opciones son las calidades que un original de esas dimensiones permite, de
// mayor a menor: los peldaños a los que llega su lado corto. Uno más pequeño que
// el menor sale en un solo peldaño a su tamaño (redondeado a par), porque un
// vídeo siempre tiene algo que reproducir. Sin dimensiones (aún no se ha medido)
// son los tres: el transcodificador recorta después lo que no llegue.
func Opciones(ancho, alto int) []int {
	if ancho <= 0 || alto <= 0 {
		return lados(Peldanos)
	}
	corto := min(ancho, alto)
	var out []int
	for _, p := range Peldanos {
		if p.Lado <= corto {
			out = append(out, p.Lado)
		}
	}
	if len(out) == 0 {
		out = append(out, corto-corto%2)
	}
	return out
}

// OpcionesDe son las del vídeo por sus dimensiones medidas (o las tres si aún no se sabe).
func OpcionesDe(v Video) []int {
	if v.Width == nil || v.Height == nil {
		return Opciones(0, 0)
	}
	return Opciones(int(*v.Width), int(*v.Height))
}

// ValidarCalidades comprueba lo que alguien elige: al menos una, sin repetir, y
// cada una entre las que el original permite (`opciones`). Devuelve la elección
// ordenada de mayor a menor, que es como se guarda.
func ValidarCalidades(elegidas, opciones []int) ([]int, error) {
	if len(elegidas) == 0 {
		return nil, fmt.Errorf("%w: elige al menos una calidad", ErrEntradaInvalida)
	}
	out := make([]int, 0, len(elegidas))
	for _, q := range elegidas {
		if slices.Contains(out, q) {
			return nil, fmt.Errorf("%w: la calidad %dp está repetida", ErrEntradaInvalida, q)
		}
		if !slices.Contains(opciones, q) {
			return nil, fmt.Errorf("%w: el original no da %dp", ErrEntradaInvalida, q)
		}
		out = append(out, q)
	}
	slices.SortFunc(out, func(a, b int) int { return b - a })
	return out, nil
}

// Resolver es lo que de verdad se genera: lo elegido que el original permite, o
// todo lo que permite si no se eligió nada (la recomendación). Si de lo elegido
// no queda nada —se eligió 1080p al subir y el original resultó ser 720p— sale
// la mayor que permite: al menos una, siempre.
func Resolver(elegidas, opciones []int) []int {
	if len(elegidas) == 0 {
		return slices.Clone(opciones)
	}
	var out []int
	for _, q := range opciones {
		if slices.Contains(elegidas, q) {
			out = append(out, q)
		}
	}
	if len(out) == 0 && len(opciones) > 0 {
		out = append(out, opciones[0])
	}
	return out
}

// Elegidas es la elección del vídeo resuelta contra lo que su original permite.
func (v Video) Elegidas() []int { return Resolver(aInts(v.Qualities), OpcionesDe(v)) }

// CalidadesHLS son las variantes del HLS que existen. Un vídeo de antes de la
// elección (hls_qualities NULL) tiene las que su original permitía, que es lo
// que se generaba entonces. La grabación de un directo es el HLS del egress, de
// una sola calidad que no es de la escalera: no cuenta.
func (v Video) CalidadesHLS() []int {
	if v.HlsQualities != nil {
		return aInts(v.HlsQualities)
	}
	if v.HlsKey == "" || v.Kind != KindUpload || v.Width == nil || v.Height == nil {
		return []int{}
	}
	return Opciones(int(*v.Width), int(*v.Height))
}

// CalidadesWebM son los WebM que existen. Uno de antes de la escalera
// (webm_qualities NULL) tiene un solo WebM, `webm/720p.webm`.
func (v Video) CalidadesWebM() []int {
	if v.WebmQualities != nil {
		return aInts(v.WebmQualities)
	}
	if v.WebmKey == "" {
		return []int{}
	}
	return []int{CalidadPorDefecto}
}

// WebMPorDefecto es la calidad que va en `webm_url`: 720p si está, si no la
// mayor por debajo, y si no la menor de las que hay. 0 si no hay WebM.
func WebMPorDefecto(calidades []int) int {
	mejor := 0
	for _, q := range calidades {
		if q <= CalidadPorDefecto && q > mejor {
			mejor = q
		}
	}
	if mejor == 0 && len(calidades) > 0 {
		mejor = slices.Min(calidades)
	}
	return mejor
}

// AInt32 pasa una lista de calidades a lo que guarda la columna (integer[]).
func AInt32(qs []int) []int32 {
	out := make([]int32, len(qs))
	for i, q := range qs {
		out[i] = int32(q) //nolint:gosec // G115: alturas de vídeo, caben de sobra
	}
	return out
}

func aInts(qs []int32) []int {
	out := make([]int, len(qs))
	for i, q := range qs {
		out[i] = int(q)
	}
	return out
}

func lados(ps []Peldano) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Lado
	}
	return out
}
