// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"net/url"
	"slices"
)

// # La app POR DEFECTO para sus documentos
//
// Decisión del dueño (25/09/2026): Calcus abre por defecto .ods y .xlsx; Docus, .odt y
// .docx; Diapos, .odp y .pptx, y CertiPDF, .pdf. Lo que declara cada una está en su
// `FyneApp.toml` (`extensiones`), y cada sistema deja hasta donde deja:
//
//   - macOS: el Info.plist declara los tipos (`empaquetar.sh`, `CFBundleDocumentTypes`) y
//     aquí se le pide a Launch Services que la app sea la de esos tipos. No pregunta.
//   - Windows: 10 y 11 no dejan que un programa se ponga por defecto solo —el hash de
//     `UserChoice` lo impide a propósito—; el instalador la registra en «Aplicaciones
//     predeterminadas» y aquí se abre SU página de esa pantalla, para un clic.
//   - Linux: lo hace `instalar.sh` con `xdg-mime` al instalar, que es cuando se sabe qué
//     había antes para devolverlo al desinstalar. Aquí no hay nada que hacer.
//
// Los ficheros de Apple (.pages, .numbers y .key) se declaran para «Abrir con», pero
// nunca se ponen por defecto: su dueño es Pages, Numbers o Keynote, y la app solo los
// abre convertidos (decisión del dueño, 15/10/2026). Lo mismo en `empaquetar.sh`: rango
// `Alternate` en el Info.plist y sin `xdg-mime default` en Linux.
//
// Se llama una vez por app y equipo (`sesion.AceptarFicheros` lo recuerda): si el usuario
// elige después otra, no se le vuelve a quitar.

// soloAbrirCon son las extensiones que [Predeterminada] no reclama.
var soloAbrirCon = map[string]bool{"pages": true, "numbers": true, "key": true}

// Predeterminada pide al sistema que esta app, que se llama `nombre`, abra por defecto
// los ficheros con esas `extensiones` (sin punto). [ErrNoDisponible] es que en este
// sistema no hay nada que hacer en tiempo de ejecución.
func Predeterminada(nombre string, extensiones []string) error {
	extensiones = slices.DeleteFunc(slices.Clone(extensiones), func(e string) bool { return soloAbrirCon[e] })
	if len(extensiones) == 0 {
		return nil
	}
	return predeterminada(nombre, extensiones)
}

// paginaPredeterminadas es la página de esta app en Configuración › Aplicaciones ›
// Aplicaciones predeterminadas de Windows 11: el nombre es el valor que el instalador
// deja en `HKCU\Software\RegisteredApplications`. Windows 10 no conoce el parámetro y
// abre la pantalla general, que también la lista.
func paginaPredeterminadas(nombre string) string {
	return "ms-settings:defaultapps?registeredAppUser=" + url.PathEscape(nombre)
}
