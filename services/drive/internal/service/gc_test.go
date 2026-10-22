// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// svcWithRetention construye un Service con la retención dada y stores falsos.
func svcWithRetention(fs *fakeStore, blobs blobStore, retention time.Duration) *Service {
	return &Service{
		cfg:   config.Config{TrashRetention: retention},
		store: fs,
		blobs: blobs,
	}
}

// TestPurgeExpiredTrashDeletesOnlyOrphanBlobs comprueba la GUARDA DE BLOB
// COMPARTIDO en la auto-purga: solo se borra de S3 el blob HUÉRFANO; el que una
// copia/versión viva aún referencia (Orphaned=false) se conserva. Y cada borrado va
// bajo SU tenant, deduplicado por (tenant, clave).
func TestPurgeExpiredTrashDeletesOnlyOrphanBlobs(t *testing.T) {
	fs := &fakeStore{gcBatches: [][]store.PurgedBlob{{
		{Tenant: "acme", StorageKey: "k-orphan", Orphaned: true},
		{Tenant: "acme", StorageKey: "k-shared", Orphaned: false},  // copia/versión viva: NO borrar
		{Tenant: "acme", StorageKey: "k-orphan", Orphaned: true},   // duplicado: borrar una vez
		{Tenant: "globex", StorageKey: "k-orphan", Orphaned: true}, // otro tenant, misma clave
	}}}
	blobs := &fakeBlobs{}
	s := svcWithRetention(fs, blobs, 30*24*time.Hour)

	n, err := s.PurgeExpiredTrash(tenantCtx(t))
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if n != 4 {
		t.Fatalf("purgados=%d, quiero 4 (elementos de la única llamada)", n)
	}
	// k-shared NUNCA se borra; k-orphan de acme una vez (dedup); k-orphan de globex una vez.
	if _, borrado := containsKey(blobs.delKeys, "k-shared"); borrado {
		t.Fatal("el blob compartido (Orphaned=false) NO debe borrarse de S3")
	}
	if len(blobs.delKeys) != 2 {
		t.Fatalf("borrados=%v, quiero 2 (k-orphan de acme + k-orphan de globex, dedup por tenant)", blobs.delKeys)
	}
}

func containsKey(keys []string, want string) (int, bool) {
	for i, k := range keys {
		if k == want {
			return i, true
		}
	}
	return 0, false
}

// TestPurgeExpiredTrashRetentionCutoff comprueba que el cutoff es now-retención (la
// retención se respeta: solo caduca lo más antiguo que la retención).
func TestPurgeExpiredTrashRetentionCutoff(t *testing.T) {
	fs := &fakeStore{}
	s := svcWithRetention(fs, &fakeBlobs{}, 30*24*time.Hour)

	before := time.Now().Add(-30 * 24 * time.Hour)
	if _, err := s.PurgeExpiredTrash(tenantCtx(t)); err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	after := time.Now().Add(-30 * 24 * time.Hour)
	if fs.gcCalls != 1 {
		t.Fatalf("gcCalls=%d, quiero 1", fs.gcCalls)
	}
	if fs.gcLastCut.Before(before.Add(-time.Second)) || fs.gcLastCut.After(after.Add(time.Second)) {
		t.Fatalf("cutoff=%v fuera de [now-30d ± 1s]", fs.gcLastCut)
	}
}

// TestPurgeExpiredTrashDisabled comprueba que una retención <= 0 desactiva la purga
// (defensa: nunca purga la papelera entera por una config a cero).
func TestPurgeExpiredTrashDisabled(t *testing.T) {
	fs := &fakeStore{gcBatches: [][]store.PurgedBlob{{{Tenant: "acme", StorageKey: "k", Orphaned: true}}}}
	blobs := &fakeBlobs{}
	s := svcWithRetention(fs, blobs, 0)

	n, err := s.PurgeExpiredTrash(tenantCtx(t))
	if err != nil || n != 0 {
		t.Fatalf("PurgeExpiredTrash con retención 0 = (%d,%v), quiero (0,nil)", n, err)
	}
	if fs.gcCalls != 0 {
		t.Fatal("con retención <= 0 no debe llamar al store")
	}
	if len(blobs.delKeys) != 0 {
		t.Fatal("con retención <= 0 no debe borrar blobs")
	}
}

// TestPurgeExpiredTrashDrains comprueba que el barrido DRENA en lotes: si un ciclo
// llena el lote (claimed == trashGCBatch) sigue; para cuando un lote no se llena.
func TestPurgeExpiredTrashDrains(t *testing.T) {
	full := make([]store.PurgedBlob, trashGCBatch) // lote lleno (claimed == batch) => sigue
	fs := &fakeStore{gcBatches: [][]store.PurgedBlob{
		full,
		{{Tenant: "acme", StorageKey: "k", Orphaned: true}}, // lote parcial => para
	}}
	s := svcWithRetention(fs, &fakeBlobs{}, time.Hour)

	n, err := s.PurgeExpiredTrash(tenantCtx(t))
	if err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if fs.gcCalls != 2 {
		t.Fatalf("gcCalls=%d, quiero 2 (drenaje en dos lotes)", fs.gcCalls)
	}
	if n != trashGCBatch+1 {
		t.Fatalf("purgados=%d, quiero %d", n, trashGCBatch+1)
	}
}

// TestPurgeExpiredTrashBarreSubidasCaducadasYFirme (FILE-08): el barrido limpia
// tanto la clave reservada como su copia temporal .firme si un finalize quedó a
// medias, respetando las claves que una fila viva aún referencie.
func TestPurgeExpiredTrashBarreSubidasCaducadasYFirme(t *testing.T) {
	fs := &fakeStore{
		byKeyErr: store.ErrDemasiadasSubidas, // distinto de nil → no referenciada
		caducadasBatches: [][]store.SubidaCaducada{{{
			Tenant:     "acme",
			StorageKey: "files/root/u1",
		}}},
	}
	blobs := &fakeBlobs{}
	s := svcWithRetention(fs, blobs, 0)

	if _, err := s.PurgeExpiredTrash(tenantCtx(t)); err != nil {
		t.Fatalf("PurgeExpiredTrash: %v", err)
	}
	if len(blobs.delKeys) != 2 || blobs.delKeys[0] != "files/root/u1" || blobs.delKeys[1] != "files/root/u1.firme" {
		t.Fatalf("delKeys=%v, quiero [files/root/u1 files/root/u1.firme]", blobs.delKeys)
	}
}
