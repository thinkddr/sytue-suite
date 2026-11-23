// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package vaultfiles

// Drive sobre la bóveda directa de punta a punta (ADR 0086): la importación
// nativa desde la fuente del servidor (la misma que hace la web) y el paso de un
// Drive cifrado ya existente a otra bóveda, que es lo que hace la app nativa con
// el Drive de Matrix. El origen de la copia aquí es otra bóveda con su propio
// servidor: el motor no distingue, y así no hace falta un Synapse.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

const (
	raizID    = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e60"
	carpetaID = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e61"
	ficheroID = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e62"
	epocaWeb  = "7f1e2d3c-4b5a-4968-8776-655443322110"
)

var informe = bytes.Repeat([]byte("informe trimestral cifrado "), 4096)

// fuenteServidor imita la fuente del servidor de Drive (`/private-storage/*`):
// `legacy` hasta que alguien la congela con una época, y entonces sirve sus
// filas y el original del fichero tal como estaban.
type fuenteServidor struct {
	mu        sync.Mutex
	state     storage.LegacyMigrationState
	rows      []storage.LegacySourceRow
	begins    int
	opens     int
	fallarAbr int // cuántas aperturas del original fallan todavía
}

func nuevaFuente() *fuenteServidor {
	sum := sha256.Sum256(informe)
	stamp := `"created_at":"2026-01-01T10:00:00Z","updated_at":"2026-01-02T10:00:00Z"`
	return &fuenteServidor{
		state: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"},
		rows: []storage.LegacySourceRow{
			{Kind: "file", ID: ficheroID, Payload: []byte(`{"id":"` + ficheroID + `","folder_id":"` + carpetaID + `","name":"informe.txt","size_bytes":` + itoa(len(informe)) + `,"content_type":"text/plain","checksum":"` + hex.EncodeToString(sum[:]) + `","version":1,` + stamp + `}`)},
			{Kind: "folder", ID: raizID, Payload: []byte(`{"id":"` + raizID + `","parent_id":null,"name":"Mi unidad","is_root":true,` + stamp + `}`)},
			{Kind: "folder", ID: carpetaID, Payload: []byte(`{"id":"` + carpetaID + `","parent_id":"` + raizID + `","name":"Documentos","is_root":false,` + stamp + `}`)},
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func (f *fuenteServidor) congelar(epoch string) {
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	f.state = storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: int64(len(f.rows)), Blobs: 1}
}

func (f *fuenteServidor) PrivateStorageState(_ context.Context, s storage.Scope) (storage.LegacyMigrationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s != scope {
		return storage.LegacyMigrationState{}, drive.ErrSource
	}
	return f.state, nil
}

func (f *fuenteServidor) BeginPrivateStorageMigration(_ context.Context, s storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begins++
	if s != scope || !storage.MigrationEpoch(epoch) {
		return storage.LegacyMigrationState{}, drive.ErrSource
	}
	if f.state.Mode == "legacy" {
		f.congelar(epoch)
	}
	return f.state, nil
}

func (f *fuenteServidor) PrivateStorageSourcePage(_ context.Context, s storage.Scope, epoch string, cursor storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s != scope || epoch != f.state.Epoch || cursor != (storage.LegacySourceCursor{}) {
		return storage.LegacySourcePage{}, drive.ErrSource
	}
	rows := make([]storage.LegacySourceRow, len(f.rows))
	for i, row := range f.rows {
		rows[i] = storage.LegacySourceRow{Kind: row.Kind, ID: row.ID, Payload: bytes.Clone(row.Payload)}
	}
	return storage.LegacySourcePage{Scope: s, Epoch: epoch, Rows: rows}, nil
}

func (f *fuenteServidor) OpenPrivateStorageSource(_ context.Context, s storage.Scope, epoch, kind, id string, size int64) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	if f.fallarAbr > 0 {
		f.fallarAbr--
		return nil, errors.New("corte de red artificial")
	}
	if s != scope || epoch != f.state.Epoch || kind != "file" || id != ficheroID || size != int64(len(informe)) {
		return nil, drive.ErrSource
	}
	return io.NopCloser(bytes.NewReader(informe)), nil
}

// bovedaDrive es una bóveda de Drive con su servidor y el diario de su dispositivo.
type bovedaDrive struct {
	fake    *pqvaulttest.Fake
	gateway *api.Drive
	account *api.Account
	vault   *pqvault.Vault
	store   *Store
	journal *offline.Store
}

func nuevaBovedaDrive(t *testing.T) *bovedaDrive {
	t.Helper()
	b := &bovedaDrive{}
	b.fake, b.gateway, b.account = pqvaulttest.New(t, scope)
	var err error
	b.journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	b.abrir(t, pqvault.Open)
	return b
}

func (b *bovedaDrive) abrir(t *testing.T, open func(context.Context, pqvault.Config) (*pqvault.Vault, error)) {
	t.Helper()
	vault, err := open(t.Context(), pqvault.Config{Scope: scope, Server: b.gateway, Registry: b.account, UnlockKey: bytes.Repeat([]byte{3}, recoverywrap.UnlockKeyLen)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	store, err := New(vault)
	if err != nil {
		t.Fatal(err)
	}
	b.vault, b.store = vault, store
}

// repositorio es el de la app (DirectImport, como NuevoRepositorioDriveBoveda).
func (b *bovedaDrive) repositorio(t *testing.T, fuente drive.Source) *drive.Repository {
	t.Helper()
	repo, err := drive.New(drive.Config{Context: t.Context(), Scope: scope, Crypto: b.store, Gateway: b.gateway, Source: fuente, Journal: b.journal, DirectImport: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	return repo
}

func leerFichero(t *testing.T, repo *drive.Repository, id string) []byte {
	t.Helper()
	body, err := repo.Download(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func comprobarDrive(t *testing.T, repo *drive.Repository) {
	t.Helper()
	root, err := repo.Root(t.Context())
	if err != nil || root.ID != raizID {
		t.Fatalf("la raíz no es la del servidor: %+v %v", root, err)
	}
	children, err := repo.Children(t.Context(), carpetaID, "")
	if err != nil {
		t.Fatal(err)
	}
	veces := 0
	for _, file := range children.Files {
		if file.ID == ficheroID && file.Name == "informe.txt" {
			veces++
		}
	}
	if veces != 1 {
		t.Fatalf("la carpeta no tiene su fichero una sola vez: %+v", children)
	}
	if !bytes.Equal(leerFichero(t, repo, ficheroID), informe) {
		t.Fatal("el fichero no vuelve igual")
	}
}

func sinTextoEnClaro(t *testing.T, b *bovedaDrive) {
	t.Helper()
	for _, blob := range b.fake.Blobs.Data {
		if bytes.Contains(blob, []byte("informe trimestral")) {
			t.Fatal("el servidor de bloques ve el fichero en claro")
		}
	}
}

// Con la fuente en `legacy` y la bóveda vacía, el nativo hace lo que la web:
// congela con su época, importa cada fila y deja el recibo; la bóveda abre.
func TestDirectImportFromLegacySource(t *testing.T) {
	fuente := nuevaFuente()
	b := nuevaBovedaDrive(t)
	repo := b.repositorio(t, fuente)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fuente.begins != 1 || fuente.state.Mode != "frozen" {
		t.Fatal("la fuente no se congeló una vez", fuente.begins, fuente.state.Mode)
	}
	comprobarDrive(t, repo)
	sinTextoEnClaro(t, b)
	if pending, err := drive.PendingDirectImport(b.journal, scope); err != nil || pending {
		t.Fatal("la importación terminada dejó trabajo en el diario", pending, err)
	}
	if _, err := repo.PrivateMigrationReceipt(t.Context()); err != nil {
		t.Fatal("la bóveda no tiene el recibo de la época", err)
	}
}

// Un corte a mitad (el original no se pudo leer) deja la época y sus intenciones
// en el diario; la siguiente apertura reanuda esa misma época sin congelar otra.
func TestDirectImportResumesAfterInterruption(t *testing.T) {
	fuente := nuevaFuente()
	fuente.fallarAbr = 1
	b := nuevaBovedaDrive(t)
	primero := b.repositorio(t, fuente)
	if err := primero.Initialize(t.Context()); err == nil {
		t.Fatal("la importación cortada no puede darse por terminada")
	}
	primero.Close()
	if pending, err := drive.PendingDirectImport(b.journal, scope); err != nil || !pending {
		t.Fatal("el corte no dejó la importación en el diario", pending, err)
	}
	epoca := fuente.state.Epoch
	segundo := b.repositorio(t, fuente)
	if err := segundo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if fuente.begins != 1 || fuente.state.Epoch != epoca {
		t.Fatal("la reanudación congeló otra época", fuente.begins)
	}
	comprobarDrive(t, segundo)
}

// Abrir otra vez una bóveda ya importada no vuelve a importar ni a escribir.
func TestDirectImportNeverImportsTwice(t *testing.T) {
	fuente := nuevaFuente()
	b := nuevaBovedaDrive(t)
	if err := b.repositorio(t, fuente).Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	registros, opens := b.fake.RecordCount(), fuente.opens
	otra := b.repositorio(t, fuente)
	if err := otra.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.fake.RecordCount() != registros || fuente.opens != opens || fuente.begins != 1 {
		t.Fatal("la segunda apertura volvió a importar", b.fake.RecordCount(), registros, fuente.opens, opens)
	}
	comprobarDrive(t, otra)
}

// Una época congelada por otro (la web, o Matrix) no se importa desde aquí sin
// que la persona lo elija; con la adopción explícita, sí.
func TestDirectImportOnlyResumesItsOwnEpoch(t *testing.T) {
	fuente := nuevaFuente()
	fuente.congelar(epocaWeb)
	b := nuevaBovedaDrive(t)
	if err := b.repositorio(t, fuente).Initialize(t.Context()); !errors.Is(err, drive.ErrNotReady) {
		t.Fatal("una época ajena se importó sin elegirlo", err)
	}
	if b.fake.RecordCount() != 0 || fuente.opens != 0 {
		t.Fatal("rechazar una época ajena escribió en la bóveda", b.fake.RecordCount())
	}
	if err := drive.AdoptRetainedSource(b.journal, scope, epocaWeb); err != nil {
		t.Fatal(err)
	}
	repo := b.repositorio(t, fuente)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	comprobarDrive(t, repo)
	if pending, _ := drive.PendingDirectImport(b.journal, scope); pending {
		t.Fatal("la adopción cumplida sigue en el diario")
	}
}

// bancoCopia: el Drive de origen (abierto, con lo importado y algo escrito
// después) y la bóveda de destino, vacía.
type bancoCopia struct {
	fuente         *fuenteServidor
	origen, bóveda *bovedaDrive
	repoOrigen     *drive.Repository
	nuevo          api.File
	nuevoDatos     []byte
	copias         int
	fallarCopia    int // a partir de qué copia de fichero falla (0: nunca)
}

func nuevoBancoCopia(t *testing.T) *bancoCopia {
	t.Helper()
	c := &bancoCopia{fuente: nuevaFuente(), origen: nuevaBovedaDrive(t), bóveda: nuevaBovedaDrive(t)}
	// Los dos servidores falsos numeran igual sus bóvedas; en la realidad, Matrix
	// y la bóveda nunca comparten sala. Estrenar la de destino le da otra.
	c.bóveda.abrir(t, pqvault.Reset)
	c.repoOrigen = c.origen.repositorio(t, c.fuente)
	if err := c.repoOrigen.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	c.nuevoDatos = bytes.Repeat([]byte("escrito después de migrar "), 900)
	var err error
	c.nuevo, err = c.repoOrigen.Upload(t.Context(), carpetaID, "nuevo.txt", "text/plain", bytes.NewReader(c.nuevoDatos), int64(len(c.nuevoDatos)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (c *bancoCopia) copiar(ctx context.Context, publication storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
	c.copias++
	if c.fallarCopia > 0 && c.copias >= c.fallarCopia {
		return storage.Reference{}, errors.New("corte artificial a mitad de la copia")
	}
	return matrix.CopyStorageFileContent(ctx, c.origen.store.StorageFiles, c.origen.gateway, c.bóveda.store.StorageFiles, c.bóveda.gateway, c.bóveda.journal, job, publication, ref, object, revision)
}

func (c *bancoCopia) migrar(t *testing.T) error {
	t.Helper()
	destino, err := drive.New(drive.Config{Context: t.Context(), Scope: scope, Crypto: c.bóveda.store, Gateway: c.bóveda.gateway, Source: c.fuente, Journal: c.bóveda.journal, DirectImport: true})
	if err != nil {
		t.Fatal(err)
	}
	defer destino.Close()
	return destino.MigrateToVault(t.Context(), c.repoOrigen, c.copiar)
}

// La copia lleva TODO el Drive (lo importado y lo escrito después), lo comprueba
// releyendo cada fichero, deja el recibo de la época al final y la bóveda abre
// como si la hubiera migrado la web. El origen no cambia.
func TestVaultMigrationCopiesEverythingAndVerifies(t *testing.T) {
	c := nuevoBancoCopia(t)
	origen := c.origen.fake.RecordCount()
	if err := c.migrar(t); err != nil {
		t.Fatal(err)
	}
	if c.origen.fake.RecordCount() != origen {
		t.Fatal("la copia escribió en el origen")
	}
	if pending, err := drive.VaultMigrationPending(c.bóveda.journal, scope); err != nil || pending {
		t.Fatal("la copia terminada sigue pendiente", pending, err)
	}
	sinTextoEnClaro(t, c.bóveda)
	repo := c.bóveda.repositorio(t, c.fuente)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal("la bóveda copiada no abre con su recibo", err)
	}
	children, err := repo.Children(t.Context(), carpetaID, "")
	if err != nil || len(children.Files) != 2 {
		t.Fatalf("faltan ficheros en la copia: %+v %v", children, err)
	}
	if !bytes.Equal(leerFichero(t, repo, ficheroID), informe) || !bytes.Equal(leerFichero(t, repo, c.nuevo.ID), c.nuevoDatos) {
		t.Fatal("un fichero copiado no es igual al original")
	}
	if c.fuente.begins != 1 {
		t.Fatal("la copia volvió a congelar la fuente del servidor", c.fuente.begins)
	}
	// Repetirla no copia nada encima de una bóveda ya completa.
	registros := c.bóveda.fake.RecordCount()
	if err := c.migrar(t); !errors.Is(err, drive.ErrVaultInUse) || c.bóveda.fake.RecordCount() != registros {
		t.Fatal("una segunda copia escribió sobre la bóveda completa", err)
	}
	// Y el origen sigue entero.
	comprobarDrive(t, c.repoOrigen)
}

// Un fallo a mitad deja el origen intacto y la bóveda sin recibo (nadie la abre,
// ni se importa encima la fuente del servidor); la copia se reanuda donde quedó.
func TestVaultMigrationFailureLeavesSourceIntactAndResumes(t *testing.T) {
	c := nuevoBancoCopia(t)
	origen := c.origen.fake.RecordCount()
	c.fallarCopia = 2
	if err := c.migrar(t); err == nil {
		t.Fatal("la copia cortada no puede darse por buena")
	}
	if c.origen.fake.RecordCount() != origen {
		t.Fatal("la copia fallida escribió en el origen")
	}
	comprobarDrive(t, c.repoOrigen)
	if !bytes.Equal(leerFichero(t, c.repoOrigen, c.nuevo.ID), c.nuevoDatos) {
		t.Fatal("el origen perdió lo escrito después")
	}
	if pending, err := drive.VaultMigrationPending(c.bóveda.journal, scope); err != nil || !pending {
		t.Fatal("la copia cortada no quedó en el diario", pending, err)
	}
	// La bóveda a medias no abre ni importa la época de otro encima.
	parcial, abiertos := c.bóveda.fake.RecordCount(), c.fuente.opens
	if err := c.bóveda.repositorio(t, c.fuente).Initialize(t.Context()); !errors.Is(err, drive.ErrNotReady) {
		t.Fatal("una copia a medias se abrió", err)
	}
	if c.bóveda.fake.RecordCount() != parcial || c.fuente.opens != abiertos {
		t.Fatal("abrir la copia a medias escribió en ella", c.bóveda.fake.RecordCount(), parcial, c.fuente.opens)
	}
	c.fallarCopia = 0
	if err := c.migrar(t); err != nil {
		t.Fatal("la copia no se reanudó", err)
	}
	repo := c.bóveda.repositorio(t, c.fuente)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leerFichero(t, repo, c.nuevo.ID), c.nuevoDatos) {
		t.Fatal("la copia reanudada no tiene lo escrito después")
	}
}

// Si el origen cambia a mitad de la copia, el plan ya no vale: la bóveda a medias
// se vacía (no tiene recibo: no hay nada de nadie) y la copia empieza de nuevo
// con lo último del origen.
func TestVaultMigrationRestartsWhenSourceChanged(t *testing.T) {
	c := nuevoBancoCopia(t)
	c.fallarCopia = 2
	if err := c.migrar(t); err == nil {
		t.Fatal("la copia cortada no puede darse por buena")
	}
	otro := []byte("otro fichero más")
	if _, err := c.repoOrigen.Upload(t.Context(), carpetaID, "otro.txt", "text/plain", bytes.NewReader(otro), int64(len(otro))); err != nil {
		t.Fatal(err)
	}
	c.fallarCopia = 0
	lleno := c.vaciable(t, true)
	// Vaciar sin la marca de la copia no se puede: la marca está activa.
	if _, err := pqvault.Reset(t.Context(), c.bóveda.config(&lleno.Expect)); !errors.Is(err, pqvault.ErrCopyInProgress) {
		t.Fatal("se vació la bóveda sin la marca de la copia", err)
	}
	c.vaciarComoLaCopia(t, lleno)
	if err := c.migrar(t); err != nil {
		t.Fatal(err)
	}
	repo := c.bóveda.repositorio(t, c.fuente)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	children, err := repo.Children(t.Context(), carpetaID, "")
	if err != nil || len(children.Files) != 3 {
		t.Fatalf("la copia repetida no tiene lo último del origen: %+v %v", children, err)
	}
}

// config es la de abrir esta bóveda; con `expect`, la de vaciarla tal como se vio.
func (b *bovedaDrive) config(expect *api.PQVaultExpect) pqvault.Config {
	return pqvault.Config{Scope: scope, Server: b.gateway, Registry: b.account, UnlockKey: bytes.Repeat([]byte{3}, recoverywrap.UnlockKeyLen), Expect: expect}
}

// vaciable copia y espera que la bóveda esté llena sin recibo: con `copia`, lo
// que hay se sabe una copia de Matrix (se puede vaciar); sin ella, no.
func (c *bancoCopia) vaciable(t *testing.T, copia bool) *drive.VaultNotEmptyError {
	t.Helper()
	err := c.migrar(t)
	var lleno *drive.VaultNotEmptyError
	if !errors.As(err, &lleno) || !errors.Is(err, drive.ErrVaultNotEmpty) || lleno.Copy != copia || lleno.CopyID == "" {
		t.Fatalf("esperaba una bóveda a medias (copia=%v): %v %+v", copia, err, lleno)
	}
	return lleno
}

// vaciarComoLaCopia hace lo que la app: vacía con la marca y lo visto, y olvida
// el plan (la marca sigue).
func (c *bancoCopia) vaciarComoLaCopia(t *testing.T, lleno *drive.VaultNotEmptyError) {
	t.Helper()
	vault, err := pqvault.Reset(api.WithPQVaultCopy(t.Context(), lleno.CopyID), c.bóveda.config(&lleno.Expect))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	if c.bóveda.store, err = New(vault); err != nil {
		t.Fatal(err)
	}
	c.bóveda.vault = vault
	if err := drive.DiscardVaultMigration(c.bóveda.journal, scope); err != nil {
		t.Fatal(err)
	}
	if m := c.bóveda.fake.CopyMarker(); m == nil || m.ID != lleno.CopyID {
		t.Fatal("vaciar se llevó la marca de la copia", m)
	}
}

// Mientras se copia, la marca está puesta y nadie más publica en la bóveda (la
// web no importa encima); al terminar, se retira y se olvida.
func TestVaultMigrationHoldsTheCopyMarkerWhileCopying(t *testing.T) {
	c := nuevoBancoCopia(t)
	c.fallarCopia = 2
	if err := c.migrar(t); err == nil {
		t.Fatal("la copia cortada no puede darse por buena")
	}
	marca := c.bóveda.fake.CopyMarker()
	if marca == nil || !marca.Active || marca.Kind != api.PQVaultCopyKind || marca.Epoch == "" {
		t.Fatalf("la copia a medias no dejó su marca: %+v", marca)
	}
	if c.bóveda.fake.AppendOther("importacion_web", []byte(`{"org.sytue.storage_pq":{}}`)) {
		t.Fatal("otro cliente publicó durante la copia")
	}
	c.fallarCopia = 0
	if err := c.migrar(t); err != nil {
		t.Fatal(err)
	}
	if ids := c.bóveda.fake.Claims(); len(ids) < 2 || ids[0] != ids[len(ids)-1] {
		t.Fatalf("la copia reanudada no siguió con su marca: %v", ids)
	}
	if m := c.bóveda.fake.CopyMarker(); m != nil {
		t.Fatalf("la copia terminada no retiró su marca: %+v", m)
	}
	if pending, err := drive.VaultMigrationPending(c.bóveda.journal, scope); err != nil || pending {
		t.Fatal("la marca terminada sigue en el diario", pending, err)
	}
}

// Otro dispositivo con la marca activa: esta copia no escribe nada y lo dice.
func TestVaultMigrationWaitsForAnotherDevicesCopy(t *testing.T) {
	c := nuevoBancoCopia(t)
	if _, err := c.bóveda.gateway.ClaimPQVaultCopy(t.Context(), scope, api.PQVaultCopyClaim{ID: "otro-dispositivo-0000000000", Holder: "native-android", Epoch: "e", TTL: time.Minute}); err != nil {
		t.Fatal(err)
	}
	registros := c.bóveda.fake.RecordCount()
	if err := c.migrar(t); !errors.Is(err, drive.ErrVaultCopyInProgress) || c.bóveda.fake.RecordCount() != registros {
		t.Fatal("se copió encima de la copia de otro dispositivo", err)
	}
}

// La carrera de la ADR 0086 (punto a): la copia a medias ya no vale y hay que
// vaciar, pero su marca caducó y la web terminó entre medias su importación. El
// vaciado con lo que vio la copia se rechaza y el recibo sigue ahí.
func TestVaultMigrationResetNeverWipesAReceiptThatLandedAfterTheRead(t *testing.T) {
	c := nuevoBancoCopia(t)
	c.fallarCopia = 2
	if err := c.migrar(t); err == nil {
		t.Fatal("la copia cortada no puede darse por buena")
	}
	otro := []byte("otro fichero más")
	if _, err := c.repoOrigen.Upload(t.Context(), carpetaID, "otro.txt", "text/plain", bytes.NewReader(otro), int64(len(otro))); err != nil {
		t.Fatal(err)
	}
	c.fallarCopia = 0
	lleno := c.vaciable(t, true)
	c.bóveda.fake.Now = func() time.Time { return time.Now().Add(2 * drive.VaultCopyTTL) }
	c.bóveda.fake.BeforeReset = func(f *pqvaulttest.Fake) {
		if !f.AppendOther("recibo_web", []byte(`{"org.sytue.storage_pq":{}}`)) {
			t.Error("con la marca caducada, la web tenía que poder publicar")
		}
	}
	registros := c.bóveda.fake.RecordCount()
	if _, err := pqvault.Reset(api.WithPQVaultCopy(t.Context(), lleno.CopyID), c.bóveda.config(&lleno.Expect)); !errors.Is(err, pqvault.ErrChanged) {
		t.Fatal("se vació una bóveda con un recibo posterior a lo visto", err)
	}
	if c.bóveda.fake.RecordCount() != registros+1 {
		t.Fatal("el vaciado rechazado borró algo", c.bóveda.fake.RecordCount(), registros)
	}
}

// Un diario perdido a mitad de copia (punto c): sin plan, la marca dice que lo
// que hay en la bóveda es una copia de Matrix. En cuanto caduca la marca vieja,
// la copia se repite vaciando con seguridad. Sin marca previa, lo que hay a
// medias es otra cosa (la web importando) y no se vacía solo.
func TestVaultMigrationRecognizesAnOrphanedCopy(t *testing.T) {
	c := nuevoBancoCopia(t)
	c.fallarCopia = 2
	if err := c.migrar(t); err == nil {
		t.Fatal("la copia cortada no puede darse por buena")
	}
	var err error
	if c.bóveda.journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{}); err != nil {
		t.Fatal(err)
	}
	c.fallarCopia = 0
	if err := c.migrar(t); !errors.Is(err, drive.ErrVaultCopyInProgress) {
		t.Fatal("la marca vieja aún vale y nadie más copia", err)
	}
	c.bóveda.fake.Now = func() time.Time { return time.Now().Add(2 * drive.VaultCopyTTL) }
	c.vaciarComoLaCopia(t, c.vaciable(t, true))
	if err := c.migrar(t); err != nil {
		t.Fatal(err)
	}
	repo := c.bóveda.repositorio(t, c.fuente)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leerFichero(t, repo, c.nuevo.ID), c.nuevoDatos) {
		t.Fatal("la copia repetida no tiene lo de Matrix")
	}

	// Sin marca previa ni plan (lo dejó otra cosa a medias, o una copia de antes
	// de la marca): no se vacía solo.
	d := nuevoBancoCopia(t)
	d.fallarCopia = 2
	if err := d.migrar(t); err == nil {
		t.Fatal("la copia cortada no puede darse por buena")
	}
	if m := d.bóveda.fake.CopyMarker(); m == nil {
		t.Fatal("sin marca")
	} else if _, err := d.bóveda.gateway.ReleasePQVaultCopy(t.Context(), scope, m.ID); err != nil {
		t.Fatal(err)
	}
	if d.bóveda.journal, err = offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{}); err != nil {
		t.Fatal(err)
	}
	d.fallarCopia = 0
	registros := d.bóveda.fake.RecordCount()
	d.vaciable(t, false)
	if d.bóveda.fake.RecordCount() != registros {
		t.Fatal("se escribió en una bóveda a medias ajena")
	}
}
