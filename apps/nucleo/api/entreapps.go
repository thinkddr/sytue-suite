// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"net/url"
	"strings"
)

// # Llevarse algo a otro producto
//
// Los intents del Hub (`services/hub/internal/intents`) componen el enlace profundo al
// compositor de otra app —«Adjuntar a un correo» abre el redactor de Mail con el fichero
// ya puesto, «Compartir en Rooms» abre la pantalla de compartir de Rooms— y no ejecutan
// nada: la autorización y la ejecución siguen en cada servicio. En nativo ese enlace es
// el ESQUEMA de la otra aplicación (`docs/CONVENTIONS.md` §13.6), así que se compone
// aquí, con los mismos parámetros que el Hub, y quien lo recibe lo lee con la función
// gemela. Emitir y leer en el mismo fichero es a propósito: dos copias de los nombres
// `attach_drive` o `share_source` se desincronizan en cuanto uno cambie
// (`TestLoQueSeEmiteEsLoQueSeLee` hace lo mismo con la apertura de ficheros).

// Redactar es lo que otra app le pide a Sytue Mail: un correo nuevo con esto ya puesto.
type Redactar struct {
	Para, Asunto string
	// Cuerpo es texto plano: el ERP deja así escrito el correo de una factura.
	Cuerpo string
	// Drive son ids de ficheros de Drive que van ADJUNTOS POR REFERENCIA: viaja el
	// enlace, no los bytes, así que adjuntar 2 GB cuesta lo mismo que un PDF.
	Drive []string
}

// RutaRedactar es la página de Mail que abre el redactor así (`attach-drive-to-mail` del
// Hub). Es la RAÍZ de la app: el redactor lo abre la bandeja al ver `action=compose` en
// la dirección. El Hub mandaba a `/compose`, que Mail no tiene (404 en producción,
// medido el 2026-09-23), y desde entonces manda aquí también.
func RutaRedactar(r Redactar) string {
	q := url.Values{"action": {"compose"}}
	for _, id := range r.Drive {
		if id = strings.TrimSpace(id); id != "" {
			q.Add("attach_drive", id)
		}
	}
	for k, v := range map[string]string{"to": r.Para, "subject": r.Asunto, "body": r.Cuerpo} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return "/?" + q.Encode()
}

// LeerRedactar desarma ese enlace en la app de Mail. Devuelve false si no es un enlace
// de Mail o no pide redactar; la ruta da igual, como en la web.
func LeerRedactar(u *url.URL) (Redactar, bool) {
	if u == nil || u.Scheme != Esquema("mail") {
		return Redactar{}, false
	}
	q := u.Query()
	if q.Get("action") != "compose" {
		return Redactar{}, false
	}
	r := Redactar{Para: q.Get("to"), Asunto: q.Get("subject"), Cuerpo: q.Get("body")}
	for _, id := range q["attach_drive"] {
		if id = strings.TrimSpace(id); id != "" {
			r.Drive = append(r.Drive, id)
		}
	}
	return r, true
}

// Compartir es lo que otra app le pide a Sytue Rooms: publicar algo en un canal que la
// persona elige allí (`share-to-chat` del Hub).
type Compartir struct {
	// Origen es de qué producto viene, con las claves de `share.source.*`: `drive`,
	// `mail`, `calendar`, `contacts`.
	Origen string
	// ID es el del recurso en su producto.
	ID string
	// Texto es el mensaje propuesto: el nombre y el enlace del recurso. Se edita allí.
	Texto string
}

// topeTextoCompartir es el `truncate(text, 1000)` del Hub.
const topeTextoCompartir = 1000

// RutaCompartir es la pantalla de compartir de Rooms (`/share`, la misma de la web).
func RutaCompartir(c Compartir) string {
	q := url.Values{"action": {"share"}, "share_source": {c.Origen}, "share_id": {c.ID}}
	if c.Texto != "" {
		q.Set("text", string([]rune(c.Texto)[:min(len([]rune(c.Texto)), topeTextoCompartir)]))
	}
	return "/share?" + q.Encode()
}

// LeerCompartir desarma ese enlace en la app de Rooms. Devuelve false si no es suyo.
func LeerCompartir(u *url.URL) (Compartir, bool) {
	if u == nil || u.Scheme != Esquema("rooms") || u.Path != "/share" {
		return Compartir{}, false
	}
	q := u.Query()
	return Compartir{Origen: q.Get("share_source"), ID: q.Get("share_id"), Texto: q.Get("text")}, true
}

// MensajeNota es lo que Sytue Notepads publica en una sala al compartir una nota: su
// ENLACE, nunca su contenido —la sala puede tener gente que no debería leerla, y quien
// lo abra pasará por la autorización de Notes—. Es el texto que compone el BFF de la web
// (`notes/src/pages/api/share.ts`): el título recortado a 200 y el enlace debajo.
//
// El enlace es el de la búsqueda (EnlaceNota), el mismo que escribe la web, con la org
// de la sesión detrás del prefijo (ADR 0056). La única diferencia es el título vacío:
// aquí cae en `sinTitulo`, que quien llama saca de su catálogo, y no en el «Nota»
// escrito a mano.
func MensajeNota(apex, org, cuaderno, nota, titulo, sinTitulo string) string {
	t := []rune(strings.TrimSpace(titulo))
	t = t[:min(len(t), 200)]
	if len(t) == 0 {
		t = []rune(sinTitulo)
	}
	return "📝 " + string(t) + "\n" + EnlaceNota(apex, org, cuaderno, nota)
}
