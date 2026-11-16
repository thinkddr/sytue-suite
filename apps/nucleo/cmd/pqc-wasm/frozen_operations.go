//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"strings"
	"syscall/js"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/matrixstate"
)

func init() {
	domainOperations["storage.frozen.validate"] = storageFrozenValidate
	domainOperations["storage.cutover.decode"] = storageCutoverDecode
}

func frozenUserArg(value js.Value) (id.UserID, error) {
	if value.Type() != js.TypeString {
		return "", storage.ErrRecord
	}
	text := value.String()
	if len(text) > 2048 || !strings.HasPrefix(text, "@") || !strings.Contains(text, ":") || strings.ContainsAny(text, "\x00\r\n\t") {
		return "", storage.ErrRecord
	}
	return id.UserID(text), nil
}

// These are bounded parsers, not enrollment, root pinning or authorization.
// The host obtains actual room/owner/controller and raw state from its own SDK.
func storageFrozenValidate(req js.Value) (any, error) {
	if !scopeArg(req).Valid() {
		return nil, storage.ErrRecord
	}
	roomValue := req.Get("room")
	if roomValue.Type() != js.TypeString {
		return nil, storage.ErrRecord
	}
	room := roomValue.String()
	if !(storage.Reference{RoomID: room, EventID: "$validation"}).Valid() {
		return nil, storage.ErrRecord
	}
	own, err := frozenUserArg(req.Get("own"))
	if err != nil {
		return nil, err
	}
	controller, err := frozenUserArg(req.Get("controller"))
	if err != nil {
		return nil, err
	}
	raw, err := bytesArg(req.Get("state"), 2<<20)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var state []*event.Event
	if json.Unmarshal(raw, &state) != nil {
		return nil, matrixstate.ErrStorageNotFrozen
	}
	reference, err := matrixstate.FrozenRoom(state, room, own, controller)
	if err != nil {
		return nil, err
	}
	return map[string]any{"room_id": reference.RoomID, "event_id": reference.EventID}, nil
}

func storageCutoverDecode(req js.Value) (any, error) {
	scope := scopeArg(req)
	if !scope.Valid() {
		return nil, storage.ErrRecord
	}
	raw, err := bytesArg(req.Get("state"), 64<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var state storage.IdentityCutoverState
	if json.Unmarshal(raw, &state) != nil || state.Validate(scope) != nil {
		return nil, storage.ErrRecord
	}
	out, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	defer clear(out)
	return bytesValue(out), nil
}
