// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"regexp"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// # A dónde lleva cada fila (formato 3)
//
// Cada fila del widget lleva la ruta de SU elemento dentro de la propia app
// (widgets.Item.Route): la parte del enlace que va detrás de `sytue-<producto>:`. No hay
// rutas nuevas: son las que cada app ya atiende por su esquema —las de los avisos del
// Hub, del buscador y de «Abrir en la aplicación»—, y la prueba de ida y vuelta de
// apps/fyne (TestCadaFilaDelWidgetAbreSuElemento) las pasa por el receptor de cada app.
// Llevan identificadores y, en Drive y CertiPDF, la ruta del fichero, que es como esas
// apps abren; nunca un token. Una que no cabe en widgets.ValidRoute sale vacía y la fila
// abre la app.

// esUUID es la forma de los identificadores que algunos receptores exigen (Contacts,
// Bases): una fila con otro id no se manda a un destino que la app rechazaría.
var esUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// RutaDia es el día de un evento en Calendar (calendario.FechaDelEnlace):
// `/?date=AAAA-MM-DD` del inicio en la zona de la app y, en uno de día completo, que es
// flotante, su fecha y `&all_day=1`. Solo la fecha: nada del evento.
func RutaDia(inicio time.Time, diaCompleto bool) string {
	if inicio.IsZero() {
		return ""
	}
	if diaCompleto {
		return widgets.Ruta("/", "date", inicio.Format(time.DateOnly), "all_day", "1")
	}
	return widgets.Ruta("/", "date", inicio.Format(time.DateOnly))
}

// RutaNota es una nota en su cuaderno (`/notebooks/<cuaderno>?note=<nota>`, la dirección
// canónica de Notepads que lee notes.LeerEntrante).
func RutaNota(cuaderno, nota string) string {
	if cuaderno == "" || nota == "" {
		return ""
	}
	return widgets.Ruta("/notebooks/"+widgets.Escapar(cuaderno), "note", nota)
}

// RutaContacto es la ficha de un contacto (`/contacts/<id>`, contactos.IDDeEnlace).
func RutaContacto(id string) string {
	if !esUUID.MatchString(id) {
		return ""
	}
	return widgets.Ruta("/contacts/" + id)
}

// RutaDrive es una carpeta o un fichero de la RAÍZ de Drive por su nombre, como manda la
// web (`?path=<carpeta>` o `?preview=<fichero>`, drive.DestinoDeEnlace): la app baja de la
// raíz nombre a nombre.
func RutaDrive(nombre string, carpeta bool) string {
	if nombre == "" {
		return ""
	}
	if carpeta {
		return widgets.Ruta("/", "path", "/"+nombre)
	}
	return widgets.Ruta("/", "preview", "/"+nombre)
}

// RutaDocumento es el editor de un documento de Docus, Calcus o Diapos
// (`/edit?id=<id>`, api.IDDeEdicion).
func RutaDocumento(id string) string {
	if id == "" {
		return ""
	}
	return widgets.Ruta("/edit", "id", id)
}

// RutaPDF es un PDF de CertiPDF por su ruta en Drive (`/pdf?path=<ruta>`, api.LeerApertura
// de «pdf»): CertiPDF abre por ruta, como su página de la web.
func RutaPDF(ruta string) string {
	if ruta == "" || ruta[0] != '/' {
		return ""
	}
	return widgets.Ruta("/pdf", "path", ruta)
}

// RutaBase es una base de Bases (`/base/<id>`, bases.DestinoDeEnlace).
func RutaBase(id string) string {
	if !esUUID.MatchString(id) {
		return ""
	}
	return widgets.Ruta("/base/" + id)
}

// RutaTareasERP es la lista de tareas del ERP (`/?vista=tareas`, api.LeerVista): la
// actividad no tiene página propia, y es donde está.
func RutaTareasERP() string { return widgets.Ruta("/", "vista", "tareas") }

// RutaAlbum es un álbum propio de Photos (`/album?id=<id>`, api.LeerAlbum).
func RutaAlbum(id string) string {
	if id == "" {
		return ""
	}
	return widgets.Ruta("/album", "id", id)
}

// RutaFoto es una foto por su original en Drive (`/abrir?file=<id>`, api.LeerApertura de
// «photos», lo que manda Drive al pulsar una imagen): Photos la abre en su visor.
func RutaFoto(ficheroDrive string) string {
	if ficheroDrive == "" {
		return ""
	}
	return widgets.Ruta("/abrir", "file", ficheroDrive)
}

// RutaVideo es un vídeo o un directo de Cast (`/watch/<id>`, cast.App.AbrirEnlace).
func RutaVideo(id string) string {
	if id == "" {
		return ""
	}
	return widgets.Ruta("/watch/" + widgets.Escapar(id))
}

// RutaCanal es un canal de Rooms (`/channels/<id>`, api.LeerCanal).
func RutaCanal(id string) string {
	if id == "" {
		return ""
	}
	return widgets.Ruta("/channels/" + widgets.Escapar(id))
}

// RutaActividad es la actividad reciente de One (`/actividad`, hub.DestinoDeRuta): lo
// reciente de otra app se abre desde allí, en la app que toca.
func RutaActividad() string { return widgets.Ruta("/actividad") }

// RutaAgenda es un evento de la agenda de hoy de One por su día (`/agenda?date=AAAA-MM-DD`,
// y `&all_day=1` en uno de día completo; hub.DiaDeAgenda): One lo abre en Calendar, en ese
// día, como la fila de su tarjeta «Hoy». Es ruta de One porque la fila es de su widget y el
// lector solo antepone el esquema propio; lleva solo la fecha, nada del evento, como
// RutaDia.
func RutaAgenda(inicio time.Time, diaCompleto bool) string {
	if inicio.IsZero() {
		return ""
	}
	if diaCompleto {
		return widgets.Ruta("/agenda", "date", inicio.Format(time.DateOnly), "all_day", "1")
	}
	return widgets.Ruta("/agenda", "date", inicio.Format(time.DateOnly))
}

// RutaHilo es una conversación de Mail (`/?thread=<hilo>`, api.LeerHilo): la misma que abre
// el aviso de un correo nuevo. Sin hilo, sin ruta.
func RutaHilo(hilo string) string {
	if hilo == "" {
		return ""
	}
	return widgets.Ruta("/", "thread", hilo)
}

// pdfEnCarpeta es la ruta de un PDF de la carpeta de CertiPDF.
func pdfEnCarpeta(nombre string) string { return "/" + api.PDFCarpeta + "/" + nombre }
