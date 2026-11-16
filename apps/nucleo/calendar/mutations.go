// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"context"
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func rowDeleted(row model.Row) bool {
	var v struct {
		DeletedAt *time.Time `json:"deleted_at"`
	}
	return json.Unmarshal(row.JSON, &v) != nil || v.DeletedAt != nil
}

// patch preserves unknown JSON columns as raw messages, including large integers.
func patch(row model.Row, changes map[string]any) (model.Row, error) {
	var values map[string]json.RawMessage
	if json.Unmarshal(row.JSON, &values) != nil || values == nil {
		return model.Row{}, model.ErrSnapshot
	}
	for k, v := range changes {
		raw, err := json.Marshal(v)
		if err != nil {
			return model.Row{}, err
		}
		values[k] = raw
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return model.Row{}, err
	}
	row.JSON = raw
	return row, nil
}

func newRow(scope storage.Scope, kind, id string, fields map[string]any) (model.Row, error) {
	fields["id"], fields["tenant"] = id, scope.Tenant
	raw, err := json.Marshal(fields)
	if err != nil {
		return model.Row{}, err
	}
	row := model.Row{Kind: kind, ID: id, JSON: raw}
	return row, model.ValidateRow(scope, row)
}

func (r *Repository) mutate(ctx context.Context, operation string, input any, prepare func(*model.Snapshot) (model.Aggregate, error)) (model.Aggregate, error) {
	raw, err := r.core.Mutate(ctx, operation, input, func(ctx context.Context) (string, string, string, []byte, error) {
		snapshot, err := r.Snapshot(ctx)
		if err != nil {
			return "", "", "", nil, err
		}
		a, err := prepare(snapshot)
		if err != nil {
			return "", "", "", nil, err
		}
		if _, err := snapshot.WithAggregates([]model.Aggregate{a}); err != nil {
			return "", "", "", nil, err
		}
		state := "live"
		for _, row := range a.Rows {
			if row.ID == a.ID && (row.Kind == "calendar" || row.Kind == "object") && rowDeleted(row) {
				state = "trash"
			}
		}
		raw, err := json.Marshal(a)
		return a.Kind, a.ID, state, raw, err
	})
	if err != nil {
		return model.Aggregate{}, err
	}
	defer clear(raw)
	var result model.Aggregate
	if json.Unmarshal(raw, &result) != nil || result.Validate(r.scope) != nil {
		return model.Aggregate{}, model.ErrSnapshot
	}
	return result, nil
}
