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
	// Snapshot no cambia nunca: ni su instantánea del modelo ni sus vistas, que se
	// calculan una vez (vistas.go).
	Snapshot struct {
		*model.Snapshot
		vistas *vistas
	}
)

func ValidateRow(scope storage.Scope, row Row) error { return model.ValidateRow(scope, row) }
func NewSnapshot(scope storage.Scope, rows []Row) (*Snapshot, error) {
	value, err := model.NewSnapshot(scope, rows)
	if err != nil {
		return nil, err
	}
	return envolver(value), nil
}

func envolver(value *model.Snapshot) *Snapshot {
	return &Snapshot{Snapshot: value, vistas: &vistas{}}
}

// derivada es value, derivada de s: sus vistas reutilizan lo que ya proyectó s.
func (s *Snapshot) derivada(value *model.Snapshot) *Snapshot {
	out := envolver(value)
	out.vistas.desde = s.vistas
	return out
}

// copia es otro envoltorio de la misma instantánea y sus vistas, que no cambian: quien
// lo recibe no puede tocar el que se guarda (foto.go).
func (s *Snapshot) copia() *Snapshot { return &Snapshot{Snapshot: s.Snapshot, vistas: s.vistas} }

func (s *Snapshot) WithAggregates(changes []Aggregate) (*Snapshot, error) {
	value, err := s.Snapshot.WithAggregates(changes)
	if err != nil {
		return nil, err
	}
	return s.derivada(value), nil
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
	if value == s.Snapshot {
		return s.copia(), nil
	}
	return s.derivada(value), nil
}
