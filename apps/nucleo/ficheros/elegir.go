// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// ExtensionesImagen son las que el selector de fotos deja elegir: las que syphotos
// indexa. Es el `accept="image/*"` de la web traído a extensiones, que es lo que
// entienden los selectores de los tres sistemas.
var ExtensionesImagen = []string{"jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif", "tif", "tiff", "bmp"}

// EnElMovil es el selector de ficheros donde el sistema no tiene un programa que lo abra:
// en el móvil lo da el toolkit, y lo pone `apps/fyne/sesion` al arrancar. Recibe las
// extensiones que se aceptan (nil, cualquiera) y devuelve rutas de COPIAS en la carpeta
// temporal de la app: el selector del móvil da `content://`, y quien llama a [Elegir] y
// a [ElegirImagenes] espera rutas. Cancelar devuelve nil, nil.
var EnElMovil func(extensiones []string) ([]string, error)

// panelPropio, si está, es el panel de abrir del sistema DENTRO de este proceso. Solo lo
// pone la variante de la Mac App Store (elegir_tienda_darwin.go): con el App Sandbox, el
// permiso sobre lo elegido es de quien enseña el panel, y un `osascript` hijo se lo
// quedaría sin poder dárselo a la app.
var panelPropio func(titulo string, extensiones []string) ([]string, error)

// ElegirImagenes abre el selector de ficheros DEL SISTEMA, con varios a la vez, y
// devuelve las rutas elegidas. Cancelar no es un error: devuelve nil, nil.
//
// Es el `<input type="file" multiple accept="image/*">` de la web. No es el diálogo de
// Fyne a propósito: ese trae su propio aspecto —que no es ni el de la web ni el del
// sistema—, elige un fichero cada vez y además arrastra una dependencia nueva de fuera
// de la UE (github.com/FyshOS) solo por abrirse.
//
// En el móvil es [EnElMovil]. Donde no hay selector (el navegador, un Linux sin zenity
// ni kdialog), devuelve ErrNoDisponible y la app lo trata como «no se ofrece».
func ElegirImagenes(titulo string) ([]string, error) {
	return ElegirConTipos(titulo, ExtensionesImagen...)
}

// ElegirConTipos es el selector del sistema filtrado por extensión (sin el punto): el
// `accept` de un `<input type="file">` —`.xfdf`, `.p12,.pfx`, `image/png,image/jpeg`—
// traído a lo que entienden los selectores de los tres sistemas y al del móvil.
// `titulo` es lo que el diálogo dice arriba y, en Linux y Windows, el nombre del filtro.
func ElegirConTipos(titulo string, extensiones ...string) ([]string, error) {
	if panelPropio != nil {
		return panelPropio(titulo, extensiones)
	}
	orden, args, err := ordenElegir(titulo, extensiones)
	if errors.Is(err, ErrNoDisponible) && EnElMovil != nil {
		return EnElMovil(extensiones)
	}
	if err != nil {
		return nil, err
	}
	// Sin plazo: la persona puede tardar lo que quiera en elegir.
	salida, err := exec.CommandContext(context.Background(), orden, args...).Output() //nolint:gosec // G204: la orden es fija por sistema y los argumentos no llevan nada de fuera salvo el título, que no pasa por un intérprete
	var salidaMala *exec.ExitError
	if errors.As(err, &salidaMala) {
		// Los tres selectores salen con código distinto de cero al CANCELAR.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rutasDe(string(salida)), nil
}

// rutasDe parte la salida del selector: una ruta por línea, sin las vacías.
func rutasDe(salida string) []string {
	var rutas []string
	for linea := range strings.Lines(salida) {
		if r := strings.TrimRight(linea, "\r\n"); rutaElegida(r) {
			rutas = append(rutas, r)
		}
	}
	return rutas
}
