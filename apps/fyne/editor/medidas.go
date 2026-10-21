// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Medidas de editor.css y de EditorDemo real: Chromium remoto 2026-10-04,
// CSS1Compat, raíz16 y Figtree cargada. Las bandas son píxeles literales;
// las clases h-*/px-*/gap-* se resuelven con la raíz actual.
const (
	AltoCabecera float32 = 48 // --editor-header-h
	AltoTira     float32 = 32 // --editor-tabs-h; tabs31 + border-b1 (con el dedo, AltoDeTira)
	AltoBanda    float32 = 68 // --editor-groups-h: 6 + 44 + 2 + 11 + 4 y su línea
	// AltoBandaEstrecha es la banda por debajo de `md`: una fila de controles sin rótulos
	// de grupo, 4 + 44 + 4 y su línea (`--editor-groups-h` de editor.css en <48rem).
	AltoBandaEstrecha float32 = 53
	AltoEstado        float32 = 28  // --editor-status-h
	AnchoCarril       float32 = 40  // --editor-rail-w; contenido39
	AnchoPanel        float32 = 288 // --editor-panel-w
)

// EditorHeader: px-3=12, gap-2=8, título h-7=28/text-sm13/peso600.
var (
	MargenCabecera            = tema.Espacio(3)
	HuecoCabecera             = tema.Espacio(2)
	AltoTitulo                = tema.Espacio(7)
	AnchoTitulo       float32 = 360             // max-w-[360px]
	AltoBotonCabecera         = tema.Espacio(8) // Button sm: h-8=32
)

// Ribbon: px-2=8, gap-1=4, tabs px-3=12, text-sm13.
var (
	MargenTira      = tema.Espacio(2)
	HuecoTira       = tema.Espacio(1)
	MargenPestana   = tema.Espacio(3)
	GrosorIndicador = tema.Espacio(0.5) // h-0.5=2
	MargenIndicador = tema.Espacio(1)   // inset-x-1=4
)

// La banda NO tiene relleno vertical: el aire es el del grupo (`pt-1.5 pb-1`). Con el
// `py-1` que llevaba en la web medía 75 y no 68 desde que la raíz pasó a 16 px, y aquí el
// grupo se quedaba en 59 de los 67 que mide su contenido: el rótulo pisaba la fila
// (medido el 2026-10-16). Banda en y80: `pt-1.5`=6 pone la fila en y86 y `min-h-11`=44
// centra un botón h-7=28 en y94.
//
// Por debajo de `md` el grupo es `py-1` y no lleva rótulo: la fila en 4, y 4 debajo.
var (
	MargenBanda         = tema.Espacio(2)
	MargenGrupo         = tema.Espacio(1.5)
	ArribaGrupo         = tema.Espacio(1.5)
	AbajoGrupo          = tema.Espacio(1)
	ArribaGrupoEstrecho = tema.Espacio(1)
	AltoFilaGrupo       = tema.Espacio(11)
	HuecoControl        = tema.Espacio(0.5)
	AltoRotuloGrupo     = tema.Texto2XS.Tam // leading-none, 11
)

// RibbonButton h-7 w-7 rounded-sm; glifo18. RibbonSelect h-7 text-xs12.
var (
	LadoBotonCinta                  = tema.Espacio(7)
	IconoBotonCinta         float32 = 18
	AltoSelectorCinta               = tema.Espacio(7)
	IzqSelector                     = tema.Espacio(1.5)
	DerSelector                     = tema.Espacio(6)
	IconoChevron            float32 = 14
	AnchoSelectorPorDefecto float32 = 120 // width ?? 120
)

// Separador: w-px h-8 mx-1.5 mt-3, y `max-md:mt-2.5` con la fila del grupo estrecho.
var (
	AltoSeparadorGrupos     = tema.Espacio(8)
	MargenSeparador         = tema.Espacio(1.5)
	ArribaSeparador         = tema.Espacio(3)
	ArribaSeparadorEstrecho = tema.Espacio(2.5)
)

// AltoDeTira es la tira de pestañas con el puntero de ahora: 32, y 44 con el dedo
// (`--editor-tabs-h: max(44px, 2.75rem)` con `pointer: coarse` en editor.css).
func AltoDeTira() float32 { return ui.SueloTactil(AltoTira) }

// Rail: botones32, x3.5 dentro del borde; py-2=8 y gap-1=4.
var (
	LadoBotonCarril  float32 = 32
	IconoBotonCarril float32 = 18
	RellenoCarril            = tema.Espacio(2)
	HuecoCarril              = tema.Espacio(1)
)

// Cabecera del panel h-10 px-3 text-sm.
var (
	AltoCabeceraPanel = tema.Espacio(10)
	MargenPanel       = tema.Espacio(3)
)

// StatusBar: px-3 gap-4 text-xs; botones24, range w-24=96 y porcentaje w-10=40.
var (
	MargenEstado             = tema.Espacio(3)
	HuecoEstado              = tema.Espacio(4)
	LadoBotonEstado  float32 = 24
	IconoBotonEstado float32 = 14
	AnchoDeslizador  float32 = 96
	AltoDeslizador   float32 = 20 // caja nativa del range en Chromium
	AnchoPorciento           = tema.Espacio(10)
)

// Zoom: el suelo es 25 y no 50 porque con 50 un A4 no cabe en un teléfono
// (`canvas/metrics.ts`).
const (
	ZoomMin  = 25
	ZoomMax  = 200
	ZoomPaso = 10
)

// CapasSombraPagina es `--editor-page-shadow` de `editor.css` en los dos temas, capa a
// capa: el folio de Docus y las diapositivas de la vista del moderador de Diapos.
func CapasSombraPagina(v tema.Variante) []tema.CapaSombra {
	if v == tema.Oscuro {
		return []tema.CapaSombra{
			{DesplY: 1, Desenfoque: 2, Color: color.NRGBA{A: 128}},
			{DesplY: 8, Desenfoque: 24, Color: color.NRGBA{A: 115}},
		}
	}
	pizarra := func(a uint8) color.NRGBA { return color.NRGBA{R: 15, G: 23, B: 42, A: a} }
	return []tema.CapaSombra{
		{DesplY: 1, Desenfoque: 2, Color: pizarra(26)},
		{DesplY: 6, Desenfoque: 20, Color: pizarra(31)},
	}
}
