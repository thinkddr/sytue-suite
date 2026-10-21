// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import "fyne.io/fyne/v2"

// Copiar lleva un valor al portapapeles y dice si HA LLEGADO, que es lo que la web sabe
// por el rechazo de `navigator.clipboard.writeText` y enseña como «No se ha podido
// copiar».
//
// `fyne.Clipboard.SetContent` no devuelve error en ningún sistema: el driver de
// escritorio solo lo reintenta en Windows —y leyendo de vuelta, que es esto mismo— y
// después lo registra; el de Android pasa el texto por JNI sin mirar qué contesta. Pero
// sí se puede SABER en todos, leyéndolo de vuelta: en los cinco destinos leer lo que la
// propia app acaba de poner es síncrono —en X11 y en Wayland la app es la dueña de la
// selección y GLFW contesta sin viajar al servidor—, y si lo que hay no es lo que se ha
// puesto, no se ha copiado.
//
// ponytail: si otro programa cambia el portapapeles justo entre las dos llamadas, sale
// «no se ha podido copiar» de más. La ventana es de microsegundos; si pasara, el aviso
// pide repetir, que es inofensivo.
func Copiar(c fyne.Clipboard, valor string) bool {
	if c == nil {
		return false
	}
	c.SetContent(valor)
	return c.Content() == valor
}
