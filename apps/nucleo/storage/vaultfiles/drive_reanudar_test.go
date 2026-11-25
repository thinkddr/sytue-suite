// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultfiles

// Lo que dejan en la bóveda las importaciones que se cortan o se cruzan (v3.24.0,
// 2026-11-24): un nativo congeló la fuente con su época, publicó un fichero entero
// y el cuerpo del siguiente (351 641 bytes) y se paró. Ni la web ni otro
// dispositivo (ni este, sin su diario) abrían ya Drive. Y dos importaciones de la
// misma época a la vez dejaban dos copias de las filas, o dos recibos: conflicto
// para siempre.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// fuenteFilas es la fuente del servidor con varias filas y sus originales, en
// páginas de 50 como `/private-storage/migration/{epoch}/rows`.
type fuenteFilas struct {
	mu     sync.Mutex
	state  storage.LegacyMigrationState
	rows   []storage.LegacySourceRow
	blobs  map[string][]byte
	begins int
	opens  int
}

func (f *fuenteFilas) PrivateStorageState(_ context.Context, s storage.Scope) (storage.LegacyMigrationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s != scope {
		return storage.LegacyMigrationState{}, drive.ErrSource
	}
	return f.state, nil
}

func (f *fuenteFilas) BeginPrivateStorageMigration(_ context.Context, s storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begins++
	if s != scope || !storage.MigrationEpoch(epoch) {
		return storage.LegacyMigrationState{}, drive.ErrSource
	}
	if f.state.Mode == "legacy" {
		now := time.Date(2026, 11, 24, 22, 10, 0, 0, time.UTC)
		var blobs int64
		for _, row := range f.rows {
			if row.Kind == "file" || row.Kind == "version" {
				blobs++
			}
		}
		f.state = storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: int64(len(f.rows)), Blobs: blobs}
	}
	return f.state, nil
}

func (f *fuenteFilas) PrivateStorageSourcePage(_ context.Context, s storage.Scope, epoch string, cursor storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s != scope || epoch != f.state.Epoch {
		return storage.LegacySourcePage{}, drive.ErrSource
	}
	page := storage.LegacySourcePage{Scope: s, Epoch: epoch, Rows: []storage.LegacySourceRow{}}
	for _, row := range f.rows {
		if cursor.Kind != "" && (row.Kind < cursor.Kind || row.Kind == cursor.Kind && row.ID <= cursor.ID) {
			continue
		}
		if len(page.Rows) == storage.MaxLegacySourceRows {
			last := page.Rows[len(page.Rows)-1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, storage.LegacySourceRow{Kind: row.Kind, ID: row.ID, Payload: bytes.Clone(row.Payload)})
	}
	return page, nil
}

func (f *fuenteFilas) OpenPrivateStorageSource(_ context.Context, s storage.Scope, epoch, kind, id string, size int64) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	data, ok := f.blobs[kind+"/"+id]
	if s != scope || epoch != f.state.Epoch || !ok || int64(len(data)) != size {
		return nil, drive.ErrSource
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

var ficherosProduccion = []struct {
	id   string
	size int
}{
	{"0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f60", 1117},
	{"0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f61", 351641},
	{"0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f62", 2048},
}

// fuenteComoEnProduccion: filas con la forma de `to_jsonb` de PostgreSQL (espacios,
// marcas de tiempo con zona), dos ficheros como los del registro de producción y
// otro detrás, una versión y un enlace compartido.
func fuenteComoEnProduccion() *fuenteFilas {
	stamp := `"created_at": "2026-01-01T10:00:00.123456+00:00", "updated_at": "2026-01-02T10:00:00.5+00:00"`
	f := &fuenteFilas{state: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}, blobs: map[string][]byte{}}
	add := func(kind, id, payload string) {
		f.rows = append(f.rows, storage.LegacySourceRow{Kind: kind, ID: id, Payload: []byte(payload)})
	}
	add("folder", raizID, `{"id": "`+raizID+`", "name": "", "is_root": true, "owner_sub": "persona-1", "parent_id": null, `+stamp+`}`)
	add("folder", carpetaID, `{"id": "`+carpetaID+`", "name": "Documentos", "is_root": false, "owner_sub": "persona-1", "parent_id": "`+raizID+`", `+stamp+`}`)
	for i, file := range ficherosProduccion {
		data := bytes.Repeat([]byte{byte('a' + i)}, file.size)
		sum := sha256.Sum256(data)
		f.blobs["file/"+file.id] = data
		add("file", file.id, `{"id": "`+file.id+`", "name": "f`+itoa(i)+`.bin", "version": 1, "checksum": "`+hex.EncodeToString(sum[:])+`", "folder_id": "`+carpetaID+`", "owner_sub": "persona-1", "size_bytes": `+itoa(file.size)+`, "content_type": "application/octet-stream", `+stamp+`}`)
	}
	version := "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f70"
	f.blobs["version/"+version] = bytes.Repeat([]byte("v"), 333)
	add("version", version, `{"id": "`+version+`", "file_id": "`+ficherosProduccion[0].id+`", "version": 1, "checksum": "", "size_bytes": 333, "content_type": "text/plain", "created_at": "2026-01-01T09:00:00+00:00"}`)
	add("share", "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f80", `{"id": "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5f80", "file_id": "`+ficherosProduccion[0].id+`", "kind": "link", "permission": "read", "created_at": "2026-01-01T09:00:00+00:00"}`)
	sort.Slice(f.rows, func(i, j int) bool {
		a, b := f.rows[i], f.rows[j]
		return a.Kind < b.Kind || a.Kind == b.Kind && a.ID < b.ID
	})
	return f
}

func otroDiario(t *testing.T) *offline.Store {
	t.Helper()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func repositorioCon(t *testing.T, b *bovedaDrive, fuente drive.Source, journal *offline.Store) *drive.Repository {
	t.Helper()
	repo, err := drive.New(drive.Config{Context: t.Context(), Scope: scope, Crypto: b.store, Gateway: b.gateway, Source: fuente, Journal: journal, DirectImport: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	return repo
}

func comprobarFuente(t *testing.T, repo *drive.Repository, f *fuenteFilas) {
	t.Helper()
	root, err := repo.Root(t.Context())
	if err != nil || root.ID != raizID {
		t.Fatalf("la raíz no es la del servidor: %+v %v", root, err)
	}
	for _, file := range ficherosProduccion {
		if !bytes.Equal(leerFichero(t, repo, file.id), f.blobs["file/"+file.id]) {
			t.Fatalf("el fichero %s no vuelve igual", file.id)
		}
	}
}

// importarCortado congela la fuente y se para como en producción: con el primer
// fichero entero y el cuerpo del segundo publicados (seis registros).
func importarCortado(t *testing.T, b *bovedaDrive, f *fuenteFilas) {
	t.Helper()
	b.fake.FailAppend = func(records int) bool { return records == 6 }
	defer func() { b.fake.FailAppend = nil }()
	if err := b.repositorio(t, f).Initialize(t.Context()); err == nil {
		t.Fatal("la importación cortada no puede darse por terminada")
	}
	if b.fake.RecordCount() != 6 || f.state.Mode != "frozen" {
		t.Fatalf("el corte no dejó lo de producción: %d registros, fuente %s", b.fake.RecordCount(), f.state.Mode)
	}
}

// El estado de producción: fuente congelada por otro (o por este dispositivo, que
// perdió su diario) y media importación sin recibo. Antes, ErrNotReady para
// siempre; ahora se termina, sin congelar otra época ni escribir dos veces lo que
// ya estaba.
func TestDirectImportTerminaLaImportacionAMediasDeOtroDiario(t *testing.T) {
	f := fuenteComoEnProduccion()
	b := nuevaBovedaDrive(t)
	importarCortado(t, b, f)
	epoca := f.state.Epoch
	repo := repositorioCon(t, b, f, otroDiario(t))
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal("la importación a medias de la época congelada no se terminó:", err)
	}
	if f.begins != 1 || f.state.Epoch != epoca {
		t.Fatal("se congeló otra época", f.begins)
	}
	comprobarFuente(t, repo, f)
	if _, err := repo.PrivateMigrationReceipt(t.Context()); err != nil {
		t.Fatal("la bóveda no tiene el recibo de la época", err)
	}
	// Y el dispositivo que la empezó (con su diario) la abre ya hecha.
	registros := b.fake.RecordCount()
	if err := b.repositorio(t, f).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.fake.RecordCount() != registros {
		t.Fatal("abrir la importación terminada volvió a escribir")
	}
}

// Con la marca de una copia de Matrix en la bóveda, lo que hay sin recibo puede ser
// esa copia: no se termina encima con la fuente del servidor.
func TestDirectImportNoAdoptaConMarcaDeCopia(t *testing.T) {
	f := fuenteComoEnProduccion()
	b := nuevaBovedaDrive(t)
	importarCortado(t, b, f)
	b.fake.Copy = &api.PQVaultCopy{ID: "copia-1", Holder: "otro", Kind: api.PQVaultCopyKind, Epoch: f.state.Epoch, StartedAt: time.Now(), ExpiresAt: time.Now().Add(-time.Minute)}
	registros := b.fake.RecordCount()
	err := repositorioCon(t, b, f, otroDiario(t)).Initialize(t.Context())
	if !errors.Is(err, drive.ErrNotReady) || drive.ImportCheck(err) != "foreign_epoch" {
		t.Fatal("se terminó encima de una copia marcada", err)
	}
	if b.fake.RecordCount() != registros {
		t.Fatal("rechazarla escribió en la bóveda")
	}
}

// duplicar publica otra copia idéntica de la cabeza de `object`, sin padres: lo
// que deja otra importación de la misma época que corría a la vez.
func duplicar(t *testing.T, b *bovedaDrive, object string, cambiar func([]byte) []byte) {
	t.Helper()
	ctx := t.Context()
	page, err := b.vault.History(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	var head *storage.ObjectVersion
	for _, entry := range page.Entries {
		if entry.Record.Role == "index" && entry.Record.ObjectID == object {
			version, err := storage.DecodeObject(entry, scope)
			if err != nil {
				t.Fatal(err)
			}
			head = &version
			break
		}
	}
	if head == nil {
		t.Fatal("no está el objeto", object)
	}
	content, err := b.vault.ReadStorageRecord(ctx, head.Commit.Content.Reference, object, "content")
	if err != nil {
		t.Fatal(err)
	}
	metadata := head.Commit.Metadata
	if cambiar != nil {
		metadata = cambiar(metadata)
		content.Payload = metadata
	}
	revision := "otra_importacion_" + itoa(b.fake.RecordCount())
	target := b.vault.PublicationTarget()
	ref, err := b.store.PutStorageRecordAt(ctx, target, revision+"_content", storage.Record{Version: 1, Scope: scope, ObjectID: object, RevisionID: revision, Role: "content", Payload: content.Payload})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.store.PutStorageObjectAt(ctx, target, revision+"_index", object, revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: head.Commit.State, Metadata: metadata, Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: ref}}); err != nil {
		t.Fatal(err)
	}
}

func objeto(kind, id string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + id))
	return "drive_" + hex.EncodeToString(sum[:])
}

func recibo(epoch string) string {
	sum := sha256.Sum256([]byte(epoch))
	return "drive_migration_" + hex.EncodeToString(sum[:])
}

// Dos importaciones de la misma época a la vez: dos copias idénticas de la raíz y
// de un fichero, y dos recibos. Antes, conflicto para siempre (nadie abría); ahora
// se juntan en una revisión con las dos por padres y Drive abre.
func TestDirectImportJuntaLasCopiasIdenticasDeDosImportaciones(t *testing.T) {
	f := fuenteComoEnProduccion()
	b := nuevaBovedaDrive(t)
	if err := b.repositorio(t, f).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	duplicar(t, b, objeto("folder", raizID), nil)
	duplicar(t, b, objeto("file", ficherosProduccion[1].id), nil)
	duplicar(t, b, recibo(f.state.Epoch), nil)
	repo := repositorioCon(t, b, f, otroDiario(t))
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal("las copias idénticas de dos importaciones no se juntaron:", err)
	}
	comprobarFuente(t, repo, f)
	// Juntas: otra apertura ya no escribe nada.
	registros := b.fake.RecordCount()
	if err := repositorioCon(t, b, f, otroDiario(t)).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.fake.RecordCount() != registros {
		t.Fatal("las copias se volvieron a juntar: no quedaron juntas")
	}
}

// Dos recibos de la misma época con inventarios distintos no son la misma lectura
// de la fuente: siguen siendo un conflicto, con su código.
func TestDirectImportRecibosDistintosSiguenEnConflicto(t *testing.T) {
	f := fuenteComoEnProduccion()
	b := nuevaBovedaDrive(t)
	if err := b.repositorio(t, f).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	duplicar(t, b, recibo(f.state.Epoch), func(metadata []byte) []byte {
		var proof map[string]any
		if err := json.Unmarshal(metadata, &proof); err != nil {
			t.Fatal(err)
		}
		proof["inventory_sha256"] = hex.EncodeToString(make([]byte, 32))
		out, _ := json.Marshal(proof)
		return out
	})
	err := repositorioCon(t, b, f, otroDiario(t)).Initialize(t.Context())
	if !errors.Is(err, drive.ErrConflict) || drive.ImportCheck(err) != "receipt" {
		t.Fatal("dos recibos distintos se dieron por buenos", err)
	}
}
