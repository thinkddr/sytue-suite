// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var errNotDraft = errors.New("matrix: event has no draft payload")

type draftContent struct {
	Version   int       `json:"v"`
	Tenant    string    `json:"tenant"`
	ChannelID string    `json:"channel_id"`
	ParentID  string    `json:"parent_id,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PutDraft persists the same encrypted private-room format as the web client.
// Empty body is a tombstone; no draft body is sent to the Rooms service.
func (r *RoomsService) PutDraft(ctx context.Context, channel, parent, body string) error {
	c, _, err := r.connection(ctx, channel)
	if err != nil {
		return err
	}
	room, _ := c.DraftRooms()
	if room == "" {
		return ErrIdentity
	}
	_, tenant := c.Identity()
	if len(body) > 1<<20 {
		return ErrIdentity
	}
	_, err = c.sendEvent(ctx, room, event.EventMessage, map[string]any{
		"msgtype": "m.text", "body": body, "org.sytue.rooms.draft": draftContent{Version: 1, Tenant: tenant, ChannelID: channel, ParentID: parent, UpdatedAt: time.Now().UTC()},
	})
	return err
}

// Drafts merges own authenticated events across current and legacy private rooms.
// Pagination never silently turns an incomplete scan into an empty draft list.
func (r *RoomsService) Drafts(ctx context.Context) ([]api.RoomsDraft, error) {
	c, err := r.Client(ctx)
	if err != nil {
		return nil, err
	}
	current, rooms := c.DraftRooms()
	if current == "" {
		return nil, ErrIdentity
	}
	if !slices.Contains(rooms, current) {
		rooms = append(rooms, current)
	}
	own, tenant := c.Identity()
	latest := map[string]api.RoomsDraft{}
	for _, room := range rooms {
		if err := collectDrafts(ctx, c, room, own, tenant, latest); err != nil {
			return nil, err
		}
	}
	out := make([]api.RoomsDraft, 0, len(latest))
	for _, draft := range latest {
		if draft.Body != "" {
			out = append(out, draft)
		}
	}
	slices.SortFunc(out, func(a, b api.RoomsDraft) int { return a.UpdatedAt.Compare(b.UpdatedAt) })
	return out, nil
}

func collectDrafts(ctx context.Context, c *Client, room, own, tenant string, latest map[string]api.RoomsDraft) error {
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := c.History(ctx, room, cursor, 100)
		if err != nil {
			return err
		}
		for _, value := range page.Events {
			draft, err := decodeDraft(value, own, tenant)
			if errors.Is(err, errNotDraft) {
				continue
			}
			if err != nil {
				return err
			}
			if draft == nil {
				continue
			}
			if _, err := c.RoomID(draft.ChannelID); err != nil {
				continue
			}
			parent := ""
			if draft.ParentID != nil {
				parent = *draft.ParentID
			}
			key := draft.ChannelID + "\x00" + parent
			prior, ok := latest[key]
			if !ok || draft.UpdatedAt.After(prior.UpdatedAt) {
				latest[key] = *draft
			}
		}
		if page.Next == "" {
			return nil
		}
		if seen[page.Next] {
			return ErrIdentity
		}
		seen[page.Next] = true
		cursor = page.Next
	}
}

func decodeDraft(value DecryptedEvent, own, tenant string) (*api.RoomsDraft, error) {
	if value.Sender != own {
		return nil, ErrIdentity
	}
	var payload struct {
		Body  string        `json:"body"`
		Draft *draftContent `json:"org.sytue.rooms.draft"` //nolint:tagliatelle // Matrix namespaced extension.
	}
	if json.Unmarshal(value.Content, &payload) != nil {
		return nil, ErrIdentity
	}
	d := payload.Draft
	if d == nil {
		return nil, errNotDraft
	} // A different event type has no draft payload.
	if d.Version != 1 || d.Tenant != tenant || d.ChannelID == "" || d.UpdatedAt.IsZero() || len(payload.Body) > 1<<20 {
		return nil, ErrIdentity
	}
	draft := &api.RoomsDraft{ChannelID: d.ChannelID, Body: payload.Body, UpdatedAt: d.UpdatedAt}
	if d.ParentID != "" {
		parent := d.ParentID
		draft.ParentID = &parent
	}
	return draft, nil
}
