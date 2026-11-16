// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los glifos PROPIOS de Sytue Rooms, copiados de
// `apps/frontend/apps/rooms/src/components/glyphs.tsx`.
//
// Están aquí y no en `iconos` por lo que dice el comentario de `iconos.TrazoDeRuta`:
// el catálogo compartido es el espejo EXACTO de `Icon.tsx` y se compara en los dos
// sentidos, así que cada app que metiera los suyos tendría que tocar una lista que
// varias apps están portando a la vez. La maquinaria —rasterizado a píxeles de
// dispositivo, las correcciones de `oksvg`, el teñido sin rehacer el mapa de bits— sí
// es compartida, que es lo que había que no copiar.
//
// Se guarda el CUERPO del `<svg>` y no una `d` porque la mitad no son un solo camino:
// la cámara lleva un `<rect>`, el reloj y la cara un `<circle>` y el «…» tres. Un
// `rect rx="2"` convertido a mano a arcos es el redibujado a ojo que el guardián
// existe para impedir, así que se copia tal cual y lo pinta `iconos.TrazoDeCuerpo`.
//
// `TestLosGlifosSonLosDeLaWeb` los compara con ese `.tsx`, forma a forma.
const (
	// glifoMas es el «…» del menú de acciones de un mensaje. No es el `ellipsis` del
	// catálogo compartido: aquel son tres puntos de camino y este tres círculos.
	glifoMas = `<circle cx="5" cy="12" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" />`
	// glifoArchivo es «Archivar» y la marca de las filas de «Archivados».
	glifoArchivo = `<path d="M3 5h18v4H3z" /><path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9" /><path d="M10 13h4" />`
	// glifoChincheta es «Fijar».
	glifoChincheta = `<path d="M9 4h6l-1 6 3 3v2H7v-2l3-3-1-6Z" /><path d="M12 15v5" />`
	// glifoMarcador es «Guardado».
	glifoMarcador = `<path d="M6 4h12a1 1 0 0 1 1 1v15l-7-4-7 4V5a1 1 0 0 1 1-1Z" />`
	// glifoVideo es la cámara: «Llamar» y la tarjeta de llamada empezada.
	glifoVideo = `<path d="M23 7 16 12l7 5V7Z" /><rect x="1" y="5" width="15" height="14" rx="2" />`
	// glifoLapiz es «Borradores» y «Editar».
	glifoLapiz = `<path d="M12 20h9" /><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />`
	// glifoHilos es la burbuja con líneas: la vista global de «Hilos».
	glifoHilos = `<path d="M21 11.5a8.38 8.38 0 0 1-8.5 8.5 9.5 9.5 0 0 1-4-.9L3 21l1.9-5.5A8.38 8.38 0 0 1 4 11.5 8.5 8.5 0 0 1 12.5 3 8.38 8.38 0 0 1 21 11.5Z" /><path d="M8.5 10h7M8.5 13.5h4.5" />`
	// glifoPapelera es «Descartar el borrador».
	glifoPapelera = `<path d="M3 6h18" /><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" /><path d="M10 11v6M14 11v6" />`
	// glifoReloj es «Mensajes programados».
	glifoReloj = `<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3.5 2" />`
	// glifoClip es «Archivos del canal».
	glifoClip = `<path d="M6.75 10v5a5.25 5.25 0 0 0 10.5 0V7a3.5 3.5 0 0 0-7 0v8.5a1.75 1.75 0 0 0 3.5 0V9" />`
	// glifoPrioridad es el «!» en cuadro: prioridad y acuse de recibo.
	glifoPrioridad = `<rect x="3.5" y="3.5" width="17" height="17" rx="4" /><path d="M12 8v4.5" /><path d="M12 16h.01" />`
	// glifoEmoji es la cara del selector de emoji.
	glifoEmoji = `<circle cx="12" cy="12" r="9" /><path d="M8.5 14a4 4 0 0 0 7 0" /><path d="M9 9h.01M15 9h.01" />`
	// glifoEnviar es el avión de papel. El chevron queda para navegar, no para enviar.
	glifoEnviar = `<path d="M21 3 14.5 21 11 13 3 9.5Z" /><path d="M21 3 11 13" />`
	// glifoEnlace es el eslabón de «Copiar el enlace».
	glifoEnlace = `<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1" /><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1" />`
	// glifoCopiar es «Copiar el texto».
	glifoCopiar = `<rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2" />`
	// glifoNota es «Guardar en Sytue Notepads».
	glifoNota = `<path d="M7 4h9a1.5 1.5 0 0 1 1.5 1.5v7L12 18H7a1.5 1.5 0 0 1-1.5-1.5v-11A1.5 1.5 0 0 1 7 4Z" /><path d="M17.5 12.5h-3.9A1.6 1.6 0 0 0 12 14.1V18" />`
	// glifoReenviar es «Reenviar…».
	glifoReenviar = `<path d="M14.5 6l5 5-5 5M19.5 11H10a5.5 5.5 0 0 0-5.5 5.5V18" />`
	// glifoNoLeido es «Marcar como no leído»: el sobre con su punto RELLENO, que se tiñe
	// al pintarlo (ver `dibujoRelleno`).
	glifoNoLeido = `<path d="M4 8a2 2 0 0 1 2-2h9M4 8v9a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6" /><path d="M4 8l8 5 3-2" /><circle cx="19" cy="6" r="2.4" fill="currentColor" stroke="none" />`
	// glifoVisto es «Visto por»: el doble tic.
	glifoVisto = `<path d="m2 12 4 4 8-9" /><path d="m10 16 1.5 1.5L22 7" />`
	// glifoImagen es `ImageGlyph`, el del «GIF» del menú «+».
	glifoImagen = `<path d="M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Z" /><path d="M3 16l5-5 4 4 3-3 6 6" /><path d="M9 9.5h.01" />`
	// glifoEstrella es «Favorito». Se pinta relleno cuando ya lo es, que es el
	// `fill={filled ? 'currentColor' : 'none'}` del mismo dibujo.
	glifoEstrella = `<path d="` + rutaEstrella + `" />`
)

// rutaEstrella es la `d` de la estrella suelta, porque la de un favorito se pinta
// RELLENA (`fill="currentColor"`) y eso lo hace `iconos.TrazoDeRutaRelleno`, que pide
// la ruta y no el cuerpo.
const rutaEstrella = "M12 3.6L14.4 9.3L20.6 9.8L15.9 13.9L17.3 19.9L12 16.7L6.7 19.9L8.1 13.9L3.4 9.8L9.6 9.3Z"

// La antesala de la llamada NO lleva iconos, y no es un olvido: `PreJoin.tsx` pone
// BOTONES CON TEXTO —«Micrófono activado» / «Cámara desactivada»— porque ahí el rótulo
// dice el ESTADO con el que se entra, no la acción. Un icono de micrófono tachado no
// distingue «está apagado» de «apágalo».

// glifoLlamadaFin es la cámara TACHADA de una llamada terminada. Vive en
// `MessageView.tsx` como SVG en línea y no en `glyphs.tsx`; su guardián comprueba que
// los tres caminos siguen ahí.
const glifoLlamadaFin = `<path d="m1 1 22 22" /><path d="M23 7 16 12l3.5 2.5V7Z" />` +
	`<path d="M14 5h-9a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h9" />`

// glifoSaltarNoLeido es la flecha hacia arriba de «Saltar a no leído»: el `<Glifo
// d="M12 19V5M6 11l6-6 6 6" size={14} />` en línea de MessageView.tsx.
const glifoSaltarNoLeido = `<path d="M12 19V5M6 11l6-6 6 6" />`

// dibujo es un glifo propio ya teñido con un token, que es como lo piden las pantallas
// y lo que esperan `ui.Boton.Glifo` y `ui.BotonIcono.Glifo`.
func dibujo(cuerpo string, lado float32, token string, v tema.Variante) *iconos.Dibujo {
	return iconos.TrazoDeCuerpo(cuerpo, lado, tema.Color(token, v)).DeToken(token)
}

// dibujoRelleno es dibujo para un cuerpo con partes `fill="currentColor"`: el trazo
// hereda el color del grupo, pero un relleno con `currentColor` no tiene de dónde
// sacarlo, así que se escribe el del token antes de pintarlo. No se vuelve a teñir: es
// para las entradas de un menú, que se construyen al abrirlo.
func dibujoRelleno(cuerpo string, lado float32, token string, v tema.Variante) *iconos.Dibujo {
	c := tema.Color(token, v)
	hex := fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	return iconos.TrazoDeCuerpo(strings.ReplaceAll(cuerpo, `fill="currentColor"`, `fill="`+hex+`"`), lado, c)
}
