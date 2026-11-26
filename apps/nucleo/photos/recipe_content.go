// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func (r *Repository) recipeContent(capture func(context.Context, storage.PublicationTarget) (recipeOrigin, error)) records.MutationContent {
	return records.MutationContent{
		Capture: func(ctx context.Context, raw []byte, target storage.PublicationTarget) ([]byte, error) {
			if capture == nil {
				return nil, ErrSource
			}
			origin, err := capture(ctx, target)
			if err != nil {
				return nil, err
			}
			var row model.EditRow
			if json.Unmarshal(raw, &row) != nil {
				return nil, ErrSource
			}
			sets, err := renditionFields(raw)
			if err != nil {
				return nil, err
			}
			if existing := sets[row.Fingerprint]; len(existing) > 0 {
				origin.ExistingSet = planBytesHash(existing)
			}
			return json.Marshal(origin)
		},
		Prepare:  r.prepareRecipeContent,
		Validate: r.validatePreparedRecipe,
		Acknowledge: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion) error {
			_, origin, err := r.recipeIntent(raw, captured, target)
			if err != nil {
				return err
			}
			set, err := r.currentRenditions(raw)
			if err != nil {
				return err
			}
			files := renditionFiles(set)
			return r.engine.AcknowledgeRetainedFiles(ctx, origin.Operation, files, accepted, func(raw []byte) ([]records.RetainedFile, error) {
				set, err := r.currentRenditions(raw)
				if err != nil {
					return nil, err
				}
				return renditionFiles(set), nil
			})
		},
	}
}

func (r *Repository) captureRecipeOrigin(ctx context.Context, id, base, operation string, target storage.PublicationTarget) (recipeOrigin, error) {
	asset, err := r.asset(ctx, id)
	if err != nil {
		return recipeOrigin{}, err
	}
	if asset.Bytes > photorender.MaxOriginal {
		return recipeOrigin{}, model.ErrInvalidInput
	}
	version, err := r.engine.ReadyVersion("original", id)
	if errors.Is(err, ErrNotFound) {
		if raw, err := r.engine.ReadRaw(ctx, "asset", id); err == nil {
			_, externa, err := r.externalAsset(raw)
			clear(raw)
			if err != nil {
				return recipeOrigin{}, err
			}
			if externa != nil {
				// Foto enlazada (FO-09): la receta lee el original de su fichero.
				if esVideo(asset.MediaType) {
					return recipeOrigin{}, model.ErrInvalidInput
				}
				descriptor := records.SnapshotRow{Kind: "original", ID: id, Size: externa.Request.Size, SHA256: externa.Request.SHA256}
				return recipeOrigin{Version: 1, Scope: r.scope, Target: target, Base: base, Operation: operation, Asset: asset, Descriptor: descriptor, Externo: externa.Path}, nil
			}
		}
		// Un vídeo copiado a la fototeca (ImportarVideo, FO-09) tampoco se edita.
		if esVideo(asset.MediaType) {
			return recipeOrigin{}, model.ErrInvalidInput
		}
		// Foto ingestada en este dispositivo: su original vive enlazado desde la
		// fila media, no como entrada binaria del catálogo.
		file, err := r.linkedOriginal(ctx, id)
		if err != nil {
			return recipeOrigin{}, err
		}
		if file.Row.Size != asset.Bytes {
			return recipeOrigin{}, ErrSource
		}
		return recipeOrigin{Version: 1, Scope: r.scope, Target: target, Base: base, Operation: operation, Asset: asset, Descriptor: file.Row, Retained: &file}, nil
	}
	if err != nil {
		return recipeOrigin{}, err
	}
	original := records.Entry{Kind: "original", ID: id, State: version.Commit.State, Binary: true, Version: version}
	catalog, err := r.engine.Catalog()
	if err != nil {
		return recipeOrigin{}, err
	}
	binaries, ok := catalog.(records.BinaryCatalog)
	if !ok {
		return recipeOrigin{}, ErrSource
	}
	descriptor, err := binaries.BinaryInfo(ctx, original)
	if err != nil {
		return recipeOrigin{}, err
	}
	if descriptor.Kind != "original" || descriptor.ID != id || descriptor.Size != asset.Bytes {
		return recipeOrigin{}, ErrSource
	}
	return recipeOrigin{Version: 1, Scope: r.scope, Target: target, Base: base, Operation: operation, Asset: asset, Original: original, Descriptor: descriptor}, nil
}

func (r *Repository) recipeIntent(raw, captured []byte, target storage.PublicationTarget) (model.EditRow, recipeOrigin, error) {
	var row model.EditRow
	var origin recipeOrigin
	if json.Unmarshal(raw, &row) != nil || json.Unmarshal(captured, &origin) != nil {
		return row, origin, ErrSource
	}
	if _, err := r.decodeEdit(raw, row.AssetID, row.ID); err != nil {
		return row, origin, err
	}
	if origin.Version != 1 || origin.Scope != r.scope || origin.Target != target || origin.Base == "" || origin.Operation == "" || len(origin.Operation) > 256 {
		return row, origin, ErrSource
	}
	if origin.Asset.ID != row.AssetID || origin.Asset.Tenant != r.scope.Tenant || origin.Asset.Owner != r.scope.Subject || origin.Asset.DeletedAt != nil {
		return row, origin, ErrSource
	}
	if !validRecipeOriginal(origin, row.AssetID) {
		return row, origin, ErrSource
	}
	return row, origin, nil
}

func (r *Repository) prepareRecipeContent(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) ([]byte, error) {
	row, origin, err := r.recipeIntent(raw, captured, target)
	if err != nil {
		return nil, err
	}
	sets, err := renditionFields(raw)
	if err != nil {
		return nil, err
	}
	if origin.ExistingSet != "" {
		if planBytesHash(sets[row.Fingerprint]) != origin.ExistingSet {
			return nil, ErrSource
		}
		return bytes.Clone(raw), nil
	}
	set := recipeRenditions{Version: 1, Asset: row.AssetID, Fingerprint: row.Fingerprint, Files: map[model.Variante]records.RetainedFile{}}
	sides, variants := renditionSides()
	pngOutput := strings.EqualFold(origin.Asset.MediaType, "image/png") || strings.HasSuffix(strings.ToLower(origin.Asset.Name), ".png")
	err = photorender.Derivar(ctx, func(ctx context.Context) ([]byte, error) { return r.capturedOriginal(ctx, origin) }, model.OrientacionDe(origin.Asset.Orientation), row.Recipe, pngOutput, sides, func(side int, format, _ string, encoded []byte) error {
		variant := variants[side]
		file, err := r.stageRendition(ctx, origin.Operation, target, row.AssetID, row.Fingerprint, variant, format, encoded)
		if err != nil {
			return err
		}
		set.Format = format
		set.Files[variant] = file
		return nil
	})
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrSource
	}
	sets[row.Fingerprint], err = json.Marshal(set)
	if err != nil {
		return nil, err
	}
	fields[renditionField], err = json.Marshal(sets)
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func (r *Repository) capturedOriginal(ctx context.Context, origin recipeOrigin) ([]byte, error) {
	if origin.Externo != "" {
		return leerExterno(ctx, origin.Externo, origin.Descriptor.Size, origin.Descriptor.SHA256, photorender.MaxOriginal)
	}
	if origin.Retained != nil {
		body := storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error {
			return r.engine.ReadRetainedFile(ctx, *origin.Retained, out)
		})
		defer func() { _ = body.Close() }()
		raw, err := io.ReadAll(io.LimitReader(body, photorender.MaxOriginal+1))
		if err == nil && len(raw) > photorender.MaxOriginal {
			err = model.ErrInvalidInput
		}
		return raw, err
	}
	catalog, err := r.engine.Catalog()
	if err != nil {
		return nil, err
	}
	binaries, ok := catalog.(records.BinaryContentCatalog)
	if !ok {
		return nil, ErrSource
	}
	body := storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error {
		descriptor, err := binaries.ReadBinary(ctx, origin.Original, out)
		if err == nil && descriptor != origin.Descriptor {
			return ErrSource
		}
		return err
	})
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, photorender.MaxOriginal+1))
	if err == nil && len(raw) > photorender.MaxOriginal {
		err = model.ErrInvalidInput
	}
	return raw, err
}

func (r *Repository) validatePreparedRecipe(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) error {
	row, origin, err := r.recipeIntent(raw, captured, target)
	if err != nil {
		return err
	}
	sets, err := renditionFields(raw)
	if err != nil {
		return err
	}
	if origin.ExistingSet != "" && planBytesHash(sets[row.Fingerprint]) != origin.ExistingSet {
		return ErrSource
	}
	set, err := r.currentRenditions(raw)
	if err != nil {
		return err
	}
	if origin.ExistingSet == "" {
		for _, file := range set.Files {
			if file.Target != target {
				return ErrSource
			}
		}
	}
	for _, file := range renditionFiles(set) {
		if err := r.engine.ValidateRetainedFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

func validRecipeOriginal(origin recipeOrigin, asset string) bool {
	if origin.Externo != "" {
		return validExternalRecipeOriginal(origin, asset)
	}
	if origin.Retained != nil {
		return validRetainedRecipeOriginal(origin, asset)
	}
	return origin.Original.Binary && origin.Original.Kind == "original" && origin.Original.ID == asset && origin.Descriptor.Kind == "original" && origin.Descriptor.ID == asset && origin.Descriptor.Size == origin.Asset.Bytes && origin.Descriptor.Validate() == nil
}

// validExternalRecipeOriginal es el original de una foto enlazada (FO-09), que se lee de
// su fichero: no puede venir mezclado con un retenido ni con una entrada de catálogo.
func validExternalRecipeOriginal(origin recipeOrigin, asset string) bool {
	empty := origin.Original.Kind == "" && origin.Original.ID == "" && !origin.Original.Binary && origin.Original.Version.Reference == (storage.Reference{})
	return empty && origin.Retained == nil && filepath.IsAbs(origin.Externo) && origin.Descriptor.Kind == "original" &&
		origin.Descriptor.ID == asset && origin.Descriptor.Size == origin.Asset.Bytes && origin.Descriptor.Validate() == nil
}

// validRetainedRecipeOriginal es el original retenido: no puede venir mezclado con una
// entrada de catálogo.
func validRetainedRecipeOriginal(origin recipeOrigin, asset string) bool {
	file := *origin.Retained
	empty := origin.Original.Kind == "" && origin.Original.ID == "" && !origin.Original.Binary
	return empty && origin.Original.Version.Commit.Metadata == nil && origin.Original.Version.Reference == (storage.Reference{}) &&
		file.Version == 1 && file.Scope == origin.Scope && file.Target == origin.Target && file.Row.Kind == "original" &&
		file.Row.ID == asset && file.Row.Validate() == nil && file.Row.Size == origin.Asset.Bytes &&
		origin.Descriptor == file.Row
}
