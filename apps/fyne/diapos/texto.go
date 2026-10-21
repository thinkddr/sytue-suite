// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
)

// # El texto de una caja
//
// En la web una caja de texto es un `contenteditable` con la hoja propia del lienzo
// (`BOX_CSS` de SlidesCanvas.tsx) dentro de un `div` con TEXT_BOX_CLASS. Es el mismo
// HTML, y el mismo `exec` de la cinta, que el folio de Docus; lo que cambia es la hoja.
//
// Por eso aquí no hay un lector de HTML propio: el texto de una caja se lee, se reparte
// en líneas, se pinta y se ESCRIBE con el modelo y el folio de Docus (`rico` y
// `docus.Folio` en modo caja), con la hoja de abajo. El formato en línea —negrita,
// cursiva, subrayado, tachado, color, resaltado, familia y tamaño de un trozo— llega así
// entero, y lo que se ve al escribir es lo que se ve al terminar.

// Los tamaños de dentro de una caja de texto, en px del ESCENARIO (el sistema de 960
// de ancho). Son `BOX_CSS`, una hoja propia del lienzo y no la escala de la suite: una
// diapositiva es un lienzo de tamaño fijo, no una página que se reflowa, así que sus
// tamaños van en px y no en rem.
//
//	.sytue-slidebox h1 { font-size: 44px; font-weight: 700; margin: 0 }
//	.sytue-slidebox h2 { font-size: 30px; font-weight: 600; margin: 0 }
//	.sytue-slidebox h3 { font-size: 24px; font-weight: 600; margin: 0 }
//	.sytue-slidebox p, ul, ol { font-size: 20px; margin: 0 0 0.3em }
//	.sytue-slidebox ul, ol { padding-left: 1.2em }  (24 px a 20: la sangría del folio)
//	.sytue-slidebox { line-height: 1.25 }
const (
	tamH1     float32 = 44
	tamH2     float32 = 30
	tamH3     float32 = 24
	tamCuerpo float32 = 20
	// Interlineado de `.sytue-slidebox`, relativo al tamaño de cada letra.
	interlineado float32 = 1.25
	// `margin: 0 0 0.3em` de p/ul/ol, en em de su propio tamaño.
	margenBloque float32 = 0.3
	// `px-1` de TEXT_BOX_CLASS: 0,25rem con el rem de la suite.
	sangriaCaja float32 = 0.25 * 14
)

// hojaCaja es `BOX_CSS` con la tinta y la letra de la diapositiva, que la caja hereda
// del escenario, y la alineación que heredan los bloques que no declaran la suya (ver
// `Lienzo.AlinearDefecto`).
//
// El texto suelto de la caja, fuera de todo bloque, va a 20 px como un párrafo.
// ponytail: en la web hereda la letra del escenario, que la hoja no fija; nadie lo
// escribe (las disposiciones, la IA y el importador ponen siempre un bloque).
func hojaCaja(tinta color.NRGBA, familia, alinear string) *rico.Hoja {
	al := rico.Izquierda
	switch alinear {
	case "center":
		al = rico.Centro
	case "right":
		al = rico.Derecha
	}
	return &rico.Hoja{
		Base:    rico.Estilo{Peso: 400, Color: tinta, Tamano: tamCuerpo, Interlineado: interlineado, Alineacion: al, Familia: familia},
		Titulos: [4]rico.EstiloTitulo{1: {Tamano: tamH1, Peso: 700}, 2: {Tamano: tamH2, Peso: 600}, 3: {Tamano: tamH3, Peso: 600}},
		Margen:  margenBloque,
	}
}

// cajaFolio es la geometría de una caja de texto para el folio de Docus: su tamaño en
// px del escenario, la escala y el `px-1`.
func cajaFolio(tam fyne.Size, k float32, h *rico.Hoja) docus.Caja {
	return docus.Caja{Ancho: tam.Width / k, Alto: tam.Height / k, Escala: k, Relleno: sangriaCaja, Hoja: h}
}
