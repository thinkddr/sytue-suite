// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// porPaginaCorreo es la página de la Entrada que se lee: la misma que la bandeja.
const porPaginaCorreo = 50

// Mail lee SIEMPRE Recibidos: el contador autoritativo del servicio y las cabeceras de
// sus últimas conversaciones (remitente, asunto y hora; nunca el cuerpo ni el
// extracto), aunque la app esté en otra carpeta, buscando o en un buzón compartido.
// Peticiones: 2.
func Mail(c Comunes, m *api.Mail) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		buzones, err := m.Mailboxes(ctx)
		if err != nil {
			return widgets.Detail{}, err
		}
		d := widgets.Detail{Counters: []widgets.Counter{{Kind: "unread", Value: int64(max(api.MailInboxUnread(buzones), 0))}}}
		entrada := ""
		for _, b := range buzones {
			if b.Role == "inbox" {
				entrada = b.ID
				break
			}
		}
		if entrada == "" {
			return d, nil
		}
		mensajes, _, err := m.Messages(ctx, entrada, "", porPaginaCorreo)
		if err != nil {
			return widgets.Detail{}, err
		}
		d.Items = cabecerasDeEntrada(c, mensajes)
		return d, nil
	}
}

// cabecerasDeEntrada agrupa por hilo (el primero que llega es el más nuevo) y deja una
// fila por conversación, sin leer si alguno de sus mensajes no se ha leído.
func cabecerasDeEntrada(c Comunes, mensajes []api.MailMessage) []widgets.Item {
	type hilo struct {
		ultimo  api.MailMessage
		sinLeer bool
	}
	var orden []string
	porHilo := map[string]*hilo{}
	for _, m := range mensajes {
		clave := m.ThreadID
		if clave == "" {
			clave = "msg:" + m.ID
		}
		h, ya := porHilo[clave]
		if !ya {
			h = &hilo{ultimo: m}
			porHilo[clave] = h
			orden = append(orden, clave)
		}
		h.sinLeer = h.sinLeer || !m.Has(api.MailFlagSeen)
	}
	items := make([]widgets.Item, 0, min(len(orden), widgets.MaxItems))
	for _, clave := range orden {
		h := porHilo[clave]
		remitente := primero(h.ultimo.FromName, h.ultimo.From)
		asunto := strings.TrimSpace(h.ultimo.Subject)
		titulo, subtitulo := remitente, asunto
		if titulo == "" {
			titulo, subtitulo = asunto, ""
		}
		if titulo == "" {
			continue
		}
		var insignia string
		if cuando := unix(h.ultimo.ReceivedAt); cuando > 0 {
			insignia = h.ultimo.ReceivedAt.In(c.zona()).Format("15:04")
		}
		items = append(items, widgets.Item{
			Title: titulo, Subtitle: subtitulo, Badge: insignia,
			StartAt: unix(h.ultimo.ReceivedAt), Unread: h.sinLeer,
		})
		if llenos(items) {
			break
		}
	}
	return widgets.SanitizeItems(items)
}
