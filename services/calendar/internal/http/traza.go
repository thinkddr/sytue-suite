// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos que deja la traza de Calendar (docs/security.md
// §1.7, platform/pkg/auditlog): bajar un calendario entero (.ics), exportarlo
// todo, compartirlo, dejar de compartirlo y conceder una petición de edición. El
// objetivo es el id del calendario; nunca un evento ni su contenido.
//
// La sincronización CalDAV no se anota petición a petición: es el cliente de la
// propia persona leyendo su agenda cada pocos minutos, y anotarla enterraría lo
// que sí importa bajo un sondeo. TestReglasDeTrazaExisten comprueba que cada
// regla sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/calendars/{id}/export.ics", Accion: "calendar.export.ics", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/calendars/{id}/shares", Accion: "calendar.share.create", Param: "id"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/calendars/{id}/shares/{shareId}", Accion: "calendar.share.revoke", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/calendars/{id}/edit-requests/{sub}/grant", Accion: "calendar.edit_request.grant", Param: "id"},
}
