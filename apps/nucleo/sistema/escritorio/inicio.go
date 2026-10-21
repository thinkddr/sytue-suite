// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"encoding/xml"
	"strings"
)

// # Abrir al iniciar sesión
//
// Es lo que hace que los avisos lleguen con la ventana cerrada: el Hub se queda en la
// bandeja (`apps/fyne/sesion.Residente`) y, si arranca solo al entrar en el equipo,
// siempre hay quien avise de lo de las apps cerradas (regla 4 de `ui/avisosistema.go`).
// Arranca con [ArgBandeja], que le dice que no enseñe la ventana.
//
// Cada escritorio lo guarda en su sitio, sin permisos de administración:
//
//   - Linux: `~/.config/autostart/<id>.desktop`, la especificación de XDG que leen
//     GNOME, Plasma, XFCE y compañía.
//   - macOS: un LaunchAgent del usuario, `~/Library/LaunchAgents/<id>.plist`, con
//     `RunAtLoad`. El sistema lo enseña en Ajustes › General › Ítems de inicio.
//   - Windows: el valor `<id>` de `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
//     que es lo que lista el Administrador de tareas en «Aplicaciones de inicio».

// ArgBandeja es el argumento con el que se arranca al iniciar sesión: la app se queda
// en la bandeja sin enseñar la ventana.
const ArgBandeja = "--bandeja"

// AlIniciarSesion hace que la app `id` (`eu.sytue.one`), cuyo ejecutable es `exe`,
// arranque sola al entrar en el equipo —`si`— o deje de hacerlo. `nombre` es el que se
// ve en la lista de inicio del sistema.
func AlIniciarSesion(id, nombre, exe string, si bool) error {
	if id == "" || exe == "" {
		return ErrNoDisponible
	}
	return alIniciarSesion(id, nombre, exe, si)
}

// ArrancaAlIniciarSesion dice si la app `id` está puesta para arrancar al entrar.
func ArrancaAlIniciarSesion(id string) bool { return id != "" && arrancaAlIniciarSesion(id) }

// entradaAutostart es el `.desktop` de XDG. En `Exec` cada argumento va entre comillas
// dobles, con la comilla doble, el acento grave, el dólar y la barra escapados, como pide
// la especificación: una ruta con espacios sin comillas arrancaría otro programa.
func entradaAutostart(nombre, exe string) string {
	citar := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return "[Desktop Entry]\nType=Application\nName=" + strings.ReplaceAll(nombre, "\n", " ") +
		"\nExec=\"" + citar.Replace(exe) + "\" " + ArgBandeja +
		"\nX-GNOME-Autostart-enabled=true\nNoDisplay=true\n"
}

// agenteLaunchd es el plist del LaunchAgent.
func agenteLaunchd(id, exe string) string {
	x := func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + x(id) + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + x(exe) + `</string>
		<string>` + ArgBandeja + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`
}

// ordenRun es el valor de la clave Run: el ejecutable entre comillas y el argumento.
func ordenRun(exe string) string { return `"` + exe + `" ` + ArgBandeja }
