// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// MailConnector consulta la API de Sytue Mail (búsqueda JMAP/REST de mensajes) y
// normaliza los correos encontrados al modelo canónico de Hub.
type MailConnector struct {
	client    *httpClient
	publicURL string
}

// NewMailConnector construye el conector de Mail.
func NewMailConnector(apiBaseURL, publicURL string, timeout time.Duration) *MailConnector {
	return &MailConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *MailConnector) Source() domain.Source { return domain.SourceMail }

// mailMessage es la forma (parcial) del mensaje que devuelve Mail.
type mailMessage struct {
	ID      string     `json:"id"`
	Subject string     `json:"subject"`
	From    string     `json:"from"`
	Preview string     `json:"preview"`
	Date    *time.Time `json:"date"`
}

// mailSearchResponse es la envoltura de resultados de Mail.
type mailSearchResponse struct {
	Messages []mailMessage `json:"messages"`
}

// Search llama a GET /api/v1/messages/search?q=... de Mail y normaliza.
func (c *MailConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))

	var resp mailSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/messages/search", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		title := m.Subject
		if title == "" {
			title = "(sin asunto)"
		}
		hits = append(hits, domain.SearchHit{
			Source:    domain.SourceMail,
			Kind:      "message",
			ID:        m.ID,
			Title:     title,
			Snippet:   joinNonEmpty(" — ", m.From, m.Preview),
			URL:       c.publicURL + "/messages/" + url.PathEscape(m.ID),
			Icon:      "mail",
			Timestamp: m.Date,
		})
	}
	return hits, nil
}

// Recent no aporta actividad todavía: Mail no expone un listado global de
// mensajes recientes consumible por Hub (listMessages exige un mailboxID; no hay
// «recientes de todas las bandejas»).
// ponytail: (nil,nil) evita una llamada que fallaría; cablear cuando Mail exponga
// GET /api/v1/messages/recent (o el conector oriente listMailboxes→INBOX→listMessages).
func (c *MailConnector) Recent(context.Context, string, int) ([]domain.SearchHit, error) {
	return nil, nil
}

// joinNonEmpty une con sep los fragmentos no vacíos.
func joinNonEmpty(sep string, parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, sep)
}

// SinLeer es cuántos correos esperan a la persona: los no leídos de sus carpetas,
// salvo las que no son de recibir —Enviados, Borradores— y las que no se esperan
// —Spam, Papelera—. Lo pinta la campana como estado vivo, porque un correo que ya
// estaba sin leer antes de que existiera el aviso no deja aviso.
func (c *MailConnector) SinLeer(ctx context.Context, token string) (int64, error) {
	var resp struct {
		Mailboxes []struct {
			Role   string `json:"role"`
			Unread int64  `json:"unread_emails"`
		} `json:"mailboxes"`
	}
	if err := c.client.getJSON(ctx, token, "/api/v1/mailboxes", nil, &resp); err != nil {
		return 0, err
	}
	var n int64
	for _, m := range resp.Mailboxes {
		switch m.Role {
		case "sent", "drafts", "junk", "trash":
		default:
			n += m.Unread
		}
	}
	return n, nil
}
