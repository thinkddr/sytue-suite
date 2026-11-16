// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

func renditionSides() ([]int, map[int]model.Variante) {
	sides := make([]int, 0, len(model.Variantes()))
	variants := map[int]model.Variante{}
	for _, variant := range model.Variantes() {
		side, _ := variant.LadoMayor()
		sides = append(sides, side)
		variants[side] = variant
	}
	return sides, variants
}

func (r *Repository) stageRendition(ctx context.Context, operation string, target storage.PublicationTarget, asset, fingerprint string, variant model.Variante, format string, encoded []byte) (records.RetainedFile, error) {
	parsed, err := uuid.Parse(asset)
	if err != nil || parsed.String() != asset {
		return records.RetainedFile{}, model.ErrInvalidInput
	}
	key, err := (model.Derivada{ActivoID: parsed, Huella: fingerprint, Variante: variant, Formato: format}).Clave()
	if err != nil {
		return records.RetainedFile{}, err
	}
	descriptor := records.SnapshotRow{Kind: "variant", ID: planBytesHash([]byte(key)), Size: int64(len(encoded)), SHA256: planBytesHash(encoded)}
	return r.engine.StageRetainedFile(ctx, operation, target, descriptor, func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(encoded)), nil })
}

func renditionFields(raw []byte) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrSource
	}
	sets := map[string]json.RawMessage{}
	if encoded := fields[renditionField]; len(encoded) > 0 {
		if string(encoded) == "null" || json.Unmarshal(encoded, &sets) != nil || sets == nil {
			return nil, ErrSource
		}
	}
	return sets, nil
}

func (r *Repository) currentRenditions(raw []byte) (recipeRenditions, error) {
	var row model.EditRow
	if json.Unmarshal(raw, &row) != nil {
		return recipeRenditions{}, ErrSource
	}
	if _, err := r.decodeEdit(raw, row.AssetID, row.ID); err != nil {
		return recipeRenditions{}, err
	}
	sets, err := renditionFields(raw)
	if err != nil {
		return recipeRenditions{}, err
	}
	set, err := r.decodeRenditions(sets[row.Fingerprint], row.AssetID, row.Fingerprint)
	return set, err
}

func (r *Repository) decodeRenditions(raw []byte, asset, fingerprint string) (recipeRenditions, error) {
	var set recipeRenditions
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) != 6 || hex.EncodeToString(decoded) != fingerprint || json.Unmarshal(raw, &set) != nil {
		return set, ErrSource
	}
	parsed, err := uuid.Parse(asset)
	if err != nil || parsed.String() != asset || set.Version != 1 || set.Asset != asset || set.Fingerprint != fingerprint || (set.Format != "jpg" && set.Format != "png") || len(set.Files) != len(model.Variantes()) {
		return set, ErrSource
	}
	for _, variant := range model.Variantes() {
		file, found := set.Files[variant]
		if !found || !r.renditionFileShape(file) {
			return set, ErrSource
		}
		key, err := (model.Derivada{ActivoID: parsed, Huella: fingerprint, Variante: variant, Formato: set.Format}).Clave()
		if err != nil || file.Row.ID != planBytesHash([]byte(key)) {
			return set, ErrSource
		}
	}
	return set, nil
}

func renditionFiles(set recipeRenditions) []records.RetainedFile {
	files := make([]records.RetainedFile, 0, len(model.Variantes()))
	for _, variant := range model.Variantes() {
		files = append(files, set.Files[variant])
	}
	return files
}

func (r *Repository) validateEditRenditions(ctx context.Context, catalog records.Catalog, raw []byte) error {
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return ErrSource
	}
	if len(fields[renditionField]) == 0 {
		return nil
	}
	var row model.EditRow
	if json.Unmarshal(raw, &row) != nil {
		return ErrSource
	}
	sets, err := renditionFields(raw)
	if err != nil {
		return err
	}
	for fingerprint, raw := range sets {
		if _, err := r.decodeRenditions(raw, row.AssetID, fingerprint); err != nil {
			return err
		}
	}
	current, err := r.currentRenditions(raw)
	if err != nil {
		return err
	}
	retained, ok := catalog.(records.RetainedFileCatalog)
	if !ok {
		return ErrSource
	}
	// Historical sets remain authenticated by this exact edit JSON; their bodies
	// and SDK commitments are checked on demand. Opening checks the current set.
	for _, file := range renditionFiles(current) {
		if err := retained.ValidateRetainedFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) linkedRendition(ctx context.Context, asset, variant, fingerprint string) (records.RetainedFile, error) {
	r.mediaMu.RLock()
	editID := r.editsByAsset[asset]
	r.mediaMu.RUnlock()
	if editID == "" {
		return r.initialRendition(ctx, asset, variant, fingerprint)
	}
	raw, err := r.engine.ReadRaw(ctx, "edit", editID)
	if err != nil {
		return records.RetainedFile{}, err
	}
	defer clear(raw)
	row, err := r.decodeEdit(raw, asset, editID)
	if err != nil {
		return records.RetainedFile{}, err
	}
	if fingerprint == "" {
		fingerprint = row.Fingerprint
	}
	sets, err := renditionFields(raw)
	if err != nil {
		return records.RetainedFile{}, err
	}
	if len(sets[fingerprint]) == 0 {
		return r.initialRendition(ctx, asset, variant, fingerprint)
	}
	set, err := r.decodeRenditions(sets[fingerprint], asset, fingerprint)
	if err != nil {
		return records.RetainedFile{}, err
	}
	file, found := set.Files[model.Variante(variant)]
	if !found {
		return records.RetainedFile{}, model.ErrInvalidInput
	}
	return file, nil
}

func (r *Repository) renditionFileShape(file records.RetainedFile) bool {
	return file.Version == 1 && file.Scope == r.scope && file.Target.Valid() && file.Link.Valid() && file.Link.RoomID == file.Target.RoomID && storage.Identifier(file.Revision) && file.Row.Kind == "variant" && file.Row.Validate() == nil
}
