// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
)

func cleanMembers(uids []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, uid := range uids {
		uid = card.CleanUID(strings.TrimSpace(uid))
		if uid != "" && !seen[uid] {
			result = append(result, uid)
			seen[uid] = true
		}
	}
	return result
}

func (s *Snapshot) findGroup(id string) (GroupView, error) {
	groups, err := s.Groups()
	if err != nil {
		return GroupView{}, err
	}
	for _, group := range groups {
		if group.ID == id && !group.BookDeleted && group.DeletedAt == nil {
			return group, nil
		}
	}
	return GroupView{}, queryError(http.StatusNotFound, "private_group_not_found")
}

func (r *Repository) CreateGroup(ctx context.Context, book, name string) (api.ContactGroup, error) {
	change, err := r.mutateAggregate(ctx, "create-group", struct{ Book, Name string }{book, name}, func(snapshot *Snapshot) (Aggregate, error) { return snapshot.NewGroup(book, name, time.Now().UTC()) })
	if err != nil {
		return api.ContactGroup{}, err
	}
	return r.group(ctx, change.ID)
}

func (r *Repository) group(ctx context.Context, id string) (api.ContactGroup, error) {
	value, err := readQuery(ctx, r, func(s *Snapshot) (GroupView, error) { return s.findGroup(id) })
	return value.ContactGroup, err
}

func (s *Snapshot) NewGroup(book, name string, now time.Time) (Aggregate, error) {
	if _, err := s.QueryGroups(book); err != nil {
		return Aggregate{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Aggregate{}, queryError(http.StatusBadRequest, "invalid_group_name")
	}
	id, object, uid := uuid.NewString(), uuid.NewString(), card.NewUID()
	primary, err := newDomainRow(s.Scope(), "group", id, map[string]any{"address_book_id": book, "object_id": object, "uid": uid, "name": name, "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return Aggregate{}, err
	}
	data, err := card.EncodeGroup(card.Group{UID: uid, Name: name}, "")
	if err != nil {
		return Aggregate{}, err
	}
	source, err := newDomainRow(s.Scope(), "card", object, map[string]any{"address_book_id": book, "uid": uid, "resource_name": card.ResourceName(uid), "kind": "group", "data": data, "size_bytes": len(data), "etag": uuid.NewString(), "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return Aggregate{}, err
	}
	result := Aggregate{Version: 1, Scope: s.Scope(), Kind: "group_state", ID: id, Rows: []Row{primary, source}}
	return result, result.Validate(s.Scope())
}

func (r *Repository) RenameGroup(ctx context.Context, id, name string, members []string) (api.ContactGroup, error) {
	_, err := r.mutateAggregate(ctx, "rename-group", struct {
		ID, Name string
		Members  []string
	}{id, name, members}, func(s *Snapshot) (Aggregate, error) { return s.UpdateGroup(id, name, members, time.Now().UTC()) })
	if err != nil {
		return api.ContactGroup{}, err
	}
	return r.group(ctx, id)
}

func (s *Snapshot) UpdateGroup(id, name string, members []string, now time.Time) (Aggregate, error) {
	current, err := s.findGroup(id)
	if err != nil {
		return Aggregate{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = current.Name
	}
	members = cleanMembers(members)
	data, err := card.EncodeGroup(card.Group{UID: current.UID, Name: name, Members: members}, current.VCard)
	if err != nil {
		return Aggregate{}, err
	}
	change, err := editAggregate(s, "group_state", id)
	if err != nil {
		return Aggregate{}, err
	}
	old := change.Rows
	change.Rows = make([]Row, 0, 2+len(members))
	for _, row := range old {
		var updated Row
		switch row.Kind {
		case "group":
			updated, err = patchRow(row, map[string]any{"name": name, "updated_at": now})
		case "card":
			updated, err = patchRow(row, map[string]any{"data": data, "size_bytes": len(data), "etag": uuid.NewString(), "updated_at": now})
		default:
			continue
		}
		if err != nil {
			return Aggregate{}, err
		}
		change.Rows = append(change.Rows, updated)
	}
	people, err := s.visiblePeople()
	if err != nil {
		return Aggregate{}, err
	}
	for _, uid := range members {
		row, err := s.memberRow(old, current.ID, current.AddressBookID, uid, people, now)
		if err != nil {
			return Aggregate{}, err
		}
		change.Rows = append(change.Rows, row)
	}
	return change, change.Validate(s.Scope())
}

func (s *Snapshot) memberRow(old []Row, group, book, uid string, people []ContactView, now time.Time) (Row, error) {
	var contact any
	for _, person := range people {
		if person.AddressBookID == book && person.UID == uid {
			contact = person.ID
			break
		}
	}
	fields := map[string]any{"group_id": group, "contact_uid": uid, "contact_id": contact}
	for _, row := range old {
		if row.Kind != "member" {
			continue
		}
		var value struct {
			UID string `json:"contact_uid"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return Row{}, ErrSnapshot
		}
		if value.UID == uid {
			return patchRow(row, fields)
		}
	}
	fields["created_at"] = now
	return newDomainRow(s.Scope(), "member", uuid.NewString(), fields)
}

func (r *Repository) DeleteGroup(ctx context.Context, id string) error {
	_, err := r.mutateAggregate(ctx, "delete-group", struct{ ID string }{id}, func(snapshot *Snapshot) (Aggregate, error) {
		if _, err := snapshot.findGroup(id); err != nil {
			return Aggregate{}, err
		}
		change, err := editAggregate(snapshot, "group_state", id)
		if err != nil {
			return Aggregate{}, err
		}
		if err := aggregateTimestamp(&change, time.Now().UTC()); err != nil {
			return Aggregate{}, err
		}
		return change, nil
	})
	return err
}

func (r *Repository) BatchGroupMembers(ctx context.Context, id string, uids []string, add bool) error {
	_, err := r.mutateAggregate(ctx, "group-members", struct {
		ID   string
		UIDs []string
		Add  bool
	}{id, uids, add}, func(snapshot *Snapshot) (Aggregate, error) {
		current, err := snapshot.findGroup(id)
		if err != nil {
			return Aggregate{}, err
		}
		members := slices.Clone(current.Members)
		for _, uid := range cleanMembers(uids) {
			at := slices.Index(members, uid)
			if add && at < 0 {
				members = append(members, uid)
			} else if !add && at >= 0 {
				members = append(members[:at], members[at+1:]...)
			}
		}
		return snapshot.UpdateGroup(id, current.Name, members, time.Now().UTC())
	})
	return err
}
