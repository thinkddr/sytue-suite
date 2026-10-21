// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// Lo que acompaña a un envío además del texto, y lo que pasa con un mensaje después:
// programarlo, reprogramarlo, confirmar su acuse y ver quién lo ha leído. Son los
// `sendMessage`, `scheduleMessage`, `updateScheduled`, `ackMessage` y
// `messageReaders` de `client/chatApi.ts`, hablando directamente con el servicio.

// RoomsAttachment es un fichero de Sytue Drive colgado de un mensaje.
type RoomsAttachment struct {
	MatrixFile  json.RawMessage `json:"-"`
	FileID      string          `json:"file_id"`
	Name        string          `json:"name,omitempty"`
	ContentType string          `json:"content_type,omitempty"`
	SizeBytes   int64           `json:"size_bytes,omitempty"`
}

// RoomsCard es la tarjeta de un mensaje: una encuesta ("poll") o una tarea ("task").
// El cuerpo del mensaje es la pregunta o el título.
type RoomsCard struct {
	Kind         string          `json:"kind,omitempty"`
	Options      []string        `json:"options,omitempty"`
	Multi        bool            `json:"multi,omitempty"`
	AssigneeSub  string          `json:"assignee_sub,omitempty"`
	AssigneeName string          `json:"assignee_name,omitempty"`
	Due          *time.Time      `json:"due,omitempty"`
	Votes        []RoomsCardVote `json:"votes,omitempty"`
}

// RoomsCardVote es quién marcó un valor de la tarjeta: la opción "0".."9" de una
// encuesta o "done" en una tarea.
type RoomsCardVote struct {
	Value string   `json:"value"`
	Count int      `json:"count"`
	Subs  []string `json:"subs,omitempty"`
}

// RoomsSend es lo que viaja con un mensaje además del texto. Lo que va a cero no viaja.
type RoomsSend struct {
	// Mentions son los `sub` de las personas @mencionadas; @channel, @here y @all los
	// saca el servicio del propio cuerpo.
	Mentions []string `json:"mentions,omitempty"`
	FileIDs  []string `json:"file_ids,omitempty"`
	// Priority es "important" o "urgent"; vacío es la normal.
	Priority    string     `json:"priority,omitempty"`
	RequiresAck bool       `json:"requires_ack,omitempty"`
	Card        *RoomsCard `json:"card,omitempty"`
}

// envio es el cuerpo de POST …/messages y …/replies.
type envio struct {
	Body string `json:"body"`
	RoomsSend
	SendAt string `json:"send_at,omitempty"`
}

// PostWith publica un mensaje en un canal con sus menciones, adjuntos, prioridad,
// acuse o tarjeta.
func (r *Rooms) PostWith(ctx context.Context, canal, texto string, o RoomsSend) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/messages", envio{Body: texto, RoomsSend: o})
}

// ReplyWith publica una respuesta en el hilo de un mensaje con sus menciones y adjuntos.
func (r *Rooms) ReplyWith(ctx context.Context, raiz, texto string, o RoomsSend) (RoomsMessage, error) {
	return PostJSON[RoomsMessage](ctx, r.c, "/messages/"+url.PathEscape(raiz)+"/replies", envio{Body: texto, RoomsSend: o})
}

// Schedule programa un mensaje para `cuando` en vez de publicarlo ya: es el mismo POST
// con `send_at`, y el servicio lo guarda en la cola de programados. Las menciones, la
// prioridad y el acuse NO viajan en un programado (el servicio no los guarda): solo el
// texto y los adjuntos.
func (r *Rooms) Schedule(ctx context.Context, canal, texto string, fileIDs []string, cuando time.Time) (RoomsScheduled, error) {
	return PostJSON[RoomsScheduled](ctx, r.c, "/channels/"+url.PathEscape(canal)+"/messages", envio{
		Body: texto, RoomsSend: RoomsSend{FileIDs: fileIDs}, SendAt: cuando.UTC().Format(time.RFC3339Nano),
	})
}

// Reschedule mueve un programado a otro instante y lo devuelve ya cambiado.
func (r *Rooms) Reschedule(ctx context.Context, id string, cuando time.Time) (RoomsScheduled, error) {
	return conJSON[RoomsScheduled](ctx, r.c, http.MethodPatch, "/scheduled/"+url.PathEscape(id),
		map[string]string{"send_at": cuando.UTC().Format(time.RFC3339Nano)})
}

// conJSON es PostJSON con otro método: un PATCH o un PUT que devuelve el recurso.
func conJSON[T any](ctx context.Context, c *Client, metodo, ruta string, cuerpo any) (T, error) {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		var cero T
		return cero, err
	}
	return JSON[T](ctx, c, Request{
		Method: metodo, Path: ruta, Body: datos, Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Ack confirma el acuse de recibo de un mensaje que lo pide y devuelve cuántos lo han
// confirmado ya.
func (r *Rooms) Ack(ctx context.Context, mensaje string) (int, error) {
	out, err := JSON[struct {
		Count int `json:"count"`
	}](ctx, r.c, Request{Method: http.MethodPost, Path: "/messages/" + url.PathEscape(mensaje) + "/ack"})
	return out.Count, err
}

// RoomsReader es alguien que ha visto un mensaje, y cuándo.
type RoomsReader struct {
	Subject string    `json:"subject"`
	Name    string    `json:"name,omitempty"`
	ReadAt  time.Time `json:"read_at"`
}

// Readers es quién ha visto un mensaje y cuánta gente hay en el canal aparte del autor,
// para decir «visto por 3 de 12».
func (r *Rooms) Readers(ctx context.Context, mensaje string) ([]RoomsReader, int, error) {
	out, err := JSON[struct {
		Readers []RoomsReader `json:"readers"`
		Total   int           `json:"total"`
	}](ctx, r.c, Request{Path: "/messages/" + url.PathEscape(mensaje) + "/readers"})
	return out.Readers, out.Total, err
}

// VoteCard marca (poner true) o retira un valor de la tarjeta de un mensaje —una opción
// de la encuesta ("0".."9") o "done" en una tarea— y devuelve el recuento ya agregado.
func (r *Rooms) VoteCard(ctx context.Context, mensaje, valor string, poner bool) ([]RoomsCardVote, error) {
	metodo := http.MethodPut
	if !poner {
		metodo = http.MethodDelete
	}
	out, err := JSON[struct {
		Votes []RoomsCardVote `json:"votes"`
	}](ctx, r.c, Request{Method: metodo, Path: "/messages/" + url.PathEscape(mensaje) + "/card/" + url.PathEscape(valor)})
	return out.Votes, err
}
