// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// # Vídeos COPIADOS a la fototeca (FO-09)
//
// Un vídeo de una carpeta del escritorio se enlaza (externo.go). El que no tiene una ruta
// estable —el del carrete del teléfono, que llega a un temporal— y el que la persona
// añade con «Añadir fotos» o soltándolo en la ventana se COPIAN, como una foto importada
// (ImportarFoto): su original pasa a ser un fichero retenido del almacén, en el mismo
// `private_media` de las fotos, así que la papelera, borrar para siempre (y soltar el
// original), exportar, la huella, las repetidas y reabrir lo tratan igual. Lo que cambia
// frente a una foto:
//
//   - El original NO se descodifica nunca: se guarda tal cual y puede pasar de
//     photorender.MaxOriginal (hasta storage.MaxFileBytes, tamanoMaximo).
//   - La fila se mide con lo que se dice del vídeo (OrigenVideo), como en uno enlazado
//     (filaDeVideo), y las derivadas salen de su FOTOGRAMA (el póster) que da quien
//     importa; sin él, un póster neutro.
//   - Un vídeo no se edita (recipe_content.go).
//
// Todo es aditivo: la edición conectada nunca llama a ImportarVideo, e ImportarFoto sigue
// rechazando lo que no es una foto o pasa de MaxOriginal.

const (
	videoIngestOperation        = "photo-video-ingest-v1"
	videoIngestStagingNamespace = "photos-private-video-v1"
)

// OrigenVideo es lo que se sabe de un vídeo al copiarlo a la fototeca.
type OrigenVideo struct {
	// Nombre es el que enseña la galería («VID_0001.mp4»).
	Nombre string
	// Tipo es el MIME del vídeo («video/mp4»…); tiene que ser de vídeo.
	Tipo string
	// Datos son sus bytes, exactamente Tamano.
	Datos  io.Reader
	Tamano int64
	// Poster es un fotograma (JPEG, PNG… lo que mida la ingesta) del que salen la
	// miniatura y la previa; nil, un póster neutro.
	Poster []byte
	// Duracion, Ancho y Alto son los del vídeo; sin lados, los del póster.
	Duracion    time.Duration
	Ancho, Alto int
	// TomadoEn es cuándo se grabó, si se sabe.
	TomadoEn *time.Time
}

// videoIngestOrigin es lo que Capture deja en el diario: el vídeo y su póster ya
// guardados en el diario sellado, para terminar tras un corte sin la fuente.
type videoIngestOrigin struct {
	Version   int                       `json:"v"`
	Scope     storage.Scope             `json:"scope"`
	Target    storage.PublicationTarget `json:"target"`
	Request   uploadIngestRequest       `json:"request"`
	Source    offline.StagedFile        `json:"source"`
	Poster    offline.StagedFile        `json:"poster"`
	Operation string                    `json:"operation"`
}

// ImportarVideo COPIA un vídeo a la fototeca: guarda sus bytes como original retenido y
// sus derivadas, sacadas del póster. job es el UUID del vídeo nuevo, como en ImportarFoto.
func (r *Repository) ImportarVideo(parent context.Context, job string, o OrigenVideo) (api.Photo, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return api.Photo{}, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return api.Photo{}, err
	}
	parsed, err := uuid.Parse(job)
	nombre := strings.TrimSpace(o.Nombre)
	tipo := strings.ToLower(strings.TrimSpace(strings.Split(o.Tipo, ";")[0]))
	if err != nil || parsed == uuid.Nil || parsed.String() != job || o.Datos == nil || !storage.ValidDriveName(nombre) || !esVideo(tipo) ||
		o.Tamano <= 0 || o.Tamano > storage.MaxFileBytes {
		return api.Photo{}, model.ErrInvalidInput
	}
	asset := r.filaDeVideo(job, nombre, tipo, o.Tamano, o.Poster, o.Duracion, o.Ancho, o.Alto, o.TomadoEn)
	poster := o.Poster
	if _, _, err := miniatura.Cabecera(poster); len(poster) == 0 || int64(len(poster)) > photorender.MaxOriginal || err != nil {
		poster = posterNeutro(asset.Width, asset.Height)
	}
	owner := r.uploadIngestOwner()
	retained := false
	stagedPoster, err := r.journal.StageFile(ctx, owner, videoIngestStagingNamespace, bytes.NewReader(poster), int64(len(poster)))
	if err != nil {
		return api.Photo{}, err
	}
	defer func() {
		if !retained {
			_ = r.journal.DeleteStagedFile(owner, videoIngestStagingNamespace, stagedPoster)
		}
	}()
	staged, err := r.journal.StageFile(ctx, owner, videoIngestStagingNamespace, contextoLector{ctx, o.Datos}, o.Tamano)
	if err != nil {
		return api.Photo{}, err
	}
	defer func() {
		if !retained {
			_ = r.journal.DeleteStagedFile(owner, videoIngestStagingNamespace, staged)
		}
	}()
	request := uploadIngestRequest{ID: job, Name: nombre, ContentType: tipo, Size: o.Tamano, SHA256: staged.SHA256}
	if !request.valid() {
		return api.Photo{}, model.ErrInvalidInput
	}
	origin := videoIngestOrigin{Version: 1, Scope: r.scope, Request: request, Source: staged, Poster: stagedPoster}
	return r.finishVideoUpload(ctx, origin, &asset, func() { retained = true })
}

// finishVideoUpload publica la fila con su original y sus derivadas. measured es la fila
// medida (nil al reanudar: el diario ya la tiene).
func (r *Repository) finishVideoUpload(ctx context.Context, origin videoIngestOrigin, measured *model.AssetRow, retain func()) (api.Photo, error) {
	request := origin.Request
	input, _ := json.Marshal(request)
	defer clear(input)
	origin.Operation = videoIngestOperation + "_" + planBytesHash(input)
	result, err := r.engine.MutateWithContent(ctx, videoIngestOperation, request.ID, func(ctx context.Context) (string, string, string, []byte, error) {
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
		if measured == nil {
			return "", "", "", nil, ErrSource
		}
		encoded, err := json.Marshal(measured)
		return "asset", request.ID, "live", encoded, err
	}, r.videoIngestContent(origin, retain))
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

func (r *Repository) videoIngestContent(want videoIngestOrigin, retain func()) records.MutationContent {
	request, operation := want.Request, want.Operation
	return records.MutationContent{
		Capture: func(_ context.Context, raw []byte, target storage.PublicationTarget) ([]byte, error) {
			var asset model.AssetRow
			if json.Unmarshal(raw, &asset) != nil || asset.ID != request.ID || asset.Bytes != request.Size ||
				want.Source.SizeBytes != request.Size || want.Source.SHA256 != request.SHA256 {
				return nil, ErrSource
			}
			origin := want
			origin.Target = target
			if retain != nil {
				retain()
			}
			return json.Marshal(origin)
		},
		Prepare: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) ([]byte, error) {
			asset, origin, err := r.videoIngestIntent(raw, captured, target, request, operation)
			if err != nil {
				return nil, err
			}
			return r.prepareVideoUpload(ctx, raw, asset, origin)
		},
		Validate: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) error {
			return r.validateVideoIngest(ctx, raw, captured, target, request, operation)
		},
		Acknowledge: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion) error {
			return r.acknowledgeVideoIngest(ctx, raw, captured, target, accepted, request, operation)
		},
	}
}

// validateVideoIngest comprueba antes de publicar que la fila y sus ficheros son los de
// este vídeo, en target.
func (r *Repository) validateVideoIngest(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, request uploadIngestRequest, operation string) error {
	if _, _, err := r.videoIngestIntent(raw, captured, target, request, operation); err != nil {
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

// acknowledgeVideoIngest da por publicados los ficheros del vídeo y suelta las copias
// que se guardaron para subirlo (el vídeo y su póster).
func (r *Repository) acknowledgeVideoIngest(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion, request uploadIngestRequest, operation string) error {
	_, origin, err := r.videoIngestIntent(raw, captured, target, request, operation)
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
	owner := r.uploadIngestOwner()
	return errors.Join(r.journal.DeleteStagedFile(owner, videoIngestStagingNamespace, origin.Source),
		r.journal.DeleteStagedFile(owner, videoIngestStagingNamespace, origin.Poster))
}

// videoIngestIntent comprueba que la fila y lo capturado son los de este vídeo.
func (r *Repository) videoIngestIntent(raw, captured []byte, target storage.PublicationTarget, request uploadIngestRequest, operation string) (model.AssetRow, videoIngestOrigin, error) {
	var asset model.AssetRow
	var origin videoIngestOrigin
	if json.Unmarshal(raw, &asset) != nil || json.Unmarshal(captured, &origin) != nil ||
		!r.validVideoOrigin(origin, target, request, operation) || !r.validVideoAsset(asset, request) {
		return asset, origin, ErrSource
	}
	return asset, origin, nil
}

// validVideoOrigin dice si lo capturado es el origen de este vídeo, en target y con su
// operación, con la copia del vídeo y un póster de tamaño posible.
func (r *Repository) validVideoOrigin(origin videoIngestOrigin, target storage.PublicationTarget, request uploadIngestRequest, operation string) bool {
	input, _ := json.Marshal(request)
	defer clear(input)
	if !request.valid() || !esVideo(request.ContentType) || operation != videoIngestOperation+"_"+planBytesHash(input) {
		return false
	}
	return origin.Version == 1 && origin.Scope == r.scope && origin.Target == target && origin.Request == request && origin.Operation == operation &&
		origin.Source.SHA256 == request.SHA256 && origin.Source.SizeBytes == request.Size && origin.Poster.SizeBytes > 0 && origin.Poster.SizeBytes <= photorender.MaxOriginal
}

// validVideoAsset dice si la fila es la que crea este vídeo: de esta persona, viva y
// sin original externo.
func (r *Repository) validVideoAsset(asset model.AssetRow, request uploadIngestRequest) bool {
	return asset.ID == request.ID && asset.Tenant == r.scope.Tenant && asset.Owner == r.scope.Subject && asset.Name == request.Name && asset.Bytes == request.Size &&
		asset.MediaType == request.ContentType && asset.DriveID == "private:"+request.ID && asset.DeletedAt == nil && len(asset.PrivateExternal) == 0
}

// prepareVideoUpload guarda el vídeo tal cual como original retenido y las derivadas de
// su póster.
func (r *Repository) prepareVideoUpload(ctx context.Context, raw []byte, asset model.AssetRow, origin videoIngestOrigin) ([]byte, error) {
	owner := r.uploadIngestOwner()
	descriptor := records.SnapshotRow{Kind: "original", ID: asset.ID, Size: origin.Request.Size, SHA256: origin.Request.SHA256}
	original, err := r.engine.StageRetainedFile(ctx, origin.Operation, origin.Target, descriptor, func() (io.ReadCloser, error) {
		return storage.PipeRead(ctx, nil, func(ctx context.Context, out io.Writer) error {
			reader, err := r.journal.OpenStagedFile(ctx, owner, videoIngestStagingNamespace, origin.Source)
			if err != nil {
				return err
			}
			defer func() { _ = reader.Close() }()
			_, err = io.Copy(out, contextoLector{ctx, io.NewSectionReader(reader, 0, origin.Source.SizeBytes)})
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
	load := func(ctx context.Context) ([]byte, error) {
		reader, err := r.journal.OpenStagedFile(ctx, owner, videoIngestStagingNamespace, origin.Poster)
		if err != nil {
			return nil, err
		}
		defer func() { _ = reader.Close() }()
		return io.ReadAll(io.LimitReader(io.NewSectionReader(reader, 0, origin.Poster.SizeBytes), photorender.MaxOriginal+1))
	}
	set := recipeRenditions{Version: 1, Asset: asset.ID, Fingerprint: fingerprint, Files: map[model.Variante]records.RetainedFile{}}
	sides, variants := renditionSides()
	err = photorender.Derivar(ctx, load, model.OrientacionDe(asset.Orientation), model.RecetaVacia(), false, sides, func(side int, format, _ string, encoded []byte) error {
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

// reanudarVideos termina las copias de vídeos que un corte dejó a medias
// (ReanudarImportaciones).
func (r *Repository) reanudarVideos(ctx context.Context) ([]api.Photo, []error) {
	completed := []api.Photo{}
	var failures []error
	for cursor := ""; ; {
		page, err := r.engine.PendingContents(ctx, videoIngestOperation, "asset", cursor, 8)
		if err != nil {
			return completed, append(failures, err)
		}
		for _, pending := range page.Items {
			var origin videoIngestOrigin
			err := json.Unmarshal(pending.Captured, &origin)
			if err == nil && pending.EntityID != origin.Request.ID {
				err = ErrSource
			}
			if err == nil {
				if _, _, err = r.videoIngestIntent(pending.JSON, pending.Captured, pending.Target, origin.Request, origin.Operation); err == nil {
					origin.Operation, origin.Target = "", storage.PublicationTarget{}
					var photo api.Photo
					if photo, err = r.finishVideoUpload(ctx, origin, nil, nil); err == nil {
						completed = append(completed, photo)
					}
				}
			}
			clear(pending.JSON)
			clear(pending.Captured)
			if err != nil {
				failures = append(failures, err)
			}
		}
		if page.Next == "" {
			return completed, failures
		}
		cursor = page.Next
	}
}

// HuellaOriginal es la huella (SHA-256 en hexadecimal, en minúsculas) del original de una
// foto o un vídeo, venga de donde venga: retenido en el almacén, enlazado o el que se
// apuntó al purgarla. Vacía si no la tiene (una foto histórica de Drive).
func (r *Repository) HuellaOriginal(parent context.Context, id string) (string, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return "", err
	}
	defer done()
	raw, err := r.engine.ReadRaw(ctx, "asset", id)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	asset, huella, err := r.huellaDeFila(raw)
	if err != nil {
		return "", err
	}
	if asset.Owner != r.scope.Subject {
		return "", ErrNotFound
	}
	return huella, nil
}
