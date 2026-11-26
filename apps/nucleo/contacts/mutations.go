// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) mutateAggregate(parent context.Context, operation string, input any, prepare func(*Snapshot) (Aggregate, error)) (Aggregate, error) {
	// hecha es lo que vio y compuso la preparación, si el motor la llamó.
	var hecha *escritura
	raw, err := r.core.Mutate(parent, operation, input, func(ctx context.Context) (string, string, string, []byte, error) {
		hecha = nil
		base, err := r.foto(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		snapshot := base.snapshot.copia()
		change, err := prepare(snapshot)
		if err != nil {
			return "", "", "", nil, err
		}
		if err := change.Validate(r.scope); err != nil {
			return "", "", "", nil, err
		}
		candidate, err := snapshot.WithAggregates([]Aggregate{change})
		if err != nil {
			return "", "", "", nil, err
		}
		if err := validateViews(candidate); err != nil {
			return "", "", "", nil, err
		}
		payload, err := json.Marshal(change)
		state := "live"
		if change.DeletedAt != nil {
			state = "trash"
		}
		if err == nil {
			hecha = &escritura{base: base, change: change, candidate: candidate, payload: slices.Clone(payload)}
		}
		return change.Kind, change.ID, state, payload, err
	})
	if err != nil {
		return Aggregate{}, err
	}
	defer clear(raw)
	var change Aggregate
	if json.Unmarshal(raw, &change) != nil || change.Validate(r.scope) != nil {
		return Aggregate{}, ErrSnapshot
	}
	r.anotarEscritura(hecha, raw)
	return change, nil
}

func patchRow(row Row, changes map[string]any) (Row, error) {
	var values map[string]json.RawMessage
	if json.Unmarshal(row.JSON, &values) != nil || values == nil {
		return Row{}, ErrSnapshot
	}

	for key, value := range changes {
		raw, err := json.Marshal(value)
		if err != nil {
			return Row{}, err
		}
		values[key] = raw
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return Row{}, err
	}
	row.JSON = raw
	return row, nil
}

func newDomainRow(scope storage.Scope, kind, id string, fields map[string]any) (Row, error) {
	fields["id"], fields["tenant"] = id, scope.Tenant
	raw, err := json.Marshal(fields)
	if err != nil {
		return Row{}, err
	}
	row := Row{Kind: kind, ID: id, JSON: raw}
	return row, ValidateRow(scope, row)
}

func editAggregate(snapshot *Snapshot, kind, id string) (Aggregate, error) {
	change, err := snapshot.Edit(kind, id)
	if err != nil {
		return Aggregate{}, queryError(http.StatusNotFound, "private_contact_resource_not_found")
	}
	return change, nil
}

func rowFields(row Row) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(row.JSON, &fields) != nil || fields == nil {
		return nil, ErrSnapshot
	}
	return fields, nil
}

func aggregateTimestamp(change *Aggregate, now time.Time) error {
	change.DeletedAt = &now
	for i, row := range change.Rows {
		if row.Kind != "book" && row.Kind != "card" && row.Kind != "contact" && row.Kind != "group" {
			continue
		}
		updated, err := patchRow(row, map[string]any{"deleted_at": now, "updated_at": now})
		if err != nil {
			return err
		}
		change.Rows[i] = updated
	}
	return nil
}
