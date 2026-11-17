// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

var _ api.PhotosRecipeData = (*Repository)(nil)

func recipeView(recipe model.Receta) api.Receta {
	view := api.Receta{Version: recipe.Version, Pasos: make([]api.PasoReceta, len(recipe.Pasos))}
	for i, step := range recipe.Pasos {
		view.Pasos[i] = api.PasoReceta{Tipo: string(step.Tipo), Params: maps.Clone(step.Params), Eje: step.Eje}
	}
	return view
}

func recipeModel(recipe api.Receta) model.Receta {
	value := model.Receta{Version: recipe.Version, Pasos: make([]model.Paso, len(recipe.Pasos))}
	for i, step := range recipe.Pasos {
		value.Pasos[i] = model.Paso{Tipo: model.TipoPaso(step.Tipo), Params: maps.Clone(step.Params), Eje: step.Eje}
	}
	return value
}

func (r *Repository) LeerReceta(ctx context.Context, id string) (api.Receta, error) {
	recipe, err := r.Recipe(ctx, id)
	return recipeView(recipe), err
}

func (r *Repository) GuardarRecetaDesde(ctx context.Context, id string, recipe api.Receta, base string) (string, error) {
	return r.SaveRecipe(ctx, id, recipeModel(recipe), base)
}

// EdicionPendiente reconstructs only inputs whose hash matches the captured WAL.
// The observed current recipe never supplies a missing base or desired step.
func (r *Repository) EdicionPendiente(parent context.Context, id string) (*api.EdicionPhotos, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	if _, err := r.asset(ctx, id); err != nil {
		return nil, err
	}
	r.mediaMu.RLock()
	editID := r.editsByAsset[id]
	r.mediaMu.RUnlock()
	if editID == "" {
		editID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(r.scope.Tenant+"\x00"+r.scope.Subject+"\x00"+id)).String()
	}
	pending, err := r.engine.ReadPendingContent(ctx, recipeOperation, "edit", editID)
	if err != nil || pending == nil {
		return nil, err
	}
	defer clear(pending.JSON)
	defer clear(pending.Captured)
	row, origin, err := r.recipeIntent(pending.JSON, pending.Captured, pending.Target)
	if err != nil || row.AssetID != id {
		return nil, ErrSource
	}
	for _, base := range []string{origin.Base, ""} {
		request := recipeRequest{Asset: id, Recipe: row.Recipe, Expected: base}
		raw, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		fingerprint := planBytesHash(raw)
		clear(raw)
		if fingerprint == pending.InputFingerprint && origin.Operation == recipeOperation+"_"+fingerprint {
			return &api.EdicionPhotos{Receta: recipeView(row.Recipe), Base: base}, nil
		}
	}
	return nil, ErrSource
}
