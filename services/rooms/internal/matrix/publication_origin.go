// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// StoragePublicationOrigin reads opaque, persisted ciphertext as the existing
// room controller. It never obtains an owner access token, creates a device,
// joins a room or decrypts data. It deliberately remains usable after withdrawal
// so a lost origin acknowledgement cannot revive the retired producer.
func (c *Client) StoragePublicationOrigin(ctx context.Context, scope storage.Scope, reference storage.Reference) (publication.OriginEvent, error) {
	expected, err := c.StorageUserID(scope)
	if err != nil {
		return publication.OriginEvent{}, err
	}
	return c.retainedPublicationOrigin(ctx, scope, reference, expected)
}

// RetainedPublicationOrigin reads only a sender already captured by the origin
// control plane. Canonical and isolated storage keep their original identities;
// an arbitrary user or another product cannot become the historical producer.
func (c *Client) RetainedPublicationOrigin(ctx context.Context, scope storage.Scope, reference storage.Reference, retainedSender string) (publication.OriginEvent, error) {
	isolated, err := c.StorageUserID(scope)
	if err != nil {
		return publication.OriginEvent{}, err
	}
	canonical, err := c.UserID(scope.Subject)
	if err != nil {
		return publication.OriginEvent{}, err
	}
	if retainedSender != isolated && retainedSender != canonical {
		return publication.OriginEvent{}, domain.ErrForbidden
	}
	return c.retainedPublicationOrigin(ctx, scope, reference, retainedSender)
}

func (c *Client) retainedPublicationOrigin(ctx context.Context, scope storage.Scope, reference storage.Reference, expected string) (publication.OriginEvent, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != scope.Tenant || !reference.Valid() {
		return publication.OriginEvent{}, domain.ErrForbidden
	}
	var wire struct {
		RoomID  string          `json:"room_id"`
		EventID string          `json:"event_id"`
		Sender  string          `json:"sender"`
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := c.call(ctx, http.MethodGet, "/_matrix/client/v3/rooms/"+url.PathEscape(reference.RoomID)+"/event/"+url.PathEscape(reference.EventID), nil, &wire); err != nil {
		return publication.OriginEvent{}, err
	}
	// The fixed room endpoint binds room_id even when the client event omits it.
	if wire.RoomID != "" && wire.RoomID != reference.RoomID {
		return publication.OriginEvent{}, domain.ErrForbidden
	}
	origin := publication.OriginEvent{Version: 1, Reference: storage.Reference{RoomID: reference.RoomID, EventID: wire.EventID}, Sender: wire.Sender, Type: wire.Type, Content: wire.Content}
	if origin.Reference != reference || origin.Sender != expected || origin.Type != "m.room.encrypted" || !origin.Valid() {
		return publication.OriginEvent{}, domain.ErrForbidden
	}
	if _, _, err := publication.WireDigest(origin.Content, false); err != nil {
		return publication.OriginEvent{}, err
	}
	return origin, nil
}
