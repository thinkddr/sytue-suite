// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"time"

	_ "image/jpeg" // la ingesta mide lo que el render podrá reabrir, nada más
	_ "image/png"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const (
	ingestMediaOperation = "ingest-media-v1"
	ingestAssetOperation = "ingest-asset-v1"
	ingestOriginalField  = "private_original"
	ingestIntentField    = "private_ingest"
)

// ingestRequest is the durable input of both ingest mutations. The drive file
// id alone fixes the WAL key, so every retry of the same photo resumes exactly
// where the interrupted one stopped.
type ingestRequest struct {
	Drive string `json:"drive_file_id"`
}

// ingestFile is the Drive view captured once; retries never re-read Drive.
type ingestFile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	Bytes     int64     `json:"bytes"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ingestOrigin captures the complete ingest intent before the first WAL write.
// It carries the pinned original and the metadata read from its bytes, so the
// media row is the only durable source the asset mutation needs.
type ingestOrigin struct {
	Version   int                       `json:"v"`
	Scope     storage.Scope             `json:"scope"`
	Target    storage.PublicationTarget `json:"target"`
	Operation string                    `json:"operation"`
	Asset     string                    `json:"asset"`
	Drive     ingestFile                `json:"drive"`
	Pin       drive.FilePin             `json:"pin"`
	Exif      exif.Metadatos            `json:"exif"`
	Width     int                       `json:"width"`
	Height    int                       `json:"height"`
	Captured  time.Time                 `json:"captured"`
}

// ingestIntent is the complete asset metadata retained on the media row. The
// deterministic asset mutation rebuilds the asset from it without Drive.
type ingestIntent struct {
	DriveID     string     `json:"drive_file_id"`
	Name        string     `json:"name"`
	MediaType   string     `json:"media_type"`
	Bytes       int64      `json:"bytes"`
	Width       int        `json:"width"`
	Height      int        `json:"height"`
	Orientation int        `json:"orientation"`
	TakenAt     *time.Time `json:"taken_at,omitempty"`
	Camera      string     `json:"camera,omitempty"`
	Lens        string     `json:"lens,omitempty"`
	Latitude    *float64   `json:"latitude,omitempty"`
	Longitude   *float64   `json:"longitude,omitempty"`
	UploadedAt  time.Time  `json:"uploaded_at"`
	Captured    time.Time  `json:"captured"`
}

func (o ingestOrigin) valid(scope storage.Scope, target storage.PublicationTarget) bool {
	if o.Version != 1 || o.Scope != scope || o.Target != target || o.Operation != ingestMediaOperation || o.Asset == "" || o.Captured.IsZero() || o.Width <= 0 || o.Height <= 0 {
		return false
	}
	return o.validDrive(scope)
}

func (o ingestOrigin) validDrive(scope storage.Scope) bool {
	return o.Drive.ID != "" && o.Drive.MediaType != "" && !o.Drive.UpdatedAt.IsZero() && o.Drive.Bytes > 0 && o.Pin.Valid() && o.Pin.SourceKind == "file" && o.Pin.SourceID == o.Drive.ID && o.Pin.SizeBytes == o.Drive.Bytes && o.Pin.Scope.Tenant == scope.Tenant && o.Pin.Scope.Subject == scope.Subject
}

func (o ingestOrigin) intent() ingestIntent {
	return ingestIntent{
		DriveID: o.Drive.ID, Name: o.Drive.Name, MediaType: o.Drive.MediaType, Bytes: o.Drive.Bytes,
		Width: o.Width, Height: o.Height, Orientation: int(o.Exif.Orientacion),
		TakenAt: o.Exif.TomadaEn, Camera: o.Exif.Camara, Lens: o.Exif.Objetivo,
		Latitude: o.Exif.Lat, Longitude: o.Exif.Lon, UploadedAt: o.Drive.UpdatedAt, Captured: o.Captured,
	}
}

func (i ingestIntent) valid() bool {
	return i.DriveID != "" && i.MediaType != "" && i.Bytes > 0 && i.Width > 0 && i.Height > 0 && !i.UploadedAt.IsZero() && !i.Captured.IsZero()
}

func (i ingestIntent) matches(origin ingestOrigin) bool {
	return i.DriveID == origin.Drive.ID && i.Name == origin.Drive.Name && i.MediaType == origin.Drive.MediaType && i.Bytes == origin.Drive.Bytes && i.UploadedAt.Equal(origin.Drive.UpdatedAt) &&
		i.Width == origin.Width && i.Height == origin.Height && i.Orientation == int(origin.Exif.Orientacion) && sameIngestText(i, origin.Exif) && sameIngestPlace(i, origin.Exif)
}

func (i ingestIntent) assetRow(id string, scope storage.Scope) model.AssetRow {
	return model.AssetRow{
		ID: id, Tenant: scope.Tenant, Owner: scope.Subject, DriveID: i.DriveID, Name: i.Name,
		MediaType: i.MediaType, Bytes: i.Bytes, Width: i.Width, Height: i.Height,
		Orientation: i.Orientation, TakenAt: i.TakenAt, UploadedAt: i.UploadedAt,
		Camera: i.Camera, Lens: i.Lens, Latitude: i.Latitude, Longitude: i.Longitude,
		CreatedAt: i.Captured, UpdatedAt: i.Captured,
	}
}

// Indexar retains a Drive file as a new photo on this verified device. The
// original never leaves the encrypted storage: the pinned Drive bytes are
// copied once, and the media row commits before the asset row, so an
// interrupted import stays invisible to the timeline until the deterministic
// asset mutation completes it.
func (r *Repository) Indexar(parent context.Context, file string) (api.Photo, error) {
	if file == "" || len(file) > 4096 {
		return api.Photo{}, model.ErrInvalidInput
	}
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return api.Photo{}, err
	}
	defer done()
	assets, err := r.assets(ctx)
	if err != nil {
		return api.Photo{}, err
	}
	asset := uuid.NewSHA1(uuid.NameSpaceOID, []byte(r.scope.Tenant+"\x00"+r.scope.Subject+"\x00"+file)).String()
	for _, existing := range assets {
		if existing.DriveID == file {
			if existing.ID == asset {
				if err := r.ingestAssetRow(ctx, file, asset); err != nil && !errors.Is(err, ErrConflict) {
					return api.Photo{}, err
				}
			}
			return photoView(existing), nil
		}
	}
	// La fase media se ACCIONA siempre, no se salta por la fila existente: con
	// la publicación a medias el motor reanuda su WAL y lo retira, y ErrConflict
	// es la mutación ya comprometida por completo (la fila existe y no hay WAL).
	if err := r.ingestMedia(ctx, file, asset); err != nil && !errors.Is(err, ErrConflict) {
		if ctx.Err() != nil {
			return api.Photo{}, ctx.Err()
		}
		return api.Photo{}, err
	}
	if err := r.ingestAssetRow(ctx, file, asset); err != nil && !errors.Is(err, ErrConflict) {
		if ctx.Err() != nil {
			return api.Photo{}, ctx.Err()
		}
		return api.Photo{}, err
	}
	row, err := r.asset(ctx, asset)
	if err != nil {
		return api.Photo{}, err
	}
	return photoView(row), nil
}

// ingestMedia commits the media row with its retained original first. The
// staged file and the row share one mutation, so the link is never advertised
// without its bytes and the bytes are never re-uploaded after acceptance.
func (r *Repository) ingestMedia(ctx context.Context, file, asset string) error {
	var origin *ingestOrigin
	_, err := r.engine.MutateWithContent(ctx, ingestMediaOperation, ingestRequest{Drive: file}, func(ctx context.Context) (string, string, string, []byte, error) {
		if _, err := r.engine.ReadRaw(ctx, "media", asset); err == nil {
			return "", "", "", nil, ErrConflict
		} else if !errors.Is(err, ErrNotFound) {
			return "", "", "", nil, err
		}
		captured, err := r.captureIngest(ctx, file, asset)
		if err != nil {
			return "", "", "", nil, err
		}
		origin = &captured
		raw, err := json.Marshal(model.MediaRow{ID: asset, Tenant: r.scope.Tenant, Owner: r.scope.Subject, AssetID: asset, Role: "original", Size: captured.Pin.SizeBytes, SHA256: captured.Pin.SHA256, ContentType: captured.Drive.MediaType})
		if err != nil {
			return "", "", "", nil, err
		}
		return "media", asset, "live", raw, nil
	}, records.MutationContent{
		Capture: func(ctx context.Context, raw []byte, target storage.PublicationTarget) ([]byte, error) {
			if origin == nil || !target.Valid() {
				return nil, ErrSource
			}
			bound := *origin
			bound.Target = target
			if !bound.valid(r.scope, target) {
				return nil, ErrSource
			}
			return json.Marshal(bound)
		},
		Prepare:  r.prepareIngestMedia,
		Validate: r.validateIngestMedia,
		Acknowledge: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion) error {
			_, _, file, _, err := r.ingestPair(raw, captured, target)
			if err != nil {
				return err
			}
			return r.engine.AcknowledgeRetainedFiles(ctx, ingestMediaOperation, []records.RetainedFile{file}, accepted, func(raw []byte) ([]records.RetainedFile, error) {
				_, _, file, err := ingestedOriginal(raw, r.scope, asset)
				if err != nil {
					return nil, err
				}
				return []records.RetainedFile{file}, nil
			})
		},
	})
	return err
}

func (r *Repository) captureIngest(ctx context.Context, file, asset string) (ingestOrigin, error) {
	if !r.scope.Valid() || r.scope.Product != "photos" || asset == "" {
		return ingestOrigin{}, ErrSource
	}
	repository, err := r.ingestDrive(ctx)
	if err != nil {
		return ingestOrigin{}, err
	}
	view, err := repository.File(ctx, file)
	if err != nil {
		return ingestOrigin{}, err
	}
	if view.ContentType == "" || view.UpdatedAt.IsZero() || view.SizeBytes <= 0 || view.SizeBytes > photorender.MaxOriginal {
		return ingestOrigin{}, model.ErrInvalidInput
	}
	pin, err := repository.PinRetainedOriginal(ctx, "file", file)
	if err != nil {
		return ingestOrigin{}, err
	}
	if pin.SizeBytes != view.SizeBytes {
		return ingestOrigin{}, ErrSource
	}
	raw, err := r.readPinnedOriginal(ctx, repository, pin)
	if err != nil {
		return ingestOrigin{}, err
	}
	defer clear(raw)
	metadata, exifErr := exif.Leer(raw)
	if exifErr != nil && !errors.Is(exifErr, exif.ErrSinExif) {
		return ingestOrigin{}, exifErr
	}
	width, height, err := ingestDimensions(raw)
	if err != nil {
		return ingestOrigin{}, err
	}
	return ingestOrigin{
		Version: 1, Scope: r.scope, Operation: ingestMediaOperation, Asset: asset,
		Drive: ingestFile{ID: view.ID, Name: view.Name, MediaType: view.ContentType, Bytes: view.SizeBytes, UpdatedAt: view.UpdatedAt},
		Pin:   pin, Exif: metadata, Width: width, Height: height, Captured: time.Now().UTC(),
	}, nil
}

func (r *Repository) prepareIngestMedia(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) ([]byte, error) {
	var origin ingestOrigin
	var current model.MediaRow
	if json.Unmarshal(captured, &origin) != nil || !origin.valid(r.scope, target) || json.Unmarshal(raw, &current) != nil {
		return nil, ErrSource
	}
	wanted := model.MediaRow{ID: origin.Asset, Tenant: r.scope.Tenant, Owner: r.scope.Subject, AssetID: origin.Asset, Role: "original", Size: origin.Pin.SizeBytes, SHA256: origin.Pin.SHA256, ContentType: origin.Drive.MediaType}
	if current != wanted {
		return nil, ErrSource
	}
	file, err := r.engine.StageRetainedFile(ctx, ingestMediaOperation, target, records.SnapshotRow{Kind: "original", ID: origin.Asset, Size: origin.Pin.SizeBytes, SHA256: origin.Pin.SHA256}, func() (io.ReadCloser, error) {
		repository, err := r.ingestDrive(ctx)
		if err != nil {
			return nil, err
		}
		body, err := r.readPinnedOriginal(ctx, repository, origin.Pin)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	})
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return nil, ErrSource
	}
	base, err := json.Marshal(model.MediaRow{ID: origin.Asset, Tenant: r.scope.Tenant, Owner: r.scope.Subject, AssetID: origin.Asset, Role: "original", Size: file.Row.Size, SHA256: file.Row.SHA256, ContentType: origin.Drive.MediaType})
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(base, &fields) != nil {
		return nil, ErrSource
	}
	if fields[ingestOriginalField], err = json.Marshal(file); err != nil {
		return nil, err
	}
	if fields[ingestIntentField], err = json.Marshal(origin.intent()); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func (r *Repository) validateIngestMedia(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) error {
	_, _, file, _, err := r.ingestPair(raw, captured, target)
	if err != nil {
		return err
	}
	return r.engine.ValidateRetainedFile(ctx, file)
}

// ingestPair decodes a prepared ingest mutation end to end: the captured
// origin, the final media row, its intent and its retained original link.
//
//nolint:unparam // ingestPair decodes the full mutation tuple; each caller uses the subset it needs
func (r *Repository) ingestPair(raw, captured []byte, target storage.PublicationTarget) (model.MediaRow, ingestIntent, records.RetainedFile, ingestOrigin, error) {
	var origin ingestOrigin
	if json.Unmarshal(captured, &origin) != nil || !origin.valid(r.scope, target) {
		return model.MediaRow{}, ingestIntent{}, records.RetainedFile{}, ingestOrigin{}, ErrSource
	}
	row, intent, file, err := ingestedOriginal(raw, r.scope, origin.Asset)
	if err != nil {
		return model.MediaRow{}, ingestIntent{}, records.RetainedFile{}, ingestOrigin{}, err
	}
	if !intent.matches(origin) {
		return model.MediaRow{}, ingestIntent{}, records.RetainedFile{}, ingestOrigin{}, ErrSource
	}
	if row.Size != origin.Pin.SizeBytes || row.SHA256 != origin.Pin.SHA256 || row.ContentType != origin.Drive.MediaType || file.Row.Size != origin.Pin.SizeBytes || file.Row.SHA256 != origin.Pin.SHA256 || file.Scope != r.scope || file.Target != target {
		return model.MediaRow{}, ingestIntent{}, records.RetainedFile{}, ingestOrigin{}, ErrSource
	}
	return row, intent, file, origin, nil
}

// ingestedOriginal decodes a published ingest media row: its descriptor, the
// retained original link and the asset intent it still owes.
func ingestedOriginal(raw []byte, scope storage.Scope, asset string) (model.MediaRow, ingestIntent, records.RetainedFile, error) {
	fields := map[string]json.RawMessage{}
	var row model.MediaRow
	if json.Unmarshal(raw, &row) != nil || json.Unmarshal(raw, &fields) != nil || row.ID != asset || row.AssetID != asset || row.Role != "original" || row.Tenant != scope.Tenant || row.Owner != scope.Subject || row.Validate() != nil {
		return row, ingestIntent{}, records.RetainedFile{}, ErrSource
	}
	var intent ingestIntent
	if json.Unmarshal(fields[ingestIntentField], &intent) != nil || !intent.valid() {
		return row, ingestIntent{}, records.RetainedFile{}, ErrSource
	}
	if intent.MediaType != row.ContentType || intent.Bytes != row.Size {
		return row, ingestIntent{}, records.RetainedFile{}, ErrSource
	}
	var file records.RetainedFile
	if json.Unmarshal(fields[ingestOriginalField], &file) != nil || !validDriveIngestFile(file, scope, asset, row) {
		return row, intent, records.RetainedFile{}, ErrSource
	}
	return row, intent, file, nil
}

func validDriveIngestFile(file records.RetainedFile, scope storage.Scope, asset string, row model.MediaRow) bool {
	return file.Version == 1 && file.Scope == scope && file.Target.Valid() && file.Link.Valid() && file.Link.RoomID == file.Target.RoomID && storage.Identifier(file.Revision) && file.Row.Kind == "original" && file.Row.ID == asset && file.Row.Size == row.Size && file.Row.SHA256 == row.SHA256 && file.Row.Validate() == nil
}

// ingestAssetRow commits the visible asset. It reads only the already
// published media row, so a crash before this point leaves a dormant ingest
// that this same deterministic mutation completes on retry.
// ingestMediaRow validates an ingested media row against the retained catalog.
// Without its asset row the ingest is still in progress: the row stays dormant
// —outside the graph and the snapshot— until the deterministic asset mutation
// completes it, so an interrupted import never breaks readiness. An ingested
// row injects its linked descriptor instead of a catalog binary.
func ingestMediaRow(ctx context.Context, catalog records.Catalog, raw []byte, scope storage.Scope, live map[string]bool) (visible, inject bool, file records.RetainedFile, err error) {
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return false, false, records.RetainedFile{}, ErrSource
	}
	if len(fields[ingestOriginalField]) == 0 {
		return true, false, records.RetainedFile{}, nil // fila del import: su binario ya es del catálogo
	}
	var row model.MediaRow
	if json.Unmarshal(raw, &row) != nil || row.Role != "original" || row.ID != row.AssetID {
		return false, false, records.RetainedFile{}, ErrSource
	}
	_, _, linked, err := ingestedOriginal(raw, scope, row.ID)
	if err != nil {
		return false, false, records.RetainedFile{}, err
	}
	retained, ok := catalog.(records.RetainedFileCatalog)
	if !ok {
		return false, false, records.RetainedFile{}, ErrSource
	}
	if err := retained.ValidateRetainedFile(ctx, linked); err != nil {
		return false, false, records.RetainedFile{}, err
	}
	if !live[row.AssetID] {
		return false, false, records.RetainedFile{}, nil // ingesta a medias: dormida
	}
	return true, true, linked, nil
}

// linkedOriginal resolves the retained original an ingested photo linked from
// its media row. A photo with neither a catalog binary nor a link is broken.
func (r *Repository) linkedOriginal(ctx context.Context, asset string) (records.RetainedFile, error) {
	raw, err := r.engine.ReadRaw(ctx, "media", asset)
	if errors.Is(err, ErrNotFound) {
		raw, err = r.engine.ReadRaw(ctx, "asset", asset)
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
		return media.Original, nil
	}
	if err != nil {
		return records.RetainedFile{}, err
	}
	defer clear(raw)
	_, _, file, err := ingestedOriginal(raw, r.scope, asset)
	return file, err
}

func (r *Repository) ingestAssetRow(ctx context.Context, file, asset string) error {
	_, err := r.engine.Mutate(ctx, ingestAssetOperation, ingestRequest{Drive: file}, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "media", asset)
		if err != nil {
			return "", "", "", nil, err
		}
		defer clear(raw)
		_, intent, _, err := ingestedOriginal(raw, r.scope, asset)
		if err != nil {
			return "", "", "", nil, err
		}
		if intent.DriveID != file {
			return "", "", "", nil, ErrSource
		}
		if _, err := r.engine.ReadRaw(ctx, "asset", asset); err == nil {
			return "", "", "", nil, ErrConflict
		} else if !errors.Is(err, ErrNotFound) {
			return "", "", "", nil, err
		}
		result, err := json.Marshal(intent.assetRow(asset, r.scope))
		if err != nil {
			return "", "", "", nil, err
		}
		return "asset", asset, "live", result, nil
	})
	return err
}

// ingestDrive resolves the already authorized Drive repository that retained
// this file. The wiring fixes it: the plan store pins originals through the
// hidden Drive application, and ingesting reuses that exact authority.
func (r *Repository) ingestDrive(ctx context.Context) (*drive.Repository, error) {
	if r.sourcePlans == nil {
		return nil, ErrSource
	}
	pins, ok := r.sourcePlans.cfg.Originals.(driveOriginalPins)
	if !ok {
		return nil, ErrSource
	}
	repository, err := pins.repository(ctx)
	if err != nil {
		return nil, err
	}
	if repository == nil {
		return nil, ErrNotReady
	}
	return repository, nil
}

func (r *Repository) readPinnedOriginal(ctx context.Context, repository *drive.Repository, pin drive.FilePin) ([]byte, error) {
	body := storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error { return repository.WritePinnedOriginal(ctx, pin, out) })
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, photorender.MaxOriginal+1))
	if err == nil && (len(raw) > photorender.MaxOriginal || int64(len(raw)) != pin.SizeBytes) {
		err = model.ErrInvalidInput
	}
	return raw, err
}

// ingestDimensions measures what this device can decode again: the editor and
// the recipes reopen the retained original, so an undecodable file is rejected
// here instead of at edit time.
func ingestDimensions(raw []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, 0, model.ErrInvalidInput
	}
	return config.Width, config.Height, nil
}

func sameIngestText(intent ingestIntent, metadata exif.Metadatos) bool {
	if intent.Camera != metadata.Camara || intent.Lens != metadata.Objetivo {
		return false
	}
	if (intent.TakenAt == nil) != (metadata.TomadaEn == nil) {
		return false
	}
	return intent.TakenAt == nil || intent.TakenAt.Equal(*metadata.TomadaEn)
}

func sameIngestPlace(intent ingestIntent, metadata exif.Metadatos) bool {
	if (intent.Latitude == nil) != (metadata.Lat == nil) || (intent.Longitude == nil) != (metadata.Lon == nil) {
		return false
	}
	if intent.Latitude != nil && (*intent.Latitude != *metadata.Lat || *intent.Longitude != *metadata.Lon) {
		return false
	}
	return true
}
