// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "slices"

// Cuánto ocupará cada calidad, para que quien sube elija sabiéndolo. Es la
// ÚNICA estimación: la piden la web y el escritorio al subir, al subir desde
// Drive y en Gestionar, y sale de las mismas tasas con las que codifica el
// transcodificador (Peldanos).
//
// Es una estimación y lo dice: las tasas son la media que pide el codificador, y
// un vídeo quieto sale por debajo. Si además no se sabe cuánto dura (al subir,
// antes de medir el original), la duración se deduce del tamaño del original con
// una tasa típica de cámara o móvil y `Aproximada` va a true.

// TasaOriginalSupuestaKbps es la tasa de un original típico (vídeo de móvil o de
// cámara a 1080p, H.264): con ella se deduce la duración de un fichero del que
// solo se sabe el tamaño.
const TasaOriginalSupuestaKbps = 8000

// OpcionEstimada es una calidad con lo que ocuparía.
type OpcionEstimada struct {
	Lado int
	// Permitida: el original llega a ella (o aún no se sabe).
	Permitida bool
	// Recomendada: forma parte de la recomendación (todas las permitidas).
	Recomendada bool
	// Bytes es lo que suman su variante HLS y su WebM, audio incluido. 0 si no
	// está permitida.
	Bytes int64
}

// Estimacion es la respuesta entera.
type Estimacion struct {
	Opciones      []OpcionEstimada
	Recomendadas  []int
	BytesOriginal int64
	DuracionMS    int64
	// Aproximada: la duración se dedujo del tamaño, no se midió.
	Aproximada bool
}

// Estimar calcula lo que ocuparía cada calidad de un original de ese tamaño,
// esa duración (nil o 0 si no se sabe) y esas dimensiones (0 si no se saben).
// Siempre lista las tres de la tabla, permitidas o no, para que la interfaz diga
// por qué no se puede elegir 1080p; y la del tamaño propio si el original es más
// pequeño que 360p.
func Estimar(bytesOriginal int64, duracionMS *int64, ancho, alto int) Estimacion {
	e := Estimacion{BytesOriginal: max(bytesOriginal, 0)}
	switch {
	case duracionMS != nil && *duracionMS > 0:
		e.DuracionMS = *duracionMS
	case bytesOriginal > 0:
		e.DuracionMS = bytesOriginal * 8 / TasaOriginalSupuestaKbps
		e.Aproximada = true
	default:
		e.Aproximada = true
	}
	opciones := Opciones(ancho, alto)
	candidatas := lados(Peldanos)
	for _, q := range opciones {
		if !slices.Contains(candidatas, q) {
			candidatas = append(candidatas, q)
		}
	}
	for _, q := range candidatas {
		o := OpcionEstimada{Lado: q, Permitida: slices.Contains(opciones, q)}
		if o.Permitida {
			o.Recomendada = true
			o.Bytes = BytesDe(q, e.DuracionMS)
			e.Recomendadas = append(e.Recomendadas, q)
		}
		e.Opciones = append(e.Opciones, o)
	}
	return e
}

// BytesDe es lo que ocupa una calidad (variante HLS + WebM, con su audio) en un
// vídeo de esa duración.
func BytesDe(lado int, duracionMS int64) int64 {
	p := PeldanoDe(lado)
	kbps := int64(p.HLSKbps + AudioHLSKbps + p.WebMKbps + AudioWebMKbps)
	// kbit/s × ms = bit; entre 8 son bytes.
	return kbps * duracionMS / 8
}
