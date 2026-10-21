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

// ContactsConnector consulta la API REST de Sytue Contacts (búsqueda de
// contactos) y normaliza los contactos encontrados al modelo canónico de Hub.
type ContactsConnector struct {
	client    *httpClient
	publicURL string
}

// NewContactsConnector construye el conector de Contacts.
func NewContactsConnector(apiBaseURL, publicURL string, timeout time.Duration) *ContactsConnector {
	return &ContactsConnector{
		client:    newHTTPClient(apiBaseURL, timeout),
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// Source implementa Searcher.
func (c *ContactsConnector) Source() domain.Source { return domain.SourceContacts }

// contactCard es la forma (parcial) del contacto que devuelve Contacts. El campo
// visible es formatted_name y el email va en la lista emails[] (vCard).
type contactCard struct {
	ID            string `json:"id"`
	FormattedName string `json:"formatted_name"`
	Org           string `json:"organization"`
	Emails        []struct {
		Email string `json:"email"`
	} `json:"emails"`
}

// contactsSearchResponse es la envoltura de resultados de Contacts.
type contactsSearchResponse struct {
	Contacts []contactCard `json:"contacts"`
}

// Search llama a GET /api/v1/contacts?q=... de Contacts (handler SearchContacts) y
// normaliza. La búsqueda de contactos vive en la colección, no en un subrecurso.
func (c *ContactsConnector) Search(ctx context.Context, token, query string, limit int) ([]domain.SearchHit, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))

	var resp contactsSearchResponse
	if err := c.client.getJSON(ctx, token, "/api/v1/contacts", q, &resp); err != nil {
		return nil, err
	}

	hits := make([]domain.SearchHit, 0, len(resp.Contacts))
	for _, ct := range resp.Contacts {
		var email string
		if len(ct.Emails) > 0 {
			email = ct.Emails[0].Email
		}
		title := ct.FormattedName
		if title == "" {
			title = email
		}
		hits = append(hits, domain.SearchHit{
			Source:  domain.SourceContacts,
			Kind:    "contact",
			ID:      ct.ID,
			Title:   title,
			Snippet: joinNonEmpty(" · ", email, ct.Org),
			URL:     c.publicURL + "/contacts/" + url.PathEscape(ct.ID),
			Icon:    "contact",
		})
	}
	return hits, nil
}

// Recent no aporta actividad: Contacts no tiene «recientes» naturales (la
// búsqueda exige q; un contacto no es actividad temporal como un fichero o
// correo), así que este origen se omite del feed de actividad reciente.
// ponytail: (nil,nil) deliberado; si algún día importa, exponer contactos por
// updated_at DESC upstream y cablearlo aquí.
func (c *ContactsConnector) Recent(context.Context, string, int) ([]domain.SearchHit, error) {
	return nil, nil
}
