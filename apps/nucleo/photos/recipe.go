// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const (
	recipeOperation = "recipe-v1"
	renditionField  = "private_renditions"
)

type recipeRequest struct {
	Asset    string       `json:"asset"`
	Recipe   model.Receta `json:"recipe"`
	Expected string       `json:"expected"`
}

type recipeOrigin struct {
	Version     int                       `json:"v"`
	Scope       storage.Scope             `json:"scope"`
	Target      storage.PublicationTarget `json:"target"`
	Base        string                    `json:"base"`
	Operation   string                    `json:"operation"`
	ExistingSet string                    `json:"existing_set,omitempty"`
	Asset       model.AssetRow            `json:"asset"`
	Original    records.Entry             `json:"original"`
	Descriptor  records.SnapshotRow       `json:"descriptor"`
	// Retained es el original de una foto ingestada en este dispositivo: un
	// fichero retenido enlazado desde la fila media, sin entrada de catálogo.
	Retained *records.RetainedFile `json:"retained,omitempty"`
}

type recipeRenditions struct {
	Version     int                                     `json:"v"`
	Asset       string                                  `json:"asset"`
	Fingerprint string                                  `json:"fingerprint"`
	Format      string                                  `json:"format"`
	Files       map[model.Variante]records.RetainedFile `json:"files"`
}

// Recipe reads the current authenticated recipe; it performs no source import.
func (r *Repository) Recipe(parent context.Context, id string) (model.Receta, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return model.Receta{}, err
	}
	defer done()
	if _, err := r.asset(ctx, id); err != nil {
		return model.Receta{}, err
	}
	return r.retainedRecipe(ctx, id)
}

// SaveRecipe commits the recipe and its complete generated renditions together.
// An explicit expected fingerprint is the editor's base, never a device clock.
func (r *Repository) SaveRecipe(parent context.Context, id string, recipe model.Receta, expected string) (string, error) {
	if err := recipe.Valida(); err != nil {
		return "", err
	}
	fingerprint, err := recipe.Huella()
	if err != nil {
		return "", err
	}
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return "", err
	}
	defer done()
	request := recipeRequest{Asset: id, Recipe: recipe, Expected: expected}
	input, _ := json.Marshal(request)
	operation := recipeOperation + "_" + planBytesHash(input)
	var base string
	result, err := r.engine.MutateWithContent(ctx, recipeOperation, request, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, actualBase, editID, err := r.prepareRecipe(ctx, request, fingerprint)
		base = actualBase
		return "edit", editID, "live", raw, err
	}, r.recipeContent(func(ctx context.Context, target storage.PublicationTarget) (recipeOrigin, error) {
		return r.captureRecipeOrigin(ctx, id, base, operation, target)
	}))
	defer clear(result)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	if err := r.indexEdits(ctx); err != nil {
		return "", err
	}
	return fingerprint, nil
}

func (r *Repository) prepareRecipe(ctx context.Context, request recipeRequest, fingerprint string) ([]byte, string, string, error) {
	if _, err := r.asset(ctx, request.Asset); err != nil {
		return nil, "", "", err
	}
	catalog, err := r.engine.Catalog()
	if err != nil {
		return nil, "", "", err
	}
	index, err := r.editIndex(ctx, catalog)
	if err != nil {
		return nil, "", "", err
	}
	editID := index[request.Asset]
	fields := map[string]json.RawMessage{}
	base, err := model.RecetaVacia().Huella()
	if err != nil {
		return nil, "", "", err
	}
	if editID != "" {
		raw, err := r.engine.ReadRaw(ctx, "edit", editID)
		if err != nil {
			return nil, "", "", err
		}
		defer clear(raw)
		row, err := r.decodeEdit(raw, request.Asset, editID)
		if err != nil {
			return nil, "", "", err
		}
		if json.Unmarshal(raw, &fields) != nil {
			return nil, "", "", ErrSource
		}
		base = row.Fingerprint
	} else {
		editID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(r.scope.Tenant+"\x00"+r.scope.Subject+"\x00"+request.Asset)).String()
		raw, _ := json.Marshal(model.EditRow{ID: editID, Tenant: r.scope.Tenant, Owner: r.scope.Subject, AssetID: request.Asset})
		_ = json.Unmarshal(raw, &fields)
	}
	if request.Expected != "" && request.Expected != base {
		return nil, "", "", ErrConflict
	}
	patched, err := patchRecipe(fields["recipe"], request.Recipe)
	if err != nil {
		return nil, "", "", err
	}
	fields["recipe"] = patched
	fields["fingerprint"], _ = json.Marshal(fingerprint)
	fields["updated_at"], _ = json.Marshal(time.Now().UTC())
	raw, err := json.Marshal(fields)
	return raw, base, editID, err
}

func (r *Repository) decodeEdit(raw []byte, asset, id string) (model.EditRow, error) {
	var row model.EditRow
	if json.Unmarshal(raw, &row) != nil || row.ID != id || row.AssetID != asset || row.Tenant != r.scope.Tenant || row.Owner != r.scope.Subject {
		return row, ErrSource
	}
	fingerprint, err := row.Recipe.Huella()
	if err != nil || row.Recipe.Valida() != nil || row.Fingerprint != fingerprint {
		return row, ErrSource
	}
	return row, nil
}

func patchRecipe(previous []byte, recipe model.Receta) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(previous) > 0 && string(previous) != "null" {
		if json.Unmarshal(previous, &fields) != nil {
			return nil, ErrSource
		}
	}
	fields["version"], _ = json.Marshal(recipe.Version)
	var old []json.RawMessage
	if raw := fields["pasos"]; len(raw) > 0 {
		if json.Unmarshal(raw, &old) != nil {
			return nil, ErrSource
		}
	}
	steps := make([]json.RawMessage, 0, len(recipe.Pasos))
	for i, step := range recipe.Pasos {
		original := json.RawMessage(nil)
		if i < len(old) {
			original = old[i]
		}
		patched, err := patchRecipeStep(original, step)
		if err != nil {
			return nil, err
		}
		steps = append(steps, patched)
	}
	fields["pasos"], _ = json.Marshal(steps)
	return json.Marshal(fields)
}

func patchRecipeStep(previous []byte, step model.Paso) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(previous) > 0 {
		var old model.Paso
		if json.Unmarshal(previous, &old) != nil {
			return nil, ErrSource
		}
		if old.Tipo == step.Tipo {
			if json.Unmarshal(previous, &fields) != nil {
				return nil, ErrSource
			}
		}
	}
	fields["tipo"], _ = json.Marshal(step.Tipo)
	delete(fields, "params")
	delete(fields, "eje")
	if len(step.Params) > 0 {
		fields["params"], _ = json.Marshal(step.Params)
	}
	if step.Eje != "" {
		fields["eje"], _ = json.Marshal(step.Eje)
	}
	return json.Marshal(fields)
}
