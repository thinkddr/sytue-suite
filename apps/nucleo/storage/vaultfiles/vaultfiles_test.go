// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultfiles

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

var scope = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "drive"}

type banco struct {
	fake    *pqvaulttest.Fake
	gateway *api.Drive
	store   *Store
	journal *offline.Store
}

func nuevoBanco(t *testing.T) banco {
	t.Helper()
	fake, drive, account := pqvaulttest.New(t, scope)
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scope, Server: drive, Registry: account, UnlockKey: bytes.Repeat([]byte{7}, recoverywrap.UnlockKeyLen)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	store, err := New(vault)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	return banco{fake: fake, gateway: drive, store: store, journal: journal}
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Un fichero de dos bloques sube cifrado, se baja igual, se lee por rangos y su
// diario de subida solo se retira cuando el catálogo lo retiene.
func TestFileOnTheVaultRoundTrips(t *testing.T) {
	b := nuevoBanco(t)
	ctx := t.Context()
	data := bytes.Repeat([]byte("sytue-boveda "), int(storage.MaxCipherBlockBytes/13)+200)
	request := storage.FileRequest{ObjectID: "file_1", RevisionID: "rev_1", SizeBytes: int64(len(data)), Metadata: []byte(`{"name":"informe.txt"}`)}
	target := b.store.PublicationTarget()
	ref, err := b.store.UploadStorageFileAt(ctx, b.gateway, b.journal, "job_1", target, request, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.fake.Blobs.Data) != 2 {
		t.Fatalf("dos bloques cifrados, no %d", len(b.fake.Blobs.Data))
	}
	for _, blob := range b.fake.Blobs.Data {
		if bytes.Contains(blob, []byte("sytue-boveda")) {
			t.Fatal("el servidor de bloques ve el texto en claro")
		}
	}
	var out bytes.Buffer
	got, err := b.store.DownloadStorageFile(ctx, b.gateway, ref, "file_1", "rev_1", &out)
	if err != nil || !bytes.Equal(out.Bytes(), data) || got.SizeBytes != request.SizeBytes {
		t.Fatalf("el fichero no vuelve igual: %v", err)
	}
	info, sum, err := b.store.StorageFileCommitment(ctx, ref, "file_1", "rev_1")
	if err != nil || sum != digest(data) || !bytes.Equal(info.Metadata, request.Metadata) {
		t.Fatalf("el compromiso no es el del fichero: %v", err)
	}
	offset := storage.MaxCipherBlockBytes - 6
	part, err := b.store.ReadStorageFileRange(ctx, b.gateway, ref, "file_1", "rev_1", offset, 4)
	if err != nil || !bytes.Equal(part, data[offset:offset+4]) {
		t.Fatalf("lectura por rangos: %q %v", part, err)
	}

	// El diario sigue hasta que el catálogo retiene este fichero exacto.
	if err := b.store.AcknowledgeStorageFile(ctx, b.journal, "job_1", ref); err == nil {
		t.Fatal("el manifiesto solo no es un catálogo que lo retenga")
	}
	if count, _ := b.journal.Count(storage.PendingUploadsNamespace); count != 1 {
		t.Fatal("el diario de subida se perdió antes de tiempo")
	}
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "file", RevisionID: "rev_1", Reference: ref}}
	version, err := b.store.PutStorageObjectAt(ctx, target, "txn_index_1", "file_1", "rev_1", commit)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.store.AcknowledgeStorageFile(ctx, b.journal, "job_1", version.Reference); err != nil {
		t.Fatal(err)
	}
	if count, _ := b.journal.Count(storage.PendingUploadsNamespace); count != 0 {
		t.Fatal("confirmado, el diario se retira")
	}
}

func TestFileContentMustBeAnAuthenticManifest(t *testing.T) {
	b := nuevoBanco(t)
	ctx := t.Context()
	target := b.store.PublicationTarget()
	data := []byte("hola")
	ref, err := b.store.UploadStorageFileAt(ctx, b.gateway, b.journal, "job_1", target, storage.FileRequest{ObjectID: "file_1", RevisionID: "rev_1", SizeBytes: 4, Metadata: []byte(`{}`)}, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	commit := func(revision string, ref storage.Reference) storage.ObjectCommit {
		return storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "file", RevisionID: revision, Reference: ref}}
	}
	if _, err := b.store.PutStorageObjectAt(ctx, target, "txn_a", "file_1", "rev_2", commit("rev_2", ref)); err == nil {
		t.Fatal("un puntero a otra revisión del manifiesto no se publica")
	}
	if _, err := b.store.PutStorageObjectAt(ctx, target, "txn_b", "file_2", "rev_1", commit("rev_1", ref)); err == nil {
		t.Fatal("un puntero al manifiesto de otro objeto no se publica")
	}
	// Sin la capa de ficheros, la bóveda sola nunca acepta un contenido de fichero.
	if _, err := b.store.Vault.PutStorageObjectAt(ctx, target, "txn_c", "file_1", "rev_1", commit("rev_1", ref)); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("la bóveda sin ficheros aceptó un fichero", err)
	}
	if _, err := b.store.PutStorageObjectAt(ctx, target, "txn_d", "file_1", "rev_1", commit("rev_1", ref)); err != nil {
		t.Fatal(err)
	}
}

func TestTamperedBlockIsRejected(t *testing.T) {
	b := nuevoBanco(t)
	ctx := t.Context()
	ref, err := b.store.UploadStorageFileAt(ctx, b.gateway, b.journal, "job_1", b.store.PublicationTarget(), storage.FileRequest{ObjectID: "file_1", RevisionID: "rev_1", SizeBytes: 5, Metadata: []byte(`{}`)}, bytes.NewReader([]byte("datos")))
	if err != nil {
		t.Fatal(err)
	}
	b.fake.Blobs.Tamper = true
	var out bytes.Buffer
	if _, err := b.store.DownloadStorageFile(ctx, b.gateway, ref, "file_1", "rev_1", &out); err == nil {
		t.Fatal("un bloque alterado no se entrega")
	}
}

// Otra intención (otra bóveda) no se redirige: la subida no empieza.
func TestUploadIsPinnedToTheVault(t *testing.T) {
	b := nuevoBanco(t)
	other := storage.PublicationTarget{RoomID: "!otra", DeviceID: b.store.PublicationTarget().DeviceID}
	if _, err := b.store.UploadStorageFileAt(t.Context(), b.gateway, b.journal, "job_1", other, storage.FileRequest{ObjectID: "file_1", RevisionID: "rev_1", SizeBytes: 1, Metadata: []byte(`{}`)}, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("se subió hacia otra bóveda")
	}
	if b.fake.Blobs.Puts != 0 {
		t.Fatal("ni un bloque antes de comprobar el destino")
	}
}
