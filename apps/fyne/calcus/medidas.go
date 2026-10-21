// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package calcus es el LIENZO de Sytue Calcus, la hoja de cálculo: la rejilla de
// celdas, sus cabeceras, la selección y la barra de fórmulas.
//
// # Qué es de aquí y qué no
//
// En la web, Docus, Calcus y Diapos son tres apps que montan el MISMO editor
// (`@sytue/office`) con un tipo de documento distinto: comparten el cromo —`EditorApp`, la cinta de trece
// pestañas, el armazón y los ocho paneles— y lo único que cambia es el lienzo. Aquí
// pasa igual: este paquete es el equivalente de
// `apps/frontend/packages/office/src/editor/sheets/SheetCanvas.tsx` y NADA más. El cromo lo
// aloja.
//
// # El motor de fórmulas no está aquí, y es a propósito
//
// Evaluar es de `apps/nucleo/formula` —451 funciones, atadas por trazas a las de la
// web—, que vive en el núcleo compartido justo para esto (`docs/CONVENTIONS.md`
// §13.2). Aquí solo se le pregunta.
//
// # Los números de este fichero salen de la web, y hay un guardián que lo comprueba
//
// `medidas_test.go` lee `SheetCanvas.tsx` y `editor.css` del propio repositorio (con
// `apps/nucleo/webref`) y compara. Un espejo escrito a mano que nadie contrasta se
// desincroniza al primer cambio de la web.
package calcus

import (
	"image/color"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// La rejilla de la web en una hoja vacía: A..Z × 1..100. Son `MIN_COLS` y `MIN_ROWS`
// de sheets/rejilla.ts. Allí la rejilla crece con lo usado y con el desplazamiento
// hasta los límites de Excel (A..XFD × 1..1.048.576) y pinta solo lo que se ve; aquí
// sigue fija en la mínima, que es la deuda de paridad de `calcus-editar`.
const (
	Columnas = 26
	Filas    = 100
)

// MaxColumnas es `MAX_COLS` de sheets/rejilla.ts: lo que admiten los saneadores de
// gráficos y segmentaciones aunque aquí la rejilla pintada sea la mínima.
const MaxColumnas = 16384

// La geometría de la rejilla, en px ABSOLUTOS de CSS.
//
// Y absolutos de verdad: en la web estos tres números están escritos como `w-[40px]`,
// `w-[96px]` y `h-[24px]`, no como `w-10`/`w-24`/`h-6`, porque las clases de Tailwind
// son rem y el JS que coloca las capas superpuestas —el asa de autorelleno, los marcos
// de las referencias, los paneles congelados— calcula con píxeles. Con el rem de 14 de
// la suite, `w-10` mide 35 y la constante seguiría diciendo 40: cinco píxeles de
// separación por columna. Aquí no hay rem que valga; son 40, 96 y 24.
const (
	// AnchoCabecera es `HEAD_W`: la columna de los números de fila.
	AnchoCabecera float32 = 40
	// AnchoCelda es `CELL_W`. En la web NO se redimensiona: `colX[c] = OX + n *
	// CELL_W`, todas las columnas miden lo mismo.
	AnchoCelda float32 = 96
	// AltoCelda es `CELL_H`, y es también el alto de la fila de cabecera.
	AltoCelda float32 = 24
)

// MargenCelda es el `px-1` de la celda: relleno a izquierda y derecha. Este SÍ es rem
// (0,25rem), así que con el rem de 14 son 3,5 px.
var MargenCelda = tema.Espacio(1)

// AltoBarra es el `h-8` de la barra de fórmulas, y AnchoRef el `w-16` del rótulo de la
// celda activa. Los dos en rem: 28 y 56.
var (
	AltoBarra = tema.Espacio(8)
	AnchoRef  = tema.Espacio(16)
	// MargenBarra es su `px-2` y HuecoBarra su `gap-2`.
	MargenBarra = tema.Espacio(2)
	HuecoBarra  = tema.Espacio(2)
	// AltoRotulo es el `h-6` del indicador de la celda activa.
	AltoRotulo = tema.Espacio(6)
	// AltoEntradaFx es el `h-5` del `<input>` de la fórmula.
	AltoEntradaFx = tema.Espacio(5)
)

// Los escalones de la rejilla. El TAMAÑO de letra sale de la escala de la suite
// (`text-sm` en la celda, `text-xs` en la cabecera) pero la caja de línea NO: las dos
// clases llevan `leading-[24px]`, que es el alto de la celda. Por eso no se pueden usar
// `tema.TextoSM` y `tema.TextoXS` tal cual.
var (
	EscalonCelda    = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: AltoCelda}
	EscalonCabecera = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: AltoCelda}
	// EscalonFx es el campo de la barra de fórmulas, y aquí el escalón va ENTERO: el
	// `<input>` es `text-sm` a secas, sin clase de interlineado, así que su caja de
	// línea es la propia de `text-sm` (1,125rem = 15,75) y no el `h-5` de su caja.
	//
	// Esta era la que estaba mal: se copió el `leading-5` del div de detrás —el que
	// tinta las referencias de la fórmula, que no está portado— y se le puso 17,5. Lo
	// cazó la sonda contra producción, que mide `11.38px/15.75px` en ese input.
	EscalonFx = tema.TextoSM
)

// GrosorSeleccion es el `inset 0 0 0 2px` del marco de la celda ancla. Va por DENTRO de
// la celda: es una sombra `inset`, así que no mueve nada de sitio.
const GrosorSeleccion float32 = 2

// MezclaRango es el 12 % del azul `--editor-select` con el que se tiñe
// el rango seleccionado.
const MezclaRango = 0.12

// Los colores del EDITOR. No son tokens de tema y por eso no salen de `tema.Color`:
// viven en `apps/frontend/packages/office/src/editor/editor.css`, que es la hoja local del
// editor, y existen precisamente porque los tokens de tema NO valen aquí — la celda se
// pinta sobre papel blanco en los DOS temas, así que un `border-border`, que en oscuro
// se aclara para contrastar contra fondos oscuros, dejaba la cuadrícula casi invisible
// en modo noche.
var (
	// Papel es `--editor-page-bg`: blanco en los dos temas.
	Papel = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	// LineaRejilla es la tinta marina al 17 % sobre papel de superficie-hoja.css.
	LineaRejilla = tokens.Aplanar(tema.Color("accent-800", tema.Claro), 0.17, Papel)
	// Tinta es `INK`, el `CONTENT_PALETTE[0]` con el que la web pinta el texto de las
	// celdas (`color: INK` en la capa de la rejilla). Por lo mismo que el papel: el
	// contenido no cambia con el tema.
	Tinta = color.NRGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 0xff}
)

// FondoLienzo sigue superficies.css: neutral-150 en claro y surface-sunken en oscuro.
func FondoLienzo(v tema.Variante) color.NRGBA {
	if v == tema.Oscuro {
		return tema.Color("surface-sunken", v)
	}
	return tema.Color("neutral-150", v)
}
