// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func (r *Repository) favoriteIndex(ctx context.Context) (FavoriteIndex, error) {
	result := FavoriteIndex{Version: 1, Scope: r.scope, ID: model.PrivateIndexID, Values: map[string]bool{}}
	raw, err := r.core.ReadRaw(ctx, model.FavoriteIndexKind, model.PrivateIndexID)
	defer clear(raw)
	if errors.Is(err, records.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if json.Unmarshal(raw, &result) != nil || result.Validate(r.scope) != nil {
		return result, ErrSnapshot
	}
	return result, nil
}

func (r *Repository) trashIndex(ctx context.Context) (TrashIndex, error) {
	result := TrashIndex{Version: 1, Scope: r.scope, ID: model.PrivateIndexID, Values: map[string]Removal{}}
	raw, err := r.core.ReadRaw(ctx, model.TrashIndexKind, model.PrivateIndexID)
	defer clear(raw)
	if errors.Is(err, records.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if json.Unmarshal(raw, &result) != nil || result.Validate(r.scope) != nil {
		return result, ErrSnapshot
	}
	return result, nil
}

func (r *Repository) SetFavorite(ctx context.Context, uid string, on bool) error {
	return r.changeFavorites(ctx, []string{uid}, on, true)
}

func (r *Repository) BatchFavorite(ctx context.Context, uids []string, on bool) error {
	return r.changeFavorites(ctx, uids, on, false)
}

func (r *Repository) changeFavorites(parent context.Context, uids []string, on, strict bool) error {
	uids = cleanMembers(uids)
	if len(uids) == 0 {
		if strict {
			return queryError(http.StatusNotFound, "private_contact_not_found")
		}
		return nil
	}
	raw, err := r.core.Mutate(parent, "favorite-index", struct {
		UIDs       []string
		On, Strict bool
	}{uids, on, strict}, func(ctx context.Context) (string, string, string, []byte, error) {
		snapshot, err := r.Snapshot(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		current, err := r.favoriteIndex(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		state, err := snapshot.ChangeFavorites(current, uids, on, strict)
		if err != nil {
			return "", "", "", nil, err
		}
		candidate, err := snapshot.WithIndexes(&state, nil)
		if err != nil {
			return "", "", "", nil, err
		}
		if err := validateViews(candidate); err != nil {
			return "", "", "", nil, err
		}
		payload, err := json.Marshal(state)
		return model.FavoriteIndexKind, model.PrivateIndexID, "live", payload, err
	})
	clear(raw)
	return err
}

func (s *Snapshot) ChangeFavorites(current FavoriteIndex, uids []string, on, strict bool) (FavoriteIndex, error) {
	if current.Validate(s.Scope()) != nil {
		return FavoriteIndex{}, ErrSnapshot
	}
	people, err := s.visiblePeople()
	if err != nil {
		return FavoriteIndex{}, err
	}
	present := map[string]bool{}
	for _, person := range people {
		present[person.UID] = true
	}
	state := current
	state.Values = make(map[string]bool, len(current.Values)+len(uids))
	for id, value := range current.Values {
		state.Values[id] = value
	}
	for _, uid := range cleanMembers(uids) {
		if !present[uid] {
			if strict {
				return FavoriteIndex{}, queryError(http.StatusNotFound, "private_contact_not_found")
			}
			continue
		}
		state.Values[uid] = on
	}
	return state, state.Validate(s.Scope())
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	return r.changeTrash(ctx, "delete", []string{id}, false)
}

func (r *Repository) BatchTrash(ctx context.Context, uids []string) error {
	return r.changeTrash(ctx, "batch-trash", cleanMembers(uids), true)
}

func (r *Repository) Restore(ctx context.Context, id string) error {
	return r.changeTrash(ctx, "restore", []string{id}, false)
}

func (r *Repository) Purge(ctx context.Context, id string) error {
	return r.changeTrash(ctx, "purge", []string{id}, false)
}

func (r *Repository) changeTrash(parent context.Context, operation string, ids []string, byUID bool) error {
	if len(ids) == 0 {
		return nil
	}
	raw, err := r.core.Mutate(parent, "trash-index", struct {
		Operation string
		IDs       []string
		ByUID     bool
	}{operation, ids, byUID}, func(ctx context.Context) (string, string, string, []byte, error) {
		snapshot, err := r.Snapshot(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		current, err := r.trashIndex(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		state, err := snapshot.ChangeTrash(current, operation, ids, byUID, time.Now().UTC())
		if err != nil {
			return "", "", "", nil, err
		}
		candidate, err := snapshot.WithIndexes(nil, &state)
		if err != nil {
			return "", "", "", nil, err
		}
		if err := validateViews(candidate); err != nil {
			return "", "", "", nil, err
		}
		payload, err := json.Marshal(state)
		return model.TrashIndexKind, model.PrivateIndexID, "live", payload, err
	})
	clear(raw)
	return err
}

func (s *Snapshot) ChangeTrash(current TrashIndex, operation string, ids []string, byUID bool, now time.Time) (TrashIndex, error) {
	if current.Validate(s.Scope()) != nil {
		return TrashIndex{}, ErrSnapshot
	}
	if operation != "delete" && operation != "batch-trash" && operation != "restore" && operation != "purge" {
		return TrashIndex{}, ErrSnapshot
	}
	people, err := s.Contacts()
	if err != nil {
		return TrashIndex{}, err
	}
	state := current
	state.Values = make(map[string]Removal, len(current.Values)+len(ids))
	for id, value := range current.Values {
		state.Values[id] = value
	}
	matched := 0
	for _, person := range people {
		key := person.ID
		if byUID {
			key = person.UID
		}
		if person.BookDeleted || !slices.Contains(ids, key) {
			continue
		}
		if operation == "restore" || operation == "purge" {
			if person.DeletedAt == nil {
				continue
			}
		} else if person.DeletedAt != nil {
			continue
		}
		value := Removal{DeletedAt: &now, Purged: operation == "purge"}
		if operation == "restore" {
			value = Removal{}
		}
		state.Values[person.ID] = value
		matched++
	}
	if matched == 0 && !byUID {
		return TrashIndex{}, queryError(http.StatusNotFound, "private_contact_not_found")
	}
	return state, state.Validate(s.Scope())
}
