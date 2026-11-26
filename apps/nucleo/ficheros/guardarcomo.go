// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// «Guardar como…»: el selector de GUARDADO del sistema (PLAN-EDICION-LOCAL O-04). Es el
// otro lado de [ElegirConTipos]: la persona elige carpeta y nombre, el sistema pregunta si
// pisa uno que ya existe, y quien llama escribe ahí.
//
//   - macOS: `choose file name` de AppleScript, el panel de guardar del Finder.
//   - Windows: el SaveFileDialog de Windows Forms por PowerShell.
//   - Linux: zenity o kdialog y, si no hay ninguno, el portal de escritorio de XDG
//     (`FileChooser.SaveFile`), lo mismo que [Elegir].
//   - iOS y Android: lo que el toolkit ponga en [GuardarEnElMovil]; sin él,
//     ErrNoDisponible y la app guarda en su carpeta ([Guardar]) y lo comparte.

// GuardarEnElMovil es el selector de guardado del móvil (el de «Archivos» en iOS, el de
// SAF en Android), si el toolkit lo da. Devuelve una ruta donde escribir; "" es cancelar.
var GuardarEnElMovil func(nombre string, extensiones []string) (string, error)

// panelGuardarPropio es el NSSavePanel dentro del proceso de la variante de la Mac App
// Store, por lo mismo que [panelPropio]. Aún no existe: con el App Sandbox, el `osascript`
// hijo se quedaría con el permiso de escribir.
var panelGuardarPropio func(titulo, nombre string, extensiones []string) (string, error)

// porPortalGuardar es el selector de guardado de repuesto: el portal de XDG en Linux.
var porPortalGuardar func(titulo, nombre string, extensiones []string) (string, error)

// GuardarComo abre el selector de guardado del sistema con `nombre` propuesto y filtrado
// por `extensiones` (sin punto; la primera es la que se propone). Devuelve la ruta
// elegida, con la extensión puesta si la persona la quitó; cancelar devuelve "", nil.
// BLOQUEA: desde una rutina propia.
func GuardarComo(titulo, nombre string, extensiones ...string) (string, error) {
	nombre = nombrePropuesto(nombre, extensiones)
	ruta, err := elegirDondeGuardar(titulo, nombre, extensiones)
	if err != nil || ruta == "" {
		return "", err
	}
	return conExtension(filepath.Clean(ruta), extensiones), nil
}

func elegirDondeGuardar(titulo, nombre string, extensiones []string) (string, error) {
	if panelGuardarPropio != nil {
		return panelGuardarPropio(titulo, nombre, extensiones)
	}
	orden, args, err := ordenGuardarComo(titulo, nombre, extensiones)
	if errors.Is(err, ErrNoDisponible) && porPortalGuardar != nil {
		return porPortalGuardar(titulo, nombre, extensiones)
	}
	if errors.Is(err, ErrNoDisponible) && GuardarEnElMovil != nil {
		return GuardarEnElMovil(nombre, extensiones)
	}
	if err != nil {
		return "", err
	}
	// Sin plazo: la persona puede tardar lo que quiera en elegir.
	salida, err := exec.CommandContext(context.Background(), orden, args...).Output() //nolint:gosec // G204: la orden es fija por sistema; título y nombre no pasan por un intérprete (en PowerShell van en Base64)
	var salidaMala *exec.ExitError
	if errors.As(err, &salidaMala) {
		return "", nil // los tres selectores salen con código distinto de cero al cancelar
	}
	if err != nil {
		return "", err
	}
	if rutas := rutasDe(string(salida)); len(rutas) > 0 {
		return rutas[0], nil
	}
	return "", nil
}

// nombrePropuesto es `nombre` sin barras (no es una ruta) y con la primera extensión si
// no trae una de las pedidas.
func nombrePropuesto(nombre string, extensiones []string) string {
	nombre = strings.TrimSpace(strings.NewReplacer("/", "-", "\\", "-").Replace(nombre))
	if nombre == "" {
		nombre = "Documento"
	}
	return conExtension(nombre, extensiones)
}

// conExtension pone la primera extensión pedida si la ruta no termina en ninguna de ellas.
func conExtension(ruta string, extensiones []string) string {
	if len(extensiones) == 0 {
		return ruta
	}
	ext := strings.TrimPrefix(filepath.Ext(ruta), ".")
	for _, e := range extensiones {
		if strings.EqualFold(ext, e) {
			return ruta
		}
	}
	return ruta + "." + extensiones[0]
}

// guionGuardarWindows es el SaveFileDialog: el cuadro «Guardar como» del Explorador, que
// pregunta él si pisa un fichero que ya existe. Título, nombre y filtro van en Base64
// ([cadenaPowerShell], FYS-12).
func guionGuardarWindows(titulo, nombre string, extensiones []string) string {
	patrones := make([]string, len(extensiones))
	for i, e := range extensiones {
		patrones[i] = titulo + " (*." + e + ")|*." + e
	}
	filtro := strings.Join(patrones, "|")
	if filtro == "" {
		filtro = "*.*|*.*"
	}
	return "Add-Type -AssemblyName System.Windows.Forms;" +
		"$d = New-Object System.Windows.Forms.SaveFileDialog;" +
		"$d.Title = " + cadenaPowerShell(titulo) + ";" +
		"$d.FileName = " + cadenaPowerShell(nombre) + ";" +
		"$d.Filter = " + cadenaPowerShell(filtro) + ";" +
		"$d.OverwritePrompt = $true;" +
		"if ($d.ShowDialog() -eq 'OK') { $d.FileName } else { exit 1 }"
}
