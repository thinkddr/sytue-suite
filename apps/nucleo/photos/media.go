// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// VariantDe resolves exact retained bytes from authenticated metadata. Missing
// derivations require an authorized device worker; no server rendering fallback
// receives plaintext. An explicit fingerprint selects only its immutable media.
func (r *Repository) VariantDe(parent context.Context, id, variant, fingerprint string) (io.ReadCloser, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	media, err := r.variant(ctx, id, variant, fingerprint)
	if err == nil {
		return storage.PipeRead(ctx, done, func(ctx context.Context, out io.Writer) error { return r.ReadVariant(ctx, media.ID, out) }), nil
	}
	if !errors.Is(err, ErrNotFound) {
		done()
		return nil, err
	}
	file, err := r.linkedRendition(ctx, id, variant, fingerprint)
	if err != nil {
		empty, _ := model.RecetaVacia().Huella()
		if errors.Is(err, ErrNotFound) && variant == "previa" && fingerprint == empty {
			return r.originalPreview(ctx, done, id)
		}
		done()
		return nil, err
	}
	return storage.PipeRead(ctx, done, func(ctx context.Context, out io.Writer) error { return r.engine.ReadRetainedFile(ctx, file, out) }), nil
}

func (r *Repository) variant(ctx context.Context, id, variant, fingerprint string) (model.MediaRow, error) {
	photo, err := r.Photo(ctx, id)
	if err != nil {
		return model.MediaRow{}, err
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return model.MediaRow{}, ErrSource
	}
	if fingerprint == "" {
		fingerprint, err = r.fingerprint(ctx, id)
		if err != nil {
			return model.MediaRow{}, err
		}
	}
	format := "jpg"
	if strings.EqualFold(photo.ContentType, "image/png") || strings.HasSuffix(strings.ToLower(photo.Name), ".png") {
		format = "png"
	}
	key, err := (model.Derivada{ActivoID: parsed, Huella: fingerprint, Variante: model.Variante(variant), Formato: format}).Clave()
	if err != nil {
		return model.MediaRow{}, err
	}
	mediaID := planBytesHash([]byte(key))
	raw, err := r.engine.ReadRaw(ctx, "media", mediaID)
	defer clear(raw)
	if err != nil {
		return model.MediaRow{}, err
	}
	var media model.MediaRow
	if json.Unmarshal(raw, &media) != nil || media.Validate() != nil || media.ID != mediaID || media.Tenant != r.scope.Tenant || media.Owner != r.scope.Subject || media.AssetID != id || media.Role != "variant" || media.Fingerprint != fingerprint || string(media.Variant) != variant || media.Format != format {
		return model.MediaRow{}, ErrSource
	}
	return media, nil
}

func (r *Repository) fingerprint(ctx context.Context, id string) (string, error) {
	recipe, err := r.retainedRecipe(ctx, id)
	if err != nil {
		return "", err
	}
	return recipe.Huella()
}

func (r *Repository) retainedRecipe(ctx context.Context, id string) (model.Receta, error) {
	r.mediaMu.RLock()
	edits, ready := r.editsByAsset, r.editsByAsset != nil
	editID := edits[id]
	r.mediaMu.RUnlock()
	if !ready {
		return model.Receta{}, ErrNotReady
	}
	if editID == "" {
		return model.RecetaVacia(), nil
	}
	raw, err := r.engine.ReadRaw(ctx, "edit", editID)
	defer clear(raw)
	if err != nil {
		return model.Receta{}, err
	}
	edit, err := r.decodeEdit(raw, id, editID)
	if err != nil {
		return model.Receta{}, err
	}
	return edit.Recipe, nil
}

// indexEdits retains only asset/edit identifiers. It reads each recipe once;
// individual tiles perform direct immutable lookups rather than library scans.
func (r *Repository) indexEdits(ctx context.Context) error {
	catalog, err := r.engine.Catalog()
	if err != nil {
		return err
	}
	index, err := r.editIndex(ctx, catalog)
	if err != nil {
		return err
	}
	r.mediaMu.Lock()
	r.editsByAsset = index
	r.mediaMu.Unlock()
	return nil
}

func (r *Repository) editIndex(ctx context.Context, catalog records.Catalog) (map[string]string, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	index := make(map[string]string)
	for _, entry := range entries {
		if entry.Kind != "edit" {
			continue
		}
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		var edit model.EditRow
		err = json.Unmarshal(raw, &edit)
		clear(raw)
		if err != nil || edit.Owner != r.scope.Subject || edit.Tenant != r.scope.Tenant || edit.AssetID == "" || edit.ID != entry.ID || index[edit.AssetID] != "" {
			return nil, ErrSource
		}
		index[edit.AssetID] = edit.ID
	}
	return index, nil
}

func (r *Repository) RefreshPrivateData(ctx context.Context) error {
	return r.engine.RefreshWithProjection(ctx, func(ctx context.Context, catalog records.Catalog) (func(), error) {
		index, err := r.editIndex(ctx, catalog)
		if err != nil {
			return nil, err
		}
		return func() { r.mediaMu.Lock(); r.editsByAsset = index; r.mediaMu.Unlock() }, nil
	})
}
