// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"fmt"
	"image/color"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los iconos PROPIOS DEL EDITOR ofimático (Docus, Calcus, Diapos).
//
// # Por qué es un catálogo aparte y no más constantes del de arriba
//
// La web los tiene aparte por lo mismo: `apps/frontend/packages/office/src/editor/icons.tsx`
// dibuja 136 trazados de cero y NO importa los de `packages/ui/Icon.tsx`. No es
// duplicación por descuido —es variación de dominio, la que `docs/CONVENTIONS.md`
// §13.2 manda respetar—: la cinta de un editor ofimático es densa y sus glifos están
// dibujados para leerse a 18 px en una fila de 24,5, mientras que los del cromo de la
// suite se dibujan para 20 en una barra de 31,5. Se ve en el mismo nombre: `undo` es
// «M9 14 4 9l5-5…» en el catálogo compartido y «M9 14 5 10l4-4…» aquí.
//
// Fundirlos daría un icono equivocado en uno de los dos sitios. Van separados, con su
// propio tipo, y el compilador no deja mezclarlos.
//
// # Qué hay y qué falta
//
// Aquí están los que la cinta nativa usa HOY. Los 95 restantes de la web están en
// `pendientesEd` (en `editor_test.go`), que el guardián compara por IGUALDAD: uno
// nuevo en la web no está ni aquí ni en esa lista, y pone el test en rojo.

// Ed es un icono del editor ofimático. El cero es EdNinguno.
type Ed uint8

// El catálogo del editor, en el orden de `icons.tsx`. El comentario de cada uno es su
// clave en la web, que es por la que hay que buscarlo allí.
const (
	// EdNinguno es el CERO, y es «no hay icono»: lo mismo y por lo mismo que Ninguno.
	EdNinguno         Ed = iota
	EdDeshacer           // undo
	EdRehacer            // redo
	EdNegrita            // bold
	EdCursiva            // italic
	EdSubrayado          // underline
	EdTachado            // strike
	EdColorTexto         // textColor
	EdResaltado          // highlight
	EdSinFormato         // clearFormat
	EdAlinearIzq         // alignLeft
	EdAlinearCentro      // alignCenter
	EdAlinearDer         // alignRight
	EdJustificar         // alignJustify
	EdListaPuntos        // listBullet
	EdListaNumeros       // listNumber
	EdMasSangria         // indent
	EdMenosSangria       // outdent
	EdInterlineado       // lineHeight
	EdRegla              // ruler
	EdPanelDerecho       // panelRight
	EdZoom               // zoom
	EdMenos              // minus
	EdMas                // plus
	EdComentario         // comment
	EdComentarioNuevo    // commentAdd
	EdComentarios        // commentList
	EdHistorial          // history
	EdCompartir          // share
	EdBuscar             // search
	EdEsquema            // outline
	EdRevision           // trackChanges
	EdIA                 // sparkles
	EdChevronIzq         // chevronLeft
	EdChevronAbajo       // chevronDown
	EdDescargar          // download
	EdTic                // check
	EdCerrar             // close
	EdAjustes            // settings
	EdTabla              // table
	EdImagen             // image
	EdEnlace             // link
	EdSeparador          // hr
	EdIndice             // toc
	EdChevronDer         // chevronRight
	EdChevronArriba      // chevronUp
	EdPapelera           // trash
	EdDiapoNueva         // slideAdd
	EdCajaTexto          // textBox
	EdPatron             // slideMaster
	EdFlecha             // arrow
	EdTamanoPagina       // pageSize
	EdCompleta           // fullscreen
	EdGrafico            // chart
	EdPresentar          // present
	EdOrganizar          // arrange
	EdCopiar             // copy
	EdSuperindice        // superscript
	EdSubindice          // subscript
	EdVertical           // pagePortrait
	EdApaisada           // pageLandscape
	EdNotaPie            // footnote
	EdDividirCeldas      // splitCells
	EdMasDecimales       // decimalMore
	EdMenosDecimales     // decimalLess
	EdFormatoNumero      // numFormat
	EdFormatoCond        // condFormat
	EdCombinar           // merge
	EdOrdenAsc           // sortAsc
	EdOrdenDesc          // sortDesc
	EdFiltro             // filter
	EdBaseDatos          // database
	EdActualizar         // refresh
	EdActualizarTodo     // refreshAll
	EdFilaArriba         // rowAbove
	EdFilaAbajo          // rowBelow
	EdColIzq             // colLeft
	EdColDer             // colRight
	EdInmovilizar        // freeze
	EdCandado            // lock
	EdEscudo             // shield
	EdObjetivo           // target
	EdEscenarios         // scenarios
	EdTablaDatos         // datatable
	EdSolver             // solver
	EdAgruparFilas       // groupRows
	EdDesagruparFilas    // ungroupRows
	EdAgruparCols        // groupCols
	EdDesagruparCols     // ungroupCols
	EdSubtotal           // subtotal
	EdSegmentacion       // slicer
	EdLibro              // book
	EdVideo              // video
	EdAudio              // audio
	EdRect               // rect
	EdLinea              // line
	EdElipse             // ellipse
	EdTriangulo          // triangle
	EdRombo              // diamond
	EdPentagono          // pentagon
	EdHexagono           // hexagon
	EdEstrella           // star
	EdCilindro           // cylinder
	EdBocadillo          // callout
	EdConRecto           // connStraight
	EdConCodo            // connElbow
	EdConCurvo           // connCurved
	EdSmartLista         // smartList
	EdSmartProceso       // smartProcess
	EdSmartCiclo         // smartCycle
	EdSmartJerarquia     // smartHierarchy
	EdSimbolo            // symbol
	EdNumeroPagina       // pageNumber
	EdSaltoPagina        // pageBreak
	EdEncabezado         // header
	EdPie                // footer
	EdRecorte            // crop
	EdNotaFinal          // endnote
	EdEntradaIndice      // indexEntry
	EdIndiceAlfa         // indexAlpha
	EdLeyenda            // caption
	EdRefCruzada         // xref
	EdCita               // citation
	EdBibliografia       // bibliography
	EdFiltroQuitar       // filterOff
	EdCampoCorresp       // mergeField
	EdPrevia             // preview
	EdCorrespondencia    // mailMerge
	EdMarcaAgua          // watermark
	EdBordePagina        // pageBorder
	EdGrabar             // record
	EdDetener            // stopRec
	EdMacro              // macro
	EdReproducir         // play
	EdEmitirCast         // cast
	EdDraw               // draw
	EdEye                // eye
	EdPointer            // pointer
	EdEraser             // eraser
	EdOpenInApp          // openInApp
	EdNombreDefinido     // definedName
	EdLapiz              // pencil
	// numEd va SIEMPRE el último y no es un icono: es cuántos hay.
	numEd
)

// catalogoEd va indexado por el propio Ed, como `catalogo`.
var catalogoEd = [...]entrada{
	EdLapiz:           {"pencil", "M4.5 19.5 4 20l.5-4L15 5.5a1.8 1.8 0 0 1 2.6 0l.9.9a1.8 1.8 0 0 1 0 2.6L8 19.5Z M13.5 7l3.5 3.5"},
	EdNombreDefinido:  {"definedName", "M3.5 12 8.5 6.5h12v11h-12Z M7.5 12h.01 M11.5 12h5.5"},
	EdOpenInApp:       {"openInApp", "M4 4.5h16a1 1 0 0 1 1 1V15a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5.5a1 1 0 0 1 1-1Z M9 20h6 M12 16v4"},
	EdEraser:          {"eraser", "M8 19h11 M4.5 14.5 13 6a2 2 0 0 1 2.8 0l2.2 2.2a2 2 0 0 1 0 2.8L10.5 19H8l-3.5-3.5a1.4 1.4 0 0 1 0-1Z"},
	EdPointer:         {"pointer", "M5.5 3.5 18.5 10l-5.6 1.9L10.7 18Z"},
	EdEye:             {"eye", "M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z M12 9.3a2.7 2.7 0 1 0 0 5.4 2.7 2.7 0 0 0 0-5.4Z"},
	EdDraw:            {"draw", "M4 20l1-4.2L16.6 4.2a2 2 0 0 1 2.8 0l.4.4a2 2 0 0 1 0 2.8L8.2 19Z M14.5 6.3l3.2 3.2"},
	EdEmitirCast:      {"cast", "M3.5 6h17v12h-5 M3.5 18h.01 M3.5 14.5a3.5 3.5 0 0 1 3.5 3.5 M3.5 11a7 7 0 0 1 7 7"},
	EdNinguno:         {"", ""},
	EdDeshacer:        {"undo", "M9 14 5 10l4-4 M5 10h9a4.5 4.5 0 0 1 0 9h-2.5"},
	EdRehacer:         {"redo", "M15 14 19 10l-4-4 M19 10h-9a4.5 4.5 0 0 0 0 9h2.5"},
	EdNegrita:         {"bold", "M7 12h6.5a3.5 3.5 0 0 1 0 7H7V5h6a3.5 3.5 0 0 1 0 7"},
	EdCursiva:         {"italic", "M11.5 5h7 M5.5 19h7 M14.5 5l-5 14"},
	EdSubrayado:       {"underline", "M6.5 4.5v6a5.5 5.5 0 0 0 11 0v-6 M5.5 19.5h13"},
	EdTachado:         {"strike", "M16.7 6.6c-.8-1.6-2.6-2.6-4.7-2.6-2.8 0-4.8 1.4-4.8 3.5 0 1.3.8 2.3 2.4 3 M7.3 17.4c.8 1.6 2.6 2.6 4.7 2.6 2.8 0 4.8-1.4 4.8-3.5 0-1.1-.6-2-1.8-2.7 M4 12h16"},
	EdColorTexto:      {"textColor", "m5.5 17 5.2-12h2.6l5.2 12 M8.2 11.5h7.6"},
	EdResaltado:       {"highlight", "M4 20h4L18.6 9.4a2 2 0 0 0 0-2.8l-1.2-1.2a2 2 0 0 0-2.8 0L4 16Z M13.1 7.9l3 3"},
	EdSinFormato:      {"clearFormat", "M5 5h12 M11 5v14 M15.5 15.5 20 20 M20 15.5l-4.5 4.5"},
	EdAlinearIzq:      {"alignLeft", "M4 6h16 M4 10h10 M4 14h16 M4 18h10"},
	EdAlinearCentro:   {"alignCenter", "M4 6h16 M7 10h10 M4 14h16 M7 18h10"},
	EdAlinearDer:      {"alignRight", "M4 6h16 M10 10h10 M4 14h16 M10 18h10"},
	EdJustificar:      {"alignJustify", "M4 6h16 M4 10h16 M4 14h16 M4 18h16"},
	EdListaPuntos:     {"listBullet", "M4 6h.01 M4 12h.01 M4 18h.01 M9 6h11 M9 12h11 M9 18h11"},
	EdListaNumeros:    {"listNumber", "M4 5.5 5.3 4.5v4.5 M3.8 13.3c0-.9.7-1.5 1.6-1.5 1 0 1.7.6 1.7 1.5 0 1.6-3.2 2.1-3.2 4.2h3.4 M10 6h10 M10 12h10 M10 18h10"},
	EdMasSangria:      {"indent", "M4 5.5h16 M12 9.5h8 M12 13.5h8 M4 17.5h16 M4 9.5l3 2-3 2"},
	EdMenosSangria:    {"outdent", "M4 5.5h16 M12 9.5h8 M12 13.5h8 M4 17.5h16 M7 9.5l-3 2 3 2"},
	EdInterlineado:    {"lineHeight", "M11 6h9 M11 12h9 M11 18h9 M5 5.5v13 M2.8 7.7 5 5.5l2.2 2.2 M2.8 16.3 5 18.5l2.2-2.2"},
	EdRegla:           {"ruler", "M4 8.5h16v7H4Z M8 8.5v3 M12 8.5v4 M16 8.5v3"},
	EdPanelDerecho:    {"panelRight", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M15 4v16"},
	EdZoom:            {"zoom", "M10.5 17a6.5 6.5 0 1 1 0-13 6.5 6.5 0 0 1 0 13Z M15.4 15.4 20 20 M10.5 8v5 M8 10.5h5"},
	EdMenos:           {"minus", "M4.5 12h15"},
	EdMas:             {"plus", "M12 4.5v15M4.5 12h15"},
	EdComentario:      {"comment", "M6 5h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7l-4.5 4v-4H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z"},
	EdComentarioNuevo: {"commentAdd", "M6 5h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7l-4.5 4v-4H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z M12 8v5 M9.5 10.5h5"},
	EdComentarios:     {"commentList", "M6 5h12a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7l-4.5 4v-4H6a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z M8 9h8 M8 12h6"},
	EdHistorial:       {"history", "M4.5 12a7.5 7.5 0 1 0 7.5-7.5c-2.2 0-4.2.9-5.6 2.4L4.5 8.8 M4.5 4.5v4.3h4.3 M12 8.5v4l2.8 1.7"},
	EdCompartir:       {"share", "M8.5 4.5H6a2 2 0 0 0-2 2V18a2 2 0 0 0 2 2h11.5a2 2 0 0 0 2-2v-2.5 M13.5 4H20v6.5 M20 4l-8.5 8.5"},
	EdBuscar:          {"search", "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-3.8-3.8"},
	EdEsquema:         {"outline", "M4.5 5.5h.01 M8 5.5h12 M7.5 12h.01 M11 12h9 M4.5 18.5h.01 M8 18.5h12"},
	EdRevision:        {"trackChanges", "M5 19.5l.8-3.3L16.1 5.9a1.8 1.8 0 0 1 2.5 2.5L8.3 18.7l-3.3.8Z M14.3 7.7l2.5 2.5 M4 5.5h6 M4 9h3.5"},
	EdIA:              {"sparkles", "M10.5 5.5 12.1 10.4 17 12 12.1 13.6 10.5 18.5 8.9 13.6 4 12 8.9 10.4Z M17.5 4 18.1 5.9 20 6.5 18.1 7.1 17.5 9 16.9 7.1 15 6.5 16.9 5.9Z"},
	EdChevronIzq:      {"chevronLeft", "M15.5 5l-7 7 7 7"},
	EdChevronAbajo:    {"chevronDown", "M5 8.5l7 7 7-7"},
	EdDescargar:       {"download", "M12 4v10.5 M7 10l5 5 5-5 M4.5 19.5h15"},
	EdTic:             {"check", "M20 6.5 9.5 17.5 4 12"},
	EdCerrar:          {"close", "M18.5 5.5 5.5 18.5M5.5 5.5l13 13"},
	EdAjustes:         {"settings", "M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6Z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z"},
	EdTabla:           {"table", "M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM3 9.5h18M9.5 5.5V18.5M14.5 5.5V18.5"},
	EdImagen:          {"image", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M4.5 16.5 9 12l4 4 2.5-2.5 4 4 M9.25 8.5a.75.75 0 1 0 1.5 0 .75.75 0 1 0-1.5 0Z"},
	EdEnlace:          {"link", "M9 15l6-6 M10.5 5.5 12 4a4.24 4.24 0 0 1 6 6l-1.5 1.5 M13.5 18.5 12 20a4.24 4.24 0 0 1-6-6l1.5-1.5"},
	EdSeparador:       {"hr", "M4 12h16 M9 6.5h6 M9 17.5h6"},
	EdIndice:          {"toc", "M4 6h9 M4 12h9 M4 18h9 M18.5 6h1.5 M18.5 12h1.5 M18.5 18h1.5"},
	EdChevronDer:      {"chevronRight", "M8.5 5l7 7-7 7"},
	EdChevronArriba:   {"chevronUp", "M5 15.5l7-7 7 7"},
	EdPapelera:        {"trash", "M4.5 7h15 M9.5 7V5a1.3 1.3 0 0 1 1.3-1.3h2.4A1.3 1.3 0 0 1 14.5 5v2 M6.5 7l.7 12.1a1.6 1.6 0 0 0 1.6 1.5h6.4a1.6 1.6 0 0 0 1.6-1.5L17.5 7 M10 11v6 M14 11v6"},
	EdDiapoNueva:      {"slideAdd", "M3.5 6h17v12h-17Z M12 9v6 M9 12h6"},
	EdCajaTexto:       {"textBox", "M4.5 6h15v12h-15Z M9 9.5h6 M12 9.5V15"},
	EdPatron:          {"slideMaster", "M4.5 5.5h15v13h-15Z M4.5 9h15 M7 11.5h6v4.5H7Z"},
	EdFlecha:          {"arrow", "M5 19 19 5 M12 5h7v7"},
	EdTamanoPagina:    {"pageSize", "M4.5 4.5h15v15h-15Z M14.5 9.5l-5 5 M14.5 12.5v-3h-3 M9.5 11.5v3h3"},
	EdCompleta:        {"fullscreen", "M4 9V5a1 1 0 0 1 1-1h4 M15 4h4a1 1 0 0 1 1 1v4 M20 15v4a1 1 0 0 1-1 1h-4 M9 20H5a1 1 0 0 1-1-1v-4"},
	EdGrafico:         {"chart", "M4 4v16h16M8 20v-4M12 20v-8M16 20v-11"},
	EdPresentar:       {"present", "M3.5 6h17v12h-17Z M10.3 9.5v5l4.4-2.5Z"},
	EdOrganizar:       {"arrange", "M4.5 4.5h9v9h-9Z M10.5 10.5h9v9h-9Z"},
	EdMasDecimales:    {"decimalMore", "M6 3.5v5 M3.5 6h5 M5.6 16.6l-.8 2.2 M8.5 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z M14.7 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z"},
	EdMenosDecimales:  {"decimalLess", "M3.5 6h5 M5.6 16.6l-.8 2.2 M8.5 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z M14.7 14.5a2.1 3.3 0 1 0 4.2 0 2.1 3.3 0 1 0-4.2 0Z"},
	EdFormatoNumero:   {"numFormat", "M4 9h16 M4 15h16 M10 4 8 20 M16 4 14 20"},
	EdFormatoCond:     {"condFormat", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M13.8 8.5 9.8 12l4 3.5"},
	EdGrabar:          {"record", "M12 4.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 1 0 0-15Z M12 9a3 3 0 1 0 0 6 3 3 0 1 0 0-6Z"},
	EdDetener:         {"stopRec", "M7 7h10v10H7Z"},
	EdMacro:           {"macro", "M4 6h9 M4 12h7 M4 18h5 M15 12.5v7l5.5-3.5Z"},
	EdReproducir:      {"play", "M8 5.5v13l11-6.5Z"},
	EdCombinar:        {"merge", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M7.5 12h3.5 M9.5 10.5 11 12l-1.5 1.5 M16.5 12h-3.5 M14.5 10.5 13 12l1.5 1.5"},
	EdOrdenAsc:        {"sortAsc", "M4 6h4 M4 12h8 M4 18h12 M18.5 19V5 M15.5 8l3-3 3 3"},
	EdOrdenDesc:       {"sortDesc", "M4 6h12 M4 12h8 M4 18h4 M18.5 5v14 M15.5 16l3 3 3-3"},
	EdFiltro:          {"filter", "M4 5h16l-6 6.5V19l-4-2v-5.5Z"},
	EdBaseDatos:       {"database", "M12 3c-4.4 0-8 1.3-8 3s3.6 3 8 3 8-1.3 8-3-3.6-3-8-3ZM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"},
	EdActualizar:      {"refresh", "M20 11.5A8 8 0 0 0 6.3 6.3L4 8.5 M4 4v4.5h4.5 M4 12.5a8 8 0 0 0 13.7 5.2L20 15.5 M20 20v-4.5h-4.5"},
	EdActualizarTodo:  {"refreshAll", "M20 11.5A8 8 0 0 0 6.3 6.3L4 8.5 M4 4v4.5h4.5 M4 12.5a8 8 0 0 0 13.7 5.2L20 15.5 M20 20v-4.5h-4.5 M9.5 11h5 M9.5 13.5h5"},
	EdFilaArriba:      {"rowAbove", "M12 2.5v7 M9 5.5l3-3 3 3 M4 13h16v7.5H4Z M4 16.75h16"},
	EdFilaAbajo:       {"rowBelow", "M4 3.5h16V11H4Z M4 7.25h16 M12 14.5v7 M9 18.5l3 3 3-3"},
	EdColIzq:          {"colLeft", "M13.5 4h7v16h-7Z M17 4v16 M10.5 12h-7 M6.5 9l-3 3 3 3"},
	EdColDer:          {"colRight", "M3.5 4h7v16h-7Z M7 4v16 M13.5 12h7 M17.5 9l3 3-3 3"},
	EdInmovilizar:     {"freeze", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M4 9.5h16 M9.5 4v16 M5.5 7.5l2.5-2.5 M6 9.4l3.4-3.4"},
	EdCandado:         {"lock", "M8 10V7a4 4 0 0 1 8 0v3M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8ZM12 14.5v2.5"},
	EdEscudo:          {"shield", "M12 3.5 19 6v5c0 4.4-3 8-7 9.5C8 18 5 14.4 5 10V6Z M9.2 11.8l2 2 3.6-3.8"},
	EdObjetivo:        {"target", "M12 20a8 8 0 1 1 0-16 8 8 0 0 1 0 16Z M12 16a4 4 0 1 1 0-8 4 4 0 0 1 0 8Z M12 12.6a.6 .6 0 1 1 0-1.2 .6 .6 0 0 1 0 1.2Z"},
	EdEscenarios:      {"scenarios", "M12 3 3 7.5 12 12 21 7.5Z M3 12 12 16.5 21 12 M3 16.5 12 21 21 16.5"},
	EdTablaDatos:      {"datatable", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M4 9h16 M9 4v16"},
	EdSolver:          {"solver", "M4 7h16 M4 12h16 M4 17h16 M17 7a2 2 0 1 1-4 0 2 2 0 1 1 4 0Z M11 12a2 2 0 1 1-4 0 2 2 0 1 1 4 0Z M18 17a2 2 0 1 1-4 0 2 2 0 1 1 4 0Z"},
	EdAgruparFilas:    {"groupRows", "M8 4H5v16h3 M12 8h8 M12 12h8 M12 16h8"},
	EdDesagruparFilas: {"ungroupRows", "M8 4H5v3 M5 17v3h3 M12 8h8 M12 12h8 M12 16h8"},
	EdAgruparCols:     {"groupCols", "M4 8V5h16v3 M8 12v8 M12 12v8 M16 12v8"},
	EdDesagruparCols:  {"ungroupCols", "M4 8V5h3 M17 5h3v3 M8 12v8 M12 12v8 M16 12v8"},
	EdSubtotal:        {"subtotal", "M17 5H7l5 7-5 7h10"},
	EdSegmentacion:    {"slicer", "M5 4.5h14v15H5Z M5 8.5h14 M8 12h8 M8 15.5h8"},
	EdFiltroQuitar:    {"filterOff", "M4 5h16l-6 6.5V19l-4-2v-5.5Z M4.5 4.5l15 15"},
	EdLibro:           {"book", "M6 4.5A1.5 1.5 0 0 1 7.5 3H18v14H7.5A1.5 1.5 0 0 0 6 18.5Z M6 18.5A1.5 1.5 0 0 0 7.5 20H18 M10 7.5h4"},
	EdCopiar:          {"copy", "M9.5 8h9A1.5 1.5 0 0 1 20 9.5v9a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 8 18.5v-9A1.5 1.5 0 0 1 9.5 8Z M5.5 16H5a1.5 1.5 0 0 1-1.5-1.5v-9A1.5 1.5 0 0 1 5 4h9a1.5 1.5 0 0 1 1.5 1.5V6"},
	EdSuperindice:     {"superscript", "M4 8l6 9 M4 17l6-9 M14.7 6.4c0-1.1.9-1.9 2.1-1.9s2.2.7 2.2 1.9c0 1.9-4.3 2.3-4.3 4.6h4.3"},
	EdVertical:        {"pagePortrait", "M7 3.5h10v17H7Z"},
	EdApaisada:        {"pageLandscape", "M3.5 7h17v10h-17Z"},
	EdDividirCeldas:   {"splitCells", "M6 4h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M12 7.5v9 M10 12H6.8 M8.2 10.5 6.8 12l1.4 1.5 M14 12h3.2 M15.8 10.5 17.2 12l-1.4 1.5"},
	EdNotaPie:         {"footnote", "M6.5 3.5H14l3.5 3.5v13.5h-11Z M14 3.5V7h3.5 M9.5 13h5 M9.5 16h3"},
	EdSubindice:       {"subscript", "M4 6l6 9 M4 15l6-9 M14.7 15.4c0-1.1.9-1.9 2.1-1.9s2.2.7 2.2 1.9c0 1.9-4.3 2.3-4.3 4.6h4.3"},
	EdVideo:           {"video", "M3.5 7.5h10a1.5 1.5 0 0 1 1.5 1.5v6a1.5 1.5 0 0 1-1.5 1.5h-10A1.5 1.5 0 0 1 2 15V9a1.5 1.5 0 0 1 1.5-1.5Z M15 11l5-3v8l-5-3Z"},
	EdAudio:           {"audio", "M4.5 9.5H8l4-3.4v11.8l-4-3.4H4.5Z M15.5 9.5a4 4 0 0 1 0 5 M18 7.2a7.5 7.5 0 0 1 0 9.6"},
	EdRect:            {"rect", "M4.5 7h15v10h-15Z"},
	EdLinea:           {"line", "M5 19 19 5"},
	EdElipse:          {"ellipse", "M3.5 12a8.5 6 0 1 0 17 0 8.5 6 0 1 0-17 0Z"},
	EdTriangulo:       {"triangle", "M12 5 20 19 4 19Z"},
	EdRombo:           {"diamond", "M12 4 20 12 12 20 4 12Z"},
	EdPentagono:       {"pentagon", "M12 3.5 20.1 9.4 17 18.9 7 18.9 3.9 9.4Z"},
	EdHexagono:        {"hexagon", "M20.5 12 16.3 18 7.7 18 3.5 12 7.7 6 16.3 6Z"},
	EdEstrella:        {"star", "M12 3.5 14.1 9.1 20.1 9.4 15.4 13.1 17 18.9 12 15.6 7 18.9 8.6 13.1 3.9 9.4 9.9 9.1Z"},
	EdCilindro:        {"cylinder", "M5 7a7 3 0 1 0 14 0 7 3 0 1 0-14 0 M5 7v10a7 3 0 0 0 14 0V7"},
	EdBocadillo:       {"callout", "M5 4.5h14a1.5 1.5 0 0 1 1.5 1.5v8a1.5 1.5 0 0 1-1.5 1.5h-7l-4 3.5V15.5H5A1.5 1.5 0 0 1 3.5 14V6A1.5 1.5 0 0 1 5 4.5Z"},
	EdConRecto:        {"connStraight", "M8.4 17 15.6 6.9 M5 19a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z M15 5a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z"},
	EdConCodo:         {"connElbow", "M7 17.2V12h9.8V6.8 M5 19a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z M15 5a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z"},
	EdConCurvo:        {"connCurved", "M7 17.2C7 10 17 14 17 6.8 M5 19a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z M15 5a2 2 0 1 0 4 0 2 2 0 1 0-4 0Z"},
	EdSmartLista:      {"smartList", "M4 5h16v3.2H4Z M4 10.4h16v3.2H4Z M4 15.8h16v3.2H4Z"},
	EdSmartProceso:    {"smartProcess", "M3.5 9h5.5v6H3.5Z M15 9h5.5v6H15Z M10 12h3.8 M12.3 10.3 14.1 12l-1.8 1.8"},
	EdSmartCiclo:      {"smartCycle", "M18.5 12a6.5 6.5 0 1 1-1.9-4.6 M18.6 5v3.4h-3.4"},
	EdSmartJerarquia:  {"smartHierarchy", "M9.5 4h5v3.4h-5Z M12 7.4v4.1 M6.8 11.5h10.2 M6.8 11.5v1.1 M17 11.5v1.1 M3.8 12.6h6v3.4h-6Z M14 12.6h6v3.4h-6Z"},
	EdSimbolo:         {"symbol", "M6.5 19.5h4v-2.6a5.8 5.8 0 1 1 3 0v2.6h4"},
	EdNumeroPagina:    {"pageNumber", "M5.5 3.5h13a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1Z M10.4 11l-.8 5 M13.6 11l-.8 5 M9 12.6h5 M8.6 14.4h5"},
	EdSaltoPagina:     {"pageBreak", "M5 8.5V5a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3.5 M5 15.5V19a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-3.5 M4 12h2.8 M10.6 12h2.8 M17.2 12h2.8"},
	EdEncabezado:      {"header", "M5.5 3.5h13a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1Z M4.5 8.5h15 M8 6h8"},
	EdPie:             {"footer", "M5.5 3.5h13a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1Z M4.5 15.5h15 M8 18h8"},
	EdRecorte:         {"crop", "M8 2v14h14 M2 8h14v14"},
	EdNotaFinal:       {"endnote", "M6.5 3.5H13l3.5 3.5v13.5h-11Z M13 3.5V7h3.5 M11.5 9.5v4 M9.7 11.7 11.5 13.5 13.3 11.7 M9 16.5h5"},
	EdEntradaIndice:   {"indexEntry", "M7 4h10v16l-5-4-5 4Z"},
	EdIndiceAlfa:      {"indexAlpha", "M5 19 8 5.5 11 19 M6 14.5h4 M14 8h6 M14 13h6 M14 18h4"},
	EdLeyenda:         {"caption", "M4.5 5h15v9.5h-15Z M4.5 18h10"},
	EdCita:            {"citation", "M7.5 8a2 2 0 0 0-2 2 2 2 0 0 0 2 2c0 1.4-.7 2.4-2 3 M16 8a2 2 0 0 0-2 2 2 2 0 0 0 2 2c0 1.4-.7 2.4-2 3"},
	EdBibliografia:    {"bibliography", "M12 6.5C10.5 5.5 8 5 5.5 5.5v11c2.5-.5 5 0 6.5 1 M12 6.5c1.5-1 4-1.5 6.5-1v11c-2.5-.5-5 0-6.5 1 M12 6.5v11"},
	EdCampoCorresp:    {"mergeField", "M10.5 8 7 12l3.5 4 M13.5 8 17 12l-3.5 4"},
	EdPrevia:          {"preview", "M2.5 12c2.4-4 5.7-6 9.5-6s7.1 2 9.5 6c-2.4 4-5.7 6-9.5 6s-7.1-2-9.5-6Z M12 15a3 3 0 1 1 0-6 3 3 0 0 1 0 6Z"},
	EdCorrespondencia: {"mailMerge", "M3.5 6.5h17v11h-17Z M4 7l8 6 8-6"},
	EdMarcaAgua:       {"watermark", "M5.5 4.5h9L18.5 8.5v11a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1v-14a1 1 0 0 1 1-1Z M14.5 4.5V8.5h4 M8 17l1.4-5 1.6 3 1.6-3 1.4 5"},
	EdBordePagina:     {"pageBorder", "M5 3.5h14a1.5 1.5 0 0 1 1.5 1.5v14a1.5 1.5 0 0 1-1.5 1.5H5a1.5 1.5 0 0 1-1.5-1.5V5A1.5 1.5 0 0 1 5 3.5Z M8 7h8 M8 17h8 M8 7v10 M16 7v10"},
	EdRefCruzada:      {"xref", "M8.5 5.5H6a1.5 1.5 0 0 0-1.5 1.5v10A1.5 1.5 0 0 0 6 18.5h2.5 M15.5 5.5H18a1.5 1.5 0 0 1 1.5 1.5v10a1.5 1.5 0 0 1-1.5 1.5h-2.5 M8.5 12h6 M12 9.5l3 2.5-3 2.5"},
}

// Clave es el nombre del icono en `editor/icons.tsx`.
func (i Ed) Clave() string { return catalogoEd[i].clave }

// String hace legible el icono en los fallos de los tests.
func (i Ed) String() string {
	if int(i) >= len(catalogoEd) {
		return fmt.Sprintf("Ed(%d)", uint8(i))
	}
	if i == EdNinguno {
		return "EdNinguno"
	}
	return catalogoEd[i].clave
}

// TrazoEd es TrazoConTinta para el catálogo del editor: el token se resuelve al
// construir, no al pintar.
func TrazoEd(i Ed, lado float32, token string, v tema.Variante) *Dibujo {
	return TrazoEdConTinta(i, lado, tema.Color(token, v))
}

// TrazoEdConTinta es TrazoEd con un color ya calculado.
func TrazoEdConTinta(i Ed, lado float32, c color.NRGBA) *Dibujo {
	d := catalogoEd[i].d
	if d == "" {
		panic(fmt.Sprintf("iconos: %s no tiene ruta en el catálogo del editor (¿EdNinguno?)", i))
	}
	return nuevoDibujo(lado, c, func(p pintura) []byte { return svgDeTrazo(d, p) })
}
