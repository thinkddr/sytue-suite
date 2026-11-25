// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos a ficheros que deja la traza de Drive
// (docs/security.md §1.7, platform/pkg/auditlog): descargar o abrir un fichero
// —por la API, por una unidad de equipo, por un canal, por WebDAV—, exportarlo
// todo, y cambiar quién lo ve. El objetivo es el id del fichero; nunca su nombre
// ni su contenido. Por WebDAV no hay id, y lo que queda es la ruta: es la de la
// propia persona, que la escribió su cliente.
//
// Lo que la ruta no dice va en el handler: crear un compartido (el fichero viene
// en el cuerpo) y la descarga por un enlace público (el tenant sale del enlace).
// TestReglasDeTrazaExisten comprueba que cada una sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/files/{id}/content", Accion: "file.download", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/files/{id}/versions/{version}/content", Accion: "file.version.download", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/files/{id}/wopi-token", Accion: "file.open", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/batch/zip", Accion: "file.download.zip"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/shared/{id}/content", Accion: "file.download.shared", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/team-drives/{id}/files/{fileID}/content", Accion: "file.download", Param: "fileID"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/channels/{channelID}/files/{fileID}/content", Accion: "file.download", Param: "fileID"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/shares/{id}", Accion: "share.revoke", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/shared/{id}/edit-requests/{sub}/grant", Accion: "share.edit_request.grant", Param: "id"},
	{Metodo: http.MethodPut, Ruta: "/api/v1/team-drives/{id}/members", Accion: "teamdrive.member.set", Param: "id"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/team-drives/{id}/members/{subject}", Accion: "teamdrive.member.remove", Param: "id"},
	{Metodo: http.MethodPut, Ruta: "/api/v1/files/{id}/label", Accion: "file.label.set", Param: "id"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/files/{id}/label", Accion: "file.label.clear", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/dav/*", Accion: "dav.download", Param: "*"},
}
