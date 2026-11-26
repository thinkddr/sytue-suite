// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Las tres capacidades de ficheros retenidos que pide Photos: la huella del fichero en
// disco, la lectura por trozos y el acuse.
func TestElPuertoSirveFicherosRetenidos(t *testing.T) {
	dir := t.TempDir()
	scope := storage.Scope{Tenant: "local", Subject: "instalacion-1", Product: "photos"}
	p, err := almacen.AbrirPuerto(dir, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Cerrar()
	enlace, err := p.StorageBindings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	datos := bytes.Repeat([]byte("0123456789"), 300)
	req := storage.FileRequest{ObjectID: uuid.NewString(), RevisionID: uuid.NewString(), SizeBytes: int64(len(datos)), Metadata: []byte(`{}`)}
	ref, err := p.UploadStorageFileAt(t.Context(), nil, nil, "trabajo-1", enlace.Target, req, bytes.NewReader(datos))
	if err != nil {
		t.Fatal("subir:", err)
	}
	suma := sha256.Sum256(datos)
	info, huella, err := p.StorageFileCommitment(t.Context(), ref, req.ObjectID, req.RevisionID)
	if err != nil || huella != hex.EncodeToString(suma[:]) || info.SizeBytes != req.SizeBytes {
		t.Fatal("compromiso:", info, huella, err)
	}
	trozo, err := p.ReadStorageFileRange(t.Context(), nil, ref, req.ObjectID, req.RevisionID, 2995, 100)
	if err != nil || string(trozo) != "56789" {
		t.Fatalf("trozo final %q %v", trozo, err)
	}
	trozo, err = p.ReadStorageFileRange(t.Context(), nil, ref, req.ObjectID, req.RevisionID, 3, 4)
	if err != nil || string(trozo) != "3456" {
		t.Fatalf("trozo %q %v", trozo, err)
	}
	if _, err := p.ReadStorageFileRange(t.Context(), nil, ref, req.ObjectID, req.RevisionID, 4000, 1); !errors.Is(err, almacen.ErrRango) {
		t.Fatal("fuera del fichero:", err)
	}
	if _, err := p.ReadStorageFileRange(t.Context(), nil, ref, req.ObjectID, "otra", 0, 1); !errors.Is(err, almacen.ErrNoEsta) {
		t.Fatal("otra revisión:", err)
	}
	if err := p.AcknowledgeStorageFileLink(t.Context(), nil, "trabajo-1", ref); err != nil {
		t.Fatal(err)
	}
	// Un objeto cambiado por fuera ya no da la huella anotada.
	ruta := filepath.Join(dir, "objetos", huella[:2], huella)
	if err := os.WriteFile(ruta, []byte("otra cosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, otra, err := p.StorageFileCommitment(t.Context(), ref, req.ObjectID, req.RevisionID); err != nil || otra == huella {
		t.Fatal("el compromiso no ve el cambio:", otra, err)
	}
}
