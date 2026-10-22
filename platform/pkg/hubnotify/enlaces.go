// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"net/url"
	"strings"
)

// # Los enlaces de los avisos
//
// Un aviso lleva a SU elemento, en la web y en la app nativa (que recibe la misma ruta
// por su esquema, `sytue-<app>:<ruta>`). Hasta el 2026-09-24 cada emisor componía el
// suyo con un `*_PUBLIC_URL` propio que ningún chart daba, así que en producción salían
// `https://mail.sytue.localhost/#msg-…`: otro host, sin el prefijo de la app y con un
// fragmento que no lee nadie. Drive mandaba a `/shared`, que no es una ruta.
//
// Aquí está la ÚNICA forma de escribirlos. El host sale del PUBLIC_URL del servicio,
// que dan todos los charts, y se queda solo con el esquema y el host; la ruta, con el
// prefijo bajo el que se sirve cada app, sale de aquí. Y desde ADR 0056 la
// ORGANIZACIÓN QUE ALOJA EL RECURSO va dentro del path, tras el prefijo
// (`/mail/<org>/?thread=…`): es lo que el emisor ya sabe —el `tenant` de la propia
// entrada de aviso— y lo que dice a qué organización apunta el enlace. Con la org
// vacía el enlace sale sin ella, que es la forma transicional que el middleware de la
// web completa con la de la sesión.
//
// `testdata/enlaces.json` guarda un ejemplo de cada uno y lo leen dos guardianes: el
// de este paquete (que la página de la web exista y lea sus parámetros) y el de
// `apps/nucleo/api` (que el receptor de la app nativa lo entienda, con org y sin ella).
//
// Con un PUBLIC_URL inservible devuelven "": mejor un aviso sin enlace que uno roto.

// EnlaceHilo abre una conversación de Sytue Mail (`MailApp.tsx` lee `?thread=`).
func EnlaceHilo(publicURL, org, hilo string) string {
	return enlace(publicURL, org, "/mail/", url.Values{"thread": {hilo}})
}

// EnlaceBuzon abre Sytue Mail en un buzón compartido o delegado (`MailApp.tsx` lee
// `?mailbox=`): donde quien lo gestiona ve quién le pide acceso.
func EnlaceBuzon(publicURL, org, cuenta string) string {
	return enlace(publicURL, org, "/mail/", url.Values{"mailbox": {cuenta}})
}

// EnlaceCuaderno abre un cuaderno de Sytue Notepads (`NotesApp.tsx` lee `?notebook=`):
// donde su dueño ve quién le pide editarlo.
func EnlaceCuaderno(publicURL, org, cuaderno string) string {
	return enlace(publicURL, org, "/notes/", url.Values{"notebook": {cuaderno}})
}

// EnlaceNota abre una nota dentro de su cuaderno (`index.astro` lee `?notebook=` y
// `?note=`): el aviso de un recordatorio, que es de quien lee la nota.
func EnlaceNota(publicURL, org, cuaderno, nota string) string {
	return enlace(publicURL, org, "/notes/", url.Values{"notebook": {cuaderno}, "note": {nota}})
}

// EnlaceNotaCompartida abre en Notepads una nota que alguien ha compartido con quien
// recibe el aviso (`NotesApp.tsx` lee `?shared=`): la abre dentro de «Compartido
// conmigo», porque el cuaderno en que vive no lo ve.
func EnlaceNotaCompartida(publicURL, org, nota string) string {
	return enlace(publicURL, org, "/notes/", url.Values{"shared": {nota}})
}

// EnlaceAlbumCompartido abre en Photos un álbum que alguien ha compartido con quien
// recibe el aviso (`pages/shared.astro` lee `?album=`).
func EnlaceAlbumCompartido(publicURL, org, album string) string {
	return enlace(publicURL, org, "/photos/shared", url.Values{"album": {album}})
}

// EnlaceAlbum abre en Photos un álbum PROPIO (`pages/album.astro` lee `?id=`): el aviso
// a su dueña de que alguien con edición le ha añadido fotos.
func EnlaceAlbum(publicURL, org, album string) string {
	return enlace(publicURL, org, "/photos/album", url.Values{"id": {album}})
}

// EnlaceEvento abre un evento de Calendar (`pages/events/[id].ts`).
func EnlaceEvento(publicURL, org, evento string) string {
	return enlace(publicURL, org, "/calendar/events/"+url.PathEscape(evento), nil)
}

// EnlaceCalendario abre Calendar con un calendario a la vista (`CalendarApp.tsx` lee
// `?calendar=`): lo que se ha compartido o donde alguien pide editar.
func EnlaceCalendario(publicURL, org, calendario string) string {
	return enlace(publicURL, org, "/calendar/", url.Values{"calendar": {calendario}})
}

// EnlaceInvitaciones abre la bandeja de invitaciones de Calendar. Quien recibe una no
// puede leer todavía el evento, que está en el calendario de quien invita: su sitio
// es la bandeja, donde lo acepta o lo rechaza.
func EnlaceInvitaciones(publicURL, org string) string {
	return enlace(publicURL, org, "/calendar/", url.Values{"invitations": {"1"}})
}

// EnlaceMensaje abre un canal de Rooms en un mensaje (`pages/channels/[channelId]`).
func EnlaceMensaje(publicURL, org, canal, mensaje string) string {
	return enlace(publicURL, org, "/rooms/channels/"+url.PathEscape(canal), url.Values{"message": {mensaje}})
}

// EnlaceDenuncias abre en Rooms la cola de denuncias de quien modera (`ChatApp.tsx`
// lee `?moderation=`).
func EnlaceDenuncias(publicURL, org string) string {
	return enlace(publicURL, org, "/rooms/", url.Values{"moderation": {"1"}})
}

// EnlaceInvitacionesRooms abre Rooms con sus invitaciones a la vista (`ChatApp.tsx` lee
// `?invitations=`): el aviso a una persona de OTRA organización a la que invitan a un
// canal o a una conversación (COM-15). Quien lo emite lo escribe SIN organización: la
// invitación se acepta desde la de quien la recibe, y el enlace no abre nada de la
// anfitriona.
func EnlaceInvitacionesRooms(publicURL, org string) string {
	return enlace(publicURL, org, "/rooms/", url.Values{"invitations": {"1"}})
}

// EnlaceCompartido abre en Drive lo que alguien ha compartido: el fichero o la carpeta
// por su id, dentro de «Compartido conmigo». Quien recibe no ve la ruta del dueño; el
// servicio resuelve ese id a lo suyo (`/shared/{id}/list`) con la verja del propio
// compartido, así que el enlace no abre nada a quien no lo tenga.
func EnlaceCompartido(publicURL, org, nodo string) string {
	return enlace(publicURL, org, "/drive/", url.Values{"section": {"shared"}, "shared": {nodo}})
}

// EnlacePeticionDrive abre Drive donde quien compartió `nodo` ve quién le pide editarlo
// y lo concede (`FileBrowser.tsx` lee `?request=`).
func EnlacePeticionDrive(publicURL, org, nodo string) string {
	return enlace(publicURL, org, "/drive/", url.Values{"request": {nodo}})
}

// EnlaceDocumento abre un documento en el editor de SU producto, según su tipo
// (`kind` del servicio de documentos): Docus, Calcus o Diapos, cada uno con su host y su
// prefijo desde el 2026-10-12 (ADR 0076). Un tipo desconocido va a Docus, que manda el
// documento a la app que le toca. `docs.<dominio>/docs` desapareció: no se genera.
func EnlaceDocumento(publicURL, org, doc, kind string) string {
	app := map[string]string{"spreadsheet": "calcus", "presentation": "diapos"}[kind]
	if app == "" {
		app = "docus"
	}
	return enlace(hostDeProducto(publicURL, app), org, "/"+app+"/edit", url.Values{"id": {doc}})
}

// EnlacePDF abre un PDF de Drive en CertiPDF por su RUTA, que es lo que su estudio
// acepta (`/certipdf/pdf?path=`): la petición de firma y la firma ya hecha.
func EnlacePDF(publicURL, org, ruta string) string {
	return enlace(hostDeProducto(publicURL, "certipdf"), org, "/certipdf/pdf", url.Values{"path": {ruta}})
}

// hostDeProducto es el PUBLIC_URL del servicio con el host del producto en el mismo
// dominio: el de documentos publica su API en `docs.<dominio>`, y sus avisos abren la
// web de cada producto en `calcus.<dominio>`, no en el host de la API.
func hostDeProducto(publicURL, sub string) string {
	u, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil {
		return ""
	}
	_, dominio, ok := strings.Cut(u.Host, ".")
	if !ok {
		return publicURL // sin dominio que conservar: enlace() decide si vale
	}
	u.Host = sub + "." + dominio
	return u.String()
}

// EnlacePeticionesERP abre el ERP en un módulo con la franja de sus peticiones de
// edición (`ErpApp.tsx` lee `?peticion=`): la administración las concede ahí.
func EnlacePeticionesERP(publicURL, org, modulo string) string {
	return enlace(publicURL, org, "/erp/", url.Values{"peticion": {modulo}})
}

// EnlaceTareas abre las tareas del ERP (`ErpApp.tsx` lee `?vista=`).
func EnlaceTareas(publicURL, org string) string {
	return enlace(publicURL, org, "/erp/", url.Values{"vista": {"tareas"}})
}

// EnlacePages abre Sytue Web en `ruta`, relativa a la app y con su consulta si la
// lleva («site/<id>», «site/<id>?tab=orders»): los avisos de pedidos, formularios,
// dominios y exportaciones de la tienda de una organización.
//
// ponytail: sin fila en testdata/enlaces.json hasta que la web de Pages tenga sus
// páginas (el guardián exige que existan); la añade quien las publique.
func EnlacePages(publicURL, org, ruta string) string {
	camino, consulta, _ := strings.Cut(strings.TrimPrefix(ruta, "/"), "?")
	q, err := url.ParseQuery(consulta)
	if err != nil {
		return ""
	}
	return enlace(publicURL, org, "/web/"+camino, q)
}

// EnlaceRegistroDeBase abre una base de Sytue Bases en un registro, con su panel de
// comentarios (`BaseApp.tsx` lee `?table=` y `?record=`): el aviso de un comentario o
// de una mención.
//
// ponytail: sin fila en testdata/enlaces.json mientras Bases no tenga app nativa (el
// guardián de apps/nucleo/api exige un receptor por producto); la forma del enlace la
// ata TestEnlaceRegistroDeBase y la añade al fichero quien publique la app de Fyne.
func EnlaceRegistroDeBase(publicURL, org, base, tabla, registro string) string {
	return enlace(publicURL, org, "/bases/base/"+url.PathEscape(base), url.Values{"table": {tabla}, "record": {registro}})
}

// EnlaceBase abre una base de Sytue Bases (`pages/base/[id].astro`): el aviso de que
// te han transferido su propiedad. Fuera de testdata/enlaces.json por lo mismo que
// EnlaceRegistroDeBase.
func EnlaceBase(publicURL, org, base string) string {
	return enlace(publicURL, org, "/bases/base/"+url.PathEscape(base), nil)
}

// enlace compone la URL del aviso: el host del PUBLIC_URL del servicio, el prefijo de
// la app y —si la trae— la organización que aloja el recurso justo detrás, que es el
// sitio canónico que el middleware de la web reescribe por dentro.
func enlace(publicURL, org, ruta string, q url.Values) string {
	u, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	if org != "" {
		ruta = conOrg(ruta, org)
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host, Path: ruta, RawQuery: q.Encode()}
	return out.String()
}

// conOrg mete la organización tras el PREFIJO de la app, que es el primer segmento de
// la ruta: `/mail/` → `/mail/<org>/`, `/docs/edit` → `/docs/<org>/edit`. La org nunca
// va al final ni en la consulta: el primer segmento tras el prefijo es SIEMPRE la
// organización, por construcción (ADR 0056).
func conOrg(ruta, org string) string {
	prefijo, resto, _ := strings.Cut(strings.TrimPrefix(ruta, "/"), "/")
	if prefijo == "" {
		return ruta
	}
	return "/" + prefijo + "/" + org + "/" + resto
}
