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
	"math"
	"slices"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los tamaños de la rejilla, que son los de sheets/rejilla.ts y no hay que confundir:
//   - el LÍMITE (`MaxColumnas`/`MaxFilas`) es el de Excel, A..XFD × 1..1.048.576: una
//     celda importada en la fila 5.000 o en la columna AB existe, se navega hasta ella y
//     ninguna operación la tira por «salirse de la rejilla»;
//   - la EXTENSIÓN (`extension`) es lo que se recorre con la barra: lo usado, lo que pide
//     la selección y lo que se ve, con la rejilla mínima de holgura por delante;
//   - lo que se PINTA son solo las celdas visibles (`visibles`), sea cual sea la extensión.
const (
	// MinColumnas y MinFilas son `MIN_COLS` y `MIN_ROWS`: A..Z × 1..100, lo que se ve
	// en una hoja vacía y la holgura que queda siempre a la derecha y debajo de lo usado.
	MinColumnas = 26
	MinFilas    = 100
	// MaxColumnas y MaxFilas son `MAX_COLS` y `MAX_ROWS`.
	MaxColumnas = 16384
	MaxFilas    = 1048576
	// adelantoCols y adelantoFilas son `ADELANTO_COLS`/`ADELANTO_FILAS`: lo que la
	// extensión va por delante de lo que se ve, para que siempre quede por dónde seguir.
	adelantoCols  = 4
	adelantoFilas = 24
)

// Vista es la `Vista` de la web: lo que se ve de la hoja, en px de maqueta (sin zoom).
type Vista struct {
	Top, Left, W, H float32
}

// extension es `extension()`: columnas y filas que se pueden recorrer. ultCol/ultFila
// son la última columna y fila usadas (-1 si la hoja está vacía); pideCols/pideFilas,
// cuántas hacen falta para que quepa la selección.
func extension(ultCol, ultFila, pideCols, pideFilas int, v Vista, anchoCelda, altoCelda float32) (cols, filas int) {
	vistaCols := int(math.Ceil(float64((v.Left+v.W)/anchoCelda))) + adelantoCols
	vistaFilas := int(math.Ceil(float64((v.Top+v.H)/altoCelda))) + adelantoFilas
	return min(MaxColumnas, max(MinColumnas, ultCol+1+MinColumnas, pideCols, vistaCols)),
		min(MaxFilas, max(MinFilas, ultFila+1+MinFilas, pideFilas, vistaFilas))
}

// saltoADatos es `saltoADatos`: el destino de Ctrl+flecha en un eje, con el criterio de
// Excel. `llenas` son las posiciones con datos de esa fila o columna, ordenadas; `tope`,
// el límite del eje.
//   - Si la celda y la siguiente tienen datos, al final del bloque.
//   - Si no, a la próxima celda con datos en esa dirección.
//   - Y si no queda ninguna, al borde de la hoja.
func saltoADatos(llenas []int, desde, paso, tope int) int {
	siguiente := desde + paso
	if siguiente < 0 || siguiente >= tope {
		return desde
	}
	en := func(i int) (int, bool) {
		if i < 0 || i >= len(llenas) {
			return 0, false
		}
		return llenas[i], true
	}
	if k, ok := slices.BinarySearch(llenas, desde); ok {
		if v, ok := en(k + paso); ok && v == siguiente {
			i := k + paso
			for {
				v, ok := en(i + paso)
				if !ok || v != llenas[i]+paso {
					return llenas[i]
				}
				i += paso
			}
		}
	}
	if paso > 0 {
		i, _ := slices.BinarySearch(llenas, desde+1)
		if v, ok := en(i); ok {
			return v
		}
		return tope - 1
	}
	i, _ := slices.BinarySearch(llenas, desde)
	if v, ok := en(i - 1); ok {
		return v
	}
	return 0
}

// La geometría de la rejilla, en px ABSOLUTOS de CSS.
//
// Y absolutos de verdad: en la web estos tres números están escritos como `w-[40px]`,
// `w-[96px]` y `h-[24px]`, no como `w-10`/`w-24`/`h-6`, porque las clases de Tailwind
// son rem y el JS que coloca las capas superpuestas —el asa de autorelleno, los marcos
// de las referencias, los paneles congelados— calcula con píxeles. Cuando la suite tuvo
// 1 rem en 14 px, `w-10` medía 35 y la constante seguía diciendo 40: cinco píxeles de
// separación por columna. Aquí no hay rem que valga; son 40, 96 y 24.
const (
	// AnchoCabecera es `HEAD_W`: la columna de los números de fila.
	AnchoCabecera float32 = 40
	// AnchoCelda es `CELL_W`: el ancho DE SERIE. Una columna con ancho propio
	// (`colWidths`, `geometria.go`) mide el suyo.
	AnchoCelda float32 = 96
	// AltoCelda es `CELL_H`, el alto de serie de una fila (`rowHeights` aparte), y es
	// también el alto de la fila de cabecera, que no cambia.
	AltoCelda float32 = 24
)

// MargenCelda es el `px-1` de la celda: relleno a izquierda y derecha. Este SÍ es rem
// (0,25rem), así que con el rem de 16 son 4 px.
var MargenCelda = tema.Espacio(1)

// AltoBarra es el `h-8` de la barra de fórmulas, y AnchoRef el `sm:w-24` del rótulo de
// la celda activa (el cuadro de nombres). Los dos en rem: 32 y 96. Por debajo de `sm`
// el cuadro es `w-16` (AnchoRefEstrecho): a 320 px el de 96 dejaba el campo de la
// fórmula por debajo del objetivo táctil de 44 px.
var (
	AltoBarra = tema.Espacio(8)
	AnchoRef  = tema.Espacio(24)
	// AnchoRefEstrecho es el `w-16` del cuadro de nombres por debajo de `sm`.
	AnchoRefEstrecho = tema.Espacio(16)
	// MargenBarra es su `px-2` y HuecoBarra su `gap-2`.
	MargenBarra = tema.Espacio(2)
	HuecoBarra  = tema.Espacio(2)
	// AltoRotulo es el `h-6` del indicador de la celda activa.
	AltoRotulo = tema.Espacio(6)
	// AltoEntradaFx es el `h-5` del `<input>` de la fórmula.
	AltoEntradaFx = tema.Espacio(5)

	// La barra desplegada (`items-start py-1`): el `<textarea>` es `h-15` (tres líneas de
	// `leading-5`) y baja `mt-0.5` para que su primera línea quede a la altura del
	// rótulo `h-6`. Lo más alto de la fila es él, así que la barra mide su `py-1`, el
	// `mt-0.5`, el `h-15` y el `border-b`: 62,25.
	MargenBarraDesplegada = tema.Espacio(1)
	BajadaCampoDesplegado = tema.Espacio(0.5)
	AltoCampoDesplegado   = tema.Espacio(15)
	AltoBarraDesplegada   = 2*MargenBarraDesplegada + BajadaCampoDesplegado + AltoCampoDesplegado + tema.Borde
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
	// EscalonFxDesplegada es el `<textarea>` de la barra desplegada, que sí lleva
	// `leading-5`: tres líneas de 17,5 en su `h-15`, las mismas que el fondo tintado.
	EscalonFxDesplegada = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: tema.Espacio(5)}
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
