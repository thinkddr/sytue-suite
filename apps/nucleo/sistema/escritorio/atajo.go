// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import "runtime"

// # El atajo global de la suite
//
// UNA combinación que funciona desde cualquier sitio del sistema, con cualquier otra
// ventana delante, y trae el buscador de la suite: el Spotlight de Sytue. La registra el
// Hub, que es quien vive en la bandeja y quien tiene el buscador.
//
// Qué combinación es la decide este fichero, en Go, para los tres escritorios:
//
//   - Linux y Windows: Ctrl+Alt+Espacio, que ningún escritorio usa de serie.
//   - macOS: ⌃⌥⌘Espacio. ⌘Espacio es Spotlight, ⌃Espacio y ⌃⌥Espacio cambian de
//     fuente de entrada y ⌥⌘Espacio abre la búsqueda del Finder: las cuatro vienen
//     puestas de serie.
//
// En Linux el atajo se pide al portal de escritorio (GlobalShortcuts), que enseña su
// propio diálogo y deja a la persona elegir otra combinación: la de aquí es solo la
// propuesta. En macOS y Windows se registra tal cual; si otra aplicación ya la tiene,
// Atajo devuelve el error y la suite sigue sin atajo.

// Combinacion es la tecla y sus modificadores, sin depender de ningún toolkit.
type Combinacion struct {
	Ctrl, Alt, Cmd bool
	// Tecla es el nombre de la tecla en el formato del portal (`space`).
	Tecla string
}

// CombinacionDe es la combinación del atajo en ese sistema.
func CombinacionDe(goos string) Combinacion {
	if goos == "darwin" {
		return Combinacion{Ctrl: true, Alt: true, Cmd: true, Tecla: "space"}
	}
	return Combinacion{Ctrl: true, Alt: true, Tecla: "space"}
}

// Portal es la combinación escrita como la pide el portal de escritorio
// (`preferred_trigger`, el formato de las shortcuts de XDG): `CTRL+ALT+space`.
func (c Combinacion) Portal() string {
	s := ""
	if c.Ctrl {
		s += "CTRL+"
	}
	if c.Alt {
		s += "ALT+"
	}
	if c.Cmd {
		s += "LOGO+"
	}
	return s + c.Tecla
}

// Atajo registra el atajo global de la suite. `id` es el identificador de la app que lo
// registra, `descripcion` lo que la persona lee en los ajustes del sistema (ya
// traducido), y `alPulsar` se llama en su propia rutina cada vez que se pulsa. Devuelve
// con qué soltarlo.
//
// En Android e iOS no hay atajos globales: ErrNoDisponible.
func Atajo(id, descripcion string, alPulsar func()) (soltar func(), err error) {
	return atajo(id, descripcion, CombinacionDe(runtime.GOOS), alPulsar)
}
