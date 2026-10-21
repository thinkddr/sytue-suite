// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/url"
)

// Lo que gestiona «Apps e integraciones» (IntegrationsModal.tsx y /api/integrations):
// webhooks entrantes y salientes, bots, puentes con Slack, Teams y Google Chat, números
// de WhatsApp Business y comandos de servidor. Los secretos en claro vuelven UNA vez, al
// crear o rotar.

// RoomsIncomingWebhook es un webhook entrante de un canal.
type RoomsIncomingWebhook struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Name      string `json:"name"`
	Icon      string `json:"icon,omitempty"`
}

// RoomsIncomingSecret es lo que vuelve al crear o rotar uno: el token y la URL en claro
// y, si el servicio tiene encendido el correo a canal, su dominio. Vacío: no se ofrece
// la dirección de correo.
type RoomsIncomingSecret struct {
	Webhook           RoomsIncomingWebhook `json:"webhook"`
	Token             string               `json:"token"`
	URL               string               `json:"url"`
	ChannelMailDomain string               `json:"channel_mail_domain,omitempty"`
}

// RoomsOutgoingWebhook es un webhook saliente.
type RoomsOutgoingWebhook struct {
	ID            string   `json:"id"`
	ChannelID     string   `json:"channel_id,omitempty"`
	TriggerWords  []string `json:"trigger_words,omitempty"`
	CallbackURL   string   `json:"callback_url"`
	Token         string   `json:"token,omitempty"`
	Dialect       string   `json:"dialect,omitempty"`
	SigningSecret string   `json:"signing_secret,omitempty"`
}

// RoomsBot es un bot de la organización.
type RoomsBot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RoomsBridge es un canal puenteado con otra plataforma.
type RoomsBridge struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Platform  string `json:"platform"`
	Target    string `json:"target"`
	LastError string `json:"last_error,omitempty"`
}

// RoomsWhatsApp es un número de WhatsApp Business conectado a un equipo.
type RoomsWhatsApp struct {
	ID            string `json:"id"`
	TeamID        string `json:"team_id"`
	PhoneNumberID string `json:"phone_number_id"`
	DisplayPhone  string `json:"display_phone"`
	VerifiedName  string `json:"verified_name,omitempty"`
}

func ruta(partes ...string) string {
	s := ""
	for i, p := range partes {
		if i%2 == 1 {
			p = url.PathEscape(p)
		}
		s += p
	}
	return s
}

// IncomingWebhooks son los del canal.
func (r *Rooms) IncomingWebhooks(ctx context.Context, canal string) ([]RoomsIncomingWebhook, error) {
	out, err := JSON[struct {
		Webhooks []RoomsIncomingWebhook `json:"webhooks"`
	}](ctx, r.c, Request{Path: ruta("/channels/", canal, "/webhooks/incoming")})
	return out.Webhooks, err
}

// CreateIncomingWebhook crea uno en el canal.
func (r *Rooms) CreateIncomingWebhook(ctx context.Context, canal, nombre, icono string) (RoomsIncomingSecret, error) {
	return PostJSON[RoomsIncomingSecret](ctx, r.c, ruta("/channels/", canal, "/webhooks/incoming"), struct {
		Name string `json:"name"`
		Icon string `json:"icon,omitempty"`
	}{nombre, icono})
}

// RotateIncomingWebhook le cambia el token.
func (r *Rooms) RotateIncomingWebhook(ctx context.Context, id string) (RoomsIncomingSecret, error) {
	return PostJSON[RoomsIncomingSecret](ctx, r.c, ruta("/webhooks/incoming/", id, "/rotate"), struct{}{})
}

// DeleteIncomingWebhook lo borra.
func (r *Rooms) DeleteIncomingWebhook(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/webhooks/incoming/", id))
}

// OutgoingWebhooks son los salientes que quien mira gestiona.
func (r *Rooms) OutgoingWebhooks(ctx context.Context) ([]RoomsOutgoingWebhook, error) {
	out, err := JSON[struct {
		Webhooks []RoomsOutgoingWebhook `json:"webhooks"`
	}](ctx, r.c, Request{Path: "/webhooks/outgoing"})
	return out.Webhooks, err
}

// CreateOutgoingWebhook crea uno; "sytue" es el dialecto por defecto y no viaja.
func (r *Rooms) CreateOutgoingWebhook(ctx context.Context, w RoomsOutgoingWebhook) (RoomsOutgoingWebhook, error) {
	if w.Dialect == "sytue" {
		w.Dialect = ""
	}
	out, err := PostJSON[struct {
		Webhook RoomsOutgoingWebhook `json:"webhook"`
	}](ctx, r.c, "/webhooks/outgoing", struct {
		CallbackURL  string   `json:"callback_url"`
		ChannelID    string   `json:"channel_id,omitempty"`
		TriggerWords []string `json:"trigger_words,omitempty"`
		Dialect      string   `json:"dialect,omitempty"`
	}{w.CallbackURL, w.ChannelID, w.TriggerWords, w.Dialect})
	return out.Webhook, err
}

// DeleteOutgoingWebhook lo borra.
func (r *Rooms) DeleteOutgoingWebhook(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/webhooks/outgoing/", id))
}

// Bots son los de la organización.
func (r *Rooms) Bots(ctx context.Context) ([]RoomsBot, error) {
	out, err := JSON[struct {
		Bots []RoomsBot `json:"bots"`
	}](ctx, r.c, Request{Path: "/bots"})
	return out.Bots, err
}

// CreateBot crea uno y devuelve su token en claro.
func (r *Rooms) CreateBot(ctx context.Context, nombre string) (RoomsBot, string, error) {
	out, err := PostJSON[struct {
		Bot   RoomsBot `json:"bot"`
		Token string   `json:"token"`
	}](ctx, r.c, "/bots", struct {
		Name string `json:"name"`
	}{nombre})
	return out.Bot, out.Token, err
}

// DeleteBot lo borra.
func (r *Rooms) DeleteBot(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/bots/", id))
}

// SetBotInChannel añade el bot al canal o lo saca de él.
func (r *Rooms) SetBotInChannel(ctx context.Context, bot, canal string, dentro bool) error {
	if dentro {
		return r.conCuerpo(ctx, http.MethodPost, ruta("/bots/", bot, "/channels/", canal), struct{}{})
	}
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/bots/", bot, "/channels/", canal))
}

// Bridges son los puentes del canal.
func (r *Rooms) Bridges(ctx context.Context, canal string) ([]RoomsBridge, error) {
	out, err := JSON[struct {
		Bridges []RoomsBridge `json:"bridges"`
	}](ctx, r.c, Request{Path: ruta("/channels/", canal, "/bridges")})
	return out.Bridges, err
}

// DeleteBridge lo desconecta.
func (r *Rooms) DeleteBridge(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/bridges/", id))
}

// WhatsApp son los números conectados.
func (r *Rooms) WhatsApp(ctx context.Context) ([]RoomsWhatsApp, error) {
	out, err := JSON[struct {
		Accounts []RoomsWhatsApp `json:"accounts"`
	}](ctx, r.c, Request{Path: "/whatsapp"})
	return out.Accounts, err
}

// DisconnectWhatsApp lo desconecta.
func (r *Rooms) DisconnectWhatsApp(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/whatsapp/", id))
}

// CreateCommand registra un comando de servidor (admin).
func (r *Rooms) CreateCommand(ctx context.Context, c RoomsCommand) (RoomsCommand, error) {
	out, err := PostJSON[struct {
		Command RoomsCommand `json:"command"`
	}](ctx, r.c, "/commands", struct {
		Name        string `json:"name"`
		RequestURL  string `json:"request_url"`
		Description string `json:"description,omitempty"`
		UsageHint   string `json:"usage_hint,omitempty"`
	}{c.Name, c.RequestURL, c.Description, c.UsageHint})
	return out.Command, err
}

// DeleteCommand lo borra (admin).
func (r *Rooms) DeleteCommand(ctx context.Context, id string) error {
	return r.sinCuerpo(ctx, http.MethodDelete, ruta("/commands/", id))
}
