// SPDX-License-Identifier: Apache-2.0
// Package contacts owns personal contact data on verified devices. Its model
// subpackage is pure and can also validate retained server migration snapshots.
package contacts

import (
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const MaxRowBytes = model.MaxRowBytes

var ErrSnapshot = model.ErrSnapshot

type (
	Row       = model.Row
	Aggregate = model.Aggregate
	Snapshot  struct{ *model.Snapshot }
)

func ValidateRow(scope storage.Scope, row Row) error { return model.ValidateRow(scope, row) }
func NewSnapshot(scope storage.Scope, rows []Row) (*Snapshot, error) {
	value, err := model.NewSnapshot(scope, rows)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Snapshot: value}, nil
}

func (s *Snapshot) WithAggregates(changes []Aggregate) (*Snapshot, error) {
	value, err := s.Snapshot.WithAggregates(changes)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Snapshot: value}, nil
}

type (
	FavoriteIndex = model.FavoriteIndex
	TrashIndex    = model.TrashIndex
	Removal       = model.Removal
)

func (s *Snapshot) WithIndexes(favorites *FavoriteIndex, trash *TrashIndex) (*Snapshot, error) {
	value, err := s.Snapshot.WithIndexes(favorites, trash)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Snapshot: value}, nil
}
