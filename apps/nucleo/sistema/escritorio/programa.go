// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import "runtime"

// AbrirPrograma abre el cliente de correo `cual` («thunderbird» u «outlook») si está en
// este equipo: el último paso de «Configurar Thunderbird» del Hub, después de crear y
// copiar su contraseña de aplicación. Ninguno de los dos deja que otro programa le cree
// la cuenta; lo que sí hacen los dos es encontrar los servidores con solo el correo
// (autoconfig y Autodiscover), así que abrirlo deja a la persona en el asistente.
//
// Se busca por el NOMBRE que registra cada instalador, no por una ruta: `open -a` en
// macOS, el «App Paths» que resuelve `ShellExecute` en Windows y el PATH en Linux. Si no
// está, devuelve error y la pantalla dice «ábrelo tú»: nada falla en silencio.
//
// ponytail: el Thunderbird de Flatpak o Snap en Linux no está en el PATH con ese nombre
// y cae al «ábrelo tú»; se añade `flatpak run org.mozilla.Thunderbird` si alguien lo pide.
func AbrirPrograma(cual string) error {
	nombres := nombresPrograma(runtime.GOOS, cual)
	if len(nombres) == 0 {
		return ErrNoDisponible
	}
	var err error
	for _, n := range nombres {
		if err = lanzarPrograma(n); err == nil {
			return nil
		}
	}
	return err
}

// nombresPrograma son los nombres con los que `goos` conoce a `cual`, en orden. En
// Windows, Outlook es `outlook.exe` (el de Office) o `olk.exe` (el nuevo).
func nombresPrograma(goos, cual string) []string {
	switch goos + "/" + cual {
	case "darwin/thunderbird":
		return []string{"Thunderbird"}
	case "darwin/outlook":
		return []string{"Microsoft Outlook"}
	case "windows/thunderbird":
		return []string{"thunderbird.exe"}
	case "windows/outlook":
		return []string{"outlook.exe", "olk.exe"}
	case "linux/thunderbird":
		return []string{"thunderbird"}
	}
	return nil
}
