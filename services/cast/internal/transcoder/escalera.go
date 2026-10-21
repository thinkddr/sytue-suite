// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"fmt"
	"strconv"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// Calidad es un peldaño de las dos escaleras: a qué lado corto se escala y con
// qué tasas se codifica en H.264 (HLS) y en VP9 (WebM). Las tasas salen de
// domain.Peldanos, la misma tabla con la que se estima el espacio al elegir.
type Calidad struct {
	// Nombre es la carpeta bajo hls/, el `name:` del var_stream_map y el nombre
	// del WebM («1080p» → hls/1080p/…, webm/1080p.webm).
	Nombre string
	// Lado es el lado corto objetivo en píxeles: el alto en horizontal, el ancho en
	// vertical. Así un vídeo de móvil a 1080×1920 sale «1080p» y no un «1920p»
	// que nadie pidió.
	Lado int
	// Tasa, Maxima y Bufer son los de x264 (-b:v, -maxrate, -bufsize).
	Tasa, Maxima, Bufer string
	// TasaWebM es la de VP9 (-b:v).
	TasaWebM string
}

// calidadDe arma el peldaño de una altura con las tasas de la tabla.
func calidadDe(lado int) Calidad {
	p := domain.PeldanoDe(lado)
	return Calidad{
		Nombre: strconv.Itoa(lado) + "p", Lado: lado,
		Tasa:     kbps(p.HLSKbps),
		Maxima:   kbps(p.HLSKbps * 107 / 100),
		Bufer:    kbps(p.HLSKbps * 3 / 2),
		TasaWebM: kbps(p.WebMKbps),
	}
}

func kbps(n int) string { return strconv.Itoa(n) + "k" }

// escaleraDe son los peldaños de esas alturas, en ese orden.
func escaleraDe(lados []int) []Calidad {
	out := make([]Calidad, 0, len(lados))
	for _, l := range lados {
		out = append(out, calidadDe(l))
	}
	return out
}

// escaleraPara decide qué peldaños produce un original de esas dimensiones de
// presentación con la elección de quien lo subió (nil = la recomendada, todos).
// NUNCA hacia arriba: solo los peldaños que el original alcanza; y si de lo
// elegido no alcanza ninguno, el mayor que alcanza (domain.Resolver). Un
// original más pequeño que el menor sale en un solo peldaño a su tamaño, con la
// tasa del menor: un vídeo siempre tiene algo que reproducir.
func escaleraPara(ancho, alto int, elegidas []int) ([]Calidad, error) {
	if ancho <= 0 || alto <= 0 {
		return nil, fmt.Errorf("%w: %d×%d", ErrSinVideo, ancho, alto)
	}
	return escaleraDe(domain.Resolver(elegidas, domain.Opciones(ancho, alto))), nil
}

// filtroEscala es el filtro de ffmpeg que lleva el lado corto a `lado` sin
// deformar: fija el alto en horizontal o el ancho en vertical y deja el otro en
// `-2`, que ffmpeg calcula por la relación de aspecto y redondea a par.
func filtroEscala(lado int, vertical bool) string {
	if vertical {
		return fmt.Sprintf("scale=%d:-2", lado)
	}
	return fmt.Sprintf("scale=-2:%d", lado)
}

// lados son las alturas de una escalera.
func lados(esc []Calidad) []int {
	out := make([]int, 0, len(esc))
	for _, c := range esc {
		out = append(out, c.Lado)
	}
	return out
}
