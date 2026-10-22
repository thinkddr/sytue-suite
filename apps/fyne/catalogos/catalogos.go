// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package catalogos son los catálogos de idioma POR APP del módulo `apps/fyne`, listos
// para `i18n.New`. Es EL sitio: no hay un segundo.
//
// # Por qué uno y no uno por paquete
//
// Por dos razones medidas, y la primera es la que obliga:
//
//  1. **Un catálogo no es de un binario, es de la WEB.** Docus, Calcus, Diapos y
//     CertiPDF son cuatro binarios nativos que comparten en la web el catálogo de la
//     ofimática (`apps/frontend/packages/office`): los 2.195 mensajes. Si cada binario
//     se trae su copia, la primera corrección de una cadena se queda en tres de los
//     cuatro. Por eso el directorio se llama como su fuente en la web —la app, o
//     `office` para el paquete—, que es la unidad real, y no como el paquete Go que lo usa.
//  2. **Repartidos, divergen.** El 2026-09-22 convivían CINCO maneras de resolver esto
//     —dos guiones generadores casi iguales, dos copias hechas a mano y este paquete—
//     porque el generador de catálogos por app escribía en una ruta que no existía.
//     Es exactamente el caso de `docs/CONVENTIONS.md` §13.2.
//
// El catálogo COMÚN de la suite NO está aquí: ese lo da `i18n.Common()`, lo embebe
// `apps/nucleo/i18n` y lo genera `apps/nucleo/scripts/gen-i18n.ts`.
//
// # Son una copia GENERADA, no se editan a mano
//
//	bun apps/fyne/scripts/gen-i18n.ts            # todos los que ya hay
//	bun apps/fyne/scripts/gen-i18n.ts mail       # uno solo (lo crea si no está)
//
// Salen de `apps/frontend/apps/<app>/src/i18n/messages` (los de `office`, del paquete
// `apps/frontend/packages/office`), que es la fuente de verdad
// mientras la web exista. Corregir una cadena se hace allí y se regenera. El generador
// corre SOLO en Linux: el ICU de macOS es otro y los vectores salen distintos.
// `TestLosCatalogosSonLosDeLaWeb` compara clave a clave, en los dos sentidos.
//
// Un idioma que la web todavía no tenga NO se escribe, y eso es correcto: `i18n.New`
// carga siempre el inglés de respaldo, igual que `createAppTranslator`. Por eso `docs`
// trae cinco idiomas y `mail` veintiséis.
package catalogos

import (
	"embed"
	"io/fs"
)

// Los directorios se llaman como la app de la WEB de la que salen, que es lo que hace
// que el generador y el guardián no necesiten ninguna tabla: la ruta de la fuente es
// `apps/frontend/apps/<directorio>/src/i18n/messages`, salvo `office`, que es el
// paquete común de la ofimática (`apps/frontend/packages/office`).
//
//go:embed */*.json
var catalogos embed.FS

// Docs es el catálogo de Sytue Docus, Calcus, Diapos y CertiPDF: en la web lo comparten
// en el paquete de la ofimática (`apps/frontend/packages/office`).
func Docs() fs.FS { return de("office") }

// Mail es el catálogo de Sytue Mail.
func Mail() fs.FS { return de("mail") }

// Portal es el catálogo de Sytue Hub. Se llama como la app de la web
// (`apps/frontend/apps/portal`), que es la fuente; el producto se llama Hub.
func Portal() fs.FS { return de("portal") }

// Console provides the shared native OAuth catalogue with the same source copy.
func Console() fs.FS { return de("console") }

// Drive es el catálogo de Sytue Drive.
func Drive() fs.FS { return de("drive") }

// Photos es el catálogo de Sytue Photos.
func Photos() fs.FS { return de("photos") }

// Calendar es el catálogo de Sytue Calendar.
func Calendar() fs.FS { return de("calendar") }

// Contacts es el catálogo de Sytue Contacts.
func Contacts() fs.FS { return de("contacts") }

// Rooms es el catálogo de Sytue Rooms. El directorio se llama como la app de la WEB
// (`apps/frontend/apps/rooms`); el `client_id` del producto sigue siendo `sytue-chat`,
// herencia del nombre viejo, y eso no toca al catálogo.
func Rooms() fs.FS { return de("rooms") }

// ERP es el catálogo de Sytue ERP.
func ERP() fs.FS { return de("erp") }

// Notes es el catálogo de Sytue Notepads. Se llama como la app de la web
// (`apps/frontend/apps/notes`), que es la fuente; el producto se llama Notepads.
func Notes() fs.FS { return de("notes") }

// Cast es el catálogo de Sytue Cast.
func Cast() fs.FS { return de("cast") }

// Bases es el catálogo de Sytue Bases.
func Bases() fs.FS { return de("bases") }

// de abre uno de los catálogos embebidos. Entra en pánico con un nombre que no existe,
// como `tema.Color`: el conjunto es CERRADO y cada nombre tiene su función pública, así
// que llegar aquí con otro es un error de programa, no de datos.
func de(app string) fs.FS {
	sub, err := fs.Sub(catalogos, app)
	if err != nil {
		panic("catalogos: " + app + ": " + err.Error())
	}
	// fs.Sub no comprueba que el directorio exista; leerlo, sí.
	if _, err := fs.ReadFile(sub, "en.json"); err != nil {
		panic("catalogos: " + app + " no tiene en.json: " + err.Error())
	}
	return sub
}
