// SPDX-License-Identifier: Apache-2.0
package model

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	FavoriteIndexKind = "favorite_index"
	TrashIndexKind    = "trash_index"
	PrivateIndexID    = "self"
)

// FavoriteIndex makes a multi-UID favorite action one encrypted revision. UID
// semantics match the legacy API: the same UID in several books is one mark.
type FavoriteIndex struct {
	Version int             `json:"v"`
	Scope   storage.Scope   `json:"scope"`
	ID      string          `json:"id"`
	Values  map[string]bool `json:"values"`
}

func (s FavoriteIndex) Validate(scope storage.Scope) error {
	if !validIndexHeader(s.Version, s.Scope, s.ID, scope) || s.Values == nil {
		return ErrSnapshot
	}
	for uid := range s.Values {
		if uid == "" || !utf8.ValidString(uid) || strings.ContainsRune(uid, 0) {
			return ErrSnapshot
		}
	}
	return nil
}

type Removal struct {
	DeletedAt *time.Time `json:"deleted_at"`
	Purged    bool       `json:"purged"`
}

// TrashIndex covers batch trash, restore and purge with one publication, without
// changing a card separately from its channels or losing group MEMBER UIDs.
type TrashIndex struct {
	Version int                `json:"v"`
	Scope   storage.Scope      `json:"scope"`
	ID      string             `json:"id"`
	Values  map[string]Removal `json:"values"`
}

func (s TrashIndex) Validate(scope storage.Scope) error {
	if !validIndexHeader(s.Version, s.Scope, s.ID, scope) || s.Values == nil {
		return ErrSnapshot
	}
	for id, value := range s.Values {
		if !validUUID(id) || value.DeletedAt != nil && value.DeletedAt.IsZero() || value.Purged && value.DeletedAt == nil {
			return ErrSnapshot
		}
	}
	return nil
}

func validIndexHeader(version int, actual storage.Scope, id string, expected storage.Scope) bool {
	return version == 1 && actual == expected && expected.Valid() && expected.Product == "contacts" && id == PrivateIndexID
}

func (s *Snapshot) WithIndexes(favorites *FavoriteIndex, trash *TrashIndex) (*Snapshot, error) {
	if favorites != nil {
		if err := favorites.Validate(s.scope); err != nil {
			return nil, err
		}
	}
	if trash != nil {
		if err := trash.Validate(s.scope); err != nil {
			return nil, err
		}
	}
	if favorites == nil && trash == nil {
		return s, nil
	}
	// Las filas comparten los buffers de s, que no cambian nunca: cada cambio de abajo
	// pone un buffer nuevo, y compose descodifica solo esos.
	rows := maps.Clone(s.rows)
	purged := map[string]bool{}
	for key, row := range rows {
		if row.Kind != "contact" {
			continue
		}
		removed, err := applyContactIndexes(s.scope, rows, key, row, favorites, trash)
		if err != nil {
			return nil, err
		}
		if removed {
			purged[row.ID] = true
		}
	}

	for id := range purged {
		owned, err := aggregateOwnedRows(s.scope, rows, "contact", id)
		if err != nil {
			return nil, err
		}
		for key := range owned {
			delete(rows, key)
		}
	}
	if err := normalizePurgedMembers(s.scope, rows, purged); err != nil {
		return nil, err
	}
	return compose(s.scope, rows, s.keptLinks(rows))
}

// IndexesKeep dice si WithIndexes(favorites, trash) dejaría cada fila de change tal
// cual la trae change, byte a byte, en s, una instantánea ya compuesta con change: sin
// quitar ninguna, sin reescribir ninguna y sin reescribir otra fila por ellas. Entonces
// componer change con WithAggregates sobre una instantánea que ya lleva los índices da
// lo mismo que componerlo antes de aplicarlos. false si no se sabe: quien llama vuelve
// a componer desde el catálogo.
func (s *Snapshot) IndexesKeep(change Aggregate, favorites *FavoriteIndex, trash *TrashIndex) bool {
	if favorites == nil && trash == nil {
		return true
	}
	rows, kept := s.indexedRows(change, favorites, trash)
	if !kept {
		return false
	}
	contacts, cards := map[string]bool{}, map[string]bool{}
	for _, row := range change.Rows {
		switch row.Kind {
		case "contact":
			contacts[row.ID] = true
		case "card":
			cards[row.ID] = true
		}
	}
	// Lo que WithIndexes hace en otras filas por otras: un contacto purgado vacía los
	// miembros que lo nombran, y otro contacto con una tarjeta del cambio la reescribiría
	// o la quitaría.
	for key, value := range s.links {
		switch {
		case value.kind == "member" && trash != nil && value.contact != "" && trash.Values[value.contact].Purged:
			if _, mine := rows[key]; mine {
				return false
			}
		case value.kind == "contact" && cards[value.object] && !contacts[value.id]:
			return false
		}
	}
	return true
}

// indexedRows son las filas de change, por su clave, tras lo que WithIndexes hace con
// cada contacto, y si siguen byte a byte como change las trae, sin quitar ninguna.
func (s *Snapshot) indexedRows(change Aggregate, favorites *FavoriteIndex, trash *TrashIndex) (map[string]Row, bool) {
	rows := make(map[string]Row, len(change.Rows))
	for _, row := range change.Rows {
		row.JSON = slices.Clone(row.JSON)
		rows[rowKey(row.Kind, row.ID)] = row
	}
	for key, row := range rows {
		if row.Kind != "contact" {
			continue
		}
		removed, err := applyContactIndexes(s.scope, rows, key, row, favorites, trash)
		if err != nil || removed {
			return nil, false
		}
	}
	for _, row := range change.Rows {
		if !bytes.Equal(rows[rowKey(row.Kind, row.ID)].JSON, row.JSON) {
			return nil, false
		}
	}
	return rows, true
}

func applyContactIndexes(scope storage.Scope, rows map[string]Row, key string, row Row, favorites *FavoriteIndex, trash *TrashIndex) (bool, error) {
	relation, err := decodeRow(scope, row)
	if err != nil {
		return false, err
	}
	if favorites != nil {
		if on, present := favorites.Values[relation.uid]; present {
			raw, _ := json.Marshal(on)
			relation.raw["is_favorite"] = raw
		}
	}
	if trash != nil {
		if removal, present := trash.Values[row.ID]; present {
			if removal.Purged {
				return true, nil
			}
			if err := applyRemoval(scope, rows, relation, removal); err != nil {
				return false, err
			}
		}
	}
	row.JSON, err = json.Marshal(relation.raw)
	if err != nil {
		return false, err
	}
	rows[key] = row
	return false, nil
}

func applyRemoval(scope storage.Scope, rows map[string]Row, relation relation, removal Removal) error {
	raw, err := json.Marshal(removal.DeletedAt)
	if err != nil {
		return err
	}
	relation.raw["deleted_at"] = raw
	objectKey := rowKey("card", relation.object)
	object, found := rows[objectKey]
	if !found {
		return ErrSnapshot
	}
	card, err := decodeRow(scope, object)
	if err != nil {
		return err
	}
	card.raw["deleted_at"] = raw
	object.JSON, err = json.Marshal(card.raw)
	if err != nil {
		return err
	}
	rows[objectKey] = object
	return nil
}
