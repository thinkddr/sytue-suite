// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"net/http"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/matrixstate"
)

var ErrStorageNotFrozen = matrixstate.ErrStorageNotFrozen

// FrozenStorageBindings checks current standard Matrix state, not a cached UI
// flag. The metadata controller must own every source room and this account must
// be unable to write, redact or change policy. No recovery material is exported.
func (c *Client) FrozenStorageBindings(ctx context.Context) (storage.FrozenBinding, error) {
	c.stateMu.RLock()
	scope, controller := c.storageScope(), c.cfg.Bootstrap.ControllerUserID
	c.stateMu.RUnlock()
	binding, err := c.PrepareStorage(ctx, scope)
	if err != nil {
		return storage.FrozenBinding{}, err
	}
	frozen := storage.FrozenBinding{Binding: binding}
	for _, room := range binding.Rooms {
		var state []*event.Event
		// Keep SDK authentication/refresh and decoding, while avoiding
		// StateAsArray's unchecked dereference of malformed null elements.
		_, err := c.cli.MakeRequest(ctx, http.MethodGet, c.cli.BuildClientURL("v3", "rooms", id.RoomID(room), "state"), nil, &state)
		if err != nil {
			return storage.FrozenBinding{}, err
		}
		fence, err := frozenStorageRoom(state, room, c.session.UserID, id.UserID(controller))
		if err != nil {
			return storage.FrozenBinding{}, err
		}
		frozen.Fences = append(frozen.Fences, fence)
	}
	current, err := c.StorageBindings(ctx)
	if err != nil {
		return storage.FrozenBinding{}, err
	}
	if !binding.Same(current) || !frozen.Valid() {
		return storage.FrozenBinding{}, ErrIdentity
	}
	return frozen, nil
}

func frozenStorageRoom(state []*event.Event, room string, own, controller id.UserID) (storage.Reference, error) {
	return matrixstate.FrozenRoom(state, room, own, controller)
}
