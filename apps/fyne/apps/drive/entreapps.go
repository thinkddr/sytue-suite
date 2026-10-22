// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Llevarse un fichero a otro producto
//
// «Adjuntar a un correo» y «Compartir en Rooms» son, en la web, dos intents del Hub
// (`attach-drive-to-mail` y `share-to-chat`) que componen el enlace al compositor de la
// otra app y navegan. Aquí la otra app es otra APLICACIÓN, así que se la llama por su
// esquema con los mismos parámetros (`api.RutaRedactar`, `api.RutaCompartir`) y, si el
// sistema no tiene quién lo atienda, se cae a la misma página en la web
// (`docs/CONVENTIONS.md` §13.6). No hace falta pasar por el Hub: no ejecuta nada, solo
// compone el enlace, y la autorización la aplica el servicio de destino al publicar.
//
// El fichero va POR REFERENCIA —su id y su enlace—, nunca sus bytes: adjuntar 2 GB
// cuesta lo mismo que un PDF, y quien lo abra pasa por los permisos de Drive.

// adjuntarACorreo abre el redactor de Sytue Mail con el fichero adjunto y su nombre de
// asunto, como la web.
func (p *Pantalla) adjuntarACorreo(el Elemento) {
	ruta := api.RutaRedactar(api.Redactar{Asunto: el.Nombre, Drive: []string{el.ID}})
	if api.AbrirEn(p.abridor(), p.apex(), "mail", p.org(), ruta) != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("action.mailFailed", nil), "")
	}
}

// compartirEnRooms abre la pantalla de compartir de Sytue Rooms con el nombre y el
// enlace del elemento como mensaje propuesto; allí se elige la sala y se publica.
func (p *Pantalla) compartirEnRooms(el Elemento) {
	ruta := api.RutaCompartir(api.Compartir{Origen: "drive", ID: el.ID, Texto: el.Nombre + "\n" + p.enlaceDe(el)})
	if api.AbrirEn(p.abridor(), p.apex(), "rooms", p.org(), ruta) != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("action.roomsFailed", nil), "")
	}
}

// SubibleACast dice si un elemento se puede llevar a Sytue Cast: un fichero de vídeo
// (`subibleACast` de la web).
func SubibleACast(el Elemento) bool {
	return !el.Carpeta && el.ID != "" && strings.HasPrefix(strings.ToLower(el.Tipo), "video/")
}

// subirACast abre «Subir a Cast» en Sytue Cast con el fichero (`/upload?drive=<id>`,
// `urlDeSubirACast` de la web): allí se ve el fichero, se elige título y visibilidad y
// se confirma la copia.
func (p *Pantalla) subirACast(el Elemento) {
	ruta := "/upload?drive=" + url.QueryEscape(el.ID)
	if api.AbrirEn(p.abridor(), p.apex(), "cast", p.org(), ruta) != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("error.connection", nil), "")
	}
}

// enlaceDe es el enlace de cara a la persona: el canónico de un fichero y la carpeta por
// su ruta (`driveFolderUrl`). La web manda `{origen}/files/{id}` para los dos y sin el
// prefijo de la app, y en producción eso es un 404 (medido el 2026-09-23). Con la
// organización de la sesión detrás del prefijo (ADR 0056) aterriza canónico.
func (p *Pantalla) enlaceDe(el Elemento) string {
	if el.Carpeta {
		return api.AppURL(p.apex(), "drive", p.org(), "?path="+url.QueryEscape(p.rutaDe(el)))
	}
	return api.EnlaceFichero(p.apex(), p.org(), el.ID)
}
