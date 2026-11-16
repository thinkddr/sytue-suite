// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const (
	uploadIngestOperation        = "photo-ingest-v1"
	uploadIngestStagingNamespace = "photos-private-stage-v1"
	ingestedMediaField           = "private_media"
)

var errIngestAccepted = errors.New("photos: this exact import is already committed")

type uploadIngestRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type uploadIngestOrigin struct {
	Version   int                       `json:"v"`
	Scope     storage.Scope             `json:"scope"`
	Target    storage.PublicationTarget `json:"target"`
	Request   uploadIngestRequest       `json:"request"`
	Source    offline.StagedFile        `json:"source"`
	Operation string                    `json:"operation"`
}

func (r *Repository) uploadIngestOwner() string {
	return r.scope.Tenant + "\x00" + r.scope.Subject + "\x00photos"
}

func (request uploadIngestRequest) valid() bool {
	id, err := uuid.Parse(request.ID)
	return err == nil && id != uuid.Nil && id.String() == request.ID && storage.ValidDriveName(request.Name) && request.Size > 0 && request.Size <= photorender.MaxOriginal && len(request.ContentType) <= 1024 && !strings.ContainsAny(request.ContentType, "\r\n\x00") && (records.SnapshotRow{Kind: "original", ID: request.ID, Size: request.Size, SHA256: request.SHA256}).Validate() == nil
}

// ImportarFoto first captures a non-seekable input in the existing encrypted
// staging store. Its exact descriptor, logical UUID and original publication
// target enter the content WAL before any network upload can begin.
func (r *Repository) ImportarFoto(parent context.Context, job, name, contentType string, input io.Reader, size int64) (api.Photo, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return api.Photo{}, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return api.Photo{}, err
	}
	parsed, err := uuid.Parse(job)
	if err != nil || parsed == uuid.Nil || parsed.String() != job || input == nil || !storage.ValidDriveName(name) || size <= 0 || size > photorender.MaxOriginal {
		return api.Photo{}, model.ErrInvalidInput
	}
	staged, err := r.journal.StageFile(ctx, r.uploadIngestOwner(), uploadIngestStagingNamespace, input, size)
	if err != nil {
		return api.Photo{}, err
	}
	retained := false
	defer func() {
		if !retained {
			_ = r.journal.DeleteStagedFile(r.uploadIngestOwner(), uploadIngestStagingNamespace, staged)
		}
	}()
	request := uploadIngestRequest{ID: job, Name: strings.TrimSpace(name), ContentType: contentType, Size: size, SHA256: staged.SHA256}
	if !request.valid() {
		return api.Photo{}, model.ErrInvalidInput
	}
	return r.finishPhotoUpload(ctx, request, staged, func() { retained = true })
}

func (r *Repository) finishPhotoUpload(ctx context.Context, request uploadIngestRequest, source offline.StagedFile, retain func()) (api.Photo, error) {
	input, _ := json.Marshal(request)
	defer clear(input)
	operation := uploadIngestOperation + "_" + planBytesHash(input)
	result, err := r.engine.MutateWithContent(ctx, uploadIngestOperation, request.ID, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "asset", request.ID)
		if err == nil {
			defer clear(raw)
			_, media, decodeErr := r.ingestedAsset(raw)
			if decodeErr != nil || media == nil || media.Request != request {
				return "", "", "", nil, ErrConflict
			}
			return "", "", "", nil, errIngestAccepted
		}
		if !errors.Is(err, ErrNotFound) {
			return "", "", "", nil, err
		}
		asset, err := r.measurePhotoUpload(ctx, request, source)
		if err != nil {
			return "", "", "", nil, err
		}
		encoded, err := json.Marshal(asset)
		return "asset", request.ID, "live", encoded, err
	}, r.uploadIngestContent(request, source, operation, retain))
	defer clear(result)
	if err != nil && ctx.Err() != nil {
		return api.Photo{}, ctx.Err()
	}
	if errors.Is(err, errIngestAccepted) {
		return r.Photo(ctx, request.ID)
	}
	if err != nil {
		return api.Photo{}, err
	}
	var asset model.AssetRow
	if json.Unmarshal(result, &asset) != nil {
		return api.Photo{}, ErrSource
	}
	return photoView(asset), nil
}

func (r *Repository) measurePhotoUpload(ctx context.Context, request uploadIngestRequest, source offline.StagedFile) (model.AssetRow, error) {
	reader, err := r.journal.OpenStagedFile(ctx, r.uploadIngestOwner(), uploadIngestStagingNamespace, source)
	if err != nil {
		return model.AssetRow{}, err
	}
	defer func() { _ = reader.Close() }()
	header, err := io.ReadAll(io.LimitReader(io.NewSectionReader(reader, 0, source.SizeBytes), exif.BytesDeCabecera))
	defer clear(header)
	if err != nil {
		return model.AssetRow{}, err
	}
	configuration, format, err := image.DecodeConfig(bytes.NewReader(header))
	if err != nil || !miniatura.Cabe(configuration) {
		return model.AssetRow{}, model.ErrInvalidInput
	}
	contentType := "image/" + format
	if format != "jpeg" && format != "png" && format != "gif" {
		return model.AssetRow{}, model.ErrInvalidInput
	}
	if request.ContentType != "" && !strings.EqualFold(strings.Split(request.ContentType, ";")[0], contentType) {
		return model.AssetRow{}, model.ErrInvalidInput
	}
	now := time.Now().UTC()
	asset := model.AssetRow{ID: request.ID, Tenant: r.scope.Tenant, Owner: r.scope.Subject, DriveID: "private:" + request.ID, Name: request.Name, MediaType: contentType, Bytes: request.Size, Width: configuration.Width, Height: configuration.Height, Orientation: 1, UploadedAt: now, CreatedAt: now, UpdatedAt: now, Tags: []string{}}
	if metadata, err := exif.Leer(header); err == nil {
		asset.Orientation = int(model.OrientacionDe(int(metadata.Orientacion)))
		asset.TakenAt, asset.Camera, asset.Lens, asset.Latitude, asset.Longitude = metadata.TomadaEn, metadata.Camara, metadata.Objetivo, metadata.Lat, metadata.Lon
	}
	return asset, nil
}

// ReanudarImportaciones uses only encrypted retained sources and the original
// WAL. A different device/room is an explicit error, never a new upload target.
func (r *Repository) ReanudarImportaciones(parent context.Context) ([]api.Photo, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	completed := []api.Photo{}
	failures := []error{}
	for cursor := ""; ; {
		page, err := r.engine.PendingContents(ctx, uploadIngestOperation, "asset", cursor, 8)
		if err != nil {
			return completed, errors.Join(append(failures, err)...)
		}
		for _, pending := range page.Items {
			photo, err := r.resumePhotoUpload(ctx, pending)
			clear(pending.JSON)
			clear(pending.Captured)
			if err != nil {
				failures = append(failures, err)
			} else {
				completed = append(completed, photo)
			}
		}
		if page.Next == "" {
			break
		}
		cursor = page.Next
	}
	return completed, errors.Join(failures...)
}

func (r *Repository) resumePhotoUpload(ctx context.Context, pending records.PendingContent) (api.Photo, error) {
	var origin uploadIngestOrigin
	if json.Unmarshal(pending.Captured, &origin) != nil || pending.EntityID != origin.Request.ID {
		return api.Photo{}, ErrSource
	}
	if _, _, err := r.uploadIngestIntent(pending.JSON, pending.Captured, pending.Target, origin.Request, origin.Operation); err != nil {
		return api.Photo{}, err
	}
	input, _ := json.Marshal(origin.Request.ID)
	defer clear(input)
	if planBytesHash(input) != pending.InputFingerprint {
		return api.Photo{}, ErrSource
	}
	return r.finishPhotoUpload(ctx, origin.Request, origin.Source, nil)
}

var _ api.PhotosDirectIngestData = (*Repository)(nil)
