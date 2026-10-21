// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import "image/color"

// # Las cinco elevaciones de theme.css
//
// Las declara `--shadow-xs … --shadow-xl`, y su comentario dice para qué es cada una:
// «xs contacto · sm tarjetas · md menús/popovers · lg overlays · xl modales».
//
// Van en **px de CSS en la hoja**, no en rem, así que el rem de 14 de esta suite NO las
// toca: un `24px` de la hoja son 24 px aquí. Es la única familia de medidas del tema de
// la que eso es verdad junto con el layout canónico.
//
// El tinte —la tinta del texto, con su punto de verde— y los cinco niveles salen de
// theme.css. El guardián compara cada capa en claro y oscuro para que la interfaz nativa
// no diverja.
//
// En OSCURO la hoja cambia el color a negro y sube el alfa. Y deja `xs` sin redefinir,
// así que en oscuro `xs` sigue siendo el del claro: no es un descuido que haya que
// arreglar aquí, es lo que pinta la web.

// CapaSombra es UNA capa de `box-shadow`: desplazamiento, radio de desenfoque y color
// con su alfa. En px de CSS.
//
// `sm` y `md` llevan dos capas —la hoja declara dos sombras separadas por coma—, y por
// eso esto se pide en lista y no de una en una. En CSS la PRIMERA de la lista se pinta encima.
type CapaSombra struct {
	DesplX, DesplY float32
	Desenfoque     float32 // el `blur-radius` de CSS, no la desviación típica
	Color          color.NRGBA
}

// Desviacion es la sigma de la gaussiana que corresponde a este radio de desenfoque.
//
// La regla es del propio CSS: un `blur-radius` de r se pinta con «una gaussiana de
// desviación típica igual a la MITAD del radio». Media docena de motores lo aproximan
// con tres desenfoques de caja, y eso es justo lo que hace `ui.NuevaSombra`.
func (c CapaSombra) Desviacion() float32 { return c.Desenfoque / 2 }

// NivelSombra es uno de los cinco escalones de elevación.
type NivelSombra uint8

// Los cinco, en el orden de la hoja.
const (
	SombraXS NivelSombra = iota
	SombraSM
	SombraMD
	SombraLG
	SombraXL
)

// tinta es el rgb(14,26,20) del tema claro con el alfa pedido.
func tinta(a float32) color.NRGBA {
	return color.NRGBA{R: 14, G: 26, B: 20, A: uint8(a*255 + 0.5)}
}

// negro es el rgb(0,0,0) del tema oscuro con el alfa pedido.
func negro(a float32) color.NRGBA {
	return color.NRGBA{A: uint8(a*255 + 0.5)}
}

// Las cinco del tema claro, copiadas de theme.css una a una.
var sombrasClaro = [...][]CapaSombra{
	SombraXS: {{DesplY: 1, Desenfoque: 2, Color: tinta(0.04)}},
	SombraSM: {
		{DesplY: 1, Desenfoque: 2, Color: tinta(0.06)},
		{DesplY: 1, Desenfoque: 3, Color: tinta(0.08)},
	},
	SombraMD: {
		{DesplY: 2, Desenfoque: 4, Color: tinta(0.05)},
		{DesplY: 12, Desenfoque: 30, Color: tinta(0.13)},
	},
	SombraLG: {{DesplY: 16, Desenfoque: 40, Color: tinta(0.16)}},
	SombraXL: {{DesplY: 24, Desenfoque: 48, Color: tinta(0.2)}},
}

// Las del tema oscuro. `xs` NO está: la hoja no lo redefine y hereda el del claro.
var sombrasOscuro = [...][]CapaSombra{
	SombraXS: nil,
	SombraSM: {
		{DesplY: 1, Desenfoque: 2, Color: negro(0.4)},
		{DesplY: 1, Desenfoque: 3, Color: negro(0.3)},
	},
	SombraMD: {{DesplY: 12, Desenfoque: 30, Color: negro(0.55)}},
	SombraLG: {{DesplY: 16, Desenfoque: 40, Color: negro(0.6)}},
	SombraXL: {{DesplY: 24, Desenfoque: 48, Color: negro(0.65)}},
}

// Sombra devuelve las capas de un nivel en la variante pedida.
//
// Un nivel que no existe es un error de quien escribe el widget, igual que un token de
// color desconocido, y entra en pánico por la misma razón: se ve al pintar y no al
// compilar, así que cuanto más ruidoso, antes se arregla.
func Sombra(n NivelSombra, v Variante) []CapaSombra {
	if int(n) >= len(sombrasClaro) {
		panic("tema: nivel de sombra desconocido")
	}
	if v == Oscuro {
		if c := sombrasOscuro[n]; c != nil {
			return c
		}
	}
	return sombrasClaro[n]
}
