// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Microsoft OneNote uses these fixed camelCase and OData wire keys.
package linkedservices

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// Personal Google Keep and Yahoo notes have no supported delegated user path.
// OneNote is read as text; creating and appending preserve the existing rich
// page. A text editor must never replace images/tables with a flattened body.
type graphNote struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Updated time.Time `json:"lastModifiedDateTime"`
	Links   struct {
		Web struct {
			Href string `json:"href"`
		} `json:"oneNoteWebUrl"`
	} `json:"links"`
}

func (g graphNote) note(collection string) Note {
	return Note{Resource: Resource{ID: g.ID, ProviderURL: safeProviderURL(g.Links.Web.Href)}, CollectionID: collection, Title: g.Title, UpdatedAt: g.Updated, CanAppend: true}
}

func (c *Client) Notes(ctx context.Context, collection, cursor string) (Page[Note], error) {
	result := Page[Note]{Items: []Note{}}
	if c.provider != "microsoft" {
		return result, ErrReadOnly
	}
	if !validID(collection) {
		return result, ErrInvalid
	}
	prefix := graph + "/onenote/sections/" + url.PathEscape(collection) + "/pages"
	target, err := c.nextURL(cursor, query(prefix, url.Values{"$top": {"100"}, "$orderby": {"lastModifiedDateTime desc"}}), prefix)
	if err != nil {
		return result, err
	}
	var wire struct {
		Items []graphNote `json:"value"`
		Next  string      `json:"@odata.nextLink"`
	}
	if err := c.json(ctx, http.MethodGet, target, nil, &wire, nil); err != nil {
		return result, err
	}
	for _, item := range wire.Items {
		result.Items = append(result.Items, item.note(collection))
	}
	result.NextCursor, err = c.cursor(wire.Next, prefix)
	return result, err
}

func (c *Client) Note(ctx context.Context, collection, id string) (Note, error) {
	if c.provider != "microsoft" {
		return Note{}, ErrReadOnly
	}
	if !validID(collection) || !validID(id) {
		return Note{}, ErrInvalid
	}
	var wire graphNote
	if err := c.json(ctx, http.MethodGet, graph+"/onenote/pages/"+url.PathEscape(id), nil, &wire, nil); err != nil {
		return Note{}, err
	}
	raw, _, err := c.request(ctx, http.MethodGet, graph+"/onenote/pages/"+url.PathEscape(id)+"/content", "", nil, nil)
	if err != nil {
		return Note{}, err
	}
	result := wire.note(collection)
	result.Text = htmltext.Text(string(raw))
	return result, nil
}

func noteParagraphs(text string) string {
	return "<p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br />") + "</p>"
}

func (c *Client) CreateNote(ctx context.Context, collection, title, text string) (Note, error) {
	if c.provider != "microsoft" {
		return Note{}, ErrReadOnly
	}
	if !validID(collection) || strings.TrimSpace(title) == "" || len(title) > 2048 || len(text) > 1<<20 {
		return Note{}, ErrInvalid
	}
	body := "<!DOCTYPE html><html><head><title>" + html.EscapeString(title) + "</title></head><body>" + noteParagraphs(text) + "</body></html>"
	raw, _, err := c.request(ctx, http.MethodPost, graph+"/onenote/sections/"+url.PathEscape(collection)+"/pages", "text/html; charset=utf-8", []byte(body), nil)
	if err != nil {
		return Note{}, err
	}
	var wire graphNote
	if err = json.Unmarshal(raw, &wire); err != nil {
		return Note{}, ErrWriteUnknown
	}
	result := wire.note(collection)
	result.Text = text
	return result, nil
}

func (c *Client) AppendNote(ctx context.Context, collection, id, text string) error {
	if c.provider != "microsoft" {
		return ErrReadOnly
	}
	if !validID(collection) || !validID(id) || strings.TrimSpace(text) == "" || len(text) > 1<<20 {
		return ErrInvalid
	}
	body := []map[string]string{{"target": "body", "action": "append", "content": noteParagraphs(text)}}
	return c.json(ctx, http.MethodPatch, graph+"/onenote/pages/"+url.PathEscape(id)+"/content", body, nil, nil)
}
