// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// externalStorageStore es lo que el enrutador necesita del store: el almacén de
// terceros de un tenant, si lo tiene encendido.
type externalStorageStore interface {
	ExternalStorageFor(ctx context.Context, tenant string) (store.ExternalStorage, error)
	ExternalStorageLocation(context.Context, string, uuid.UUID) (store.ExternalStorage, error)
	objectstore.ExternalObjects
}

// ErrExternalStorageOff se devuelve cuando un blob vive en el almacén de
// terceros y ese almacén no se puede abrir (desconectado o sin KEK).
var ErrExternalStorageOff = errors.New("service: almacén de terceros no disponible")

// blobRouter routes by a key pinned to the connection's physical location.
// Active configuration selects new writes; historical locations remain readable
// after switching accounts or disabling new external writes.
type blobRouter struct {
	base   blobStore
	ext    externalStorageStore
	cipher *keycrypto.Cipher
	// conn presta los tokens de Google Drive y OneDrive; nil = esas clases no están.
	conn *integrationsClient
	// tunnel habla con el proxy de la plataforma para el NAS del cliente; nil = no está.
	tunnel *tunnelClient

	mu    sync.Mutex
	cache map[string]entradaExterna
}

type entradaExterna struct {
	locationID uuid.UUID
	blobs      blobStore // nil = el tenant no tiene almacén de terceros
	expires    time.Time
}

const cacheExterna = time.Minute

func newBlobRouter(base blobStore, ext externalStorageStore, kek string, conn *integrationsClient) *blobRouter {
	r := &blobRouter{base: base, ext: ext, conn: conn, cache: map[string]entradaExterna{}}
	if raw := strings.TrimSpace(kek); raw != "" {
		if key, err := base64.StdEncoding.DecodeString(raw); err == nil {
			if c, err := keycrypto.New(key); err == nil {
				r.cipher = c
			}
		}
	}
	return r
}

// Enabled indica si el tenant tiene almacén de terceros encendido y abrible:
// entonces los blobs NUEVOS van allí (clave con prefijo).
func (r *blobRouter) Enabled(ctx context.Context, t tenancy.Tenant) bool {
	b, err := r.externo(ctx, t)
	return err == nil && b != nil
}

// Forget olvida la entrada cacheada de un tenant: tras conectar o desconectar
// el almacén, la siguiente operación vuelve a leer el store.
func (r *blobRouter) Forget(tenant string) {
	r.mu.Lock()
	for key := range r.cache {
		if key == tenant || strings.HasPrefix(key, tenant+"/") {
			delete(r.cache, key)
		}
	}
	r.mu.Unlock()
}

func (r *blobRouter) pick(ctx context.Context, t tenancy.Tenant, key string) (blobStore, error) {
	if !store.EsExterno(key) {
		return r.base, nil
	}
	location := "legacy"
	if strings.HasPrefix(key, store.ExternalPrefix+"v1/") {
		parts := strings.SplitN(strings.TrimPrefix(key, store.ExternalPrefix+"v1/"), "/", 2)
		if len(parts) != 2 || parts[1] == "" {
			return nil, ErrExternalStorageOff
		}
		id, err := uuid.Parse(parts[0])
		if err != nil || id == uuid.Nil {
			return nil, ErrExternalStorageOff
		}
		location = id.String()
	}
	entry, err := r.externoEn(ctx, t, location)
	if err != nil {
		return nil, err
	}
	if entry.blobs == nil {
		return nil, ErrExternalStorageOff
	}
	return entry.blobs, nil
}

func (r *blobRouter) externo(ctx context.Context, t tenancy.Tenant) (blobStore, error) {
	entry, err := r.externoEn(ctx, t, "")
	return entry.blobs, err
}

// nuevaClave freezes the location before upload, including when another process
// switches the tenant's active connection between selection and Put.
func (r *blobRouter) nuevaClave(ctx context.Context, t tenancy.Tenant, key string) (string, error) {
	entry, err := r.externoEn(ctx, t, "")
	if err != nil {
		return "", errors.Join(ErrExternalStorageOff, err)
	}
	if entry.blobs == nil {
		return key, nil
	}
	if entry.locationID == uuid.Nil {
		return "", ErrExternalStorageOff
	}
	return store.ExternalPrefix + "v1/" + entry.locationID.String() + "/" + key, nil
}

func (r *blobRouter) externoEn(ctx context.Context, t tenancy.Tenant, location string) (entradaExterna, error) {
	if r.ext == nil {
		return entradaExterna{}, nil
	}
	cacheKey := t.Slug + "/" + location
	if location == "" {
		cacheKey = t.Slug
	}
	r.mu.Lock()
	entry, ok := r.cache[cacheKey]
	r.mu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry, nil
	}
	var cfg store.ExternalStorage
	var err error
	if location == "" {
		cfg, err = r.ext.ExternalStorageFor(ctx, t.Slug)
	} else {
		id := uuid.Nil
		if location != "legacy" {
			id, err = uuid.Parse(location)
			if err != nil {
				return entradaExterna{}, ErrExternalStorageOff
			}
		}
		cfg, err = r.ext.ExternalStorageLocation(ctx, t.Slug, id)
	}
	switch {
	case err == nil:
		secret := ""
		if cfg.SecretKeyEnc != "" {
			if r.cipher == nil {
				return entradaExterna{}, ErrExternalStorageOff
			}
			secret, err = r.cipher.Decrypt(cfg.SecretKeyEnc)
			if err != nil {
				return entradaExterna{}, fmt.Errorf("service: abrir la clave del almacén de terceros: %w", err)
			}
		}
		entry.blobs, err = r.abrir(t, cfg, secret)
		if err != nil {
			return entradaExterna{}, err
		}
		entry.locationID = cfg.LocationID
	case errors.Is(err, domain.ErrNotFound):
		if location != "" {
			return entradaExterna{}, ErrExternalStorageOff
		}
	default:
		return entradaExterna{}, err
	}
	entry.expires = time.Now().Add(cacheExterna)
	r.mu.Lock()
	r.cache[cacheKey] = entry
	r.mu.Unlock()
	return entry, nil
}

// El resto es el reparto, operación a operación.

func (r *blobRouter) Put(ctx context.Context, t tenancy.Tenant, key string, body io.Reader, size int64, opts objectstore.PutOptions) error {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return err
	}
	return b.Put(ctx, t, key, body, size, opts)
}

func (r *blobRouter) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return nil, err
	}
	return b.Get(ctx, t, key)
}

func (r *blobRouter) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return err
	}
	return b.Delete(ctx, t, key)
}

func (r *blobRouter) PresignGet(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error) {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return nil, err
	}
	return b.PresignGet(ctx, t, key, expiry)
}

type presignerConTamano interface {
	PresignPutConTamano(ctx context.Context, t tenancy.Tenant, key string, size int64, expiry time.Duration) (*url.URL, error)
}

func (r *blobRouter) PresignPut(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error) {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return nil, err
	}
	return b.PresignPut(ctx, t, key, expiry)
}

func (r *blobRouter) PresignPutConTamano(ctx context.Context, t tenancy.Tenant, key string, size int64, expiry time.Duration) (*url.URL, error) {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return nil, err
	}
	if pt, ok := b.(presignerConTamano); ok {
		return pt.PresignPutConTamano(ctx, t, key, size, expiry)
	}
	return b.PresignPut(ctx, t, key, expiry)
}

// Existe y Copiar los usa la deduplicación (compartir.go).
// Copiar entre dos almacenes distintos —del bucket propio del tenant al espacio
// compartido del servicio, o al revés— no puede ser una copia server-side: se
// lee de uno y se escribe en el otro, en streaming.
func (r *blobRouter) Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error) {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return false, err
	}
	return b.Existe(ctx, t, key)
}

func (r *blobRouter) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	bo, err := r.pick(ctx, t, origen)
	if err != nil {
		return err
	}
	bd, err := r.pick(ctx, t, destino)
	if err != nil {
		return err
	}
	if bo == bd {
		return bo.Copiar(ctx, t, origen, destino)
	}
	rc, err := bo.Get(ctx, t, origen)
	if err != nil {
		return err
	}
	defer rc.Close()
	return bd.Put(ctx, t, destino, rc, -1, objectstore.PutOptions{})
}

func (r *blobRouter) Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error) {
	b, err := r.pick(ctx, t, key)
	if err != nil {
		return 0, err
	}
	return b.Stat(ctx, t, key)
}

var _ blobStore = (*blobRouter)(nil)

// abrir construye el cliente del almacén de terceros según su clase.
func (r *blobRouter) abrir(t tenancy.Tenant, cfg store.ExternalStorage, secret string) (blobStore, error) {
	switch cfg.Kind {
	case "gdrive", "onedrive", "dropbox":
		if r.conn == nil {
			return nil, ErrConnectorsOff
		}
		instancia := cfg.InstanceID
		tok := func(ctx context.Context) (string, error) { return r.conn.Token(ctx, t, instancia) }
		if cfg.Kind == "gdrive" {
			return objectstore.NewGoogleDrive(tok, externalObjectIndex{r.ext}), nil
		}
		if cfg.Kind == "dropbox" {
			return objectstore.NewDropbox(tok), nil
		}
		return objectstore.NewOneDrive(tok), nil
	case "webdav", "nas":
		w, err := objectstore.NewWebDAV(cfg.Endpoint, cfg.BasePath, cfg.AccessKey, secret)
		if err != nil {
			return nil, err
		}
		return w, nil
	default: // "s3"
		// El endpoint lo escribe la organización: solo por TLS y por la barrera SSRF,
		// que valida la IP al conectar. minio no sigue redirecciones.
		if !cfg.UseSSL {
			return nil, ErrDestinoNoPermitido
		}
		c, err := objectstore.New(objectstore.Config{
			Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, UseSSL: true,
			AccessKey: cfg.AccessKey, SecretKey: secret,
			Transport: ssrfhttp.Transport(),
		})
		if err != nil {
			return nil, fmt.Errorf("service: cliente del almacén de terceros: %w", err)
		}
		return blobStat{c}, nil
	}
}

// Los adaptadores compartidos no dependen de los errores del dominio de Drive.
type externalObjectIndex struct{ objectstore.ExternalObjects }

func (e externalObjectIndex) GetExternalObject(ctx context.Context, tenant, key string) (string, error) {
	id, err := e.ExternalObjects.GetExternalObject(ctx, tenant, key)
	if errors.Is(err, domain.ErrNotFound) {
		return "", objectstore.ErrNotFound
	}
	return id, err
}

// Aliases keep the service error contract stable for HTTP and callers.
var (
	ErrPresignUnsupported = objectstore.ErrPresignUnsupported
	ErrDestinoNoPermitido = objectstore.ErrDestinoNoPermitido
)
