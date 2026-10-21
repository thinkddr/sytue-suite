// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

// WhatsApp Business (Cloud API de Meta): cada cliente que escribe al número de
// la organización tiene su canal en Rooms, y lo que el equipo contesta en ese
// canal le llega por WhatsApp. Aquí vive lo PURO: tipos, firma y lectura del
// webhook. La red y la base están en service y store.

// WhatsAppAccount es un número de WhatsApp Business conectado a un equipo.
type WhatsAppAccount struct {
	ID            uuid.UUID `json:"id"`
	Tenant        string    `json:"-"`
	TeamID        uuid.UUID `json:"team_id"`
	PhoneNumberID string    `json:"phone_number_id"`
	DisplayPhone  string    `json:"display_phone"`
	VerifiedName  string    `json:"verified_name,omitempty"`
	InstanceID    string    `json:"-"`
	Ready         bool      `json:"-"`
	// AccessToken (token permanente de un usuario del sistema) y AppSecret (el
	// que firma los webhooks) se guardan cifrados y no salen nunca por la API.
	AccessToken string    `json:"-"`
	AppSecret   string    `json:"-"`
	CreatedBy   string    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// WhatsAppChat liga un cliente (su wa_id) con el canal donde se le atiende.
type WhatsAppChat struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	WaID          string
	ChannelID     uuid.UUID
	ContactName   string
	LastInboundAt time.Time
}

// WhatsAppInbound contains routing and deduplication metadata only. Provider
// content, contact names and media are deliberately never decoded or imported.
type WhatsAppInbound struct {
	PhoneNumberID string
	WaID          string
	MessageID     string
	At            time.Time
}

const WhatsAppInboundNotice = "Nueva actividad en WhatsApp. El contenido no se importa a Rooms."

// MaxWhatsAppText es el tope de caracteres de un mensaje de texto de WhatsApp.
const MaxWhatsAppText = 4096

// ValidWhatsAppSignature comprueba X-Hub-Signature-256 («sha256=<hex>»): el
// HMAC-SHA256 del cuerpo CRUDO con el secreto de la app de Meta. Sin esto,
// cualquiera que conozca la URL podría escribir en nombre de un cliente.
func ValidWhatsAppSignature(appSecret string, body []byte, header string) bool {
	hexsig, ok := strings.CutPrefix(strings.TrimSpace(header), "sha256=")
	if !ok || appSecret == "" {
		return false
	}
	return keycrypto.VerifyHMACSHA256(appSecret, body, hexsig)
}

// ParseWhatsAppWebhook saca los mensajes de clientes de un aviso de la Cloud API.
// Los avisos de estado (entregado, leído) y lo que no es un mensaje se ignoran.
func ParseWhatsAppWebhook(body []byte) ([]WhatsAppInbound, error) {
	var aviso struct {
		Entry []struct {
			Changes []struct {
				Field string `json:"field"`
				Value struct {
					Metadata struct {
						PhoneNumberID string `json:"phone_number_id"`
					} `json:"metadata"`
					Messages []whatsappMessage `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &aviso); err != nil {
		return nil, fmt.Errorf("aviso de WhatsApp ilegible: %w", err)
	}
	var out []WhatsAppInbound
	for _, e := range aviso.Entry {
		for _, c := range e.Changes {
			if c.Field != "" && c.Field != "messages" {
				continue
			}
			for _, m := range c.Value.Messages {
				if m.ID == "" || !ValidWaID(m.From) {
					continue
				}
				in := WhatsAppInbound{
					PhoneNumberID: c.Value.Metadata.PhoneNumberID, WaID: m.From,
					MessageID: m.ID, At: time.Now().UTC(),
				}
				var segundos int64
				if _, err := fmt.Sscan(m.Timestamp, &segundos); err == nil && segundos > 0 {
					in.At = time.Unix(segundos, 0).UTC()
				}
				out = append(out, in)
			}
		}
	}
	return out, nil
}

type whatsappMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
}

// ValidWaID acepta el identificador de WhatsApp de un cliente: su número en
// formato internacional, solo cifras.
func ValidWaID(s string) bool {
	if len(s) < 6 || len(s) > 15 {
		return false
	}
	return strings.Trim(s, "0123456789") == ""
}

// WhatsAppChannelSlug es el slug del canal de un cliente. Lleva el principio del
// id de la cuenta porque el mismo cliente puede escribir a dos números de la
// organización que atiende el mismo equipo.
func WhatsAppChannelSlug(accountID uuid.UUID, waID string) string {
	return "wa-" + waID + "-" + accountID.String()[:6]
}
