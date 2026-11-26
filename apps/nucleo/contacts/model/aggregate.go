// SPDX-License-Identifier: Apache-2.0
package model

import (
	"encoding/json"
	"maps"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Aggregate is one atomic encrypted revision of a book, contact or group. It
// carries all that entity's original rows, so a network interruption cannot
// publish a card separately from its channels or a group without its members.
// Retained source rows remain immutable; these revisions override their views.
type Aggregate struct {
	Version   int           `json:"v"`
	Scope     storage.Scope `json:"scope"`
	Kind      string        `json:"kind"`
	ID        string        `json:"id"`
	Rows      []Row         `json:"rows"`
	Purged    bool          `json:"purged,omitempty"`
	DeletedAt *time.Time    `json:"deleted_at,omitempty"`
}

func aggregateEntity(kind string) string {
	switch kind {
	case "book_state":
		return "book"
	case "contact_state":
		return "contact"
	case "group_state":
		return "group"
	default:
		return ""
	}
}

func (a Aggregate) validHeader(scope storage.Scope, entity string) bool {
	return a.Version == 1 && a.Scope == scope && scope.Valid() && scope.Product == "contacts" && entity != "" && validUUID(a.ID)
}

func (a Aggregate) Validate(scope storage.Scope) error {
	entity := aggregateEntity(a.Kind)
	if !a.validHeader(scope, entity) {
		return ErrSnapshot
	}
	if a.Purged {
		if a.Kind != "contact_state" || len(a.Rows) != 0 || a.DeletedAt == nil || a.DeletedAt.IsZero() {
			return ErrSnapshot
		}
		return nil
	}
	relations := make(map[string]relation, len(a.Rows))
	var primary relation
	for _, row := range a.Rows {
		value, err := decodeRow(scope, row)
		if err != nil {
			return err
		}
		key := rowKey(row.Kind, row.ID)
		if _, exists := relations[key]; exists {
			return ErrSnapshot
		}
		relations[key] = value
		if row.Kind == entity && row.ID == a.ID {
			primary = value
		}
	}
	if primary.id == "" {
		return ErrSnapshot
	}
	var deletedAt *time.Time
	if raw, found := primary.raw["deleted_at"]; found && json.Unmarshal(raw, &deletedAt) != nil {
		return ErrSnapshot
	}
	if (a.DeletedAt == nil) != (deletedAt == nil) || a.DeletedAt != nil && !a.DeletedAt.Equal(*deletedAt) {
		return ErrSnapshot
	}
	return validateAggregateRelations(entity, primary, relations)
}

func validateAggregateRelations(entity string, primary relation, all map[string]relation) error {
	if entity == "book" {
		if len(all) != 1 {
			return ErrSnapshot
		}
		return nil
	}
	object, found := all[rowKey("card", primary.object)]
	if !found || object.book != primary.book || object.uid != primary.uid {
		return ErrSnapshot
	}
	for _, row := range all {
		if row.kind == entity && row.id == primary.id || row.kind == "card" && row.id == primary.object {
			continue
		}
		switch entity {
		case "contact":
			if (row.kind != "email" && row.kind != "phone") || row.contact != primary.id {
				return ErrSnapshot
			}
		case "group":
			if row.kind != "member" || row.group != primary.id {
				return ErrSnapshot
			}
		default:
			return ErrSnapshot
		}
	}
	return nil
}

// WithAggregates composes authenticated current revisions over a complete
// immutable baseline. A book tombstone hides its descendants without a partial
// multi-object delete. Contact purge turns resolved member hints into legal
// unresolved UIDs while retaining the group's original membership and vCard.
func (s *Snapshot) WithAggregates(changes []Aggregate) (*Snapshot, error) {
	// Las filas que se quedan de s comparten sus buffers, que no cambian nunca, y sus
	// enlaces: solo se descodifican las filas de los cambios (y los miembros que se
	// normalizan abajo).
	rows := maps.Clone(s.rows)
	links := maps.Clone(s.links)
	seen := map[string]bool{}
	purged := map[string]bool{}
	for _, change := range changes {
		if err := change.Validate(s.scope); err != nil {
			return nil, err
		}
		key := rowKey(change.Kind, change.ID)
		if seen[key] {
			return nil, ErrSnapshot
		}
		seen[key] = true
		for key := range aggregateOwnedLinks(links, aggregateEntity(change.Kind), change.ID) {
			delete(rows, key)
			delete(links, key)
		}
		if change.Purged {
			purged[change.ID] = true
			continue
		}
		for _, row := range change.Rows {
			key := rowKey(row.Kind, row.ID)
			if _, collision := rows[key]; collision {
				return nil, ErrSnapshot
			}
			row.JSON = slices.Clone(row.JSON)
			relation, err := decodeRow(s.scope, row)
			if err != nil {
				return nil, err
			}
			rows[key], links[key] = row, relation.link()
		}
	}
	if err := normalizePurgedMemberLinks(s.scope, rows, links, purged); err != nil {
		return nil, err
	}
	return compose(s.scope, rows, links)
}

// aggregateOwnedLinks es aggregateOwnedRows sobre los enlaces ya descodificados de
// las mismas filas.
func aggregateOwnedLinks(links map[string]link, entity, id string) map[string]bool {
	owned := map[string]bool{}
	primary, found := links[rowKey(entity, id)]
	if !found {
		return owned
	}
	owned[rowKey(entity, id)] = true
	if entity == "book" {
		return owned
	}
	owned[rowKey("card", primary.object)] = true
	for key, row := range links {
		if row.kind != "email" && row.kind != "phone" && row.kind != "member" {
			continue
		}
		if entity == "contact" && (row.kind == "email" || row.kind == "phone") && row.contact == id || entity == "group" && row.kind == "member" && row.group == id {
			owned[key] = true
		}
	}
	return owned
}

// normalizePurgedMemberLinks es normalizePurgedMembers descodificando solo los
// miembros de un contacto purgado; con su JSON nuevo pierden su enlace.
func normalizePurgedMemberLinks(scope storage.Scope, rows map[string]Row, links map[string]link, purged map[string]bool) error {
	if len(purged) == 0 {
		return nil
	}
	for key, value := range links {
		if value.kind != "member" || !purged[value.contact] {
			continue
		}
		row := rows[key]
		relation, err := decodeRow(scope, row)
		if err != nil {
			return err
		}
		relation.raw["contact_id"] = json.RawMessage("null")
		row.JSON, err = json.Marshal(relation.raw)
		if err != nil {
			return err
		}
		rows[key] = row
		delete(links, key)
	}
	return nil
}

func aggregateOwnedRows(scope storage.Scope, rows map[string]Row, entity, id string) (map[string]bool, error) {
	owned := map[string]bool{}
	primary, found := rows[rowKey(entity, id)]
	if !found {
		return owned, nil
	}
	relation, err := decodeRow(scope, primary)
	if err != nil {
		return nil, err
	}
	owned[rowKey(entity, id)] = true
	if entity == "book" {
		return owned, nil
	}
	owned[rowKey("card", relation.object)] = true
	for key, row := range rows {
		if row.Kind != "email" && row.Kind != "phone" && row.Kind != "member" {
			continue
		}
		related, err := decodeRow(scope, row)
		if err != nil {
			return nil, err
		}
		if entity == "contact" && (row.Kind == "email" || row.Kind == "phone") && related.contact == id || entity == "group" && row.Kind == "member" && related.group == id {
			owned[key] = true
		}
	}
	return owned, nil
}

func normalizePurgedMembers(scope storage.Scope, rows map[string]Row, purged map[string]bool) error {
	for key, row := range rows {
		if row.Kind != "member" {
			continue
		}
		relation, err := decodeRow(scope, row)
		if err != nil {
			return err
		}
		if !purged[relation.contact] {
			continue
		}
		relation.raw["contact_id"] = json.RawMessage("null")
		row.JSON, err = json.Marshal(relation.raw)
		if err != nil {
			return err
		}
		rows[key] = row
	}
	return nil
}

// Edit returns an independent aggregate with all current rows belonging to the
// entity. Unknown source columns and the original card are retained by callers.
func (s *Snapshot) Edit(kind, id string) (Aggregate, error) {
	entity := aggregateEntity(kind)
	if entity == "" {
		return Aggregate{}, ErrSnapshot
	}
	owned := aggregateOwnedLinks(s.links, entity, id)
	if len(owned) == 0 {
		return Aggregate{}, ErrSnapshot
	}
	// En el orden de Rows, con buffers propios, sin copiar todas las filas.
	result := Aggregate{Version: 1, Scope: s.scope, Kind: kind, ID: id, Rows: make([]Row, 0, len(owned))}
	for key := range owned {
		if row, found := s.rows[key]; found {
			row.JSON = slices.Clone(row.JSON)
			result.Rows = append(result.Rows, row)
		}
	}
	slices.SortFunc(result.Rows, compareRows)
	for _, row := range result.Rows {
		if row.Kind != entity || row.ID != id {
			continue
		}
		var value struct {
			DeletedAt *time.Time `json:"deleted_at"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return Aggregate{}, ErrSnapshot
		}
		result.DeletedAt = value.DeletedAt
	}
	return result, result.Validate(s.scope)
}
