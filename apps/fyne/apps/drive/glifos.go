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
// NO son los de `Icon.tsx` aunque se llamen igual: `download`, `upload` y `trash` de
// Drive tienen otro dibujo (el cajón con la flecha, no la línea de base). Confundirlos
// es un píxel que sí se ve.
//
// `TestLosGlifosSonLosDeLaWeb` los compara con el `.tsx`, uno a uno.
const (
	glifoSubir     = `<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12" />`
	glifoDescargar = `<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3" />`
	glifoMas       = `<circle cx="12" cy="5" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="12" cy="19" r="1" />`
	glifoLapiz     = `<path d="M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />`
	glifoCopia     = `<rect x="9" y="9" width="12" height="12" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />`
	glifoMover     = `<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" /><path d="M9 14h6m-2.5-2.5L15 14l-2.5 2.5" />`
	glifoEnlace    = `<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />`
	glifoReloj     = `<path d="M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18ZM12 7v5l3 2" />`
	glifoPapelera  = `<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M10 11v6M14 11v6" />`
	glifoCorreo    = `<path d="M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Zm-1 1.5 9 6 9-6" />`
	glifoUsuarios  = `<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm7-7.87a4 4 0 0 1 0 7.74M22 21v-2a4 4 0 0 0-3-3.87" />`
	glifoCandado   = `<rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />`
)

// glifosWeb es el nombre de cada uno en `glyphs.tsx`, que es por el que el guardián lo
// busca allí.
var glifosWeb = map[string]string{
	"upload": glifoSubir, "download": glifoDescargar, "more": glifoMas, "pencil": glifoLapiz,
	"copy": glifoCopia, "folderMove": glifoMover, "link": glifoEnlace, "clock": glifoReloj,
	"trash": glifoPapelera, "mail": glifoCorreo, "users": glifoUsuarios, "lock": glifoCandado,
}

// glifoCon pinta uno de estos con un color ya calculado: el del rótulo de un botón, que
// puede ir atenuado.
func glifoCon(cuerpo string, lado float32, c color.NRGBA) *iconos.Dibujo {
	return iconos.TrazoDeCuerpo(cuerpo, lado, c)
}
