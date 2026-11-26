// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
)

var _ api.ContactsBatchData = (*Repository)(nil)

// BatchVCard junta en un `.vcf` las vCard guardadas de esos contactos, por id y en el
// orden pedido. Los borrados o que no existen no vienen (como en el servicio).
func (r *Repository) BatchVCard(ctx context.Context, ids []string) ([]byte, error) {
	if len(ids) > api.MaxLoteContactos {
		return nil, queryError(400, "contact_batch_too_large")
	}
	people, err := readQuery(ctx, r, (*Snapshot).visiblePeople)
	if err != nil {
		return nil, err
	}
	porID := make(map[string]string, len(people))
	for _, p := range people {
		porID[p.ID] = p.VCard
	}
	var b strings.Builder
	visto := map[string]bool{}
	for _, id := range ids {
		tarjeta, ok := porID[id]
		if !ok || visto[id] || tarjeta == "" {
			continue
		}
		visto[id] = true
		b.WriteString(tarjeta)
		if !strings.HasSuffix(tarjeta, "\n") {
			b.WriteString("\r\n")
		}
	}
	return []byte(b.String()), nil
}

// BatchRestore es el «Deshacer» de BatchTrash: devuelve de la papelera lo último que se
// tiró de cada UID (de cada libreta donde viva) y dice cuántos volvieron.
func (r *Repository) BatchRestore(parent context.Context, uids []string) (int, error) {
	uids = cleanMembers(uids)
	if len(uids) == 0 {
		return 0, nil
	}
	vueltos := 0
	raw, err := r.core.Mutate(parent, "trash-index", struct {
		Operation string
		IDs       []string
		ByUID     bool
	}{"batch-restore", uids, true}, func(ctx context.Context) (string, string, string, []byte, error) {
		snapshot, err := r.Snapshot(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		current, err := r.trashIndex(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		state, n, err := snapshot.RestoreLatest(current, uids)
		if err != nil {
			return "", "", "", nil, err
		}
		vueltos = n
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
	return vueltos, err
}

// RestoreLatest saca de la papelera, por cada UID y libreta, el contacto borrado más
// reciente. Un UID que no está en la papelera no cuenta.
func (s *Snapshot) RestoreLatest(current TrashIndex, uids []string) (TrashIndex, int, error) {
	if current.Validate(s.Scope()) != nil {
		return TrashIndex{}, 0, ErrSnapshot
	}
	// Los guardados, solo para leer.
	people, err := s.contactos()
	if err != nil {
		return TrashIndex{}, 0, err
	}
	state := current
	state.Values = make(map[string]Removal, len(current.Values))
	for id, value := range current.Values {
		state.Values[id] = value
	}
	ultimo := map[string]ContactView{}
	for _, person := range people {
		if person.BookDeleted || person.DeletedAt == nil || !slices.Contains(uids, person.UID) {
			continue
		}
		clave := person.UID + "\x00" + person.AddressBookID
		if prev, ok := ultimo[clave]; !ok || person.DeletedAt.After(*prev.DeletedAt) {
			ultimo[clave] = person
		}
	}
	for _, person := range ultimo {
		state.Values[person.ID] = Removal{}
	}
	return state, len(ultimo), state.Validate(s.Scope())
}
