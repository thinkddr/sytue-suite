// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sistema son las dos capacidades del SISTEMA OPERATIVO que TODA app de la
// suite necesita y ningún toolkit da igual: en qué idioma habla el dueño de la máquina
// y si pide tema oscuro.
//
// # No depende de ningún toolkit, y eso es el contrato
//
// Fuera de Android y de Apple este paquete es Go y la biblioteca estándar, nada más (en
// macOS e iOS pregunta el idioma a Foundation, que no es ningún toolkit). Lo era casi:
// hasta el 2026-09-22 vivían aquí los avisos del sistema, y su `gioui.org/io/event`
// hacía que importar `sistema` metiese Gio entero en el `go.mod` de quien lo hiciera.
// Con el toolkit ya jubilado eso dejó a las apps de Fyne sin poder preguntar el idioma
// —«missing go.sum entry for gioui.org/io/event»— y tres se escribieron su rodeo: leer
// `LANG` a mano, usar `lang.SystemLocale()` de Fyne, o quedarse en inglés. Los avisos
// pasaron al subpaquete `sistema/avisos`, y se retiraron con Gio el 2026-09-23: su capa
// nativa colgaba de la actividad de Gio. Los de escritorio están en `sistema/escritorio`.
//
// Queda un import de Gio en `sistema_android.go`, y solo ahí: `app.JavaVM()` y
// `app.AppContext()` son de dónde cuelga el JNI. No se compila en ningún destino de
// escritorio. El techo está escrito en `go.mod`.
//
// # Es de LAS CATORCE APPS y no de una
//
// En el móvil cada app de la suite pregunta lo mismo al mismo sistema, así que lo que
// aquí se conteste lo contesta para todos los productos a la vez. Copiarlo por
// producto es justo lo que `docs/CONVENTIONS.md` §13.2 persigue.
//
// El idioma y el tema no piden permiso, y no es casualidad: se preguntan a
// `java.util.Locale` y a `android.content.res.Configuration`, que son lo que el sistema
// le cuenta a cualquier app sin condiciones. Quien sí pide permiso son los avisos, y
// por eso su guardián —`TestElManifiestoNoGanaPermisosQueNadieUsa`— se fue con ellos.
//
// # Y lo poco que es de un sistema y no de una app
//
// El nombre del equipo con el que sale una contraseña de aplicación en Mi cuenta
// (equipo.go), y el puente al llavero y a `NSFileProviderManager` con el que Drive se
// pone en el Finder y en Archivos de iOS (archivos.go).
package sistema

import (
	"os"
	"strings"
)

// Idioma es la etiqueta BCP 47 del idioma del usuario ("es-ES", "pt-BR", "zh-Hans-CN"),
// o "" si no se sabe.
//
// Existe por un fallo medido: en Android **no hay variables de entorno de idioma**.
// `LANG` y `LC_ALL` llegan vacías, y `cmd/movil` las leía como único origen, así que los
// catorce paquetes salían en INGLÉS en cualquier teléfono, con veintiséis idiomas
// publicados. El sistema sí lo sabe; había que preguntárselo.
//
// El orden es a propósito: manda el sistema, y el entorno es el repuesto. En un móvil no
// hay entorno; en un escritorio o en un contenedor sí, y ahí `LANG` es la respuesta
// correcta.
func Idioma() string { return idiomaCon(delSistema()) }

// idiomaCon es [Idioma] con lo que haya contestado el sistema, para probar el orden sin
// depender del sistema de quien ejecuta las pruebas.
func idiomaCon(delSistema string) string {
	if delSistema != "" {
		return delSistema
	}
	// POSIX lo dice como `es_ES.UTF-8`, que no es BCP 47 hasta quitarle la codificación
	// y cambiar el «_». `C` y `POSIX` no son idiomas: son «sin configurar».
	for _, variable := range []string{"LC_ALL", "LANG"} {
		v, _, _ := strings.Cut(os.Getenv(variable), ".")
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		return strings.ReplaceAll(v, "_", "-")
	}
	return ""
}

// Oscuro dice si el sistema pide tema oscuro. `sabido` es false cuando la plataforma no
// lo cuenta, y entonces `oscuro` no significa nada: decide quien llama.
//
// La web no pasa por aquí —`cmd/web` se lo pregunta a `matchMedia`, que tiene el DOM
// delante— y el escritorio todavía no lo pregunta a nadie.
func Oscuro() (oscuro, sabido bool) { return temaOscuro() }
