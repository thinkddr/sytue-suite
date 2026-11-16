// SPDX-License-Identifier: Apache-2.0
package search

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Metadata contains routing and authorization facts only. No body, attachment
// name, content key, search term or decrypted card crosses this endpoint.
type Metadata struct {
	ID          string             `json:"id"`
	Tenant      string             `json:"tenant"`
	ChannelID   string             `json:"channel_id"`
	ParentID    *string            `json:"parent_id,omitempty"`
	AuthorSub   string             `json:"author_sub"`
	AuthorName  string             `json:"author_name,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	EditedAt    *time.Time         `json:"edited_at,omitempty"`
	MatrixEvent *storage.Reference `json:"matrix_event,omitempty"`
	Type        string             `json:"type,omitempty"`
	ChannelSlug string             `json:"channel_slug"`
	ChannelName string             `json:"channel_name"`
	Saved       bool               `json:"saved"`
	Pinned      bool               `json:"pinned"`
}
type (
	Page struct {
		Messages []Metadata `json:"messages"`
		Next     string     `json:"next_cursor,omitempty"`
	}
	Identity struct {
		ID     string `json:"id"`
		Tenant string `json:"tenant"`
	}
)

func (m Metadata) Identity() Identity { return Identity{ID: m.ID, Tenant: m.Tenant} }
func (m Metadata) Document() Document {
	return Document{AuthorSub: m.AuthorSub, AuthorName: m.AuthorName, ChannelSlug: m.ChannelSlug, ChannelName: m.ChannelName, CreatedAt: m.CreatedAt, Saved: m.Saved, Pinned: m.Pinned}
}

// Position is an exclusive keyset bound, scoped to the authenticated active
// organization and subject. Tenant breaks the theoretical cross-tenant UUID tie.
type Position struct {
	Version   int       `json:"v"`
	Scope     string    `json:"scope"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
	Tenant    string    `json:"tenant"`
}

var ErrCursor = errors.New("rooms: invalid metadata cursor")

//nolint:nilnil // an empty cursor means the first page; a nil Position is valid, not an error
func DecodeCursor(raw, scope, actor string) (*Position, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 2048 {
		return nil, ErrCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrCursor
	}
	var p Position
	if json.Unmarshal(data, &p) != nil || p.Version != 1 || p.Scope != scope || p.Actor != actor || p.CreatedAt.IsZero() || p.Tenant == "" {
		return nil, ErrCursor
	}
	id, err := uuid.Parse(p.ID)
	if err != nil || id == uuid.Nil || id.String() != p.ID {
		return nil, ErrCursor
	}
	return &p, nil
}

func EncodeCursor(m Metadata, scope, actor string) string {
	raw, _ := json.Marshal(Position{Version: 1, Scope: scope, Actor: actor, CreatedAt: m.CreatedAt, ID: m.ID, Tenant: m.Tenant})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func Compare(a, b Metadata) int {
	if n := b.CreatedAt.Compare(a.CreatedAt); n != 0 {
		return n
	}
	if a.ID > b.ID {
		return -1
	}
	if a.ID < b.ID {
		return 1
	}
	if a.Tenant > b.Tenant {
		return -1
	}
	if a.Tenant < b.Tenant {
		return 1
	}
	return 0
}
