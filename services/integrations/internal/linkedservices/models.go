// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package linkedservices

import (
	"encoding/json"
	"time"
)

// Remote DTOs deliberately differ from native suite records: their identifiers
// belong to another provider, writes require its version, and some records can
// only be viewed without losing unsupported provider content.
type Collection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color,omitempty"`
	TimeZone string `json:"time_zone,omitempty"`
	ReadOnly bool   `json:"read_only"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type Resource struct {
	ID          string `json:"id"`
	ETag        string `json:"etag,omitempty"`
	ReadOnly    bool   `json:"read_only"`
	ProviderURL string `json:"provider_url,omitempty"`
	// Retained only inside an adapter. PATCH updates the explicitly edited
	// fields; it never flattens the provider's complete document into this DTO.
	Raw json.RawMessage `json:"-"`
	DAV string          `json:"-"`
}

type Attendee struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type Event struct {
	Resource
	CollectionID string     `json:"collection_id"`
	Summary      string     `json:"summary"`
	Description  string     `json:"description,omitempty"`
	Location     string     `json:"location,omitempty"`
	Start        time.Time  `json:"start"`
	End          time.Time  `json:"end"`
	TimeZone     string     `json:"time_zone,omitempty"`
	AllDay       bool       `json:"all_day"`
	Recurrence   []string   `json:"recurrence,omitempty"`
	Attendees    []Attendee `json:"attendees,omitempty"`
}

// Nil means unchanged. An explicit empty value clears only that field. The
// provider's other fields, recurrence exceptions, properties and attachments
// remain in place. Editors must send changes rather than a reconstructed event.
type EventPatch struct {
	Summary     *string     `json:"summary,omitempty"`
	Description *string     `json:"description,omitempty"`
	Location    *string     `json:"location,omitempty"`
	Start       *time.Time  `json:"start,omitempty"`
	End         *time.Time  `json:"end,omitempty"`
	TimeZone    *string     `json:"time_zone,omitempty"`
	AllDay      *bool       `json:"all_day,omitempty"`
	Recurrence  *[]string   `json:"recurrence,omitempty"`
	Attendees   *[]Attendee `json:"attendees,omitempty"`
}

type Contact struct {
	Resource
	CollectionID string   `json:"collection_id"`
	Name         string   `json:"name"`
	GivenName    string   `json:"given_name,omitempty"`
	FamilyName   string   `json:"family_name,omitempty"`
	Organization string   `json:"organization,omitempty"`
	Title        string   `json:"title,omitempty"`
	Note         string   `json:"note,omitempty"`
	Emails       []string `json:"emails,omitempty"`
	Phones       []string `json:"phones,omitempty"`
	// Displayed as read-only text unless an adapter supports editing their
	// structured representation. They are never dropped by other edits.
	Addresses []string `json:"addresses,omitempty"`
}

type ContactPatch struct {
	Name         *string   `json:"name,omitempty"`
	GivenName    *string   `json:"given_name,omitempty"`
	FamilyName   *string   `json:"family_name,omitempty"`
	Organization *string   `json:"organization,omitempty"`
	Title        *string   `json:"title,omitempty"`
	Note         *string   `json:"note,omitempty"`
	Emails       *[]string `json:"emails,omitempty"`
	Phones       *[]string `json:"phones,omitempty"`
}

type Note struct {
	Resource
	CollectionID string    `json:"collection_id"`
	Title        string    `json:"title"`
	Text         string    `json:"text,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Appending a paragraph is distinct from replacing a rich OneNote page.
	// It preserves images, tables, positioning and other existing elements.
	CanAppend bool `json:"can_append"`
}
