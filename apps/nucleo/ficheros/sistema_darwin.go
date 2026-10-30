// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package ficheros

import (
	"context"
	"os"
	"path/filepath"
)

// Carpeta es «~/Downloads». En macOS esa ruta NO se traduce —lo que se traduce es cómo la
// enseña el Finder, con un .localized dentro—, así que aquí no hay nada que consultar:
// componerla es la respuesta correcta, no un atajo.
func Carpeta() (string, error) {
	casa, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(casa, "Downloads"), nil
}

// Revelar abre el Finder con el fichero ya seleccionado (`open -R`). Es parte del sistema:
// no hay caso de fallback que valga la pena, y si `open` fallara vuelve su error.
func Revelar(ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	return lanzar("open", "-R", abs)
}

// abrirConContexto entrega el fichero a la aplicación que LaunchServices tenga asociada a su tipo: es
// el doble clic del Finder, y quien elige es la configuración de la persona, no Sytue.
func abrirConContexto(ctx context.Context, ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	return lanzarConContexto(ctx, "open", abs)
}

// Imprimir entrega el fichero a la cadena de impresión del sistema.
//
// macOS no expone el diálogo de impresión a un proceso que no sea una app Cocoa con su
// NSPrintOperation, así que el FALLBACK —y aquí es el camino normal— es abrirlo en el
// visor asociado (Vista Previa para un PDF), donde ⌘P es el diálogo NATIVO con las
// impresoras que la persona ya tiene. Mandarlo con `lp` a la impresora por defecto sin
// preguntar sería peor: imprimir es irreversible y gasta papel de alguien.
func Imprimir(ruta string) error { return Abrir(ruta) }

// elegirApple es el `choose file` de AppleScript con selección múltiple: el panel de
// abrir del Finder, el mismo que abre el navegador con `<input type="file" multiple>`.
const elegirApple = `set elegidos to choose file with multiple selections allowed
set salida to ""
repeat with f in elegidos
	set salida to salida & POSIX path of f & linefeed
end repeat
return salida`

// Elegir abre el panel de abrir del sistema, con selección múltiple, y devuelve las
// rutas elegidas. Cancelar devuelve nada, sin error. BLOQUEA: desde una rutina propia.
func Elegir() ([]string, error) {
	if panelPropio != nil {
		return panelPropio("", nil)
	}
	return elegirCon("osascript", "-e", elegirApple)
}
