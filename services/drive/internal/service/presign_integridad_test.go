// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// blobsQueCambian es un almacén en el que quien subió el fichero VUELVE a escribir
// en su clave —la URL de subida sigue viva un cuarto de hora— en cuanto el
// servicio la ha leído o copiado una vez.
type blobsQueCambian struct {
	*mapBlobs
	clave   string
	despues []byte
	hecho   bool
}

func (b *blobsQueCambian) reescribir() {
	if !b.hecho {
		b.hecho = true
		b.data[b.clave] = b.despues
	}
}

func (b *blobsQueCambian) PresignPut(_ context.Context, _ tenancy.Tenant, key string, _ time.Duration) (*url.URL, error) {
	b.clave = key
	return url.Parse("https://s3.example.test/bucket/key?X-Amz-Signature=abc")
}

type alCerrar struct {
	io.ReadCloser
	despues func()
}

func (a *alCerrar) Close() error {
	err := a.ReadCloser.Close()
	a.despues()
	return err
}

func (b *blobsQueCambian) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	rc, err := b.mapBlobs.Get(ctx, t, key)
	if err != nil || key != b.clave {
		return rc, err
	}
	return &alCerrar{ReadCloser: rc, despues: b.reescribir}, nil
}

func (b *blobsQueCambian) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	err := b.mapBlobs.Copiar(ctx, t, origen, destino)
	if origen == b.clave && b.data[b.clave] != nil && !bytes.Equal(b.data[b.clave], b.despues) {
		b.reescribir()
	}
	return err
}

// FILE-04. La clave por contenido `shared/drive/<organización>/<sha256>` es común a toda la
// organización: lo que haya debajo tiene que ser lo que dice su nombre. Quien
// sube por URL prefirmada puede reescribir su objeto después de que el servicio
// lo haya medido, así que nada de lo que se mida puede salir de esa clave.
func TestFinalizeNoComparteLoQueCambiaDespuesDeMedirlo(t *testing.T) {
	legitimo := []byte("un documento que cualquiera puede subir")
	otro := bytes.Repeat([]byte("x"), len(legitimo))
	suma := sha256.Sum256(legitimo)
	deLegitimo := "shared/drive/acme/" + hex.EncodeToString(suma[:])

	fs := &fakeStore{
		root:     domain.Folder{ID: uuid.New(), IsRoot: true},
		quota:    domain.Quota{LimitBytes: 1000},
		byKeyErr: domain.ErrNotFound,
		file:     domain.File{ID: uuid.New(), Name: "conocido.pdf"},
	}
	mb := &blobsQueCambian{mapBlobs: newMapBlobs(), despues: otro}
	s := newSvc(fs, mb, nil)
	ctx := tenantCtx(t)

	res, err := s.PresignUpload(ctx, "/conocido.pdf", int64(len(legitimo)), "application/pdf", "user-1")
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	mb.data[mb.clave] = legitimo // la subida directa
	if _, err := s.FinalizeUpload(ctx, res.Handle); err != nil {
		t.Fatalf("FinalizeUpload: %v", err)
	}

	if got, hay := mb.data[deLegitimo]; hay && !bytes.Equal(got, legitimo) {
		t.Fatalf("la clave por contenido del documento guarda otra cosa: %q", got)
	}
	if fs.newFile == nil {
		t.Fatal("no se registró el fichero")
	}
	registrado := sha256.Sum256(mb.data[fs.newFile.StorageKey])
	if hex.EncodeToString(registrado[:]) != fs.newFile.Checksum {
		t.Fatalf("la fila apunta a %q, cuyo contenido no casa con su checksum %q", fs.newFile.StorageKey, fs.newFile.Checksum)
	}
	if int64(len(mb.data[fs.newFile.StorageKey])) != fs.newFile.SizeBytes {
		t.Fatalf("la fila dice %d bytes y el objeto tiene %d", fs.newFile.SizeBytes, len(mb.data[fs.newFile.StorageKey]))
	}
}
