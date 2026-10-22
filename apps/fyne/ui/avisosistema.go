// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// El aviso del SISTEMA: lo que interrumpe a alguien cuando la ventana no está delante.
//
// # La decisión vive aquí, en Go, y la capa nativa solo ejecuta
//
// Es el criterio de `docs/CONVENTIONS.md` §13.6 y del ADR 0039: lo que se decide —qué
// merece interrumpir, con qué texto, cuántas veces— se escribe UNA vez para las cinco
// plataformas, y debajo solo queda «enseña esto». Aquí debajo está
// `fyne.App.SendNotification`, que es exactamente esa línea: `UNUserNotificationCenter`
// en macOS y iOS, el centro de notificaciones en Windows, `org.freedesktop.Notifications`
// por D-Bus en Linux y `Notification.Builder` por JNI en Android. Ninguna regla nuestra
// baja ahí.
//
// # Las tres reglas, y por qué cada una
//
//  1. **Solo lo que no está leído.** Un aviso que el Hub ya marca como leído llegó por
//     otra pantalla; repetirlo en el escritorio es ruido sin información.
//  2. **Una vez por aviso.** Hay DOS caminos por los que llega el mismo —el flujo SSE y
//     la carga inicial que lo repesca cuando el flujo estaba caído—, y ninguno debe
//     sonar dos veces por el mismo correo.
//  3. **Solo los de ESTA aplicación.** Es la regla que la web no necesita y la nativa
//     sí: en la web hay una pestaña con una campana, y aquí hay TRECE procesos con la
//     misma barra, cada uno con su flujo del Hub abierto. Sin esta regla, un mensaje de
//     Sytue Rooms suena una vez por aplicación abierta. Quien avisa de un correo es
//     Sytue Mail, que además es la que se abre al tocarlo.
//  4. **Y el Hub, lo de las que están CERRADAS.** Con la regla 3 sola, un correo que
//     llega con Mail cerrado no lo avisa nadie. El Hub es la bandeja de la suite y
//     puede quedarse en la bandeja del sistema con la ventana cerrada
//     ([sesion.Residente]); él avisa de lo que es de una app que no está abierta en esta
//     máquina —su enchufe no contesta, [sesion.Abierta]— y de lo que no es de ninguna.
//     En el móvil no hay enchufes que preguntar ni bandeja: en Android el Hub se queda
//     vivo con su servicio en primer plano (`escritorio.MantenerAvisos`) y avisa de TODO
//     lo que no es suyo, porque las demás apps, en cuanto se van, Android las mata.
//     ponytail: si la app del aviso sigue viva en ese momento, suena dos veces (la suya
//     y la del Hub); Android no deja a una app saber qué otras corren. Se quita cuando
//     el Hub pueda preguntar a las demás, como hace el enchufe en escritorio.
//
// El **número sobre el icono** —el Dock, el lanzador— lo decide [insigniaDe] y lo pone
// `nucleo/sistema/escritorio`, que dice en qué sistemas llega de verdad.
//
// Pulsar el aviso lleva a su destino, como pulsarlo en la campana ([avisosBarra.pulsar]),
// donde el sistema sabe avisar del clic: Linux, macOS empaquetado y Windows
// (`escritorio.Avisar`). En el resto sale el `SendNotification` de Fyne, que no lleva ni
// callback ni URL, y el destino sigue a un clic de distancia en la campana.
//
// ponytail: tampoco hay ACCIONES en el aviso —«responder», «archivar»—.

// avisoDelSistema decide si un aviso del Hub tiene que salir al sistema operativo.
//
// Es una función y no un método a propósito: es LA regla, se prueba sola y no necesita
// ni ventana ni sesión.
//
// `abierta` dice si la app de un origen está abierta; solo se le pregunta al Hub, y solo
// cuando todo lo demás ya dice que sí, porque cuesta llamar a un enchufe.
func avisoDelSistema(propio string, n api.HubNotification, yaAnunciado bool, abierta func(origen string) bool) bool {
	if n.Read || yaAnunciado || n.Source == "" || propio == "" {
		return false
	}
	return n.Source == propio || (propio == productoHub && !abierta(n.Source))
}

// productoHub es la clave de Sytue Hub, cuyo identificador es el de la suite
// (`eu.sytue.one`) y no `eu.sytue.hub`.
const productoHub = "one"

// insigniaDe es el número que va sobre el icono de ESTA aplicación.
//
// Por la misma razón que la regla 3: con trece iconos en el Dock, el mismo total en todos
// es ruido. Cada app cuenta lo SUYO sin leer, y el Hub —que es la bandeja de toda la
// suite, como la campana de la web— lleva el total del servicio.
//
// ponytail: lo suyo se cuenta sobre los últimos [MaxAvisos] que tiene cargados la barra,
// no sobre todo el histórico: con más de treinta avisos sin leer de una sola app el
// número se queda corto. El techo se levanta cuando el Hub dé la cuenta por origen.
func insigniaDe(propio string, items []api.HubNotification, sinLeer int) int {
	if propio == productoHub {
		return sinLeer
	}
	n := 0
	for _, it := range items {
		if !it.Read && it.Source == propio {
			n++
		}
	}
	return n
}

// productoDeID es la clave de producto de esta aplicación, sacada del identificador que
// declara su `FyneApp.toml` (`eu.sytue.mail` → `mail`). Es la misma clave que usa el Hub
// en el `source` de sus avisos y la que pide `api.AbrirURL`.
//
// Vacía cuando el binario corre sin metadatos —los tests, y `go run` sin empaquetar—, y
// entonces no se anuncia nada: es el lado seguro, porque la alternativa es que trece
// procesos se crean todos «la aplicación del aviso».
func productoDeID(id string) string {
	if i := strings.LastIndex(id, "."); i >= 0 {
		id = id[i+1:]
	}
	if p, ok := productoDeApp[id]; ok {
		return p
	}
	return id
}

// productoDeApp son las cuatro apps cuyo identificador no es su clave de producto: la
// web es una sola (Sytue Docs) y los servicios avisan con `docs`, `sheets`, `slides` y
// `pdf`. Sin esto Docus no se enteraba de ningún aviso suyo y el Hub avisaba de todos,
// con Docus abierta o no (`eu.sytue.docs` no existe).
var productoDeApp = map[string]string{"docus": "docs", "calcus": "sheets", "diapos": "slides", "certipdf": "pdf"}

// idDeProducto es lo contrario: el identificador de la app nativa de una clave de
// producto, que es a quien pregunta [sesion.Abierta]. `chat` y `meet` son Rooms.
func idDeProducto(p string) string {
	for app, prod := range productoDeApp {
		if prod == p {
			return "eu.sytue." + app
		}
	}
	if p == "chat" || p == "meet" {
		p = "rooms"
	}
	return "eu.sytue." + p
}

// anunciar saca el aviso al sistema operativo si le toca, y apunta que ya salió.
func (a *avisosBarra) anunciar(n api.HubNotification) {
	app := fyne.CurrentApp()
	a.mu.Lock()
	ya := a.anunciados[n.ID]
	a.mu.Unlock()
	if !avisoDelSistema(productoDeID(app.Metadata().ID), n, ya, abiertaEn(app)) {
		return
	}
	a.mu.Lock()
	ya = a.anunciados[n.ID] // otra rutina pudo adelantarse mientras se preguntaba
	a.anunciados[n.ID] = true
	a.mu.Unlock()
	if ya {
		return
	}
	// El texto es el del Hub, ya recortado por el servicio: el asunto de un correo o el
	// principio de un comentario, nunca el cuerpo entero ni un documento.
	m := app.Metadata()
	// El título, en el idioma de esta máquina (`params`); el cuerpo es el del emisor.
	titulo, cuerpo := TituloAviso(a.t, n), CuerpoAviso(a.t.Locale(), n)
	if err := escritorio.Avisar(escritorio.Aviso{
		ID: m.ID, Nombre: m.Name, Esquema: m.Custom["esquema"], Titulo: titulo, Cuerpo: cuerpo,
		// Con la app ya cerrada no hay `AlPulsar` que llamar: el aviso lleva su destino.
		Destino:  escritorio.DestinoDe(n.ID, n.Source, n.URL),
		AlPulsar: func() { fyne.Do(func() { a.pulsar(n) }) },
	}); err != nil {
		sesion.AvisoDeRespaldo(app, titulo, cuerpo)
	}
}

// abiertaEn es la pregunta de la regla 4 en esta plataforma. En el móvil no se puede
// hacer y se contesta «abierta», que deja callado al Hub: mejor un aviso que falta que
// el mismo dos veces.
func abiertaEn(app fyne.App) func(string) bool {
	if app.Driver().Device().IsMobile() {
		return func(string) bool { return false } // el Hub avisa de todo (regla 4)
	}
	return func(origen string) bool { return sesion.Abierta(idDeProducto(origen)) }
}
