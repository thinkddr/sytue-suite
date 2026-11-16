// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"math"
	"strconv"
	"strings"
)

// ColorDeUsuario es un tono de la paleta de colores de usuario: su id estable (el token
// que guarda Mail) y el hex que se guarda y se pinta.
type ColorDeUsuario struct {
	ID    string
	Valor string
}

// Clave es la del catálogo común con el nombre del tono («Rojo», «Gris»…).
func (c ColorDeUsuario) Clave() string { return "userColor." + c.ID }

// NRGBA es el tono listo para pintar.
func (c ColorDeUsuario) NRGBA() color.NRGBA {
	// Con bitSize 8 cada canal cabe en un byte por construcción.
	canal := func(i int) uint8 {
		v, _ := strconv.ParseUint(c.Valor[i:i+2], 16, 8)
		return uint8(v)
	}
	return color.NRGBA{R: canal(1), G: canal(3), B: canal(5), A: 0xff}
}

// PaletaDeUsuario es `PALETA_DE_USUARIO` de `@sytue/config` (paleta-usuario.ts), la única
// que se ofrece al elegir el color de una etiqueta de Mail, un cuaderno o una etiqueta de
// Notes, un calendario o una marca de sensibilidad. Doce tonos con la MISMA luminancia
// relativa: 3:1 contra la hoja en los cuatro modos y rótulo blanco a 4,5:1 (lo mide
// `contraste-paleta.test.ts`; el test de aquí comprueba que el espejo es idéntico).
//
// Es una lista fija y no un token de la hoja porque el valor se GUARDA: no cambia con el
// modo, y quien lo lea desde otro cliente (CalDAV) tiene que ver el mismo hex.
var PaletaDeUsuario = []ColorDeUsuario{
	{"red", "#c44744"},
	{"clay", "#b65623"},
	{"brass", "#946a10"},
	{"sage", "#64793c"},
	{"green", "#2f8147"},
	{"teal", "#098070"},
	{"cyan", "#097d91"},
	{"blue", "#2a72c7"},
	{"indigo", "#6564d6"},
	{"plum", "#9259b8"},
	{"rose", "#b84b7c"},
	{"slate", "#677380"},
}

// pesoLuz es `W_LUZ` de la web: la luz pesa la mitad que el matiz al buscar el más
// cercano, porque los tonos de las paletas de antes eran casi todos más oscuros.
const pesoLuz = 0.5

// TonoDeUsuario es `tonoDeUsuario` de la web: el tono con que se PINTA un color guardado.
// El suyo si es un id o un hex de la paleta; si no, el más cercano en OKLab. El valor
// guardado no se reescribe: solo se pinta así. `false` si no hay color o no se entiende.
func TonoDeUsuario(guardado string) (ColorDeUsuario, bool) {
	if guardado == "" {
		return ColorDeUsuario{}, false
	}
	for _, c := range PaletaDeUsuario {
		if c.ID == guardado {
			return c, true
		}
	}
	hex, ok := hexDeUsuario(guardado)
	if !ok {
		return ColorDeUsuario{}, false
	}
	l, a, b := oklab(hex)
	mejor, distancia := 0, math.Inf(1)
	for i, c := range PaletaDeUsuario {
		pl, pa, pb := oklab(c.Valor)
		dl := pesoLuz * (l - pl)
		d := dl*dl + (a-pa)*(a-pa) + (b-pb)*(b-pb)
		if d < distancia {
			mejor, distancia = i, d
		}
	}
	return PaletaDeUsuario[mejor], true
}

// PintarDeUsuario es el color con que se pinta un color guardado, o `reserva` si no hay
// tono (vacío o ilegible).
func PintarDeUsuario(guardado string, reserva color.Color) color.Color {
	if c, ok := TonoDeUsuario(guardado); ok {
		return c.NRGBA()
	}
	return reserva
}

// hexDeUsuario normaliza a `#rrggbb`: acepta `#rgb`, `#rrggbb` y el `#rrggbbaa` de
// algunos clientes CalDAV (la transparencia no se pinta), con o sin almohadilla.
func hexDeUsuario(guardado string) (string, bool) {
	h := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(guardado), "#"))
	if len(h) != 3 && len(h) != 6 && len(h) != 8 {
		return "", false
	}
	if _, err := strconv.ParseUint(h, 16, 32); err != nil {
		return "", false
	}
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	return "#" + h[:6], true
}

// oklab pasa un `#rrggbb` a OKLab, con los coeficientes de la web.
func oklab(hex string) (float64, float64, float64) {
	n, _ := strconv.ParseUint(hex[1:7], 16, 32)
	lin := func(v uint64) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	r, g, b := lin(n>>16), lin((n>>8)&255), lin(n&255)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return 0.2104542553*l + 0.793617785*m - 0.0040720468*s,
		1.9779984951*l - 2.428592205*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.808675766*s
}
