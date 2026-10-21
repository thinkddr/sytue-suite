// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Lo que piden los comandos «/» del compositor (`runCommand` y `runRemoteCommand` de
// `ChatApp.tsx`): cambiar el tema o el propósito del canal, salir de él, invitar o sacar
// a alguien del equipo, y los comandos de SERVIDOR que registra la organización.

// RoomsChannelPatch es lo que cambia de un canal; lo que va a nil no se toca.
type RoomsChannelPatch struct {
	Name    *string `json:"name,omitempty"`
	Topic   *string `json:"topic,omitempty"`
	Purpose *string `json:"purpose,omitempty"`
}

// UpdateChannel renombra un canal o le cambia el tema o el propósito, y lo devuelve ya
// cambiado.
func (r *Rooms) UpdateChannel(ctx context.Context, canal string, cambio RoomsChannelPatch) (RoomsChannel, error) {
	return conJSON[RoomsChannel](ctx, r.c, http.MethodPatch, "/channels/"+url.PathEscape(canal), cambio)
}

// LeaveChannel saca a quien mira del canal.
func (r *Rooms) LeaveChannel(ctx context.Context, canal string) error {
	return r.sinCuerpo(ctx, http.MethodPost, "/channels/"+url.PathEscape(canal)+"/leave")
}

// AddTeamMember invita a alguien al equipo (`/invite`).
func (r *Rooms) AddTeamMember(ctx context.Context, equipo, persona, nombre string) error {
	return r.conCuerpo(ctx, http.MethodPost, "/teams/"+url.PathEscape(equipo)+"/members", struct {
		Subject string `json:"subject"`
		Name    string `json:"name,omitempty"`
	}{persona, nombre})
}

// RemoveTeamMember saca a alguien del equipo (`/remove`).
func (r *Rooms) RemoveTeamMember(ctx context.Context, equipo, persona string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, "/teams/"+url.PathEscape(equipo)+"/members/"+url.PathEscape(persona))
}

// RoomsCommand es un comando de SERVIDOR: su nombre y su ayuda los escribió quien lo
// registró, así que no pasan por el catálogo.
//
// ID, RequestURL, Token y SigningSecret los ve quien administra, en «Apps e
// integraciones»: el servicio los recorta para el resto.
type RoomsCommand struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	UsageHint     string `json:"usage_hint,omitempty"`
	RequestURL    string `json:"request_url,omitempty"`
	Token         string `json:"token,omitempty"`
	SigningSecret string `json:"signing_secret,omitempty"`
}

// Commands son los comandos de servidor de la organización, para el autocompletado.
func (r *Rooms) Commands(ctx context.Context) ([]RoomsCommand, error) {
	out, err := JSON[struct {
		Commands []RoomsCommand `json:"commands"`
	}](ctx, r.c, Request{Path: "/commands"})
	return out.Commands, err
}

// RoomsCommandResult es lo que contesta un comando de servidor: el texto y si ya se
// publicó en el canal ("in_channel") o solo lo ve quien lo ejecutó ("ephemeral").
type RoomsCommandResult struct {
	Text         string `json:"text"`
	ResponseType string `json:"response_type"`
}

// ExecuteCommand ejecuta la línea entera («/desplegar prod») en ese canal: el servicio
// la parte, llama al tercero y devuelve su respuesta.
func (r *Rooms) ExecuteCommand(ctx context.Context, canal, linea string) (RoomsCommandResult, error) {
	return PostJSON[RoomsCommandResult](ctx, r.c, "/commands/execute", map[string]string{"channel_id": canal, "input": linea})
}

// RoomsNotifyPrefs son las preferencias de aviso de quien mira: las palabras clave y
// «No molestar» en minutos desde medianoche. `DNDStart == DNDEnd` es que no hay.
type RoomsNotifyPrefs struct {
	Keywords []string `json:"keywords"`
	DNDStart int      `json:"dnd_start"`
	DNDEnd   int      `json:"dnd_end"`
}

// NotifyPrefs son las preferencias de aviso de quien mira.
func (r *Rooms) NotifyPrefs(ctx context.Context) (RoomsNotifyPrefs, error) {
	return JSON[RoomsNotifyPrefs](ctx, r.c, Request{Path: "/me/notify-prefs"})
}

// PutNotifyPrefs las guarda y devuelve como quedaron (el servicio normaliza).
func (r *Rooms) PutNotifyPrefs(ctx context.Context, p RoomsNotifyPrefs) (RoomsNotifyPrefs, error) {
	if p.Keywords == nil {
		p.Keywords = []string{} // una lista, como la web, y no `null`
	}
	return conJSON[RoomsNotifyPrefs](ctx, r.c, http.MethodPut, "/me/notify-prefs", p)
}

// RoomsUnfurl es la vista previa de un enlace (OpenGraph), cacheada y protegida de SSRF
// en el servicio. ImageURL, si la hay, es la ruta del PROXY del servicio
// (`/api/v1/unfurl/imagen/…`): la imagen no se pide al sitio de origen.
type RoomsUnfurl struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
}

// Unfurl es la vista previa de un enlace http o https.
func (r *Rooms) Unfurl(ctx context.Context, enlace string) (RoomsUnfurl, error) {
	out, err := JSON[struct {
		Unfurl RoomsUnfurl `json:"unfurl"`
	}](ctx, r.c, Request{Path: "/unfurl", Query: url.Values{"url": {enlace}}})
	return out.Unfurl, err
}

// UnfurlImage abre la miniatura de una vista previa por la ruta que dio Unfurl.
func (r *Rooms) UnfurlImage(ctx context.Context, ruta string) (io.ReadCloser, error) {
	resp, err := r.c.Do(ctx, Request{Path: strings.TrimPrefix(ruta, "/api/v1")})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
