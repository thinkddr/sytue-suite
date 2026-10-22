// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/url"
	"time"
)

// Linked records live at the account's provider. Their IDs and ETags are opaque,
// and edits are patches: a native editor must not rebuild a whole remote record.
type LinkedServiceCapability struct {
	Service   string `json:"service"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}
type RemoteCollection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color,omitempty"`
	TimeZone string `json:"time_zone,omitempty"`
	ReadOnly bool   `json:"read_only"`
}
type RemotePage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}
type RemoteResource struct {
	ID          string `json:"id"`
	ETag        string `json:"etag,omitempty"`
	ReadOnly    bool   `json:"read_only"`
	ProviderURL string `json:"provider_url,omitempty"`
}
type RemoteAttendee struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}
type RemoteEvent struct {
	RemoteResource
	CollectionID string           `json:"collection_id"`
	Summary      string           `json:"summary"`
	Description  string           `json:"description,omitempty"`
	Location     string           `json:"location,omitempty"`
	Start        time.Time        `json:"start"`
	End          time.Time        `json:"end"`
	TimeZone     string           `json:"time_zone,omitempty"`
	AllDay       bool             `json:"all_day"`
	Recurrence   []string         `json:"recurrence,omitempty"`
	Attendees    []RemoteAttendee `json:"attendees,omitempty"`
}
type RemoteEventPatch struct {
	Summary     *string    `json:"summary,omitempty"`
	Description *string    `json:"description,omitempty"`
	Location    *string    `json:"location,omitempty"`
	Start       *time.Time `json:"start,omitempty"`
	End         *time.Time `json:"end,omitempty"`
	TimeZone    *string    `json:"time_zone,omitempty"`
	AllDay      *bool      `json:"all_day,omitempty"`
}
type RemoteContact struct {
	RemoteResource
	CollectionID string   `json:"collection_id"`
	Name         string   `json:"name"`
	GivenName    string   `json:"given_name,omitempty"`
	FamilyName   string   `json:"family_name,omitempty"`
	Organization string   `json:"organization,omitempty"`
	Title        string   `json:"title,omitempty"`
	Note         string   `json:"note,omitempty"`
	Emails       []string `json:"emails,omitempty"`
	Phones       []string `json:"phones,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
}
type RemoteContactPatch struct {
	Name         *string   `json:"name,omitempty"`
	GivenName    *string   `json:"given_name,omitempty"`
	FamilyName   *string   `json:"family_name,omitempty"`
	Organization *string   `json:"organization,omitempty"`
	Title        *string   `json:"title,omitempty"`
	Note         *string   `json:"note,omitempty"`
	Emails       *[]string `json:"emails,omitempty"`
	Phones       *[]string `json:"phones,omitempty"`
}
type RemoteNote struct {
	RemoteResource
	CollectionID string    `json:"collection_id"`
	Title        string    `json:"title"`
	Text         string    `json:"text,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	CanAppend    bool      `json:"can_append"`
}

func linkedPath(account ExternalMailAccount, suffix string) string {
	return ruta("/linked-services/", account.InstanceID, "/"+suffix)
}

func linkedQuery(account ExternalMailAccount, collection string) url.Values {
	return url.Values{"collection": {collection}, "expected_address": {account.Address}}
}

func (i *Integraciones) LinkedCollections(ctx context.Context, account ExternalMailAccount, service string) ([]RemoteCollection, error) {
	result, err := JSON[RemotePage[RemoteCollection]](ctx, i.c, Request{Path: linkedPath(account, url.PathEscape(service)+"/collections"), Query: linkedQuery(account, "")})
	return result.Items, err
}

func (i *Integraciones) LinkedEvents(ctx context.Context, account ExternalMailAccount, collection string, from, to time.Time, cursor string) (RemotePage[RemoteEvent], error) {
	q := linkedQuery(account, collection)
	q.Set("from", from.Format(time.RFC3339))
	q.Set("to", to.Format(time.RFC3339))
	q.Set("cursor", cursor)
	return JSON[RemotePage[RemoteEvent]](ctx, i.c, Request{Path: linkedPath(account, "calendar/events"), Query: q})
}

func (i *Integraciones) LinkedEvent(ctx context.Context, account ExternalMailAccount, collection, id string) (RemoteEvent, error) {
	q := linkedQuery(account, collection)
	q.Set("id", id)
	return JSON[RemoteEvent](ctx, i.c, Request{Path: linkedPath(account, "calendar/event"), Query: q})
}

func (i *Integraciones) SaveLinkedEvent(ctx context.Context, account ExternalMailAccount, collection, id, etag string, patch RemoteEventPatch) (RemoteEvent, error) {
	return PostJSON[RemoteEvent](ctx, i.c, linkedPath(account, "calendar/save"), map[string]any{"expected_address": account.Address, "collection": collection, "id": id, "etag": etag, "patch": patch})
}

func (i *Integraciones) LinkedContacts(ctx context.Context, account ExternalMailAccount, collection, cursor string) (RemotePage[RemoteContact], error) {
	q := linkedQuery(account, collection)
	q.Set("cursor", cursor)
	return JSON[RemotePage[RemoteContact]](ctx, i.c, Request{Path: linkedPath(account, "contacts/items"), Query: q})
}

func (i *Integraciones) LinkedContact(ctx context.Context, account ExternalMailAccount, collection, id string) (RemoteContact, error) {
	q := linkedQuery(account, collection)
	q.Set("id", id)
	return JSON[RemoteContact](ctx, i.c, Request{Path: linkedPath(account, "contacts/item"), Query: q})
}

func (i *Integraciones) SaveLinkedContact(ctx context.Context, account ExternalMailAccount, collection, id, etag string, patch RemoteContactPatch) (RemoteContact, error) {
	return PostJSON[RemoteContact](ctx, i.c, linkedPath(account, "contacts/save"), map[string]any{"expected_address": account.Address, "collection": collection, "id": id, "etag": etag, "patch": patch})
}

func (i *Integraciones) LinkedNotes(ctx context.Context, account ExternalMailAccount, collection, cursor string) (RemotePage[RemoteNote], error) {
	q := linkedQuery(account, collection)
	q.Set("cursor", cursor)
	return JSON[RemotePage[RemoteNote]](ctx, i.c, Request{Path: linkedPath(account, "notes/items"), Query: q})
}

func (i *Integraciones) LinkedNote(ctx context.Context, account ExternalMailAccount, collection, id string) (RemoteNote, error) {
	q := linkedQuery(account, collection)
	q.Set("id", id)
	return JSON[RemoteNote](ctx, i.c, Request{Path: linkedPath(account, "notes/item"), Query: q})
}

func (i *Integraciones) CreateLinkedNote(ctx context.Context, account ExternalMailAccount, collection, title, text string) (RemoteNote, error) {
	return PostJSON[RemoteNote](ctx, i.c, linkedPath(account, "notes/write"), map[string]string{"expected_address": account.Address, "collection": collection, "title": title, "text": text})
}

func (i *Integraciones) AppendLinkedNote(ctx context.Context, account ExternalMailAccount, collection, id, text string) error {
	_, err := PostJSON[struct{}](ctx, i.c, linkedPath(account, "notes/write"), map[string]string{"expected_address": account.Address, "collection": collection, "id": id, "text": text})
	return err
}
