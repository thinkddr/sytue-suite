// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Denunciar y bloquear en Rooms (ADR 0073). Quién denuncia y quién bloquea lo pone
// el servicio a partir del token: aquí solo viaja el OTRO (el mensaje, la persona).

// MotivosDeDenuncia es la lista cerrada del servicio, en el orden en que se enseña.
var MotivosDeDenuncia = []string{"harassment", "sexual", "violence", "spam", "other"}

// Acciones que cierran una denuncia: son, a la vez, el estado en que queda.
const (
	DenunciaDescartada       = "dismissed"
	DenunciaMensajeBorrado   = "message_deleted"
	DenunciaMiembroExpulsado = "member_removed"
)

// RoomsBlock es una persona bloqueada por quien mira.
type RoomsBlock struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
}

// RoomsReport es una denuncia vista por quien modera. No trae quién denunció.
type RoomsReport struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelKind string `json:"channel_kind"`
	TargetSub   string `json:"target_sub"`
	TargetName  string `json:"target_name"`
	MessageID   string `json:"message_id"`
	// MessageBody es el texto leído en vivo: vacío si no hay mensaje o ya se borró.
	MessageBody string    `json:"message_body"`
	Reason      string    `json:"reason"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

type cuerpoDenuncia struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// ReportMessage denuncia un mensaje y, con él, a su autor.
func (r *Rooms) ReportMessage(ctx context.Context, mensaje, motivo, nota string) error {
	return r.conCuerpo(ctx, http.MethodPost, "/messages/"+url.PathEscape(mensaje)+"/report", cuerpoDenuncia{motivo, nota})
}

// ReportMember denuncia a una persona en el canal que se comparte con ella.
func (r *Rooms) ReportMember(ctx context.Context, canal, persona, motivo, nota string) error {
	return r.conCuerpo(ctx, http.MethodPost,
		"/channels/"+url.PathEscape(canal)+"/members/"+url.PathEscape(persona)+"/report", cuerpoDenuncia{motivo, nota})
}

// Block bloquea a alguien con quien se comparte el canal.
func (r *Rooms) Block(ctx context.Context, canal, persona string) (RoomsBlock, error) {
	return JSON[RoomsBlock](ctx, r.c, Request{
		Method: http.MethodPut, Path: "/channels/" + url.PathEscape(canal) + "/members/" + url.PathEscape(persona) + "/block",
	})
}

// Blocks es a quién tiene bloqueado quien mira.
func (r *Rooms) Blocks(ctx context.Context) ([]RoomsBlock, error) {
	out, err := JSON[struct {
		Items []RoomsBlock `json:"items"`
	}](ctx, r.c, Request{Path: "/me/blocks"})
	return out.Items, err
}

// Unblock quita un bloqueo.
func (r *Rooms) Unblock(ctx context.Context, persona string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/me/blocks/"+url.PathEscape(persona))
}

// Reports es la cola de denuncias que quien mira puede moderar; vacía si no modera.
func (r *Rooms) Reports(ctx context.Context) ([]RoomsReport, error) {
	out, err := JSON[struct {
		Items []RoomsReport `json:"items"`
	}](ctx, r.c, Request{Path: "/moderation/reports"})
	return out.Items, err
}

// ResolveReport cierra una denuncia con una de las tres acciones.
func (r *Rooms) ResolveReport(ctx context.Context, id, accion string) error {
	return r.conCuerpo(ctx, http.MethodPost, "/moderation/reports/"+url.PathEscape(id)+"/resolve",
		struct {
			Action string `json:"action"`
		}{accion})
}
