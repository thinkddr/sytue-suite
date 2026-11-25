// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos a documentos que deja la traza de Docs
// (docs/security.md §1.7, platform/pkg/auditlog): abrir un documento (cargar su
// estado), exportarlo y bajar la exportación, exportarlo todo, y cambiar quién lo
// ve: permisos, enlaces y su contraseña, peticiones de edición concedidas. El
// objetivo es el id del documento; nunca su título ni su contenido. La lectura
// por un enlace público la anota su handler, que es quien sabe de qué tenant es.
// TestReglasDeTrazaExisten comprueba que cada regla sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/documents/{id}/state", Accion: "doc.open", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/documents/{id}/exports", Accion: "doc.export", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/documents/{id}/exports/{blobId}", Accion: "doc.export.download", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodPut, Ruta: "/api/v1/documents/{id}/permissions", Accion: "doc.permission.set", Param: "id"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/documents/{id}/permissions", Accion: "doc.permission.revoke", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/documents/{id}/shares", Accion: "doc.link.create", Param: "id"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/documents/{id}/shares/{linkId}", Accion: "doc.link.revoke", Param: "id"},
	{Metodo: http.MethodPut, Ruta: "/api/v1/documents/{id}/shares/{linkId}/password", Accion: "doc.link.password", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/documents/{id}/edit-requests/{sub}/grant", Accion: "doc.edit_request.grant", Param: "id"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/documents/{id}/versions/{versionId}/restore", Accion: "doc.version.restore", Param: "id"},
}
