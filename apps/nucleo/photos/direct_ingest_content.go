// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type ingestedMedia struct {
	Version    int                  `json:"v"`
	Request    uploadIngestRequest  `json:"request"`
	Original   records.RetainedFile `json:"original"`
	Renditions recipeRenditions     `json:"renditions"`
}

func (r *Repository) uploadIngestContent(request uploadIngestRequest, source offline.StagedFile, operation string, retain func()) records.MutationContent {
	return records.MutationContent{
		Capture: func(_ context.Context, raw []byte, target storage.PublicationTarget) ([]byte, error) {
			var asset model.AssetRow
			if json.Unmarshal(raw, &asset) != nil || asset.ID != request.ID || source.SizeBytes != request.Size || source.SHA256 != request.SHA256 {
				return nil, ErrSource
			}
			origin := uploadIngestOrigin{Version: 1, Scope: r.scope, Target: target, Request: request, Source: source, Operation: operation}
			if retain != nil {
				retain()
			}
			return json.Marshal(origin)
		},
		Prepare: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) ([]byte, error) {
			asset, origin, err := r.uploadIngestIntent(raw, captured, target, request, operation)
			if err != nil {
				return nil, err
			}
			return r.preparePhotoUpload(ctx, raw, asset, origin)
		},
		Validate: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) error {
			return r.validateUploadIngest(ctx, raw, captured, target, request, operation)
		},
		Acknowledge: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion) error {
			return r.acknowledgeUploadIngest(ctx, raw, captured, target, accepted, request, operation)
		},
	}
}

func (r *Repository) validateUploadIngest(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, request uploadIngestRequest, operation string) error {
	if _, _, err := r.uploadIngestIntent(raw, captured, target, request, operation); err != nil {
		return err
	}
	_, media, err := r.ingestedAsset(raw)
	if err != nil || media == nil || media.Request != request {
		return ErrSource
	}
	if err := r.validateIngestPublication(ctx, *media); err != nil {
		return err
	}
	for _, file := range ingestedFiles(*media) {
		if file.Target != target {
			return ErrSource
		}
		if err := r.engine.ValidateRetainedFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) acknowledgeUploadIngest(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion, request uploadIngestRequest, operation string) error {
	_, origin, err := r.uploadIngestIntent(raw, captured, target, request, operation)
	if err != nil {
		return err
	}
	_, media, err := r.ingestedAsset(raw)
	if err != nil || media == nil {
		return ErrSource
	}
	if err := r.engine.AcknowledgeRetainedFiles(ctx, operation, ingestedFiles(*media), accepted, func(selected []byte) ([]records.RetainedFile, error) {
		_, actual, err := r.ingestedAsset(selected)
		if err != nil || actual == nil || actual.Request != request {
			return nil, ErrSource
		}
		return ingestedFiles(*actual), nil
	}); err != nil {
		return err
	}
	return r.journal.DeleteStagedFile(r.uploadIngestOwner(), uploadIngestStagingNamespace, origin.Source)
}

func (r *Repository) validateIngestPublication(ctx context.Context, wanted ingestedMedia) error {
	raw, err := r.engine.ReadRaw(ctx, "asset", wanted.Request.ID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer clear(raw)
	asset, current, err := r.ingestedAsset(raw)
	if err != nil {
		return err
	}
	if current == nil || asset.DeletedAt != nil || current.Request != wanted.Request {
		return ErrConflict
	}
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(wanted)
	defer clear(a)
	defer clear(b)
	if !bytes.Equal(a, b) {
		return ErrConflict
	}
	return nil
}

func (r *Repository) uploadIngestIntent(raw, captured []byte, target storage.PublicationTarget, request uploadIngestRequest, operation string) (model.AssetRow, uploadIngestOrigin, error) {
	var asset model.AssetRow
	var origin uploadIngestOrigin
	input, _ := json.Marshal(request)
	defer clear(input)
	if json.Unmarshal(raw, &asset) != nil || json.Unmarshal(captured, &origin) != nil || !request.valid() || operation != uploadIngestOperation+"_"+planBytesHash(input) || origin.Version != 1 || origin.Scope != r.scope || origin.Target != target || origin.Request != request || origin.Operation != operation || origin.Source.SHA256 != request.SHA256 || origin.Source.SizeBytes != request.Size {
		return asset, origin, ErrSource
	}
	if asset.ID != request.ID || asset.Tenant != r.scope.Tenant || asset.Owner != r.scope.Subject || asset.Name != request.Name || asset.Bytes != request.Size || asset.DriveID != "private:"+request.ID || asset.DeletedAt != nil {
		return asset, origin, ErrSource
	}
	return asset, origin, nil
}

func (r *Repository) preparePhotoUpload(ctx context.Context, raw []byte, asset model.AssetRow, origin uploadIngestOrigin) ([]byte, error) {
	descriptor := records.SnapshotRow{Kind: "original", ID: asset.ID, Size: origin.Request.Size, SHA256: origin.Request.SHA256}
	original, err := r.engine.StageRetainedFile(ctx, origin.Operation, origin.Target, descriptor, func() (io.ReadCloser, error) {
		return storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error {
			reader, err := r.journal.OpenStagedFile(ctx, r.uploadIngestOwner(), uploadIngestStagingNamespace, origin.Source)
			if err != nil {
				return err
			}
			defer func() { _ = reader.Close() }()
			_, err = io.Copy(out, io.NewSectionReader(reader, 0, origin.Source.SizeBytes))
			return err
		}), nil
	})
	if err != nil {
		return nil, err
	}
	fingerprint, err := model.RecetaVacia().Huella()
	if err != nil {
		return nil, err
	}
	set := recipeRenditions{Version: 1, Asset: asset.ID, Fingerprint: fingerprint, Files: map[model.Variante]records.RetainedFile{}}
	sides, variants := renditionSides()
	err = photorender.Derivar(ctx, func(ctx context.Context) ([]byte, error) { return r.uploadIngestOriginal(ctx, origin.Source) }, model.OrientacionDe(asset.Orientation), model.RecetaVacia(), asset.MediaType == "image/png", sides, func(side int, format, _ string, encoded []byte) error {
		variant := variants[side]
		file, err := r.stageRendition(ctx, origin.Operation, origin.Target, asset.ID, fingerprint, variant, format, encoded)
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
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrSource
	}
	fields[ingestedMediaField], _ = json.Marshal(ingestedMedia{Version: 1, Request: origin.Request, Original: original, Renditions: set})
	return json.Marshal(fields)
}

func (r *Repository) uploadIngestOriginal(ctx context.Context, source offline.StagedFile) ([]byte, error) {
	reader, err := r.journal.OpenStagedFile(ctx, r.uploadIngestOwner(), uploadIngestStagingNamespace, source)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(io.LimitReader(io.NewSectionReader(reader, 0, source.SizeBytes), photorender.MaxOriginal+1))
	if err == nil && (int64(len(raw)) != source.SizeBytes || planBytesHash(raw) != source.SHA256) {
		err = ErrSource
	}
	return raw, err
}

func (r *Repository) validIngestedOriginalFile(file records.RetainedFile, asset model.AssetRow, sha256Hex string) bool {
	return file.Version == 1 && file.Scope == r.scope && file.Target.Valid() && file.Link.Valid() && file.Link.RoomID == file.Target.RoomID && storage.Identifier(file.Revision) && file.Row.Kind == "original" && file.Row.ID == asset.ID && file.Row.Size == asset.Bytes && file.Row.SHA256 == sha256Hex && file.Row.Validate() == nil
}

func (r *Repository) ingestedAsset(raw []byte) (model.AssetRow, *ingestedMedia, error) {
	var asset model.AssetRow
	if json.Unmarshal(raw, &asset) != nil {
		return asset, nil, ErrSource
	}
	if len(asset.PrivateMedia) == 0 {
		return asset, nil, nil
	} //nolint:nilnil // Historical indexed originals have no inline private file set.
	if asset.Tenant != r.scope.Tenant || asset.Owner != r.scope.Subject {
		return asset, nil, ErrSource
	}
	var media ingestedMedia
	if json.Unmarshal(asset.PrivateMedia, &media) != nil || media.Version != 1 || !media.Request.valid() || media.Request.ID != asset.ID || media.Request.Name != asset.Name || media.Request.Size != asset.Bytes || asset.DriveID != "private:"+asset.ID {
		return asset, nil, ErrSource
	}
	if !r.validIngestedOriginalFile(media.Original, asset, media.Request.SHA256) {
		return asset, nil, ErrSource
	}
	fingerprint, _ := model.RecetaVacia().Huella()
	encoded, err := json.Marshal(media.Renditions)
	if err != nil {
		return asset, nil, err
	}
	defer clear(encoded)
	if _, err := r.decodeRenditions(encoded, asset.ID, fingerprint); err != nil {
		return asset, nil, err
	}
	return asset, &media, nil
}

func ingestedFiles(media ingestedMedia) []records.RetainedFile {
	return append([]records.RetainedFile{media.Original}, renditionFiles(media.Renditions)...)
}

func (r *Repository) validateIngestedAssets(ctx context.Context, catalog records.Catalog, rows []model.Row) ([]model.MediaRow, error) {
	retained, ok := catalog.(records.RetainedFileCatalog)
	linked := []model.MediaRow{}
	for _, row := range rows {
		if row.Kind != "asset" {
			continue
		}
		asset, media, err := r.ingestedAsset(row.JSON)
		if err != nil {
			return nil, err
		}
		if media == nil {
			continue
		}
		if !ok {
			return nil, ErrSource
		}
		for _, file := range ingestedFiles(*media) {
			if err := retained.ValidateRetainedFile(ctx, file); err != nil {
				return nil, err
			}
		}
		linked = append(linked, model.MediaRow{ID: asset.ID, Tenant: r.scope.Tenant, Owner: r.scope.Subject, AssetID: asset.ID, Role: "original", Size: asset.Bytes, SHA256: media.Original.Row.SHA256, ContentType: asset.MediaType})
	}
	return linked, nil
}

func (r *Repository) initialRendition(ctx context.Context, asset, variant, fingerprint string) (records.RetainedFile, error) {
	empty, _ := model.RecetaVacia().Huella()
	if fingerprint != "" && fingerprint != empty {
		return records.RetainedFile{}, ErrNotFound
	}
	raw, err := r.engine.ReadRaw(ctx, "asset", asset)
	if err != nil {
		return records.RetainedFile{}, err
	}
	defer clear(raw)
	_, media, err := r.ingestedAsset(raw)
	if err != nil {
		return records.RetainedFile{}, err
	}
	if media == nil {
		return records.RetainedFile{}, ErrNotFound
	}
	file, ok := media.Renditions.Files[model.Variante(variant)]
	if !ok {
		return records.RetainedFile{}, model.ErrInvalidInput
	}
	return file, nil
}
