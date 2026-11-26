// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
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

// # Originales externos y vídeos (PLAN-EDICION-LOCAL §6.8, FO-09)
//
// Hasta aquí todo original de la fototeca era un fichero RETENIDO del almacén, con su
// huella por nombre (ImportarFoto). Sytue Office enseña además lo que ya está en una
// carpeta del equipo SIN COPIARLO: la fila de la foto lleva, en vez de `private_media`,
// un `private_external` con la RUTA del fichero, su tamaño, su fecha y la huella SHA-256
// de sus bytes, más las derivadas (miniatura y previa), que sí se generan y se guardan
// en el almacén como siempre: la galería se ve aunque el fichero falte.
//
// Un vídeo entra igual, como original externo, con su duración y un FOTOGRAMA (el póster)
// del que salen las derivadas: este paquete no sabe descodificar vídeo, así que el
// fotograma lo da quien importa (`cmd-local/photos`, con `apps/fyne/video` o la miniatura
// del sistema). Sin fotograma, un póster neutro.
//
// Lo que cambia y lo que no:
//
//   - Las derivadas, los álbumes, los favoritos, la papelera, la búsqueda: igual.
//   - El original se lee del fichero, y se comprueba su huella: si ya no está o ya no es
//     el mismo, `Original` lo dice (ErrOriginalExterno) y la foto lo enseña
//     (api.Photo.EstadoExterno) sin romper la galería. Recolocar lo vuelve a encontrar por
//     su huella si se ha movido o renombrado.
//   - Editar una foto externa funciona: la receta lee el original del fichero. Un vídeo
//     no se edita.
//   - Todo es aditivo: la edición conectada nunca escribe `private_external`.

const (
	externalIngestOperation        = "photo-external-v1"
	externalIngestStagingNamespace = "photos-private-external-v1"
	externalMediaField             = "private_external"
)

// ErrOriginalExterno: el original externo ya no está en su ruta o ya no es el mismo.
var ErrOriginalExterno = errors.New("photos: el original ya no está donde estaba o ha cambiado")

// OrigenExterno es lo que se sabe de un fichero del equipo al enlazarlo en la fototeca.
type OrigenExterno struct {
	// Ruta es la del fichero, absoluta.
	Ruta string
	// Nombre es el que enseña la galería; vacío, el del fichero.
	Nombre string
	// Tipo es el MIME; en una foto, vacío o el que diga su cabecera; un vídeo lo tiene
	// que decir («video/mp4»…).
	Tipo string
	// Poster es, en un vídeo, un fotograma (JPEG, PNG… lo que mida la ingesta) del que
	// salen la miniatura y la previa; nil, un póster neutro. Se ignora en una foto.
	Poster []byte
	// Duracion, Ancho y Alto son los del vídeo; sin lados, los del póster.
	Duracion    time.Duration
	Ancho, Alto int
	// TomadoEn es cuándo se grabó el vídeo, si se sabe; en una foto, cuándo se hizo si
	// su cabecera no lo dice (la de una copia de Sytue, que lo sabía). nil: no se sabe.
	TomadoEn *time.Time
	// SubidoEn es cuándo se subió a Sytue la foto o el vídeo de una copia de «Tus datos»:
	// la línea de tiempo pone ahí lo que no tiene fecha de captura, como hacía Sytue.
	// nil: ahora, como cualquier importada.
	SubidoEn *time.Time
}

type externalRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

func (q externalRequest) valid() bool {
	id, err := uuid.Parse(q.ID)
	return err == nil && id != uuid.Nil && id.String() == q.ID && storage.ValidDriveName(q.Name) && q.Size > 0 && q.Size <= storage.MaxFileBytes &&
		q.ContentType != "" && len(q.ContentType) <= 1024 && !strings.ContainsAny(q.ContentType, "\r\n\x00") &&
		(records.SnapshotRow{Kind: "original", ID: q.ID, Size: q.Size, SHA256: q.SHA256}).Validate() == nil
}

// externalMedia es el `private_external` de la fila: dónde está el original y las
// derivadas guardadas en el almacén.
type externalMedia struct {
	Version    int              `json:"v"`
	Request    externalRequest  `json:"request"`
	Path       string           `json:"path"`
	ModTime    time.Time        `json:"mod_time"`
	Renditions recipeRenditions `json:"renditions"`
}

// externalOrigin es lo que Capture deja en el diario: con ello Prepare vuelve a hacer
// las derivadas tras un corte, sin el fichero en memoria.
type externalOrigin struct {
	Version   int                       `json:"v"`
	Scope     storage.Scope             `json:"scope"`
	Target    storage.PublicationTarget `json:"target"`
	Request   externalRequest           `json:"request"`
	Path      string                    `json:"path"`
	ModTime   time.Time                 `json:"mod_time"`
	Poster    *offline.StagedFile       `json:"poster,omitempty"`
	Operation string                    `json:"operation"`
}

func esVideo(tipo string) bool { return strings.HasPrefix(strings.ToLower(tipo), "video/") }

// ImportarExterno enlaza en la fototeca el fichero o.Ruta sin copiarlo: mide la foto (o
// toma lo que se dice del vídeo), guarda sus derivadas en el almacén y apunta la ruta y
// la huella del original. job es el UUID de la foto nueva, como en ImportarFoto.
func (r *Repository) ImportarExterno(parent context.Context, job string, o OrigenExterno) (api.Photo, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return api.Photo{}, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return api.Photo{}, err
	}
	parsed, err := uuid.Parse(job)
	if err != nil || parsed == uuid.Nil || parsed.String() != job || !filepath.IsAbs(o.Ruta) {
		return api.Photo{}, model.ErrInvalidInput
	}
	ruta := filepath.Clean(o.Ruta)
	nombre := strings.TrimSpace(o.Nombre)
	if nombre == "" {
		nombre = filepath.Base(ruta)
	}
	info, suma, cabecera, err := huellaDeFichero(ctx, ruta)
	if err != nil {
		return api.Photo{}, err
	}
	video := esVideo(o.Tipo)
	asset, err := r.medirExterno(job, nombre, o, video, info.Size(), cabecera)
	if err != nil {
		return api.Photo{}, err
	}
	request := externalRequest{ID: job, Name: nombre, ContentType: asset.MediaType, Size: info.Size(), SHA256: suma}
	if !request.valid() {
		return api.Photo{}, model.ErrInvalidInput
	}
	var poster *offline.StagedFile
	retained := false
	if video {
		datos := o.Poster
		if _, _, err := miniatura.Cabecera(datos); len(datos) == 0 || err != nil {
			datos = posterNeutro(asset.Width, asset.Height)
		}
		staged, err := r.journal.StageFile(ctx, r.uploadIngestOwner(), externalIngestStagingNamespace, bytes.NewReader(datos), int64(len(datos)))
		if err != nil {
			return api.Photo{}, err
		}
		poster = &staged
		defer func() {
			if !retained {
				_ = r.journal.DeleteStagedFile(r.uploadIngestOwner(), externalIngestStagingNamespace, staged)
			}
		}()
	}
	origin := externalOrigin{Version: 1, Scope: r.scope, Request: request, Path: ruta, ModTime: info.ModTime().UTC(), Poster: poster}
	return r.finishExternal(ctx, origin, &asset, func() { retained = true })
}

// huellaDeFichero lee el fichero entero una vez: su tamaño, su SHA-256 y su cabecera.
func huellaDeFichero(ctx context.Context, ruta string) (os.FileInfo, string, []byte, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: un fichero que la persona enlaza
	if err != nil {
		return nil, "", nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, "", nil, model.ErrInvalidInput
	}
	cabecera := make([]byte, min(info.Size(), exif.BytesDeCabecera))
	if _, err := io.ReadFull(f, cabecera); err != nil {
		return nil, "", nil, err
	}
	h := sha256.New()
	_, _ = h.Write(cabecera)
	if _, err := io.Copy(h, contextoLector{ctx, f}); err != nil {
		return nil, "", nil, err
	}
	return info, hex.EncodeToString(h.Sum(nil)), cabecera, nil
}

// contextoLector corta una lectura larga (un vídeo de varios GB) si se cancela.
type contextoLector struct {
	ctx context.Context
	r   io.Reader
}

func (c contextoLector) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// medirExterno es la fila de la foto o del vídeo, antes de las derivadas.
func (r *Repository) medirExterno(id, nombre string, o OrigenExterno, video bool, tamano int64, cabecera []byte) (model.AssetRow, error) {
	return MedirExterno(r.scope, r.formatosExtra, id, nombre, o, video, tamano, cabecera)
}

// MedirExterno es la fila de una foto o un vídeo enlazado (ImportarExterno) de scope,
// antes de sus derivadas; extra son los formatos de foto admitidos además de JPEG, PNG
// y GIF.
func MedirExterno(scope storage.Scope, extra map[string]bool, id, nombre string, o OrigenExterno, video bool, tamano int64, cabecera []byte) (model.AssetRow, error) {
	var fila model.AssetRow
	if video {
		fila = FilaDeVideo(scope, id, nombre, o.Tipo, tamano, o.Poster, o.Duracion, o.Ancho, o.Alto, o.TomadoEn)
	} else {
		// Una foto se mide como al importarla; y tiene que caber en lo que la ingesta
		// sabe derivar.
		if tamano > photorender.MaxOriginal {
			return model.AssetRow{}, model.ErrInvalidInput
		}
		var err error
		if fila, err = FilaDeCabecera(scope, extra, id, nombre, o.Tipo, tamano, cabecera); err != nil {
			return fila, err
		}
		if fila.TakenAt == nil && o.TomadoEn != nil && !o.TomadoEn.IsZero() {
			t := o.TomadoEn.UTC()
			fila.TakenAt = &t
		}
	}
	if o.SubidoEn != nil && !o.SubidoEn.IsZero() {
		fila.UploadedAt = o.SubidoEn.UTC()
	}
	return fila, nil
}

// filaDeVideo es la fila de un vídeo, enlazado o copiado, con lo que se dice de él: sus
// lados o, sin ellos, los del póster (y sin póster, 1280×720), su duración y cuándo se
// grabó.
func (r *Repository) filaDeVideo(id, nombre, tipo string, tamano int64, poster []byte, duracion time.Duration, ancho, alto int, tomado *time.Time) model.AssetRow {
	return FilaDeVideo(r.scope, id, nombre, tipo, tamano, poster, duracion, ancho, alto, tomado)
}

// FilaDeVideo es la fila de un vídeo de scope (filaDeVideo), la misma en las dos
// fototecas de Sytue Office.
func FilaDeVideo(scope storage.Scope, id, nombre, tipo string, tamano int64, poster []byte, duracion time.Duration, ancho, alto int, tomado *time.Time) model.AssetRow {
	if ancho <= 0 || alto <= 0 {
		if cfg, _, err := miniatura.Cabecera(poster); err == nil && cfg.Width > 0 && cfg.Height > 0 {
			ancho, alto = cfg.Width, cfg.Height
		} else {
			ancho, alto = 1280, 720
		}
	}
	tipo = strings.ToLower(strings.Split(tipo, ";")[0])
	now := time.Now().UTC()
	asset := model.AssetRow{
		ID: id, Tenant: scope.Tenant, Owner: scope.Subject, DriveID: "private:" + id, Name: nombre, MediaType: tipo, Bytes: tamano,
		Width: ancho, Height: alto, Orientation: 1, UploadedAt: now, CreatedAt: now, UpdatedAt: now, Tags: []string{}, DurationMs: duracion.Milliseconds(),
	}
	if tomado != nil && !tomado.IsZero() {
		t := tomado.UTC()
		asset.TakenAt = &t
	}
	return asset
}

// posterNeutro es el póster de un vídeo del que no se sabe sacar ningún fotograma: un
// gris oscuro con sus proporciones.
func posterNeutro(ancho, alto int) []byte {
	w, h := 640, 360
	if ancho > 0 && alto > 0 {
		h = max(1, min(1280, w*alto/ancho))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	gris := color.NRGBA{R: 0x2b, G: 0x2d, B: 0x31, A: 0xff}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = gris.R, gris.G, gris.B, gris.A
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 80})
	return b.Bytes()
}

// finishExternal publica la fila con sus derivadas. measured es la fila medida (nil al
// reanudar: el diario ya la tiene).
func (r *Repository) finishExternal(ctx context.Context, origin externalOrigin, measured *model.AssetRow, retain func()) (api.Photo, error) {
	request := origin.Request
	input, _ := json.Marshal(request)
	defer clear(input)
	operation := externalIngestOperation + "_" + planBytesHash(input)
	origin.Operation = operation
	result, err := r.engine.MutateWithContent(ctx, externalIngestOperation, request.ID, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "asset", request.ID)
		if err == nil {
			defer clear(raw)
			_, media, decodeErr := r.externalAsset(raw)
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
	}, r.externalContent(origin, retain))
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
	return conEstado(photoView(asset)), nil
}

func (r *Repository) externalContent(want externalOrigin, retain func()) records.MutationContent {
	return records.MutationContent{
		Capture: func(_ context.Context, raw []byte, target storage.PublicationTarget) ([]byte, error) {
			var asset model.AssetRow
			if json.Unmarshal(raw, &asset) != nil || asset.ID != want.Request.ID || asset.Bytes != want.Request.Size {
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
			asset, origin, err := r.externalIntent(raw, captured, target, want.Request, want.Operation)
			if err != nil {
				return nil, err
			}
			return r.prepareExternal(ctx, raw, asset, origin)
		},
		Validate: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget) error {
			return r.validateExternal(ctx, raw, captured, target, want)
		},
		Acknowledge: func(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion) error {
			return r.acknowledgeExternal(ctx, raw, captured, target, accepted, want)
		},
	}
}

// validateExternal comprueba antes de publicar que la fila y sus derivadas son las de la
// importación want, en target.
func (r *Repository) validateExternal(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, want externalOrigin) error {
	if _, _, err := r.externalIntent(raw, captured, target, want.Request, want.Operation); err != nil {
		return err
	}
	_, media, err := r.externalAsset(raw)
	if err != nil || media == nil || media.Request != want.Request {
		return ErrSource
	}
	if err := r.validateExternalPublication(ctx, *media); err != nil {
		return err
	}
	for _, file := range renditionFiles(media.Renditions) {
		if file.Target != target {
			return ErrSource
		}
		if err := r.engine.ValidateRetainedFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

// acknowledgeExternal da por publicadas las derivadas de la importación want y suelta
// el póster que se guardó para ella.
func (r *Repository) acknowledgeExternal(ctx context.Context, raw, captured []byte, target storage.PublicationTarget, accepted storage.ObjectVersion, want externalOrigin) error {
	_, origin, err := r.externalIntent(raw, captured, target, want.Request, want.Operation)
	if err != nil {
		return err
	}
	_, media, err := r.externalAsset(raw)
	if err != nil || media == nil {
		return ErrSource
	}
	if err := r.engine.AcknowledgeRetainedFiles(ctx, origin.Operation, renditionFiles(media.Renditions), accepted, func(selected []byte) ([]records.RetainedFile, error) {
		_, actual, err := r.externalAsset(selected)
		if err != nil || actual == nil || actual.Request != want.Request {
			return nil, ErrSource
		}
		return renditionFiles(actual.Renditions), nil
	}); err != nil {
		return err
	}
	if origin.Poster != nil {
		return r.journal.DeleteStagedFile(r.uploadIngestOwner(), externalIngestStagingNamespace, *origin.Poster)
	}
	return nil
}

func (r *Repository) externalIntent(raw, captured []byte, target storage.PublicationTarget, request externalRequest, operation string) (model.AssetRow, externalOrigin, error) {
	var asset model.AssetRow
	var origin externalOrigin
	if json.Unmarshal(raw, &asset) != nil || json.Unmarshal(captured, &origin) != nil ||
		!r.validExternalOrigin(origin, target, request, operation) || !r.validExternalAsset(asset, request) {
		return asset, origin, ErrSource
	}
	return asset, origin, nil
}

// validExternalOrigin dice si lo capturado es el origen de esta petición, en target y
// con su operación (y con póster si, y solo si, es un vídeo).
func (r *Repository) validExternalOrigin(origin externalOrigin, target storage.PublicationTarget, request externalRequest, operation string) bool {
	input, _ := json.Marshal(request)
	defer clear(input)
	if !request.valid() || operation != externalIngestOperation+"_"+planBytesHash(input) ||
		origin.Version != 1 || origin.Scope != r.scope || origin.Target != target || origin.Request != request || origin.Operation != operation || !filepath.IsAbs(origin.Path) {
		return false
	}
	return (origin.Poster != nil) == esVideo(request.ContentType)
}

// validExternalAsset dice si la fila es la que crea la petición: de esta persona, viva y
// sin derivadas privadas.
func (r *Repository) validExternalAsset(asset model.AssetRow, request externalRequest) bool {
	return asset.ID == request.ID && asset.Tenant == r.scope.Tenant && asset.Owner == r.scope.Subject && asset.Name == request.Name && asset.Bytes == request.Size &&
		asset.MediaType == request.ContentType && asset.DriveID == "private:"+request.ID && asset.DeletedAt == nil && len(asset.PrivateMedia) == 0
}

// prepareExternal guarda las derivadas en el almacén: las de la foto, del fichero (con
// su huella comprobada); las de un vídeo, de su póster.
func (r *Repository) prepareExternal(ctx context.Context, raw []byte, asset model.AssetRow, origin externalOrigin) ([]byte, error) {
	fingerprint, err := model.RecetaVacia().Huella()
	if err != nil {
		return nil, err
	}
	load := func(ctx context.Context) ([]byte, error) {
		if origin.Poster != nil {
			return r.posterGuardado(ctx, *origin.Poster)
		}
		return leerExterno(ctx, origin.Path, origin.Request.Size, origin.Request.SHA256, photorender.MaxOriginal)
	}
	set := recipeRenditions{Version: 1, Asset: asset.ID, Fingerprint: fingerprint, Files: map[model.Variante]records.RetainedFile{}}
	sides, variants := renditionSides()
	orientation := model.OrientacionDe(asset.Orientation)
	pngOutput := asset.MediaType == "image/png"
	err = photorender.Derivar(ctx, load, orientation, model.RecetaVacia(), pngOutput, sides, func(side int, format, _ string, encoded []byte) error {
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
	fields[externalMediaField], _ = json.Marshal(externalMedia{Version: 1, Request: origin.Request, Path: origin.Path, ModTime: origin.ModTime, Renditions: set})
	return json.Marshal(fields)
}

func (r *Repository) posterGuardado(ctx context.Context, staged offline.StagedFile) ([]byte, error) {
	reader, err := r.journal.OpenStagedFile(ctx, r.uploadIngestOwner(), externalIngestStagingNamespace, staged)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(io.LimitReader(io.NewSectionReader(reader, 0, staged.SizeBytes), photorender.MaxOriginal+1))
}

// leerExterno lee entero un original externo de hasta tope bytes y comprueba que es el
// mismo: el tamaño y la huella apuntados.
func leerExterno(ctx context.Context, ruta string, tamano int64, huella string, tope int64) ([]byte, error) {
	if tamano > tope {
		return nil, model.ErrInvalidInput
	}
	var b bytes.Buffer
	if err := copiarExterno(ctx, &b, ruta, tamano, huella); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// copiarExterno copia a out el original externo comprobando su tamaño y su huella; si
// no es el mismo, ErrOriginalExterno (lo copiado hasta ahí ya está en out: quien escribe
// un fichero tiene que tirarlo).
func copiarExterno(ctx context.Context, out io.Writer, ruta string, tamano int64, huella string) error {
	f, err := os.Open(ruta) //nolint:gosec // G304: el original que la persona enlazó
	if errors.Is(err, os.ErrNotExist) {
		return ErrOriginalExterno
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || info.Size() != tamano {
		return ErrOriginalExterno
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), contextoLector{ctx, io.LimitReader(f, tamano+1)}); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != huella {
		return ErrOriginalExterno
	}
	return nil
}

func (r *Repository) validateExternalPublication(ctx context.Context, wanted externalMedia) error {
	raw, err := r.engine.ReadRaw(ctx, "asset", wanted.Request.ID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer clear(raw)
	asset, current, err := r.externalAsset(raw)
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

// externalAsset decodifica la fila y su original externo; media es nil si no lo tiene.
func (r *Repository) externalAsset(raw []byte) (model.AssetRow, *externalMedia, error) {
	var asset model.AssetRow
	if json.Unmarshal(raw, &asset) != nil {
		return asset, nil, ErrSource
	}
	if len(asset.PrivateExternal) == 0 {
		return asset, nil, nil //nolint:nilnil // sin original externo
	}
	if len(asset.PrivateMedia) > 0 || asset.Tenant != r.scope.Tenant || asset.Owner != r.scope.Subject {
		return asset, nil, ErrSource
	}
	var media externalMedia
	if json.Unmarshal(asset.PrivateExternal, &media) != nil || media.Version != 1 || !media.Request.valid() || media.Request.ID != asset.ID ||
		media.Request.Name != asset.Name || media.Request.Size != asset.Bytes || media.Request.ContentType != asset.MediaType ||
		asset.DriveID != "private:"+asset.ID || !filepath.IsAbs(media.Path) {
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

// huellaDeFila es la huella del original de una fila, venga de donde venga: retenido,
// externo o el que se apuntó al purgarla. Vacía si no la tiene.
func (r *Repository) huellaDeFila(raw []byte) (model.AssetRow, string, error) {
	asset, media, err := r.ingestedAsset(raw)
	if err != nil {
		return asset, "", err
	}
	if media != nil {
		return asset, strings.ToLower(media.Request.SHA256), nil
	}
	_, externa, err := r.externalAsset(raw)
	if err != nil {
		return asset, "", err
	}
	if externa != nil {
		return asset, strings.ToLower(externa.Request.SHA256), nil
	}
	return asset, strings.ToLower(asset.OriginalSHA256), nil
}

// estadoRuta mira, sin leerlo, si el original externo sigue en su sitio: que esté y con
// el mismo tamaño. Que los bytes sean los mismos se comprueba al leerlo.
func estadoRuta(ruta string, tamano int64) string {
	info, err := os.Stat(ruta)
	switch {
	case err != nil || !info.Mode().IsRegular():
		return api.ExternoFalta
	case info.Size() != tamano:
		return api.ExternoCambiado
	}
	return ""
}

func estadoExterno(media externalMedia) string { return estadoRuta(media.Path, media.Request.Size) }

// rutaExternaDe es la ruta del original externo de una fila, sin validarla (la fila ya
// pasó por el catálogo); vacía si no es externo.
func rutaExternaDe(a model.AssetRow) string {
	if len(a.PrivateExternal) == 0 {
		return ""
	}
	var m struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(a.PrivateExternal, &m)
	return m.Path
}

// conEstado añade a la vista de una foto con original externo si ese fichero sigue ahí.
func conEstado(v api.Photo) api.Photo {
	if v.Externo != "" {
		v.EstadoExterno = estadoRuta(v.Externo, v.SizeBytes)
	}
	return v
}

// conEstados es conEstado para lo que se devuelve (una página, no la galería entera).
func conEstados(vistas []api.Photo) []api.Photo {
	for i := range vistas {
		vistas[i] = conEstado(vistas[i])
	}
	return vistas
}

// originalExterno copia el original externo de asset, si lo tiene. ok es false si el
// original no es externo.
func (r *Repository) originalExterno(ctx context.Context, asset string, out io.Writer) (bool, error) {
	raw, err := r.engine.ReadRaw(ctx, "asset", asset)
	if err != nil {
		return false, err
	}
	defer clear(raw)
	_, media, err := r.externalAsset(raw)
	if err != nil || media == nil {
		return false, err
	}
	return true, copiarExterno(ctx, out, media.Path, media.Request.Size, media.Request.SHA256)
}

// RutaExterna es dónde está el original externo de una foto (el fichero de un vídeo,
// para reproducirlo o abrirlo en Cast) y su estado; ok es false si no es externo.
func (r *Repository) RutaExterna(parent context.Context, id string) (ruta, estado string, ok bool, err error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return "", "", false, err
	}
	defer done()
	raw, err := r.engine.ReadRaw(ctx, "asset", id)
	if err != nil {
		return "", "", false, err
	}
	defer clear(raw)
	_, media, err := r.externalAsset(raw)
	if err != nil || media == nil {
		return "", "", false, err
	}
	return media.Path, estadoExterno(*media), true, nil
}

// Recolocar busca entre candidatas (rutas de ficheros del equipo: lo que hay ahora en
// las carpetas vigiladas) los originales externos que ya no están en su sitio, por su
// tamaño primero y su huella después, y apunta la ruta nueva: así una foto movida o
// renombrada dentro de una carpeta vigilada vuelve a tener original. Devuelve cuántas
// ha recolocado. Las que no aparecen siguen diciendo que les falta.
func (r *Repository) Recolocar(parent context.Context, candidatas []string) (int, error) {
	ctx, done, err := r.engine.Start(parent)
	if err != nil {
		return 0, err
	}
	defer done()
	if _, err := r.engine.Catalog(); err != nil {
		return 0, err
	}
	versions, err := r.engine.Selected("asset")
	if err != nil {
		return 0, err
	}
	porTamano, enUso, err := r.externasPerdidas(ctx, versions)
	if err != nil {
		return 0, err
	}
	if len(porTamano) == 0 {
		return 0, ctx.Err()
	}
	hechas := 0
	for _, c := range candidatas {
		if err := ctx.Err(); err != nil {
			return hechas, err
		}
		c = filepath.Clean(c)
		info, err := os.Stat(c)
		if err != nil || !info.Mode().IsRegular() || enUso[c] || len(porTamano[info.Size()]) == 0 {
			continue
		}
		_, suma, _, err := huellaDeFichero(ctx, c)
		if err != nil {
			continue
		}
		lista := porTamano[info.Size()]
		for i, p := range lista {
			if p.media.Request.SHA256 != suma {
				continue
			}
			if err := r.moverExterno(ctx, p.id, c, info.ModTime().UTC()); err != nil {
				return hechas, err
			}
			hechas++
			enUso[c] = true
			porTamano[info.Size()] = append(lista[:i:i], lista[i+1:]...)
			break
		}
	}
	return hechas, nil
}

// externaPerdida es una foto externa cuyo original no está donde estaba.
type externaPerdida struct {
	id    string
	media externalMedia
}

// externasPerdidas reparte las fotos externas de versions: las que no están donde
// estaban, por tamaño del original, y las rutas de las que sí (que ya están en uso).
func (r *Repository) externasPerdidas(ctx context.Context, versions []storage.ObjectVersion) (map[int64][]externaPerdida, map[string]bool, error) {
	porTamano := map[int64][]externaPerdida{}
	enUso := map[string]bool{}
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return nil, nil, err
		}
		asset, media, err := r.externalAsset(raw)
		clear(raw)
		if err != nil {
			return nil, nil, err
		}
		if media == nil || asset.PurgedAt != nil {
			continue
		}
		if estadoExterno(*media) == "" {
			enUso[filepath.Clean(media.Path)] = true
			continue
		}
		porTamano[media.Request.Size] = append(porTamano[media.Request.Size], externaPerdida{asset.ID, *media})
	}
	return porTamano, enUso, nil
}

// moverExterno apunta la ruta nueva del original externo de una foto.
func (r *Repository) moverExterno(ctx context.Context, id, ruta string, cuando time.Time) error {
	raw, err := r.engine.ReadRaw(ctx, "asset", id)
	if err != nil {
		return err
	}
	var row model.AssetRow
	err = json.Unmarshal(raw, &row)
	clear(raw)
	if err != nil {
		return ErrSource
	}
	papelera := row.DeletedAt != nil
	return r.mutateAsset(ctx, "external-move", id, []string{id, ruta}, papelera, func(fields map[string]json.RawMessage) {
		var media externalMedia
		if json.Unmarshal(fields[externalMediaField], &media) != nil {
			return
		}
		media.Path, media.ModTime = ruta, cuando
		fields[externalMediaField], _ = json.Marshal(media)
	})
}

// reanudarExternos termina los enlaces que un corte dejó a medias (ReanudarImportaciones).
func (r *Repository) reanudarExternos(ctx context.Context) ([]api.Photo, []error) {
	completed := []api.Photo{}
	var failures []error
	for cursor := ""; ; {
		page, err := r.engine.PendingContents(ctx, externalIngestOperation, "asset", cursor, 8)
		if err != nil {
			return completed, append(failures, err)
		}
		for _, pending := range page.Items {
			var origin externalOrigin
			err := json.Unmarshal(pending.Captured, &origin)
			if err == nil && pending.EntityID != origin.Request.ID {
				err = ErrSource
			}
			if err == nil {
				if _, _, err = r.externalIntent(pending.JSON, pending.Captured, pending.Target, origin.Request, origin.Operation); err == nil {
					origin.Operation, origin.Target = "", storage.PublicationTarget{}
					var photo api.Photo
					if photo, err = r.finishExternal(ctx, origin, nil, nil); err == nil {
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
