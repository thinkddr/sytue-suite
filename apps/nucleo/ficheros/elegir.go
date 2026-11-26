// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ExtensionesImagen son las que el selector de fotos deja elegir: las que syphotos
// indexa. Es el `accept="image/*"` de la web traído a extensiones, que es lo que
// entienden los selectores de los tres sistemas.
var ExtensionesImagen = []string{"jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif", "tif", "tiff", "bmp"}

// EnElMovil es el selector de ficheros donde el sistema no tiene un programa que lo abra:
// en el móvil lo da el toolkit, y lo pone `apps/fyne/anfitrion` al arrancar (por `sesion`
// en la edición conectada y por `local.Abrir` en Sytue Office; en iOS, movil_ios.go). Recibe las
// extensiones que se aceptan (nil, cualquiera) y devuelve rutas de COPIAS en la carpeta
// temporal de la app: el selector del móvil da `content://`, y quien llama a [Elegir] y
// a [ElegirImagenes] espera rutas. Cancelar devuelve nil, nil.
var EnElMovil func(extensiones []string) ([]string, error)

// EnFotos es el selector de la FOTOTECA del sistema donde la hay y es otro que el de
// ficheros: en iOS, el de la app Fotos (PHPicker) y no el de «Archivos». Lo usan
// [ElegirImagenes] y [ElegirFotosYVideos]; recibe las extensiones pedidas (todas de
// imagen o de vídeo) y ofrece también los vídeos si alguna es de vídeo. Devuelve rutas
// de copias, como [EnElMovil]. nil: no lo hay.
var EnFotos func(extensiones []string) ([]string, error)

// panelPropio, si está, es el panel de abrir del sistema DENTRO de este proceso. Solo lo
// pone la variante de la Mac App Store (elegir_tienda_darwin.go): con el App Sandbox, el
// permiso sobre lo elegido es de quien enseña el panel, y un `osascript` hijo se lo
// quedaría sin poder dárselo a la app.
var panelPropio func(titulo string, extensiones []string) ([]string, error)

// porPortalElegir es el selector de repuesto cuando no hay programa que lanzar: en Linux,
// el portal de escritorio de XDG (sistema_linux.go). Devuelve ErrNoDisponible si tampoco
// lo hay. nil donde no existe.
var porPortalElegir func(titulo string, extensiones []string) ([]string, error)

// ElegirImagenes abre el selector de ficheros DEL SISTEMA, con varios a la vez, y
// devuelve las rutas elegidas. Cancelar no es un error: devuelve nil, nil.
//
// Es el `<input type="file" multiple accept="image/*">` de la web. No es el diálogo de
// Fyne a propósito: ese trae su propio aspecto —que no es ni el de la web ni el del
// sistema—, elige un fichero cada vez y además arrastra una dependencia nueva de fuera
// de la UE (github.com/FyshOS) solo por abrirse.
//
// En el móvil es [EnElMovil]. Donde no hay selector (el navegador, un Linux sin zenity,
// kdialog ni portal de escritorio), devuelve ErrNoDisponible y la app lo trata como «no se ofrece».
//
// Donde el sistema tiene un selector propio de la fototeca ([EnFotos], iOS), es ese: ahí
// están las fotos, con sus álbumes, y no en «Archivos».
func ElegirImagenes(titulo string) ([]string, error) {
	return ElegirFotosYVideos(titulo, ExtensionesImagen...)
}

// ElegirFotosYVideos es [ElegirImagenes] con las extensiones que se pidan, de imagen y de
// vídeo (Photos importa vídeos). Si todas lo son y el sistema tiene selector de la
// fototeca ([EnFotos], iOS), es ese, con los vídeos si se piden; si no, el de ficheros
// filtrado por ellas ([ElegirConTipos]).
func ElegirFotosYVideos(titulo string, extensiones ...string) ([]string, error) {
	if EnFotos != nil && soloFotosYVideos(extensiones) {
		return EnFotos(extensiones)
	}
	return ElegirConTipos(titulo, extensiones...)
}

// ExtensionesVideo son las de vídeo que el selector de la fototeca entiende como tales.
var ExtensionesVideo = []string{"mp4", "m4v", "mov", "mkv", "webm", "avi", "mpg", "mpeg", "ogv", "3gp", "3g2"}

// soloFotosYVideos dice si todas las extensiones son de imagen o de vídeo (y hay alguna).
func soloFotosYVideos(extensiones []string) bool {
	for _, e := range extensiones {
		e = strings.ToLower(strings.TrimPrefix(e, "."))
		if !slices.Contains(ExtensionesImagen, e) && !slices.Contains(ExtensionesVideo, e) {
			return false
		}
	}
	return len(extensiones) > 0
}

// ConVideos dice si alguna de las extensiones es de vídeo: el selector de la fototeca
// ofrece entonces también los vídeos.
func ConVideos(extensiones []string) bool {
	for _, e := range extensiones {
		if slices.Contains(ExtensionesVideo, strings.ToLower(strings.TrimPrefix(e, "."))) {
			return true
		}
	}
	return false
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
	if errors.Is(err, ErrNoDisponible) && porPortalElegir != nil {
		return porPortalElegir(titulo, extensiones)
	}
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

// panelCarpetaPropio es [panelPropio] para elegir una carpeta: solo la variante de la
// Mac App Store, por lo mismo (el permiso del App Sandbox es de quien enseña el panel).
var panelCarpetaPropio func(titulo string) (string, error)

// porPortalCarpeta es [porPortalElegir] para una carpeta: el portal de XDG en Linux.
var porPortalCarpeta func(titulo string) (string, error)

// ElegirCarpeta abre el selector de CARPETAS del sistema y devuelve la ruta elegida, sin
// barra final. Cancelar no es un error: devuelve "", nil.
//
// Es el `<input type="file" webkitdirectory>` de la web: subir una carpeta entera sin
// tener que arrastrarla, que en un portátil sin ratón cuesta. `choose folder` en macOS,
// el FolderBrowserDialog de Windows, zenity o kdialog en Linux —y si no, el portal—. En
// el móvil devuelve ErrNoDisponible y la app no lo ofrece: el selector del sistema elige
// ficheros, no carpetas que recorrer.
func ElegirCarpeta(titulo string) (string, error) {
	if panelCarpetaPropio != nil {
		return panelCarpetaPropio(titulo)
	}
	orden, args, err := ordenElegirCarpeta(titulo)
	if errors.Is(err, ErrNoDisponible) && porPortalCarpeta != nil {
		return porPortalCarpeta(titulo)
	}
	if err != nil {
		return "", err
	}
	salida, err := exec.CommandContext(context.Background(), orden, args...).Output() //nolint:gosec // G204: la orden es fija por sistema y el título no pasa por un intérprete
	var salidaMala *exec.ExitError
	if errors.As(err, &salidaMala) {
		return "", nil // cancelar
	}
	if err != nil {
		return "", err
	}
	rutas := rutasDe(string(salida))
	if len(rutas) == 0 {
		return "", nil
	}
	// `POSIX path of` acaba en «/»: sin limpiarla, quien la recorra tomaría la propia
	// carpeta por su padre y el árbol subiría sin su nombre arriba.
	return filepath.Clean(rutas[0]), nil
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

// cadenaPowerShell codifica un texto en Base64 para pasárselo a PowerShell sin abrir
// un literal entre comillas: PowerShell cierra una cadena entre comillas simples también
// con las tipográficas (’ ‘ ‚ ‛), así que un título traducido como «'L’app'» o una ruta
// temporal con apóstrofo tipográfico rompía el guion (FYS-12).
func cadenaPowerShell(s string) string {
	return "[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(s)) + "'))"
}

func guionElegirWindows(titulo string, extensiones []string) string {
	patrones := make([]string, len(extensiones))
	for i, e := range extensiones {
		patrones[i] = "*." + e
	}
	return "Add-Type -AssemblyName System.Windows.Forms;" +
		"$d = New-Object System.Windows.Forms.OpenFileDialog;" +
		"$d.Multiselect = $true;" +
		"$d.Title = " + cadenaPowerShell(titulo) + ";" +
		"$d.Filter = " + cadenaPowerShell(titulo+"|"+strings.Join(patrones, ";")) + ";" +
		"if ($d.ShowDialog() -eq 'OK') { $d.FileNames -join \"`n\" } else { exit 1 }"
}

func guionCopiarImagenWindows(ruta string) string {
	return "Add-Type -AssemblyName System.Windows.Forms, System.Drawing;" +
		"[System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile(" + cadenaPowerShell(ruta) + "))"
}

// guionCarpetaWindows es el FolderBrowserDialog: el «Buscar carpeta» del sistema. El
// título va en Base64, como en [guionElegirWindows] (FYS-12).
func guionCarpetaWindows(titulo string) string {
	return "Add-Type -AssemblyName System.Windows.Forms;" +
		"$d = New-Object System.Windows.Forms.FolderBrowserDialog;" +
		"$d.Description = " + cadenaPowerShell(titulo) + ";" +
		"$d.ShowNewFolderButton = $false;" +
		"if ($d.ShowDialog() -eq 'OK') { $d.SelectedPath } else { exit 1 }"
}

// guionCarpetaApple es `choose folder` de AppleScript: el panel NATIVO del Finder para
// una carpeta, que devuelve su ruta POSIX. Cancelar sale con error (-128), que es «nada».
func guionCarpetaApple(titulo string) string {
	return fmt.Sprintf("return POSIX path of (choose folder with prompt %q)", titulo)
}
