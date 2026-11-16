// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// Los glifos PROPIOS de Sytue Drive que el catálogo compartido no trae, copiados de
// `apps/frontend/apps/drive/src/components/glyphs.tsx`.
//
// Seis ya están en `iconos` (carpeta, fichero, los tres de Docs y la estrella) y se
// piden por su constante. Estos se quedan aquí por lo mismo que los de Rooms: el
// catálogo es el espejo de UN fichero y lo están ampliando varias apps a la vez. Se
// guarda el CUERPO del `<svg>` porque la mitad no son un solo camino —el «⋯» son tres
// círculos, la copia un `<rect>` y un camino— y `iconos.TrazoDeCuerpo` los pinta tal
// cual.
//
// `TestLosGlifosSonLosDeLaWeb` los compara con el `.tsx`, uno a uno.
const (
	glifoMas     = `<circle cx="12" cy="5" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="12" cy="19" r="1" />`
	glifoLapiz   = `<path d="M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />`
	glifoCopia   = `<rect x="9" y="9" width="12" height="12" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />`
	glifoMover   = `<path d="M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17" /><path d="M9 15h6M12.5 12.5 15 15l-2.5 2.5" />`
	glifoCarpeta = `<path d="M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17" />`
)

// Los que Drive pide por un nombre que también es del catálogo COMPARTIDO. Se
// dibujaban aquí con OTRO trazado —la papelera, la descarga, el enlace, el candado,
// las personas, varios con la geometría de Feather—, así que la misma acción se veía
// distinta en Drive que en el resto de la suite. Ahora la web los pinta con `Icon` y
// aquí salen de `iconos`, envueltos como cuerpo para que las pantallas, que mezclan
// estos con los propios de varias figuras, no cambien.
var (
	glifoSubir     = cuerpoComun(iconos.Subir)     // `upload`
	glifoDescargar = cuerpoComun(iconos.Descargar) // `download`
	glifoEnlace    = cuerpoComun(iconos.Enlace)    // `link`
	glifoReloj     = cuerpoComun(iconos.Reloj)     // `clock`
	glifoPapelera  = cuerpoComun(iconos.Papelera)  // `trash`
	glifoCorreo    = cuerpoComun(iconos.Correo)    // `mail`
	glifoUsuarios  = cuerpoComun(iconos.Usuarios)  // `users`
	glifoCandado   = cuerpoComun(iconos.Candado)   // `lock`
)

// cuerpoComun es la ruta de un icono del catálogo compartido como un único `<path>`.
func cuerpoComun(i iconos.Icono) string { return `<path d="` + i.Ruta() + `" />` }

// glifosWeb es el nombre de cada uno en `glyphs.tsx`, que es por el que el guardián lo
// busca allí.
var glifosWeb = map[string]string{
	"more": glifoMas, "pencil": glifoLapiz, "copy": glifoCopia, "folderMove": glifoMover,
	"folder": glifoCarpeta,
}

// glifoCon pinta uno de estos con un color ya calculado: el del rótulo de un botón, que
// puede ir atenuado.
func glifoCon(cuerpo string, lado float32, c color.NRGBA) *iconos.Dibujo {
	return iconos.TrazoDeCuerpo(cuerpo, lado, c)
}
