// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"net/url"
	"regexp"
	"strings"
)

// # Los enlaces de los avisos del Hub, del lado de quien los recibe
//
// Los escribe `platform/pkg/hubnotify` (enlaces.go) y llegan a la app nativa por su
// esquema, con la ruta de la web: `sytue-mail:/mail/?thread=…` y, desde ADR 0056,
// `sytue-mail:/mail/<org>/?thread=…` —la organización que aloja el recurso, detrás del
// prefijo—. Cada receptor los lee con su función de aquí, y
// `TestLosAvisosAbrenSuElementoEnLaAppNativa` las prueba contra los ejemplos que el
// guardián de hubnotify ata al código que los escribe, con org y sin ella: la forma sin
// org es la transicional que sigue circulando en enlaces guardados.
//
// Viven aquí y no en cada app porque aquí no hay Fyne: el guardián puede leerlas
// todas sin montar trece interfaces.

// LeerHilo es la conversación de un enlace a Mail, `/mail/?thread=<id>`: el aviso de un
// correo nuevo y la búsqueda de la web. Con o sin el prefijo.
func LeerHilo(u *url.URL) (string, bool) {
	if !raizDe(u, "mail") {
		return "", false
	}
	h := u.Query().Get("thread")
	return h, h != ""
}

// LeerCanal es el canal y el mensaje de un enlace a Rooms, en las tres formas que lee la
// web (`ChatApp.tsx`): `/rooms/channels/<canal>?message=<id>` —la de los avisos y la de
// «Copiar enlace»—, la misma con la organización detrás del prefijo
// (`/rooms/<org>/channels/<canal>`) y `/rooms/?channel=<canal>`.
func LeerCanal(u *url.URL) (canal, mensaje string, ok bool) {
	if u == nil || u.Scheme != Esquema("rooms") {
		return "", "", false
	}
	resto := strings.Trim(strings.TrimPrefix(u.Path, "/rooms"), "/")
	q := u.Query()
	if c, hay := strings.CutPrefix(resto, "channels/"); hay && c != "" && !strings.Contains(c, "/") {
		canal = c
	} else if resto == "" {
		// La forma con consulta: /rooms/?channel=<canal>.
		canal = q.Get("channel")
	} else if org, demas, hay := strings.Cut(resto, "/"); hay && esSlugDeOrg(org) {
		// Con la organización delante: /rooms/<org>/channels/<canal>.
		if c, hay := strings.CutPrefix(demas, "channels/"); hay && c != "" && !strings.Contains(c, "/") {
			canal = c
		}
	}
	return canal, q.Get("message"), canal != ""
}

// LeerDenuncias dice si el enlace abre la cola de denuncias de Rooms
// (`/rooms/?moderation=1`, con la organización detrás del prefijo o sin ella): el aviso
// que recibe quien modera (`hubnotify.EnlaceDenuncias`; `ChatApp.tsx` lee `?moderation=`).
func LeerDenuncias(u *url.URL) bool { return raizDeRoomsCon(u, "moderation") }

// LeerInvitacionesRooms dice si el enlace abre las invitaciones de Rooms
// (`/rooms/?invitations=1`, con la organización detrás del prefijo o sin ella): el aviso a
// quien invitan desde otra organización (`hubnotify.EnlaceInvitacionesRooms`, COM-15).
func LeerInvitacionesRooms(u *url.URL) bool { return raizDeRoomsCon(u, "invitations") }

// raizDeRoomsCon: el enlace es la raíz de Rooms, con org o sin ella, y `?<clave>=1`.
func raizDeRoomsCon(u *url.URL, clave string) bool {
	if u == nil || u.Scheme != Esquema("rooms") || u.Query().Get(clave) != "1" {
		return false
	}
	resto := strings.Trim(strings.TrimPrefix(u.Path, "/rooms"), "/")
	return resto == "" || esSlugDeOrg(resto)
}

// LeerSeccion es la sección de Drive de un enlace `/drive/?section=<clave>`, con las
// claves de `Section` de DriveSidebar.tsx: el aviso de «compartido contigo» manda a
// `shared`, porque quien recibe no ve la ruta del dueño.
func LeerSeccion(u *url.URL) (string, bool) {
	if !raizDe(u, "drive") {
		return "", false
	}
	s := u.Query().Get("section")
	return s, s != ""
}

// LeerInvitaciones dice si un enlace a Calendar pide su bandeja de invitaciones,
// `/calendar/?invitations=1`: el aviso de una invitación.
// LeerCalendario es el calendario del aviso de una petición de edición
// (`hubnotify.EnlaceCalendario`): el dueño ve en él quién se lo pide.
func LeerCalendario(u *url.URL) (string, bool) {
	if !raizDe(u, "calendar") {
		return "", false
	}
	c := u.Query().Get("calendar")
	return c, c != ""
}

// LeerPeticionDrive es el fichero o la carpeta del aviso de una petición de edición
// (`hubnotify.EnlacePeticionDrive`): quien lo compartió ve en él quién se lo pide.
func LeerPeticionDrive(u *url.URL) (string, bool) {
	if !raizDe(u, "drive") {
		return "", false
	}
	r := u.Query().Get("request")
	return r, r != ""
}

// LeerPeticionERP es el módulo del aviso de una petición de edición
// (`hubnotify.EnlacePeticionesERP`): la administración la atiende en él.
func LeerPeticionERP(u *url.URL) (string, bool) {
	if !raizDe(u, "erp") {
		return "", false
	}
	m := u.Query().Get("peticion")
	return m, m != ""
}

// LeerBuzon es el buzón compartido del aviso de una petición de acceso
// (`hubnotify.EnlaceBuzon`): quien lo gestiona la atiende en él.
func LeerBuzon(u *url.URL) (string, bool) {
	if !raizDe(u, "mail") {
		return "", false
	}
	b := u.Query().Get("mailbox")
	return b, b != ""
}

// LeerCuaderno es el cuaderno de un aviso de Notepads (`hubnotify.EnlaceCuaderno`).
func LeerCuaderno(u *url.URL) (string, bool) {
	if !raizDe(u, "notes") {
		return "", false
	}
	c := u.Query().Get("notebook")
	return c, c != ""
}

// LeerNotaCompartida es la nota del aviso `notes.shared` de una nota suelta
// (`hubnotify.EnlaceNotaCompartida`, `/notes/?shared=<id>`): se abre en «Compartido
// conmigo», porque su cuaderno no lo ve quien la recibe.
func LeerNotaCompartida(u *url.URL) (string, bool) {
	if !raizDe(u, "notes") {
		return "", false
	}
	n := u.Query().Get("shared")
	return n, n != ""
}

// LeerAlbumCompartido es el álbum del aviso `photos.shared`
// (`hubnotify.EnlaceAlbumCompartido`, `/photos/shared?album=<id>`), con la organización
// detrás del prefijo o sin ella.
func LeerAlbumCompartido(u *url.URL) (string, bool) {
	return enPaginaDePhotos(u, "shared", "album")
}

// enPaginaDePhotos lee `?<param>=` de un enlace a la página `pagina` de Photos, con la
// organización detrás del prefijo o sin ella.
func enPaginaDePhotos(u *url.URL, pagina, param string) (string, bool) {
	if u == nil || u.Scheme != Esquema("photos") {
		return "", false
	}
	resto := strings.Trim(strings.TrimPrefix(u.Path, rutaDe["photos"]), "/")
	if org, demas, hay := strings.Cut(resto, "/"); hay && esSlugDeOrg(org) {
		resto = demas
	}
	v := u.Query().Get(param)
	return v, resto == pagina && v != ""
}

// LeerAlbum es el álbum propio del aviso `photos.album.added`
// (`hubnotify.EnlaceAlbum`, `/photos/album?id=<id>`), con la organización detrás del
// prefijo o sin ella.
func LeerAlbum(u *url.URL) (string, bool) {
	return enPaginaDePhotos(u, "album", "id")
}

func LeerInvitaciones(u *url.URL) bool {
	return raizDe(u, "calendar") && u.Query().Get("invitations") != ""
}

// LeerVista es la vista del ERP de un enlace `/erp/?vista=<vista>`: el aviso de una
// tarea asignada manda a `tareas`.
func LeerVista(u *url.URL) (string, bool) {
	if !raizDe(u, "erp") {
		return "", false
	}
	v := u.Query().Get("vista")
	return v, v != ""
}

// LeerCompartido es lo compartido que abre un enlace a Drive, `?shared=<id>` (el aviso de
// «compartido contigo», `hubnotify.EnlaceCompartido`): el id del fichero o la carpeta.
func LeerCompartido(u *url.URL) (string, bool) {
	if !raizDe(u, "drive") {
		return "", false
	}
	id := u.Query().Get("shared")
	return id, id != ""
}

// uuidEvento es el `UUID` de `pages/events/[id].ts`.
var uuidEvento = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// LeerEvento es el id de `…/events/<id>`, con el prefijo que sea delante (`/calendar`,
// el de la web) y —si trae— la organización detrás de él, que es la forma de los avisos
// desde ADR 0056. Lo mandan los recordatorios y la búsqueda.
func LeerEvento(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}
	partes := strings.Split(strings.Trim(u.Path, "/"), "/")
	if n := len(partes); n >= 2 && partes[n-2] == "events" && uuidEvento.MatchString(partes[n-1]) {
		return partes[n-1], true
	}
	return "", false
}

// esSlugDeOrg dice si `s` tiene la forma de un slug de organización: minúsculas, dígitos
// y guiones, sin guiones al borde. El formato lo fija `platform/pkg/tenancy` y lo replica
// la web (`RE_SLUG_ORG` de packages/config/src/organizacion.ts); este módulo no depende
// del de la plataforma por diseño, igual que la web no puede importar Go.
var reSlugOrg = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func esSlugDeOrg(s string) bool { return reSlugOrg.MatchString(s) }

// raizDe dice si u es la raíz de la app de `producto` por su esquema: con o sin prefijo,
// y —desde ADR 0056— con la organización de la que viene el aviso detrás del prefijo
// (`/mail/<org>/`), que es un único segmento con forma de slug. La org no se devuelve:
// quien abre ya trae la suya en el token, y quien autoriza es el servicio.
func raizDe(u *url.URL, producto string) bool {
	if u == nil || u.Scheme != Esquema(producto) {
		return false
	}
	resto := strings.Trim(strings.TrimPrefix(u.Path, rutaDe[producto]), "/")
	return resto == "" || (!strings.Contains(resto, "/") && esSlugDeOrg(resto))
}
