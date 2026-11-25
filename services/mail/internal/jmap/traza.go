// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos al correo que deja la traza de Mail
// (docs/security.md §1.7, platform/pkg/auditlog): abrir o descargar un mensaje
// o un adjunto, leer un hilo, entrar en un buzón compartido, exportarlo todo y
// dar acceso a un buzón. El objetivo es el id del mensaje, del blob, del hilo o
// de la cuenta; nunca el asunto, el remitente ni el cuerpo.
//
// Lo que NO pasa por aquí, dicho para que nadie lo dé por cubierto: el lote JMAP
// (POST /jmap, cuyo Email/get va dentro del cuerpo) y el IMAP, que no es HTTP.
// TestReglasDeTrazaExisten comprueba que cada regla sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/messages/{messageID}", Accion: "mail.message.read", Param: "messageID"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/messages/{messageID}/raw", Accion: "mail.message.download", Param: "messageID"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/threads/{threadID}/messages", Accion: "mail.thread.read", Param: "threadID"},
	{Metodo: http.MethodGet, Ruta: "/jmap/download/{accountId}/{blobId}/{name}", Accion: "mail.blob.download", Param: "blobId"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/shared-mailboxes/{accountID}/mailboxes/{mailboxID}/messages", Accion: "mail.shared.read", Param: "accountID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/shared-mailboxes/{accountID}/grants", Accion: "mail.mailbox.share", Param: "accountID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/shared-mailboxes/{accountID}/edit-requests/{sub}/grant", Accion: "mail.edit_request.grant", Param: "accountID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/delegation", Accion: "mail.delegation.grant"},
}
