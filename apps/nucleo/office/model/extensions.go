// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import "encoding/json"

// The encrypted record retains the original source bytes. Materializing and
// editing a document must also retain fields understood only by newer clients;
// numbers in extensions remain RawMessage and never pass through float64.
//
//nolint:nilnil // a document without unknown fields has no extensions; a nil map is the canonical empty value
func extensions(raw []byte, known []string, target any) (map[string]json.RawMessage, error) {
	if err := json.Unmarshal(raw, target); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for _, key := range known {
		delete(fields, key)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return fields, nil
}

func withExtensions(value any, extra map[string]json.RawMessage, known []string) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(extra) == 0 {
		return raw, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	reserved := make(map[string]bool, len(known))
	for _, key := range known {
		reserved[key] = true
	}
	for key, value := range extra {
		if !reserved[key] {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

var (
	stateFields     = []string{"seq", "lamport", "elements", "registers"}
	elementFields   = []string{"dot", "after", "value", "deleted", "lamport"}
	registerFields  = []string{"value", "dot", "lamport"}
	operationFields = []string{"kind", "dot", "after", "target", "key", "value", "lamport", "op_id", "cambios", "base"}
)

func (s *State) UnmarshalJSON(raw []byte) error {
	type wire State
	var value wire
	extra, err := extensions(raw, stateFields, &value)
	if err != nil {
		return err
	}
	value.Extensions = extra
	*s = State(value)
	return nil
}

func (s State) MarshalJSON() ([]byte, error) {
	type wire State
	return withExtensions(wire(s), s.Extensions, stateFields)
}

func (s *Element) UnmarshalJSON(raw []byte) error {
	type wire Element
	var value wire
	extra, err := extensions(raw, elementFields, &value)
	if err != nil {
		return err
	}
	value.Extensions = extra
	*s = Element(value)
	return nil
}

func (s Element) MarshalJSON() ([]byte, error) {
	type wire Element
	return withExtensions(wire(s), s.Extensions, elementFields)
}

func (s *Register) UnmarshalJSON(raw []byte) error {
	type wire Register
	var value wire
	extra, err := extensions(raw, registerFields, &value)
	if err != nil {
		return err
	}
	value.Extensions = extra
	*s = Register(value)
	return nil
}

func (s Register) MarshalJSON() ([]byte, error) {
	type wire Register
	return withExtensions(wire(s), s.Extensions, registerFields)
}

func (s *Op) UnmarshalJSON(raw []byte) error {
	type wire Op
	var value wire
	extra, err := extensions(raw, operationFields, &value)
	if err != nil {
		return err
	}
	value.Extensions = extra
	*s = Op(value)
	return nil
}

func (s Op) MarshalJSON() ([]byte, error) {
	type wire Op
	return withExtensions(wire(s), s.Extensions, operationFields)
}
