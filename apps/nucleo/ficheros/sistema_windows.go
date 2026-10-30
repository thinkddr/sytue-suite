// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

import (
	"context"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Carpeta es la de descargas que tenga CONFIGURADA el perfil, preguntándosela al sistema
// (FOLDERID_Downloads). Componer «%USERPROFILE%\Downloads» falla en cuanto alguien la
// mueve a otro disco o la redirige por directiva, que en un puesto de empresa es
// justamente lo normal; ahí se descargaría a una carpeta que no mira nadie.
//
// Fallback si el sistema no contesta: la ruta compuesta, que es donde está el 90 % de
// las veces.
func Carpeta() (string, error) {
	if ruta, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DEFAULT); err == nil && ruta != "" {
		return ruta, nil
	}
	casa, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(casa, "Downloads"), nil
}

// Revelar abre el Explorador con el fichero ya seleccionado. `/select,<ruta>` va como UN
// solo argumento —así lo quiere explorer.exe— y por eso no se separa en dos.
//
// explorer.exe devuelve un código de salida distinto de cero aunque le haya salido bien,
// pero aquí no se espera a que termine, así que da igual.
func Revelar(ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	return lanzar("explorer.exe", "/select,"+abs)
}

// abrirConContexto entrega el fichero a la aplicación asociada a su extensión en el registro: es el
// doble clic del Explorador, y quien elige es la configuración de la persona, no Sytue.
func abrirConContexto(_ context.Context, ruta string) error { return porShell(ruta, "open") }

// Imprimir usa el verbo `print` del shell: Windows se lo pasa al manejador registrado
// para ese tipo de fichero, que enseña SU diálogo de impresión —el nativo, con las
// impresoras del equipo—. Es el único de los tres sistemas donde imprimir es una
// operación del escritorio y no «ábrelo en el visor y pulsa Ctrl+P».
//
// Fallback: un tipo de fichero sin verbo `print` registrado devuelve error, y quien llama
// cae en Abrir, que es lo mismo que hacen macOS y Linux.
func Imprimir(ruta string) error { return porShell(ruta, "print") }

func porShell(ruta, verbo string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	verboUTF16, err := windows.UTF16PtrFromString(verbo)
	if err != nil {
		return err
	}
	ficheroUTF16, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}
	// SW_SHOWNORMAL: el visor sale a la vista. Un diálogo de impresión escondido detrás
	// de la ventana es un «no ha pasado nada» para quien mira.
	return windows.ShellExecute(0, verboUTF16, ficheroUTF16, nil, nil, windows.SW_SHOWNORMAL)
}

// elegirWindows es el `OpenFileDialog` de Windows Forms con `Multiselect`: el diálogo
// de abrir del Explorador. `-STA` porque los diálogos comunes lo exigen.
const elegirWindows = `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Multiselect = $true
if ($d.ShowDialog() -eq 'OK') { $d.FileNames -join "` + "`" + `n" }`

// Elegir abre el diálogo de abrir del sistema, con selección múltiple, y devuelve las
// rutas elegidas. Cancelar devuelve nada, sin error. BLOQUEA: desde una rutina propia.
func Elegir() ([]string, error) {
	return elegirCon("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", elegirWindows)
}
