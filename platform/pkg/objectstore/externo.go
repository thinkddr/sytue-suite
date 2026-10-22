// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"io"
	"net/url"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// BlobStore es la superficie de blobs que comparten S3 y los almacenes conectados.
// No cifra ni descifra contenido. Las claves, permisos y el cifrado de extremo
// a extremo pertenecen al cliente y al servicio que autoriza cada operación.
type BlobStore interface {
	Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts PutOptions) error
	Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, t tenancy.Tenant, key string) error
	PresignGet(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error)
	// PresignPut firma una URL de subida PUT directa a S3 (clave+método+caducidad
	// acotados). Los proveedores que no firman devuelven ErrPresignUnsupported.
	PresignPut(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error)
	// Stat devuelve el tamaño del objeto; ErrNotFound si no existe.
	Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error)
	// Existe y Copiar comprueban y copian objetos sin cambiar sus bytes.
	Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error)
	Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error
}
