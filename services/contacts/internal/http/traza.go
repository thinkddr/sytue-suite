// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos que deja la traza de Contacts (docs/security.md
// §1.7, platform/pkg/auditlog): exportar la libreta (vCard o CSV) y bajar la
// ficha de un contacto. El objetivo es el id del contacto; nunca sus datos.
//
// Como en Calendar, la sincronización CardDAV del teléfono o la libreta de
// escritorio no se anota petición a petición. TestReglasDeTrazaExisten comprueba
// que cada regla sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/export-csv", Accion: "contacts.export.csv"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/contacts/{id}/vcard", Accion: "contacts.contact.export", Param: "id"},
}
