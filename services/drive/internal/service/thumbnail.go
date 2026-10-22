// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// Miniaturas server-side de imágenes ráster. Solo formatos que decodifica la
// STDLIB (JPEG, PNG, GIF): sin dependencias nuevas. PDF y vídeo quedan FUERA de
// alcance (requieren decodificadores pesados). WebP/AVIF/SVG tampoco: la stdlib no
// los decodifica y no añadimos deps. La derivada se cachea en el mismo object store
// (keyed por checksum del fichero) para no regenerarla; el gating por permiso lo
// impone GetFile (RLS: un fichero ajeno/inexistente → ErrNotFound, nunca se genera).
const (
	// thumbMaxDim es el lado máximo (px) de la miniatura, manteniendo el aspecto.
	//
	// 512 y no 256: las superficies que la consumen la pintan a 256 px CSS (el adjunto
	// del chat, con max-h-64), y en una pantalla 2x —cualquier portátil de los últimos
	// años— una fuente de 256 px se ve a media resolución. El coste es asumible: una
	// miniatura JPEG de 512 px ronda las decenas de KB frente a los megabytes del
	// original, que es de lo que se trataba huir.
	//
	// Cambiar este número INVALIDA las miniaturas cacheadas por sí solo: forma parte de
	// thumbVersion, así que la clave del object store y el ETag cambian con él.
	thumbMaxDim = 512
	// thumbMaxInputBytes acota el tamaño del blob de ORIGEN que leemos en memoria: la
	// segunda barrera (un fichero enorme aunque sus dimensiones sean plausibles).
	thumbMaxInputBytes = 40 << 20 // 40 MiB
)

// turnoMiniaturas: cuántas miniaturas se generan a la vez en el proceso.
var turnoMiniaturas = make(chan struct{}, 2)

// Thumbnail devuelve (generándola perezosamente y cacheándola) una miniatura
// pequeña de un fichero de imagen ráster. Contrato:
//
//   - GATING: GetFile aplica la verja de acceso (RLS + aislamiento por tenant). Un
//     fichero inaccesible/ajeno/inexistente → ErrNotFound; NUNCA se genera de un
//     fichero que el usuario no puede ver (deny-by-default, como «Compartido conmigo»).
//   - TIPO: solo JPEG/PNG/GIF (validado por content-type); otro → ErrUnsupportedMedia.
//   - CACHÉ: la derivada se guarda en el object store bajo una clave por-fichero+checksum
//     (thumbKey). La primera llamada decodifica+redimensiona+cachea; las siguientes la
//     sirven directa del object store sin regenerar (la clave cambia si el contenido —y
//     por tanto el checksum— cambia, invalidando la caché automáticamente).
//   - GUARDA anti-bomba: miniatura.Render mira la cabecera y rechaza lo que no cabe
//     (miniatura.Cabe: píxeles Y memoria) ANTES de decodificar; el origen se lee acotado a
//     thumbMaxInputBytes, y de dos en dos (turnoMiniaturas).
//
// Devuelve el lector de la miniatura (el llamador lo cierra), su content-type de
// salida (image/jpeg o image/png) y un ETag estable (derivado del checksum).
func (s *Service) Thumbnail(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, string, error) {
	f, err := s.store.GetFile(ctx, id) // verja de acceso (RLS): gatea permiso y tenant
	if err != nil {
		return nil, "", "", err
	}
	return s.thumbnailOf(ctx, f)
}

// thumbnailOf genera/sirve la miniatura de un fichero YA AUTORIZADO. La separación
// existe porque cada ámbito tiene su propia verja: RLS personal (Thumbnail) o
// membresía del canal (ChannelThumbnail, channelfiles.go). Nunca se llama sin haber
// resuelto antes el acceso al fichero.
func (s *Service) thumbnailOf(ctx context.Context, f domain.File) (io.ReadCloser, string, string, error) {
	outType, ok := thumbOutputType(f.ContentType)
	if !ok {
		return nil, "", "", domain.ErrUnsupportedMedia
	}
	t, _ := tenancy.From(ctx)
	key := thumbKey(f)
	etag := thumbETag(f)

	// Caché: si la derivada ya existe, se sirve directa (sin regenerar).
	if rc, err := s.blobs.Get(ctx, t, key); err == nil {
		return rc, outType, etag, nil
	} else if !errors.Is(err, objectstore.ErrNotFound) {
		return nil, "", "", fmt.Errorf("service: leer miniatura cacheada: %w", err)
	}

	// Miss: leer el origen (acotado en bytes), generar y cachear. Con turno:
	// cada miniatura que se genera tiene hasta 40 MiB del origen en memoria
	// mientras espera el suyo en miniatura.Render, y sin tope cuarenta peticiones
	// a la vez llenaban el pod (FILE-07).
	select {
	case turnoMiniaturas <- struct{}{}:
		defer func() { <-turnoMiniaturas }()
	case <-ctx.Done():
		return nil, "", "", ctx.Err()
	}
	src, err := s.blobs.Get(ctx, t, f.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return nil, "", "", domain.ErrNotFound
		}
		return nil, "", "", fmt.Errorf("service: abrir blob de origen: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(src, thumbMaxInputBytes+1))
	_ = src.Close()
	if err != nil {
		return nil, "", "", fmt.Errorf("service: leer blob de origen: %w", err)
	}
	if int64(len(data)) > thumbMaxInputBytes {
		return nil, "", "", domain.ErrUnsupportedMedia // demasiado grande para miniaturar
	}

	out, err := miniatura.Render(data, thumbMaxDim, outType)
	if err != nil {
		// El paquete compartido no conoce el dominio de Drive y devuelve su propio
		// error de entrada. Se traduce AQUÍ para que la capa HTTP siga viendo
		// ErrUnsupportedMedia y respondiendo 415, que es lo que hacía cuando el
		// renderizador vivía en este fichero. Sin esta línea, un fichero que no
		// es imagen pasaría de 415 a 500.
		if errors.Is(err, miniatura.ErrMedioNoSoportado) {
			return nil, "", "", domain.ErrUnsupportedMedia
		}
		return nil, "", "", err
	}

	// Cachear la derivada (best effort): si el Put falla, igual servimos la miniatura
	// recién generada; se reintentará en la siguiente petición.
	if err := s.blobs.Put(ctx, t, key, bytes.NewReader(out), int64(len(out)), objectstore.PutOptions{
		ContentType: outType,
	}); err != nil {
		log.From(ctx).Warn("drive: cachear miniatura", "key", key, "error", err)
	}
	return io.NopCloser(bytes.NewReader(out)), outType, etag, nil
}

// thumbOutputType valida el content-type de ORIGEN (solo imágenes ráster de la
// stdlib) y devuelve el content-type de SALIDA de la miniatura, o ok=false si no se
// soporta. GIF→PNG y PNG→PNG (preservan transparencia/primer frame); JPEG→JPEG
// (foto opaca, más pequeña). El tipo de salida es una función determinista del de
// origen: no hace falta persistirlo para el cache-hit, se recalcula igual.
func thumbOutputType(contentType string) (string, bool) {
	ct := contentType
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i] // descarta parámetros (charset, etc.)
	}
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg", true
	case "image/png":
		return "image/png", true
	case "image/gif":
		return "image/png", true
	default:
		return "", false
	}
}

// thumbVersion es la parte de la clave/ETag que INVALIDA la caché: el checksum del
// fichero (cambia con cada subida) MÁS el lado máximo con el que se generó. Si faltara el
// checksum (datos heredados), cae al número de versión.
//
// La dimensión va aquí y no es un adorno: sin ella, subir thumbMaxDim no habría servido
// de nada. La derivada cacheada se seguiría sirviendo al tamaño viejo —misma clave— y el
// navegador que ya la tuviera recibiría un 304 con el mismo ETag. Un cambio de una línea
// que no cambia nada y no falla en ningún sitio.
func thumbVersion(f domain.File) string {
	base := f.Checksum
	if base == "" {
		base = "v" + strconv.Itoa(f.Version)
	}
	return base + "-" + strconv.Itoa(thumbMaxDim)
}

// thumbKey deriva la clave del object store para la miniatura cacheada. Estable por
// (fichero, contenido); no lleva prefijo de tenant (lo añade objectstore).
func thumbKey(f domain.File) string {
	return "thumbs/" + f.ID.String() + "/" + thumbVersion(f)
}

// thumbETag es un ETag fuerte estable de la miniatura (deriva del checksum): cambia
// cuando el contenido del fichero cambia, permitiendo revalidación por If-None-Match.
func thumbETag(f domain.File) string {
	return `"` + thumbVersion(f) + `-thumb"`
}
