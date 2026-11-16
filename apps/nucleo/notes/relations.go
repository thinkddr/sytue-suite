// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type rowRelation struct {
	ID         string  `json:"id"`
	ParentID   *string `json:"parent_id"`
	NotebookID string  `json:"notebook_id"`
	GroupID    *string `json:"group_id"`
}

func (r *Repository) validateRelations(ctx context.Context, catalog records.Catalog) error {
	versions, err := catalog.Entries()
	if err != nil {
		return err
	}
	books := map[string]*string{}
	notes := map[string]rowRelation{}
	for _, version := range versions {
		if version.Kind != "notebook" && version.Kind != "note" {
			continue
		}
		raw, err := catalog.ReadJSON(ctx, version)
		if err != nil {
			return err
		}
		var relation rowRelation
		err = json.Unmarshal(raw, &relation)
		clear(raw)
		if err != nil || relation.ID != version.ID {
			return ErrSource
		}
		if version.Kind == "notebook" {
			books[relation.ID] = relation.ParentID
		} else {
			notes[relation.ID] = relation
		}
	}
	if err := validateNotebookParents(books); err != nil {
		return err
	}
	return validateNoteGroups(books, notes)
}

func validateNotebookParents(books map[string]*string) error {
	for id := range books {
		seen := map[string]bool{}
		for id != "" {
			if seen[id] {
				return ErrSource
			}
			seen[id] = true
			parent, ok := books[id]
			if !ok {
				return ErrSource
			}
			if parent == nil {
				break
			}
			id = *parent
		}
	}
	return nil
}

func validateNoteGroups(books map[string]*string, notes map[string]rowRelation) error {
	for id, note := range notes {
		if _, found := books[note.NotebookID]; !found {
			return ErrSource
		}
		seen := map[string]bool{}
		for id != "" {
			if seen[id] {
				return ErrSource
			}
			seen[id] = true
			current, found := notes[id]
			if !found || current.NotebookID != note.NotebookID {
				return ErrSource
			}
			if current.GroupID == nil {
				break
			}
			id = *current.GroupID
		}
	}
	return nil
}

// checkNoteRelations rejects a move or grouping change before publishing any
// content. In particular, moving a group cannot strand its children elsewhere.
func (r *Repository) checkNoteRelations(ctx context.Context, id, notebook string, group *string) error {
	versions, err := r.selected("note")
	if err != nil {
		return err
	}
	rows := map[string]rowRelation{}
	for _, version := range versions {
		raw, err := r.readJSON(ctx, version)
		if err != nil {
			return err
		}
		var value rowRelation
		err = json.Unmarshal(raw, &value)
		clear(raw)
		if err != nil {
			return ErrSource
		}
		rows[value.ID] = value
	}
	rows[id] = rowRelation{ID: id, NotebookID: notebook, GroupID: group}
	books, err := r.notebooks(ctx)
	if err != nil {
		return err
	}
	parents := make(map[string]*string, len(books))
	for id, book := range books {
		parents[id] = book.ParentID
	}
	return validateNoteGroups(parents, rows)
}
