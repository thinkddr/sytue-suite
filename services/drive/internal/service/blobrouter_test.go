// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type fakeExternal struct {
	cfg map[string]store.ExternalStorage
	n   int
	err error
}

func (*fakeExternal) PutExternalObject(context.Context, string, string, string) error { return nil }
func (*fakeExternal) GetExternalObject(context.Context, string, string) (string, error) {
	return "", domain.ErrNotFound
}
func (*fakeExternal) DeleteExternalObject(context.Context, string, string) error { return nil }

func (f *fakeExternal) ExternalStorageFor(_ context.Context, tenant string) (store.ExternalStorage, error) {
	f.n++
	if f.err != nil {
		return store.ExternalStorage{}, f.err
	}
	if c, ok := f.cfg[tenant]; ok {
		return c, nil
	}
	return store.ExternalStorage{}, domain.ErrNotFound
}

// El reparto por clave es lo que decide dónde acaban los bytes y quién paga la
// cuota: si falla, los ficheros «externos» acaban en Sytue o dejan de leerse.
func TestElEnrutadorRepartePorClave(t *testing.T) {
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = byte(i)
	}
	c, _ := keycrypto.New(kek)
	sellada, _ := c.Encrypt("secreta")
	ext := &fakeExternal{cfg: map[string]store.ExternalStorage{
		"acme": {Endpoint: "s3.ejemplo.com", Bucket: "acme-drive", UseSSL: true, AccessKey: "ak", SecretKeyEnc: sellada, Enabled: true},
	}}
	base := &fakeBlobs{}
	r := newBlobRouter(base, ext, base64.StdEncoding.EncodeToString(kek), nil)
	ctx := context.Background()
	acme, otra := tenancy.Tenant{Slug: "acme"}, tenancy.Tenant{Slug: "otra"}

	if !r.Enabled(ctx, acme) || r.Enabled(ctx, otra) {
		t.Fatal("solo acme tiene bucket propio")
	}
	if b, _ := r.pick(ctx, acme, "carpeta/fichero"); b != base {
		t.Fatal("una clave sin prefijo va al almacén de Sytue, tenga o no bucket propio")
	}
	if b, err := r.pick(ctx, acme, store.ExternalPrefix+"carpeta/fichero"); err != nil || b == base {
		t.Fatalf("una clave con prefijo va al bucket propio: %v", err)
	}
	if _, err := r.pick(ctx, otra, store.ExternalPrefix+"x"); !errors.Is(err, ErrExternalStorageOff) {
		t.Fatalf("sin bucket propio, una clave externa no se puede servir: %v", err)
	}
	// La caché evita releer el store; Forget la vacía.
	antes := ext.n
	r.Enabled(ctx, acme)
	if ext.n != antes {
		t.Fatal("la segunda consulta debe salir de la caché")
	}
	r.Forget("acme")
	r.Enabled(ctx, acme)
	if ext.n != antes+1 {
		t.Fatal("tras Forget se vuelve a leer el store")
	}
	// Sin KEK no se puede abrir la clave sellada de un S3 de terceros.
	apagado := newBlobRouter(base, ext, "", nil)
	if apagado.Enabled(ctx, acme) {
		t.Fatal("sin KEK no se abre un bucket de terceros con clave sellada")
	}
}

// Los ficheros del almacén de terceros NO se deduplican en shared/drive/:
// deben quedarse intactos en el bucket del cliente.
func TestCompartirBlobNoDeduplicaClavesExternas(t *testing.T) {
	s := &Service{}
	ctx := context.Background()
	tenant := tenancy.Tenant{Slug: "acme"}
	externa := store.ExternalPrefix + "docs/contrato.pdf"
	hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	got := s.compartirBlob(ctx, tenant, externa, hash)
	if got != externa {
		t.Fatalf("compartirBlob(%q) = %q; esperaba que la clave externa no se deduplicara", externa, got)
	}
}

func TestUnavailableExternalStorageNeverFallsBackForNewUploads(t *testing.T) {
	for _, scenario := range []struct {
		name string
		err  error
		cfg  store.ExternalStorage
	}{
		{name: "configuration unavailable", err: errors.New("configuration unavailable")},
		{name: "missing KEK", cfg: store.ExternalStorage{SecretKeyEnc: "encv1:fixture", UseSSL: true}},
		{name: "missing OAuth connector", cfg: store.ExternalStorage{Kind: "gdrive", InstanceID: "connected-account"}},
		{name: "unsafe external endpoint", cfg: store.ExternalStorage{Kind: "s3", UseSSL: false}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, presign := range []bool{false, true} {
				s, fs, base := presignSvc(t)
				scenario.cfg.Enabled = true
				ext := &fakeExternal{err: scenario.err, cfg: map[string]store.ExternalStorage{"acme": scenario.cfg}}
				router := newBlobRouter(base, ext, "", nil)
				s.router, s.blobs = router, router
				body := strings.NewReader("private bytes")
				var err error
				if presign {
					_, err = s.PresignUpload(tenantCtx(t), "/private.bin", 50, "application/octet-stream", "ada")
				} else {
					_, err = s.UploadFile(tenantCtx(t), uuid.New(), "private.bin", "application/octet-stream", "ada", body)
				}
				if err == nil {
					t.Fatalf("unavailable external storage silently used Sytue (presign=%v)", presign)
				}
				if !errors.Is(err, ErrExternalStorageOff) || (scenario.err != nil && !errors.Is(err, scenario.err)) {
					t.Fatalf("destination error lost its safe classification or original cause: %v", err)
				}
				if body.Len() != len("private bytes") || len(base.putBody) != 0 || base.lastPutKey != "" || fs.newFile != nil || fs.uploadReservations != 0 {
					t.Fatalf("failed destination consumed or stored content (presign=%v)", presign)
				}
			}
		})
	}
}

func TestNewBlobDestinationDistinguishesMissingStorageFromFailedStorage(t *testing.T) {
	base := &fakeBlobs{}
	ext := &fakeExternal{cfg: map[string]store.ExternalStorage{}}
	s := &Service{router: newBlobRouter(base, ext, "", nil)}
	ctx := tenantCtx(t)
	tenant, _ := tenancy.From(ctx)
	key, err := s.claveDeBlob(ctx, tenant, uuid.New(), "private.bin")
	if err != nil || key == "" || store.EsExterno(key) {
		t.Fatalf("a tenant without external storage cannot use its own storage: %s %v", key, err)
	}
	s.router.Forget(tenant.Slug)
	ext.cfg[tenant.Slug] = store.ExternalStorage{Kind: "gdrive", InstanceID: "connected-account"}
	s.router.conn = &integrationsClient{secret: "fixture"}
	key, err = s.claveDeBlob(ctx, tenant, uuid.New(), "private.bin")
	if err != nil || !store.EsExterno(key) {
		t.Fatalf("connected storage did not retain its destination: %s %v", key, err)
	}
}

func TestElIndiceCompartidoTraduceLaAusenciaDelDominio(t *testing.T) {
	index := externalObjectIndex{&fakeExternal{}}
	if _, err := index.GetExternalObject(context.Background(), "acme", "ext/blob"); !errors.Is(err, objectstore.ErrNotFound) {
		t.Fatalf("el adaptador compartido necesita su error de ausencia: %v", err)
	}
}
