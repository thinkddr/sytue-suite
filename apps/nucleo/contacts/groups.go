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
	// Las guardadas, solo para leer: lo que sale es una copia.
	groups, err := s.grupos()
	if err != nil {
		return GroupView{}, err
	}
	for _, group := range groups {
		if group.ID == id && !group.BookDeleted && group.DeletedAt == nil {
			group.Members = slices.Clone(group.Members)
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
	return s.newGroupWith(book, card.Group{UID: card.NewUID(), Name: name}, "", now)
}

// newGroupWith es NewGroup con el UID, los miembros y la vCard de base de g (la de una
// cuenta sincronizada, `externos.go`); NewGroup da un UID nuevo, sin miembros ni base.
func (s *Snapshot) newGroupWith(book string, g card.Group, base string, now time.Time) (Aggregate, error) {
	if _, err := s.QueryGroups(book); err != nil {
		return Aggregate{}, err
	}
	name := strings.TrimSpace(g.Name)
	if name == "" {
		return Aggregate{}, queryError(http.StatusBadRequest, "invalid_group_name")
	}
	id, object, uid := uuid.NewString(), uuid.NewString(), g.UID
	primary, err := newDomainRow(s.Scope(), "group", id, map[string]any{"address_book_id": book, "object_id": object, "uid": uid, "name": name, "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return Aggregate{}, err
	}
	members := cleanMembers(g.Members)
	data, err := card.EncodeGroup(card.Group{UID: uid, Name: name, Members: members}, base)
	if err != nil {
		return Aggregate{}, err
	}
	source, err := newDomainRow(s.Scope(), "card", object, map[string]any{"address_book_id": book, "uid": uid, "resource_name": card.ResourceName(uid), "kind": "group", "data": data, "size_bytes": len(data), "etag": uuid.NewString(), "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return Aggregate{}, err
	}
	result := Aggregate{Version: 1, Scope: s.Scope(), Kind: "group_state", ID: id, Rows: []Row{primary, source}}
	if len(members) > 0 {
		people, err := s.visiblePeople()
		if err != nil {
			return Aggregate{}, err
		}
		indice, err := indexarMiembros(nil, people)
		if err != nil {
			return Aggregate{}, err
		}
		for _, m := range members {
			row, err := s.memberRow(indice, id, book, m, now)
			if err != nil {
				return Aggregate{}, err
			}
			result.Rows = append(result.Rows, row)
		}
	}
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
	return s.updateGroupWith(current, name, members, current.VCard, now)
}

// updateGroupWith es UpdateGroup sobre la vCard de base que se dé (la de una cuenta
// sincronizada, `externos.go`); UpdateGroup usa la que ya tiene el grupo.
func (s *Snapshot) updateGroupWith(current GroupView, name string, members []string, base string, now time.Time) (Aggregate, error) {
	id := current.ID
	name = strings.TrimSpace(name)
	if name == "" {
		name = current.Name
	}
	members = cleanMembers(members)
	data, err := card.EncodeGroup(card.Group{UID: current.UID, Name: name, Members: members}, base)
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
	indice, err := indexarMiembros(old, people)
	if err != nil {
		return Aggregate{}, err
	}
	for _, uid := range members {
		row, err := s.memberRow(indice, current.ID, current.AddressBookID, uid, now)
		if err != nil {
			return Aggregate{}, err
		}
		change.Rows = append(change.Rows, row)
	}
	return change, change.Validate(s.Scope())
}

// indiceMiembros es lo que mira memberRow, preparado una vez por cada escritura de una
// etiqueta (antes se recorría entero por cada miembro, y una etiqueta con miles era
// cuadrática): de cada UID, la primera fila de miembro de old que lo lleva; y de cada
// libreta y UID, el primero de people (en su orden) que lo tiene.
type indiceMiembros struct {
	filas    map[string]Row
	personas map[string]string
}

func indexarMiembros(old []Row, people []ContactView) (indiceMiembros, error) {
	m := indiceMiembros{filas: map[string]Row{}, personas: make(map[string]string, len(people))}
	for _, row := range old {
		if row.Kind != "member" {
			continue
		}
		var value struct {
			UID string `json:"contact_uid"`
		}
		if json.Unmarshal(row.JSON, &value) != nil {
			return m, ErrSnapshot
		}
		if _, visto := m.filas[value.UID]; !visto {
			m.filas[value.UID] = row
		}
	}
	for _, person := range people {
		clave := person.AddressBookID + "\x00" + person.UID
		if _, visto := m.personas[clave]; !visto {
			m.personas[clave] = person.ID
		}
	}
	return m, nil
}

// memberRow es la fila del miembro uid de la etiqueta group de book: la que ya tenía (en
// indice), con el contacto que lo resuelve si lo hay, o una nueva.
func (s *Snapshot) memberRow(indice indiceMiembros, group, book, uid string, now time.Time) (Row, error) {
	var contact any
	if id, ok := indice.personas[book+"\x00"+uid]; ok {
		contact = id
	}
	fields := map[string]any{"group_id": group, "contact_uid": uid, "contact_id": contact}
	if row, ok := indice.filas[uid]; ok {
		return patchRow(row, fields)
	}
	fields["created_at"] = now
	return newDomainRow(s.Scope(), "member", uuid.NewString(), fields)
}

func (r *Repository) DeleteGroup(ctx context.Context, id string) error {
	_, err := r.mutateAggregate(ctx, "delete-group", struct{ ID string }{id}, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.PrepararBorrarGrupo(id, time.Now().UTC())
	})
	return err
}

func (r *Repository) BatchGroupMembers(ctx context.Context, id string, uids []string, add bool) error {
	_, err := r.mutateAggregate(ctx, "group-members", struct {
		ID   string
		UIDs []string
		Add  bool
	}{id, uids, add}, func(snapshot *Snapshot) (Aggregate, error) {
		return snapshot.PrepararMiembros(id, uids, add, time.Now().UTC())
	})
	return err
}
