// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Matrix carries the product's camelCase client envelope. The HTTP metadata
// structs use snake_case and must not silently change task ownership on another
// SDK when a message crosses between web and Fyne.
//
//nolint:tagliatelle // Matrix carries the shared client envelope, not the HTTP metadata DTO.
type wireCard struct {
	Kind         string              `json:"kind,omitempty"`
	Options      []string            `json:"options,omitempty"`
	Multi        bool                `json:"multi,omitempty"`
	AssigneeSub  string              `json:"assigneeSub,omitempty"`
	AssigneeName string              `json:"assigneeName,omitempty"`
	Due          *time.Time          `json:"due,omitempty"`
	Votes        []api.RoomsCardVote `json:"votes,omitempty"`
}

func (c MessageContent) MarshalJSON() ([]byte, error) {
	type plain MessageContent
	var card *wireCard
	if c.Card != nil {
		v := c.Card
		card = &wireCard{v.Kind, v.Options, v.Multi, v.AssigneeSub, v.AssigneeName, v.Due, v.Votes}
	}
	return json.Marshal(struct {
		plain
		Card *wireCard `json:"card,omitempty"`
	}{plain(c), card})
}

func (c *MessageContent) UnmarshalJSON(data []byte) error {
	type plain MessageContent
	var value struct {
		plain
		Card *wireCard `json:"card,omitempty"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*c = MessageContent(value.plain)
	if value.Card != nil {
		v := value.Card
		c.Card = &api.RoomsCard{Kind: v.Kind, Options: v.Options, Multi: v.Multi, AssigneeSub: v.AssigneeSub, AssigneeName: v.AssigneeName, Due: v.Due, Votes: v.Votes}
	}
	return nil
}
