// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Lo que hace falta para SERVIR objetos grandes en streaming, no solo leerlos
// entero: un reproductor de vídeo pide rangos (`Range: bytes=…`) y un vídeo tiene
// muchos ficheros que se borran juntos. Lo usa Cast (/media y la purga de un
// vídeo); antes de él nadie servía rangos desde el servicio.

// Objeto es un objeto abierto para servirlo, entero o en parte.
type Objeto struct {
	// Body es el cuerpo (entero o el rango). Lo cierra quien lee.
	Body io.ReadCloser
	// Size es el tamaño de lo que se devuelve en Body.
	Size int64
	// Total es el tamaño del objeto completo.
	Total       int64
	ContentType string
	// ContentRange va relleno cuando se sirvió un rango (`bytes a-b/total`).
	ContentRange string
	LastModified time.Time
}

// ErrRangoInvalido: la cabecera Range pedía algo fuera del objeto (416).
var ErrRangoInvalido = fmt.Errorf("objectstore: rango no satisfacible")

// RangoNoSatisfacible es ErrRangoInvalido con el tamaño del objeto, que el 416
// tiene que decir (`Content-Range: bytes */<total>`).
type RangoNoSatisfacible struct{ Total int64 }

func (e *RangoNoSatisfacible) Error() string {
	return fmt.Sprintf("%v (el objeto tiene %d bytes)", ErrRangoInvalido, e.Total)
}

// Is hace que errors.Is(err, ErrRangoInvalido) valga.
func (e *RangoNoSatisfacible) Is(objetivo error) bool { return objetivo == ErrRangoInvalido }

// GetRange abre un objeto entero o el rango que pide la cabecera `Range` tal cual
// llega (`bytes=a-b`, `bytes=a-`, `bytes=-n`; vacía = entero). Solo un rango:
// los reproductores no piden más. Un objeto guardado comprimido (PutOptions.
// Comprimir) no se sirve por aquí: los rangos serían de los bytes comprimidos.
func (c *Client) GetRange(ctx context.Context, t tenancy.Tenant, key, rango string) (*Objeto, error) {
	full := tenantKey(t, key)
	mc, bucket, _ := c.backend(t, key)
	info, err := mc.StatObject(ctx, bucket, full, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("objectstore: stat %q: %w", full, err)
	}
	if codificacionDe(info.UserMetadata) == "gzip" {
		return nil, fmt.Errorf("objectstore: %q está comprimido y no se sirve por rangos", full)
	}
	inicio, fin, parcial, err := parsearRango(rango, info.Size)
	if err != nil {
		return nil, &RangoNoSatisfacible{Total: info.Size}
	}
	opts := minio.GetObjectOptions{}
	if parcial {
		if err := opts.SetRange(inicio, fin); err != nil {
			return nil, fmt.Errorf("objectstore: rango %q: %w", rango, err)
		}
	}
	obj, err := mc.GetObject(ctx, bucket, full, opts)
	if err != nil {
		return nil, fmt.Errorf("objectstore: get %q: %w", full, err)
	}
	out := &Objeto{Body: obj, Size: info.Size, Total: info.Size, ContentType: info.ContentType, LastModified: info.LastModified}
	if parcial {
		out.Size = fin - inicio + 1
		out.ContentRange = "bytes " + strconv.FormatInt(inicio, 10) + "-" + strconv.FormatInt(fin, 10) + "/" + strconv.FormatInt(info.Size, 10)
	}
	return out, nil
}

// parsearRango interpreta una cabecera Range de un solo rango contra un objeto de
// `total` bytes. Devuelve inicio y fin inclusivos.
func parsearRango(rango string, total int64) (inicio, fin int64, parcial bool, err error) {
	rango = strings.TrimSpace(rango)
	if rango == "" {
		return 0, total - 1, false, nil
	}
	spec, ok := strings.CutPrefix(rango, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		// Otra unidad o varios rangos: se sirve entero, que es lo que RFC 9110 permite.
		return 0, total - 1, false, nil
	}
	a, b, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, total - 1, false, nil
	}
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case a == "" && b == "":
		return 0, total - 1, false, nil
	case a == "":
		// Los últimos n bytes.
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false, ErrRangoInvalido
		}
		if n > total {
			n = total
		}
		return total - n, total - 1, true, nil
	default:
		inicio, err := strconv.ParseInt(a, 10, 64)
		if err != nil || inicio < 0 || inicio >= total {
			return 0, 0, false, ErrRangoInvalido
		}
		fin = total - 1
		if b != "" {
			fin, err = strconv.ParseInt(b, 10, 64)
			if err != nil || fin < inicio {
				return 0, 0, false, ErrRangoInvalido
			}
			if fin >= total {
				fin = total - 1
			}
		}
		return inicio, fin, true, nil
	}
}

// SizePrefix suma lo que ocupa todo lo que cuelga de un prefijo (los ficheros de
// un vídeo, para declarar lo que de verdad ocupa tras quitarle o ponerle
// derivadas).
func (c *Client) SizePrefix(ctx context.Context, t tenancy.Tenant, prefix string) (int64, error) {
	full := tenantKey(t, prefix)
	if !strings.HasSuffix(full, "/") {
		full += "/"
	}
	mc, bucket, _ := c.backend(t, prefix)
	var total int64
	for obj := range mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: full, Recursive: true}) {
		if obj.Err != nil {
			return 0, fmt.Errorf("objectstore: listar %q: %w", full, obj.Err)
		}
		total += obj.Size
	}
	return total, nil
}

// DeletePrefix borra todo lo que cuelga de un prefijo (los ficheros de un vídeo:
// original, HLS, WebM, póster). Sigue borrando aunque uno falle y devuelve el
// primer error al final, para que una purga a medias se reintente entera.
func (c *Client) DeletePrefix(ctx context.Context, t tenancy.Tenant, prefix string) error {
	full := tenantKey(t, prefix)
	if !strings.HasSuffix(full, "/") {
		full += "/"
	}
	mc, bucket, _ := c.backend(t, prefix)
	var primero error
	for obj := range mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: full, Recursive: true}) {
		if obj.Err != nil {
			return fmt.Errorf("objectstore: listar %q: %w", full, obj.Err)
		}
		if err := mc.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil && primero == nil {
			primero = fmt.Errorf("objectstore: delete %q: %w", obj.Key, err)
		}
	}
	return primero
}
