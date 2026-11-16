// SPDX-License-Identifier: Apache-2.0
// Package matrixstate shares native Matrix routing-state validation with the
// browser core. It reads public state only, and never chooses an identity,
// authenticates a network endpoint, or stores application content.
package matrixstate

import (
	"encoding/json"
	"errors"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrStorageNotFrozen = errors.New("matrix: private source is not read-only")

// FrozenRoom checks the controller, room version and every effective power
// level using the same maintained native SDK event model.
func FrozenRoom(state []*event.Event, room string, own, controller id.UserID) (storage.Reference, error) {
	if own == "" || controller == "" || own == controller {
		return storage.Reference{}, ErrStorageNotFrozen
	}
	create, powers, err := frozenStorageStateEvents(state)
	if err != nil {
		return storage.Reference{}, err
	}
	if create.Sender != controller || !sameFrozenStateRoom(create, room) || !sameFrozenStateRoom(powers, room) {
		return storage.Reference{}, ErrStorageNotFrozen
	}
	levels, err := frozenStoragePowers(create, powers)
	if err != nil {
		return storage.Reference{}, err
	}
	if !frozenStorageReadOnly(levels, own, controller) {
		return storage.Reference{}, ErrStorageNotFrozen
	}
	ref := storage.Reference{RoomID: room, EventID: powers.ID.String()}
	creationRef := storage.Reference{RoomID: room, EventID: create.ID.String()}
	if !creationRef.Valid() || !ref.Valid() {
		return storage.Reference{}, ErrStorageNotFrozen
	}
	return ref, nil
}

func sameFrozenStateRoom(value *event.Event, room string) bool {
	return value.RoomID == "" || value.RoomID.String() == room
}

func frozenStorageStateEvents(state []*event.Event) (*event.Event, *event.Event, error) {
	var create, powers *event.Event
	for _, value := range state {
		if value == nil {
			return nil, nil, ErrStorageNotFrozen
		}
		if value.Type.Type != event.StateCreate.Type && value.Type.Type != event.StatePowerLevels.Type {
			continue
		}
		if value.StateKey == nil || *value.StateKey != "" {
			return nil, nil, ErrStorageNotFrozen
		}
		switch value.Type.Type {
		case event.StateCreate.Type:
			if create != nil {
				return nil, nil, ErrStorageNotFrozen
			}
			create = value
		case event.StatePowerLevels.Type:
			if powers != nil {
				return nil, nil, ErrStorageNotFrozen
			}
			powers = value
		}
	}
	if create == nil || powers == nil {
		return nil, nil, ErrStorageNotFrozen
	}
	return create, powers, nil
}

func frozenStorageReadOnly(levels *event.PowerLevelsEventContent, own, controller id.UserID) bool {
	ownLevel := levels.GetUserLevel(own)
	if levels.GetUserLevel(controller) <= ownLevel || levels.EventsDefault <= ownLevel || levels.StateDefault() <= ownLevel || levels.Invite() <= ownLevel || levels.Kick() <= ownLevel || levels.Ban() <= ownLevel || levels.Redact() <= ownLevel {
		return false
	}
	for _, level := range levels.Events {
		if level <= ownLevel {
			return false
		}
	}
	return levels.GetEventLevel(event.EventEncrypted) > ownLevel
}

// Room state arrives as raw Content, and SDK parsing can leave a
// partially decoded Parsed value after an error. Decode the original public
// content strictly before asking the SDK about implicit creator powers.
func frozenStoragePowers(create, powers *event.Event) (*event.PowerLevelsEventContent, error) {
	var creation event.CreateEventContent
	var levels event.PowerLevelsEventContent
	if frozenStorageContent(create.Content, &creation) != nil || frozenStorageContent(powers.Content, &levels) != nil || (creation.RoomVersion != "11" && creation.RoomVersion != "12") {
		return nil, ErrStorageNotFrozen
	}
	levels.CreateEvent = &event.Event{Sender: create.Sender, Content: event.Content{Parsed: &creation}}
	return &levels, nil
}

func frozenStorageContent(content event.Content, target any) error {
	raw := content.VeryRaw
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(content)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(raw, target)
}
