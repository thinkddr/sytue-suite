// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ReglasDeTraza son los accesos que deja la traza de Rooms (docs/security.md
// §1.7, platform/pkg/auditlog): quién entra o sale de un equipo o un canal y con
// qué papel (también aceptando la invitación de otra organización), crear,
// cambiar, archivar o borrar canales, la política de contacto con gente de
// fuera, conectar webhooks, puentes y bots (lo que saca mensajes fuera), entrar
// en una llamada (el JWT de LiveKit), grabarla y bajar o borrar la grabación, y
// exportarlo todo. El objetivo es el id del canal, el equipo, la sala o la
// grabación; nunca un mensaje. Los adjuntos de un canal viven en Drive y se
// anotan allí.
// TestReglasDeTrazaExisten comprueba que cada regla sigue casando con una ruta.
var ReglasDeTraza = []auditlog.Regla{
	{Metodo: http.MethodGet, Ruta: "/api/v1/export", Accion: "export"},
	{Metodo: http.MethodPatch, Ruta: "/api/v1/admin/channels/{channelID}", Accion: "rooms.admin.channel.archive", Param: "channelID"},
	// Quién abrió o cerró la organización a la gente de fuera (mig 0083).
	{Metodo: http.MethodPut, Ruta: "/api/v1/admin/external-contact", Accion: "rooms.admin.external_contact.update"},
	// Entrar en una conversación de otra organización (ADR 0040): como un alta en
	// un canal, pero la decide quien entra.
	{Metodo: http.MethodPost, Ruta: "/api/v1/me/invitations/{channelID}/accept", Accion: "rooms.invitation.accept", Param: "channelID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/teams/{teamID}/members", Accion: "rooms.team.member.add", Param: "teamID"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/teams/{teamID}/members/{sub}", Accion: "rooms.team.member.remove", Param: "teamID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/teams/{teamID}/channels", Accion: "rooms.channel.create", Param: "teamID"},
	{Metodo: http.MethodPatch, Ruta: "/api/v1/channels/{channelID}", Accion: "rooms.channel.update", Param: "channelID"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/channels/{channelID}", Accion: "rooms.channel.delete", Param: "channelID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/channels/{channelID}/members", Accion: "rooms.channel.member.add", Param: "channelID"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/channels/{channelID}/members/{sub}", Accion: "rooms.channel.member.remove", Param: "channelID"},
	{Metodo: http.MethodPatch, Ruta: "/api/v1/channels/{channelID}/members/{sub}", Accion: "rooms.channel.member.role", Param: "channelID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/channels/{channelID}/webhooks/incoming", Accion: "rooms.channel.webhook.create", Param: "channelID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/channels/{channelID}/bridges", Accion: "rooms.channel.bridge.create", Param: "channelID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/bots/{botID}/channels/{channelID}", Accion: "rooms.bot.channel.add", Param: "channelID"},
}

// ReglasDeTrazaDeLlamadas son las de la videollamada, que se monta en el mismo
// router (Deps.MountExtra): solo casan con el servicio entero.
var ReglasDeTrazaDeLlamadas = []auditlog.Regla{
	{Metodo: http.MethodPost, Ruta: "/api/v1/rooms/{id}/join", Accion: "rooms.call.join", Param: "id"},
	{Metodo: http.MethodGet, Ruta: "/api/v1/rooms/{id}/recordings/{recID}", Accion: "rooms.recording.download", Param: "recID"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/rooms/{id}/recordings/{recID}", Accion: "rooms.recording.delete", Param: "recID"},
	{Metodo: http.MethodPost, Ruta: "/api/v1/rooms/{id}/recording", Accion: "rooms.recording.start", Param: "id"},
	{Metodo: http.MethodDelete, Ruta: "/api/v1/rooms/{id}/recording", Accion: "rooms.recording.stop", Param: "id"},
}
