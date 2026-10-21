// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Plataformas con las que un canal se puede espejar. Son las de la columna
// `platform` de channel_bridges (mig. 0033).
const (
	BridgeSlack      = "slack"
	BridgeTeams      = "teams"
	BridgeGoogleChat = "googlechat"
)

// MaxBridgesPerChannel acota cuántos destinos repite un canal: cada mensaje es
// un POST por puente dentro del consumidor del outbox.
const MaxBridgesPerChannel = 5

// Bridge espeja los mensajes de un canal de Rooms en un canal de Slack, Microsoft
// Teams o Google Chat, a través del webhook entrante que ofrece cada uno. La
// vuelta la hace el webhook entrante de Rooms (con sus alias /hooks/slack|teams|
// googlechat), así que un canal puede quedar conectado en los dos sentidos sin
// instalar una app en el otro lado.
type Bridge struct {
	ID        uuid.UUID `json:"id"`
	Tenant    string    `json:"-"`
	ChannelID uuid.UUID `json:"channel_id"`
	Platform  string    `json:"platform"`
	// WebhookURL es un secreto: quien la tiene publica en el canal ajeno. Se
	// guarda cifrada y no sale nunca por la API.
	WebhookURL string `json:"-"`
	// OAuth account and provider destination, with no access token in Rooms.
	InstanceID string `json:"instance_id,omitempty"`
	RemoteID   string `json:"remote_id,omitempty"`
	// Target es la URL enmascarada, para reconocer el destino en la lista.
	Target      string     `json:"target"`
	LastError   string     `json:"last_error,omitempty"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ValidBridgePlatform dice si la plataforma es una de las soportadas.
func ValidBridgePlatform(p string) bool {
	return p == BridgeSlack || p == BridgeTeams || p == BridgeGoogleChat
}

// ValidBridgeURL acepta solo el webhook entrante AUTÉNTICO de cada plataforma:
// https, sin credenciales en la URL y en el host del proveedor. Es una lista
// blanca a propósito: un puente reenvía TODO lo que se escribe en el canal, y
// una URL cualquiera lo convertiría en una fuga de conversaciones a un tercero.
func ValidBridgeURL(platform, raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch platform {
	case BridgeSlack:
		return host == "hooks.slack.com" && len(u.Path) > 1
	case BridgeTeams:
		// Conectores clásicos (webhook.office.com) y flujos de Workflows, que
		// publican en Azure Logic Apps o en Power Platform.
		for _, sufijo := range []string{".webhook.office.com", ".logic.azure.com", ".powerplatform.com"} {
			if strings.HasSuffix(host, sufijo) {
				return len(u.Path) > 1
			}
		}
		return false
	case BridgeGoogleChat:
		return host == "chat.googleapis.com" && strings.HasPrefix(u.Path, "/v1/spaces/")
	}
	return false
}

// MaskBridgeURL deja el host y los cuatro últimos caracteres de la ruta: basta
// para distinguir dos puentes y no sirve para publicar.
func MaskBridgeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	cola := strings.TrimRight(u.Path, "/")
	if len(cola) > 4 {
		cola = cola[len(cola)-4:]
	}
	return u.Hostname() + "/…" + cola
}
