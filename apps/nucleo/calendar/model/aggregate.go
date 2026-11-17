// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Aggregate publishes one Calendar domain revision. An object and all its
// normalized events, attendees and reminders are committed together. Retained
// source rows stay immutable; a calendar tombstone hides its whole descendant
// graph without requiring a partially observable multi-object delete.
type Aggregate struct {
	Version   int           `json:"v"`
	Scope     storage.Scope `json:"scope"`
	Kind      string        `json:"kind"`
	ID        string        `json:"id"`
	Rows      []Row         `json:"rows"`
	DeletedAt *time.Time    `json:"deleted_at,omitempty"`
}

func aggregatePrimary(kind string) string {
	switch kind {
	case "calendar_state":
		return "calendar"
	case "object_state":
		return "object"
	default:
		return ""
	}
}

func (a Aggregate) Validate(scope storage.Scope) error {
	primary := aggregatePrimary(a.Kind)
	if a.Version != 1 || a.Scope != scope || !scope.Valid() || scope.Product != "calendar" || primary == "" || !canonicalUUID(a.ID) {
		return ErrSnapshot
	}
	all := make(map[string]relation, len(a.Rows))
	for _, row := range a.Rows {
		rel, err := decodeRow(scope, row)
		if err != nil {
			return err
		}
		key := rowKey(row.Kind, row.ID)
		if _, exists := all[key]; exists {
			return ErrSnapshot
		}
		all[key] = rel
	}
	root, found := all[rowKey(primary, a.ID)]
	if !found {
		return ErrSnapshot
	}
	if !sameDeletion(a.DeletedAt, root.DeletedAt) {
		return ErrSnapshot
	}
	if primary == "calendar" {
		if len(all) != 1 || root.Owner != scope.Subject {
			return ErrSnapshot
		}
		return nil
	}
	return validateObjectAggregate(scope, a.ID, root, all)
}

func validateObjectAggregate(scope storage.Scope, id string, root relation, all map[string]relation) error {
	for _, rel := range all {
		switch rel.kind {
		case "object":
			if rel.ID != id {
				return ErrSnapshot
			}
		case "event":
			if rel.Object != id || rel.Calendar != root.Calendar || rel.UID != root.UID {
				return ErrSnapshot
			}
		case "attendee":
			if _, ok := all[rowKey("event", rel.Event)]; !ok {
				return ErrSnapshot
			}
		case "reminder":
			event, ok := all[rowKey("event", rel.Event)]
			if !ok || rel.Object != id || rel.Calendar != root.Calendar || rel.Owner != scope.Subject || rel.UID != event.UID {
				return ErrSnapshot
			}
		default:
			return ErrSnapshot
		}
	}
	return nil
}

func sameDeletion(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && !a.IsZero() && a.Equal(*b)
}

func objectRows(scope storage.Scope, rows map[string]Row, id string) (map[string]bool, error) {
	owned := map[string]bool{rowKey("object", id): true}
	events := map[string]bool{}
	for key, row := range rows {
		if row.Kind != "event" && row.Kind != "reminder" {
			continue
		}
		rel, err := decodeRow(scope, row)
		if err != nil {
			return nil, err
		}
		if rel.Object == id {
			owned[key] = true
			if row.Kind == "event" {
				events[row.ID] = true
			}
		}
	}
	for key, row := range rows {
		if row.Kind != "attendee" {
			continue
		}
		rel, err := decodeRow(scope, row)
		if err != nil {
			return nil, err
		}
		if events[rel.Event] {
			owned[key] = true
		}
	}
	return owned, nil
}

func (s *Snapshot) Edit(kind, id string) (Aggregate, error) {
	primary := aggregatePrimary(kind)
	if primary == "" {
		return Aggregate{}, ErrSnapshot
	}
	rootRow, ok := s.rows[rowKey(primary, id)]
	if !ok {
		return Aggregate{}, ErrSnapshot
	}
	root, err := decodeRow(s.scope, rootRow)
	if err != nil {
		return Aggregate{}, err
	}
	owned := map[string]bool{rowKey(primary, id): true}
	if primary == "object" {
		var err error
		owned, err = objectRows(s.scope, s.rows, id)
		if err != nil {
			return Aggregate{}, err
		}
	}
	a := Aggregate{Version: 1, Scope: s.scope, Kind: kind, ID: id, DeletedAt: root.DeletedAt}
	for _, row := range s.Rows() {
		if owned[rowKey(row.Kind, row.ID)] {
			a.Rows = append(a.Rows, row)
		}
	}
	return a, a.Validate(s.scope)
}

// WithAggregates composes selected authenticated revisions over the immutable
// source. Revisions cannot claim another object's rows or silently omit an
// original event: invitations and processed outbox may still refer to it.
func (s *Snapshot) WithAggregates(changes []Aggregate) (*Snapshot, error) {
	rows := map[string]Row{}
	for key, row := range s.rows {
		row.JSON = slices.Clone(row.JSON)
		rows[key] = row
	}
	seen := map[string]bool{}
	for _, a := range changes {
		if err := a.Validate(s.scope); err != nil {
			return nil, err
		}
		key := rowKey(a.Kind, a.ID)
		if seen[key] {
			return nil, ErrSnapshot
		}
		seen[key] = true
		primary := aggregatePrimary(a.Kind)
		owned := map[string]bool{rowKey(primary, a.ID): true}
		if primary == "object" {
			var err error
			owned, err = objectRows(s.scope, rows, a.ID)
			if err != nil {
				return nil, err
			}
		}
		replacement := map[string]bool{}
		for _, row := range a.Rows {
			replacement[rowKey(row.Kind, row.ID)] = true
		}
		for key := range owned {
			if _, exists := rows[key]; exists && !replacement[key] {
				return nil, ErrSnapshot
			}
			delete(rows, key)
		}
		for _, row := range a.Rows {
			key := rowKey(row.Kind, row.ID)
			if _, collision := rows[key]; collision {
				return nil, ErrSnapshot
			}
			row.JSON = slices.Clone(row.JSON)
			rows[key] = row
		}
	}
	result := make([]Row, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	return NewSnapshot(s.scope, result)
}

func (s *Snapshot) Row(kind, id string) (Row, bool) {
	row, ok := s.rows[rowKey(kind, id)]
	row.JSON = slices.Clone(row.JSON)
	return row, ok
}

// Deleted reports effective visibility, including a calendar/object tombstone.
// Hidden rows remain part of the validated graph and exact retained inventory.
func (s *Snapshot) Deleted(row Row) bool {
	rel, err := decodeRow(s.scope, row)
	if err != nil {
		return true
	}
	if rel.DeletedAt != nil {
		return true
	}
	if row.Kind == "attendee" {
		parent, ok := s.rows[rowKey("event", rel.Event)]
		return !ok || s.Deleted(parent)
	}
	if rel.Calendar != "" {
		if parent, ok := s.rows[rowKey("calendar", rel.Calendar)]; ok {
			var v struct {
				DeletedAt *time.Time `json:"deleted_at"`
			}
			if json.Unmarshal(parent.JSON, &v) != nil || v.DeletedAt != nil {
				return true
			}
		}
	}
	if rel.Object != "" {
		if parent, ok := s.rows[rowKey("object", rel.Object)]; ok {
			var v struct {
				DeletedAt *time.Time `json:"deleted_at"`
			}
			if json.Unmarshal(parent.JSON, &v) != nil || v.DeletedAt != nil {
				return true
			}
		}
	}
	return false
}
