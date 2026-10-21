// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

var errBoom = errors.New("boom")

// fakeStore implementa dataStore en memoria.
type fakeStore struct {
	uploadReservations int
	retenido           bool // SujetoRetenido (docs.go)
	folderPath         string
	folderPathErr      error

	quota       domain.Quota
	quotaErr    error
	root        domain.Folder
	rootErr     error
	folder      domain.Folder
	folderErr   error
	deleted     []store.DeletedFile
	delFldErr   error
	file        domain.File
	fileErr     error
	labelSet    *domain.Label // último snapshot pedido a SetFileLabel
	newFile     *store.NewFileRecord
	createErr   error
	delFile     store.DeletedFile
	delFileErr  error
	copyFile    domain.File
	copyFileErr error
	copyFolder  domain.Folder
	copyFoldErr error
	lastCopySrc uuid.UUID
	lastCopyDst uuid.UUID
	lastCopyPar *uuid.UUID
	trashFold   []domain.Folder
	trashFile   []domain.File
	trashErr    error
	restFile    domain.File
	restFerr    error
	restFold    domain.Folder
	restFldErr  error
	purgeFile   store.DeletedFile
	purgeFErr   error
	purgeFold   []store.DeletedFile
	purgeFldEr  error
	gcBatches   [][]store.PurgedBlob // una entrada por llamada; su len = «claimed» de esa llamada
	gcErr       error
	gcCalls     int
	gcLastCut   time.Time
	page        store.FilePage
	pageErr     error
	share       domain.Share
	shareErr    error
	newShare    *store.NewShareRecord
	resolveSh   domain.Share
	resolveTn   string
	resolveHash string
	resolveErr  error
	byName      domain.File
	byNameErr   error
	byKey       domain.File
	byKeyErr    error
	subfolders  []domain.Folder

	finalizadas      map[string]bool
	caducadasBatches [][]store.SubidaCaducada

	recentEntries []store.Entry
	recentNext    *store.RecentCursor
	recentErr     error
	favEntries    []store.Entry
	favErr        error
	setFavErr     error
	lastFavOn     bool
	lastFavFile   *uuid.UUID
	lastFavFolder *uuid.UUID

	sharedEntries []store.SharedEntry
	sharedErr     error
	shareForRes   domain.Share
	shareForErr   error
	lastGroups    []string

	versions       []domain.FileVersion
	versionsNext   *int
	versionsErr    error
	getVersion     domain.FileVersion
	getVersionErr  error
	restoreVer     domain.File
	restoreVerErr  error
	lastRestoreVer int

	batchMoveRes     store.BatchResult
	batchMoveErr     error
	batchTrashDel    []store.DeletedFile
	batchTrashN      int
	batchTrashErr    error
	zipEntries       []store.ZipEntry
	zipErr           error
	lastZipMax       int
	lastBatchFiles   []uuid.UUID
	lastBatchFolders []uuid.UUID
	lastBatchTarget  uuid.UUID
}

func (f *fakeStore) EnsureQuota(context.Context, int64) (domain.Quota, error) {
	return f.quota, f.quotaErr
}
func (f *fakeStore) EnsureRoot(context.Context) (domain.Folder, error) { return f.root, f.rootErr }
func (f *fakeStore) GetQuota(context.Context) (domain.Quota, error)    { return f.quota, f.quotaErr }

// EnsureBase hace lo que las dos juntas, y falla por lo mismo: primero la cuota
// y luego la raíz, para que un `quotaErr` siga saliendo antes que un `rootErr`
// —que es el orden que tenían las llamadas encadenadas—.
func (f *fakeStore) EnsureBase(context.Context, int64, bool) (domain.Folder, domain.Quota, error) {
	if f.quotaErr != nil {
		return domain.Folder{}, domain.Quota{}, f.quotaErr
	}
	if f.rootErr != nil {
		return domain.Folder{}, domain.Quota{}, f.rootErr
	}
	return f.root, f.quota, nil
}

func (f *fakeStore) GetFolder(context.Context, uuid.UUID) (domain.Folder, error) {
	return f.folder, f.folderErr
}

func (f *fakeStore) ListChildren(context.Context, uuid.UUID) ([]domain.Folder, []domain.File, error) {
	return nil, nil, f.folderErr
}

func (f *fakeStore) ListSubfolders(context.Context, uuid.UUID) ([]domain.Folder, error) {
	return f.subfolders, f.folderErr
}

// ResolveUnder sobre los datos planos del doble: busca el ÚLTIMO tramo entre las
// subcarpetas y entre los ficheros por nombre, y devuelve lo que encuentre —los
// dos si los dos están—. No decide cuál gana: eso es del llamante, y por eso
// `ResolvePath` y `PublicBrowse` pueden seguir prefiriendo cosas distintas.
// El SQL de verdad se prueba contra Postgres en el store.
func (f *fakeStore) ResolveUnder(_ context.Context, _ uuid.UUID, segs []string) (*domain.Folder, *domain.File, error) {
	if f.folderErr != nil {
		return nil, nil, f.folderErr
	}
	if len(segs) == 0 {
		return nil, nil, nil
	}
	ultimo := segs[len(segs)-1]
	var carpeta *domain.Folder
	for i := range f.subfolders {
		if strings.EqualFold(f.subfolders[i].Name, ultimo) {
			carpeta = &f.subfolders[i]
			break
		}
	}
	var fichero *domain.File
	if f.byNameErr == nil && strings.EqualFold(f.byName.Name, ultimo) {
		fichero = &f.byName
	}
	return carpeta, fichero, nil
}

func (f *fakeStore) CreateFolder(context.Context, uuid.UUID, string, string, bool) (domain.Folder, error) {
	return f.folder, f.folderErr
}

func (f *fakeStore) RenameFolder(context.Context, uuid.UUID, string) (domain.Folder, error) {
	return f.folder, f.folderErr
}

func (f *fakeStore) MoveFolder(context.Context, uuid.UUID, uuid.UUID) (domain.Folder, error) {
	return f.folder, f.folderErr
}

func (f *fakeStore) DeleteFolder(context.Context, uuid.UUID) ([]store.DeletedFile, error) {
	return f.deleted, f.delFldErr
}

func (f *fakeStore) GetFile(context.Context, uuid.UUID) (domain.File, error) {
	return f.file, f.fileErr
}

func (f *fakeStore) ReservarSubida(_ context.Context, _ string, tamano int64, _ time.Time, conCuota bool) error {
	f.uploadReservations++
	if conCuota && f.quota.UsedBytes+tamano > f.quota.LimitBytes {
		return domain.ErrQuotaExceeded
	}
	return nil
}

func (f *fakeStore) SubidaFinalizada(_ context.Context, clave string) error {
	if f.finalizadas[clave] {
		return store.ErrSubidaYaFinalizada
	}
	if f.finalizadas == nil {
		f.finalizadas = make(map[string]bool)
	}
	f.finalizadas[clave] = true
	return nil
}

func (f *fakeStore) ReclamarSubidasCaducadas(context.Context, time.Time, int) ([]store.SubidaCaducada, error) {
	if len(f.caducadasBatches) == 0 {
		return nil, nil
	}
	b := f.caducadasBatches[0]
	f.caducadasBatches = f.caducadasBatches[1:]
	return b, nil
}

func (f *fakeStore) GetFileByStorageKey(context.Context, string) (domain.File, error) {
	return f.byKey, f.byKeyErr
}

func (f *fakeStore) CreateFile(_ context.Context, rec store.NewFileRecord) (domain.File, error) {
	f.newFile = &rec
	return f.file, f.createErr
}

func (f *fakeStore) RenameFile(context.Context, uuid.UUID, string) (domain.File, error) {
	return f.file, f.fileErr
}

// SetFileLabel guarda el snapshot pedido y lo refleja en el fichero devuelto.
func (f *fakeStore) SetFileLabel(_ context.Context, _ uuid.UUID, label domain.Label) (domain.File, error) {
	f.labelSet = &label
	if f.fileErr != nil {
		return domain.File{}, f.fileErr
	}
	out := f.file
	if label.ID == "" {
		out.Label = nil
	} else {
		l := label
		out.Label = &l
	}
	return out, nil
}

func (f *fakeStore) MoveFile(context.Context, uuid.UUID, uuid.UUID) (domain.File, error) {
	return f.file, f.fileErr
}

func (f *fakeStore) DeleteFile(context.Context, uuid.UUID) (store.DeletedFile, error) {
	return f.delFile, f.delFileErr
}

func (f *fakeStore) CopyFile(_ context.Context, srcID, destFolderID uuid.UUID, _ string) (domain.File, error) {
	f.lastCopySrc, f.lastCopyDst = srcID, destFolderID
	return f.copyFile, f.copyFileErr
}

func (f *fakeStore) CopiaParaDuenya(_ context.Context, srcID, destFolderID uuid.UUID, _, _ string) (domain.File, bool, error) {
	f.lastCopySrc, f.lastCopyDst = srcID, destFolderID
	return f.copyFile, false, f.copyFileErr
}

func (f *fakeStore) CopyFolder(_ context.Context, srcID uuid.UUID, destParentID *uuid.UUID, _ string) (domain.Folder, error) {
	f.lastCopySrc, f.lastCopyPar = srcID, destParentID
	return f.copyFolder, f.copyFoldErr
}

func (f *fakeStore) ListTrash(context.Context) ([]domain.Folder, []domain.File, error) {
	return f.trashFold, f.trashFile, f.trashErr
}

func (f *fakeStore) RestoreFile(context.Context, uuid.UUID) (domain.File, error) {
	return f.restFile, f.restFerr
}

func (f *fakeStore) FicheroEnCualquierEstado(context.Context, uuid.UUID) (domain.File, error) {
	return f.file, f.fileErr
}

func (f *fakeStore) SujetoRetenido(context.Context, string) (bool, error) {
	return f.retenido, nil
}

func (f *fakeStore) RestoreFolder(context.Context, uuid.UUID) (domain.Folder, error) {
	return f.restFold, f.restFldErr
}

func (f *fakeStore) PurgeFile(context.Context, uuid.UUID) (store.DeletedFile, error) {
	return f.purgeFile, f.purgeFErr
}

func (f *fakeStore) PurgeFolder(context.Context, uuid.UUID) ([]store.DeletedFile, error) {
	return f.purgeFold, f.purgeFldEr
}

func (f *fakeStore) ClaimAndPurgeExpiredTrash(_ context.Context, cutoff time.Time, _ int) (int, []store.PurgedBlob, error) {
	f.gcCalls++
	f.gcLastCut = cutoff
	if f.gcErr != nil {
		return 0, nil, f.gcErr
	}
	if len(f.gcBatches) == 0 {
		return 0, nil, nil
	}
	batch := f.gcBatches[0]
	f.gcBatches = f.gcBatches[1:]
	return len(batch), batch, nil
}

func (f *fakeStore) ListFilesPage(context.Context, uuid.UUID, store.FileCursor, int) (store.FilePage, error) {
	return f.page, f.pageErr
}

func (f *fakeStore) GetFileByName(context.Context, uuid.UUID, string) (domain.File, error) {
	return f.byName, f.byNameErr
}

func (f *fakeStore) FolderPath(context.Context, uuid.UUID) (string, error) {
	return f.folderPath, f.folderPathErr
}

func (f *fakeStore) SearchFiles(context.Context, string, int) ([]store.Entry, error) {
	return nil, f.fileErr
}

func (f *fakeStore) SetQuotaLimit(context.Context, int64) (domain.Quota, error) {
	return f.quota, f.quotaErr
}

func (f *fakeStore) ListRecent(context.Context, store.RecentCursor, int) ([]store.Entry, *store.RecentCursor, error) {
	return f.recentEntries, f.recentNext, f.recentErr
}

func (f *fakeStore) ListFavorites(context.Context, string) ([]store.Entry, error) {
	return f.favEntries, f.favErr
}

func (f *fakeStore) SetFavorite(_ context.Context, _ string, fileID, folderID *uuid.UUID, on bool) error {
	f.lastFavFile, f.lastFavFolder, f.lastFavOn = fileID, folderID, on
	return f.setFavErr
}

func (f *fakeStore) CreateShare(_ context.Context, rec store.NewShareRecord) (domain.Share, error) {
	f.newShare = &rec
	return f.share, f.shareErr
}

func (f *fakeStore) ListSharesForFile(context.Context, uuid.UUID) ([]domain.Share, error) {
	return nil, f.shareErr
}

func (f *fakeStore) ListSharesForFolder(context.Context, uuid.UUID) ([]domain.Share, error) {
	return nil, f.shareErr
}
func (f *fakeStore) RevokeShare(context.Context, uuid.UUID) error { return f.shareErr }
func (f *fakeStore) ResolveLink(context.Context, string) (domain.Share, string, string, error) {
	return f.resolveSh, f.resolveTn, f.resolveHash, f.resolveErr
}

func (f *fakeStore) ListSharedWithMe(_ context.Context, _ string, groups []string) ([]store.SharedEntry, error) {
	f.lastGroups = groups
	return f.sharedEntries, f.sharedErr
}

func (f *fakeStore) PedirEdicion(context.Context, uuid.UUID, peticiones.Peticion) (bool, error) {
	return true, nil
}

func (f *fakeStore) PeticionesEdicion(context.Context, uuid.UUID, string) ([]peticiones.Peticion, error) {
	return nil, nil
}

func (f *fakeStore) ResolverPeticionEdicion(context.Context, uuid.UUID, string, string, bool) error {
	return nil
}

func (f *fakeStore) PedidasPor(context.Context, string, []uuid.UUID) (map[uuid.UUID]bool, error) {
	return map[uuid.UUID]bool{}, nil
}

func (f *fakeStore) ShareForResource(_ context.Context, _ string, groups []string, _ uuid.UUID) (domain.Share, error) {
	f.lastGroups = groups
	return f.shareForRes, f.shareForErr
}

func (f *fakeStore) ListVersions(context.Context, uuid.UUID, int, int) ([]domain.FileVersion, *int, error) {
	return f.versions, f.versionsNext, f.versionsErr
}

func (f *fakeStore) GetVersion(context.Context, uuid.UUID, int) (domain.FileVersion, error) {
	return f.getVersion, f.getVersionErr
}

func (f *fakeStore) RestoreVersion(_ context.Context, _ uuid.UUID, version int, _ string) (domain.File, error) {
	f.lastRestoreVer = version
	return f.restoreVer, f.restoreVerErr
}

func (f *fakeStore) BatchMove(_ context.Context, fileIDs, folderIDs []uuid.UUID, target uuid.UUID) (store.BatchResult, error) {
	f.lastBatchFiles, f.lastBatchFolders, f.lastBatchTarget = fileIDs, folderIDs, target
	return f.batchMoveRes, f.batchMoveErr
}

func (f *fakeStore) BatchTrash(_ context.Context, fileIDs, folderIDs []uuid.UUID) ([]store.DeletedFile, int, error) {
	f.lastBatchFiles, f.lastBatchFolders = fileIDs, folderIDs
	return f.batchTrashDel, f.batchTrashN, f.batchTrashErr
}

func (f *fakeStore) CollectZip(_ context.Context, fileIDs, folderIDs []uuid.UUID, maxFiles int) ([]store.ZipEntry, error) {
	f.lastBatchFiles, f.lastBatchFolders, f.lastZipMax = fileIDs, folderIDs, maxFiles
	return f.zipEntries, f.zipErr
}

func (f *fakeStore) ExternalStorage(context.Context) (store.ExternalStorage, error) {
	return store.ExternalStorage{}, domain.ErrNotFound
}

func (f *fakeStore) SaveExternalStorage(_ context.Context, e store.ExternalStorage, _ string) (store.ExternalStorage, error) {
	return e, nil
}

func (f *fakeStore) MarkExternalStorageChecked(context.Context, bool, string) error { return nil }

var _ dataStore = (*fakeStore)(nil)

// fakeBlobs implementa blobStore capturando la última operación.
type fakeBlobs struct {
	putBody     []byte
	putErr      error
	getBody     string
	getErr      error
	delKeys     []string
	delErr      error
	presign     *url.URL
	preErr      error
	presignPut  *url.URL
	prePutErr   error
	lastPutKey  string
	statSize    int64
	statErr     error
	lastStatKey string
	existen     map[string]bool
	existeErr   error
	copias      []string
	copiaErr    error
}

func (b *fakeBlobs) Put(_ context.Context, _ tenancy.Tenant, _ string, r io.Reader, _ int64, _ objectstore.PutOptions) error {
	if b.putErr != nil {
		return b.putErr
	}
	body, _ := io.ReadAll(r)
	b.putBody = body
	return nil
}

func (b *fakeBlobs) Get(context.Context, tenancy.Tenant, string) (io.ReadCloser, error) {
	if b.getErr != nil {
		return nil, b.getErr
	}
	return io.NopCloser(strings.NewReader(b.getBody)), nil
}

func (b *fakeBlobs) Delete(_ context.Context, _ tenancy.Tenant, key string) error {
	b.delKeys = append(b.delKeys, key)
	return b.delErr
}

func (b *fakeBlobs) PresignGet(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return b.presign, b.preErr
}

func (b *fakeBlobs) PresignPut(_ context.Context, _ tenancy.Tenant, key string, _ time.Duration) (*url.URL, error) {
	b.lastPutKey = key
	return b.presignPut, b.prePutErr
}

func (b *fakeBlobs) Stat(_ context.Context, _ tenancy.Tenant, key string) (int64, error) {
	b.lastStatKey = key
	return b.statSize, b.statErr
}

func (b *fakeBlobs) Existe(_ context.Context, _ tenancy.Tenant, key string) (bool, error) {
	return b.existen[key], b.existeErr
}

func (b *fakeBlobs) Copiar(_ context.Context, _ tenancy.Tenant, origen, destino string) error {
	b.copias = append(b.copias, origen+"→"+destino)
	return b.copiaErr
}

var _ blobStore = (*fakeBlobs)(nil)

type capturePub struct{ types []string }

func (c *capturePub) Publish(_ context.Context, ev events.Event) error {
	c.types = append(c.types, ev.Type)
	return nil
}

func newSvc(fs *fakeStore, blobs blobStore, pub EventPublisher) *Service {
	return &Service{
		cfg: config.Config{
			MaxUploadBytes:    100,
			PresignTTL:        time.Minute,
			DefaultQuotaBytes: 1 << 30,
			S3SecretKey:       "test-presign-secret",
		},
		store: fs,
		blobs: blobs,
		pub:   pub,
	}
}

func tenantCtx(t *testing.T) context.Context {
	t.Helper()
	tn, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	return tenancy.Into(context.Background(), tn)
}

func TestEnsureTenant(t *testing.T) {
	fs := &fakeStore{root: domain.Folder{ID: uuid.New()}, quota: domain.Quota{LimitBytes: 100}}
	s := newSvc(fs, &fakeBlobs{}, nil)
	if _, _, err := s.EnsureTenant(tenantCtx(t)); err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	// Error de cuota se propaga.
	fs.quotaErr = errBoom
	if _, _, err := s.EnsureTenant(tenantCtx(t)); !errors.Is(err, errBoom) {
		t.Fatalf("error de cuota debe propagarse, got %v", err)
	}
}

func TestCreateFolderEmits(t *testing.T) {
	fs := &fakeStore{folder: domain.Folder{ID: uuid.New(), Name: "docs"}}
	pub := &capturePub{}
	s := newSvc(fs, &fakeBlobs{}, pub)
	if _, err := s.CreateFolder(tenantCtx(t), uuid.New(), "docs", "sub", false); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.folder.created" {
		t.Fatalf("esperaba folder.created, got %v", pub.types)
	}
	// Error del store no emite.
	fs.folderErr = errBoom
	pub.types = nil
	if _, err := s.CreateFolder(tenantCtx(t), uuid.New(), "x", "sub", false); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse, got %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe emitir tras error")
	}
}

func TestDeleteFolderSoftKeepsBlobs(t *testing.T) {
	// Soft delete: NO se purga ningún blob (se conservan para restaurar).
	fs := &fakeStore{deleted: []store.DeletedFile{{StorageKey: "k1"}, {StorageKey: "k2"}}}
	blobs := &fakeBlobs{}
	pub := &capturePub{}
	s := newSvc(fs, blobs, pub)
	if err := s.DeleteFolder(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if len(blobs.delKeys) != 0 {
		t.Fatalf("el borrado a papelera NO debe purgar blobs, got %v", blobs.delKeys)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.folder.deleted" {
		t.Fatalf("esperaba folder.deleted, got %v", pub.types)
	}
}

func TestPurgeFolderPurgesBlobs(t *testing.T) {
	// Blobs huérfanos (BlobOrphaned): la purga los borra de S3.
	fs := &fakeStore{purgeFold: []store.DeletedFile{
		{StorageKey: "k1", BlobOrphaned: true},
		{StorageKey: "k2", BlobOrphaned: true},
	}}
	blobs := &fakeBlobs{}
	pub := &capturePub{}
	s := newSvc(fs, blobs, pub)
	if err := s.PurgeFolder(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("PurgeFolder: %v", err)
	}
	if len(blobs.delKeys) != 2 {
		t.Fatalf("purga definitiva debe borrar 2 blobs, got %v", blobs.delKeys)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.folder.purged" {
		t.Fatalf("esperaba folder.purged, got %v", pub.types)
	}
}

// Un blob NO huérfano (lo comparte una copia superviviente por referencia) NO se
// borra al purgar: evita perder los datos de la copia.
func TestPurgeKeepsReferencedBlobs(t *testing.T) {
	fs := &fakeStore{purgeFold: []store.DeletedFile{
		{StorageKey: "shared", BlobOrphaned: false}, // aún referenciado por una copia
		{StorageKey: "solo", BlobOrphaned: true},
	}}
	blobs := &fakeBlobs{}
	s := newSvc(fs, blobs, &capturePub{})
	if err := s.PurgeFolder(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("PurgeFolder: %v", err)
	}
	if len(blobs.delKeys) != 1 || blobs.delKeys[0] != "solo" {
		t.Fatalf("solo debe borrarse el blob huérfano, got %v", blobs.delKeys)
	}

	// Igual en la purga de un fichero suelto todavía referenciado por una copia.
	fs2 := &fakeStore{purgeFile: store.DeletedFile{StorageKey: "shared", BlobOrphaned: false}}
	blobs2 := &fakeBlobs{}
	s2 := newSvc(fs2, blobs2, &capturePub{})
	if err := s2.PurgeFile(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("PurgeFile: %v", err)
	}
	if len(blobs2.delKeys) != 0 {
		t.Fatalf("un blob aún referenciado NO debe borrarse, got %v", blobs2.delKeys)
	}
}

// Un viewer (PermRead) NO puede copiar (copiar es crear en el destino): el
// servicio deniega antes de tocar el store (deny-by-default, como RestoreVersion).
func TestCopyDeniedForViewer(t *testing.T) {
	fs := &fakeStore{copyFile: domain.File{ID: uuid.New()}, copyFolder: domain.Folder{ID: uuid.New()}}
	s := newSvc(fs, &fakeBlobs{}, &capturePub{})
	if _, err := s.CopyFile(tenantCtx(t), uuid.New(), uuid.New(), "u", domain.PermRead); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("CopyFile viewer => %v, quiero ErrPermission", err)
	}
	if _, err := s.CopyFolder(tenantCtx(t), uuid.New(), nil, "u", domain.PermRead); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("CopyFolder viewer => %v, quiero ErrPermission", err)
	}
	if fs.lastCopySrc != uuid.Nil {
		t.Fatal("no debe llamar al store si se deniega el permiso")
	}
}

// Copiar sin destino explícito (uuid.Nil) usa la carpeta del origen (GetFile la da,
// gateado por RLS) y emite drive.file.copied.
func TestCopyFileDefaultsToSourceFolder(t *testing.T) {
	srcFolder := uuid.New()
	fs := &fakeStore{
		file:     domain.File{ID: uuid.New(), FolderID: srcFolder}, // lo devuelve GetFile
		copyFile: domain.File{ID: uuid.New(), Name: "Copia de f"},
		quota:    domain.Quota{LimitBytes: 1 << 30},
	}
	pub := &capturePub{}
	s := newSvc(fs, &fakeBlobs{}, pub)
	if _, err := s.CopyFile(tenantCtx(t), uuid.New(), uuid.Nil, "u", domain.PermWrite); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	if fs.lastCopyDst != srcFolder {
		t.Fatalf("dest por defecto = %v, quiero la carpeta del origen %v", fs.lastCopyDst, srcFolder)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.file.copied" {
		t.Fatalf("esperaba file.copied, got %v", pub.types)
	}
}

func TestRestoreFileAndFolderEmit(t *testing.T) {
	fs := &fakeStore{
		restFile: domain.File{ID: uuid.New()},
		restFold: domain.Folder{ID: uuid.New()},
	}
	pub := &capturePub{}
	s := newSvc(fs, &fakeBlobs{}, pub)
	if _, err := s.RestoreFile(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}
	if _, err := s.RestoreFolder(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("RestoreFolder: %v", err)
	}
	want := []string{"drive.file.restored", "drive.folder.restored"}
	if len(pub.types) != 2 || pub.types[0] != want[0] || pub.types[1] != want[1] {
		t.Fatalf("eventos = %v, quiero %v", pub.types, want)
	}
	// Error del store no emite.
	fs.restFerr = errBoom
	pub.types = nil
	if _, err := s.RestoreFile(tenantCtx(t), uuid.New()); !errors.Is(err, errBoom) {
		t.Fatalf("error debe propagarse, got %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe emitir tras error")
	}
}

func TestUploadFileInvalidName(t *testing.T) {
	s := newSvc(&fakeStore{}, &fakeBlobs{}, nil)
	if _, err := s.UploadFile(tenantCtx(t), uuid.New(), "", "text/plain", "sub", strings.NewReader("x")); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("nombre inválido: %v", err)
	}
}

func TestUploadFileNoTenant(t *testing.T) {
	s := newSvc(&fakeStore{}, &fakeBlobs{}, nil)
	if _, err := s.UploadFile(context.Background(), uuid.New(), "f.txt", "", "sub", strings.NewReader("x")); err == nil {
		t.Fatal("sin tenant debe fallar")
	}
}

func TestUploadFileHappyPath(t *testing.T) {
	fs := &fakeStore{file: domain.File{ID: uuid.New(), Name: "f.txt", SizeBytes: 5}}
	blobs := &fakeBlobs{}
	pub := &capturePub{}
	s := newSvc(fs, blobs, pub)
	_, err := s.UploadFile(tenantCtx(t), uuid.New(), "f.txt", "", "sub", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if string(blobs.putBody) != "hello" {
		t.Errorf("cuerpo subido: %q", blobs.putBody)
	}
	if fs.newFile.SizeBytes != 5 {
		t.Errorf("tamaño calculado: %d", fs.newFile.SizeBytes)
	}
	if fs.newFile.ContentType != "application/octet-stream" {
		t.Errorf("content-type por defecto: %q", fs.newFile.ContentType)
	}
	if fs.newFile.Checksum == "" {
		t.Error("checksum vacío")
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.file.created" {
		t.Fatalf("esperaba file.created, got %v", pub.types)
	}
}

func TestUploadFileExceedsQuota(t *testing.T) {
	blobs := &fakeBlobs{}
	s := newSvc(&fakeStore{}, blobs, nil)
	big := strings.Repeat("a", 200) // > MaxUploadBytes (100)
	if _, err := s.UploadFile(tenantCtx(t), uuid.New(), "f.txt", "", "sub", strings.NewReader(big)); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("exceso de tamaño debe dar ErrQuotaExceeded, got %v", err)
	}
	// El blob se compensa (borrado).
	if len(blobs.delKeys) != 1 {
		t.Fatalf("blob no compensado, delKeys=%v", blobs.delKeys)
	}
}

func TestUploadFileMetadataError(t *testing.T) {
	fs := &fakeStore{createErr: errBoom}
	blobs := &fakeBlobs{}
	s := newSvc(fs, blobs, nil)
	if _, err := s.UploadFile(tenantCtx(t), uuid.New(), "f.txt", "", "sub", strings.NewReader("x")); !errors.Is(err, errBoom) {
		t.Fatalf("error de metadato debe propagarse, got %v", err)
	}
	if len(blobs.delKeys) != 1 {
		t.Fatal("blob debe compensarse si el metadato falla")
	}
}

func TestOpenFileContent(t *testing.T) {
	fs := &fakeStore{file: domain.File{ID: uuid.New(), StorageKey: "k"}}
	blobs := &fakeBlobs{getBody: "data"}
	s := newSvc(fs, blobs, nil)
	_, rc, err := s.OpenFileContent(tenantCtx(t), uuid.New())
	if err != nil {
		t.Fatalf("OpenFileContent: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "data" {
		t.Fatalf("contenido: %q", got)
	}
	// Blob ausente mapea a ErrNotFound.
	blobs.getErr = objectstore.ErrNotFound
	if _, _, err := s.OpenFileContent(tenantCtx(t), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("blob ausente debe dar ErrNotFound, got %v", err)
	}
}

func TestPresignDownload(t *testing.T) {
	u, _ := url.Parse("https://s3/presigned")
	fs := &fakeStore{file: domain.File{StorageKey: "k"}}
	s := newSvc(fs, &fakeBlobs{presign: u}, nil)
	got, err := s.PresignDownload(tenantCtx(t), uuid.New())
	if err != nil {
		t.Fatalf("PresignDownload: %v", err)
	}
	if got.String() != "https://s3/presigned" {
		t.Fatalf("URL: %s", got)
	}
}

func TestDeleteFileSoftKeepsBlob(t *testing.T) {
	// Soft delete: NO se purga el blob (se conserva para restaurar).
	fs := &fakeStore{delFile: store.DeletedFile{StorageKey: "k9"}}
	blobs := &fakeBlobs{}
	pub := &capturePub{}
	s := newSvc(fs, blobs, pub)
	if err := s.DeleteFile(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if len(blobs.delKeys) != 0 {
		t.Fatalf("el borrado a papelera NO debe purgar el blob, got %v", blobs.delKeys)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.file.deleted" {
		t.Fatalf("esperaba file.deleted, got %v", pub.types)
	}
}

func TestPurgeFilePurgesBlob(t *testing.T) {
	fs := &fakeStore{purgeFile: store.DeletedFile{StorageKey: "k9", BlobOrphaned: true}}
	blobs := &fakeBlobs{}
	pub := &capturePub{}
	s := newSvc(fs, blobs, pub)
	if err := s.PurgeFile(tenantCtx(t), uuid.New()); err != nil {
		t.Fatalf("PurgeFile: %v", err)
	}
	if len(blobs.delKeys) != 1 || blobs.delKeys[0] != "k9" {
		t.Fatalf("purga definitiva debe borrar el blob: %v", blobs.delKeys)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.file.purged" {
		t.Fatalf("esperaba file.purged, got %v", pub.types)
	}
}

func TestCreateShareValidation(t *testing.T) {
	s := newSvc(&fakeStore{}, &fakeBlobs{}, nil)
	ctx := tenantCtx(t)
	// Ni file ni folder (ambos nil).
	if _, err := s.CreateShare(ctx, CreateShareInput{}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("share sin objetivo: %v", err)
	}
	fid := uuid.New()
	folderID := uuid.New()
	// Ambos a la vez.
	if _, err := s.CreateShare(ctx, CreateShareInput{FileID: &fid, FolderID: &folderID}); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("share con ambos objetivos: %v", err)
	}
}

func TestCreateShareLinkGeneratesToken(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{share: domain.Share{ID: uuid.New(), Kind: domain.ShareLink}}
	pub := &capturePub{}
	s := newSvc(fs, &fakeBlobs{}, pub)
	if _, err := s.CreateShare(tenantCtx(t), CreateShareInput{FileID: &fid, Kind: domain.ShareLink}); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if fs.newShare.Token == "" {
		t.Error("enlace debe generar token")
	}
	if fs.newShare.Permission != domain.PermRead {
		t.Errorf("permiso por defecto = read, got %q", fs.newShare.Permission)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.share.created" {
		t.Fatalf("esperaba share.created, got %v", pub.types)
	}
}

func TestCreateShareLinkWithPassword(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{share: domain.Share{ID: uuid.New(), Kind: domain.ShareLink}}
	s := newSvc(fs, &fakeBlobs{}, &capturePub{})
	if _, err := s.CreateShare(tenantCtx(t), CreateShareInput{FileID: &fid, Kind: domain.ShareLink, Password: "s3cr3t"}); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if fs.newShare.PasswordHash == "" {
		t.Fatal("enlace con contraseña debe persistir un hash")
	}
	if fs.newShare.PasswordHash == "s3cr3t" {
		t.Fatal("la contraseña NUNCA debe guardarse en claro")
	}
	ok, err := domain.VerifyPassword("s3cr3t", fs.newShare.PasswordHash)
	if err != nil || !ok {
		t.Fatalf("el hash guardado no verifica la contraseña: ok=%v err=%v", ok, err)
	}
}

func TestCreateShareUserEmitsHubNotification(t *testing.T) {
	received := make(chan hubnotify.Notification, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/notifications" {
			http.NotFound(w, r)
			return
		}
		var notif hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&notif)
		received <- notif
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	fid := uuid.New()
	fs := &fakeStore{
		file:  domain.File{ID: fid, Name: "report.pdf"},
		share: domain.Share{ID: uuid.New(), Kind: domain.ShareUser, Subject: "user-sub-123"},
	}
	s := newSvc(fs, &fakeBlobs{}, &capturePub{})
	s.cfg.PublicURL = "https://drive.sytue.eu"
	s.SetHubNotifier(hubnotify.New(srv.URL, "test-secret"))

	ctx := authn.Into(tenantCtx(t), authn.Claims{Subject: "alice", Name: "Alicia Pérez"})
	_, err := s.CreateShare(ctx, CreateShareInput{
		FileID:    &fid,
		Kind:      domain.ShareUser,
		Subject:   "user-sub-123",
		CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	select {
	case n := <-received:
		if n.Source != "drive" || n.Type != "drive.share.created" {
			t.Errorf("tipo/origen inesperado: %+v", n)
		}
		if n.Subject != "user-sub-123" {
			t.Errorf("subject esperado 'user-sub-123', got %q", n.Subject)
		}
		if !strings.Contains(n.Title, "report.pdf") {
			t.Errorf("título no contiene el nombre del archivo: %q", n.Title)
		}
		if n.Tenant != "acme" {
			t.Errorf("tenant esperado 'acme', got %q", n.Tenant)
		}
		// Abre EL FICHERO, dentro de «Compartido conmigo» y en el host de Drive, no `/shared` en un
		// `.sytue.localhost`; y dice QUIÉN, por su nombre y no por su sub.
		if n.URL != "https://drive.sytue.eu/drive/acme/?section=shared&shared="+fid.String() {
			t.Errorf("enlace = %q", n.URL)
		}
		if n.Params["actor"] != "Alicia Pérez" || n.Params["name"] != "report.pdf" {
			t.Errorf("params = %+v", n.Params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando notificación en Hub")
	}
}

func TestPublicResolvePasswordGate(t *testing.T) {
	hash, err := domain.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	fid := uuid.New()
	fs := &fakeStore{resolveSh: domain.Share{FileID: &fid}, resolveTn: "acme", resolveHash: hash}
	s := newSvc(fs, &fakeBlobs{}, nil)

	// Sin prueba: se exige contraseña.
	if _, _, err := s.PublicResolve(context.Background(), "tok", ""); !errors.Is(err, domain.ErrPasswordRequired) {
		t.Fatalf("sin prueba => %v, quiero ErrPasswordRequired", err)
	}
	// Prueba correcta: acceso concedido.
	if _, tn, err := s.PublicResolve(context.Background(), "tok", publicProof(hash)); err != nil || tn != "acme" {
		t.Fatalf("prueba válida: tn=%q err=%v", tn, err)
	}
	// Prueba incorrecta: se sigue exigiendo.
	if _, _, err := s.PublicResolve(context.Background(), "tok", "bogus"); !errors.Is(err, domain.ErrPasswordRequired) {
		t.Fatalf("prueba inválida => %v, quiero ErrPasswordRequired", err)
	}
}

func TestPublicResolveNoPassword(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{resolveSh: domain.Share{FileID: &fid}, resolveTn: "acme"}
	s := newSvc(fs, &fakeBlobs{}, nil)
	if _, tn, err := s.PublicResolve(context.Background(), "tok", ""); err != nil || tn != "acme" {
		t.Fatalf("enlace sin contraseña: tn=%q err=%v", tn, err)
	}
	// El error de ResolveLink se propaga.
	fs.resolveErr = errBoom
	if _, _, err := s.PublicResolve(context.Background(), "bad", ""); !errors.Is(err, errBoom) {
		t.Fatalf("error de ResolveLink debe propagarse, got %v", err)
	}
}

func TestPublicVerify(t *testing.T) {
	hash, _ := domain.HashPassword("hunter2")
	fs := &fakeStore{resolveSh: domain.Share{FolderID: ptr(uuid.New())}, resolveTn: "acme", resolveHash: hash}
	s := newSvc(fs, &fakeBlobs{}, nil)
	// Correcta => devuelve la prueba (== publicProof del hash).
	proof, err := s.PublicVerify(context.Background(), "tok", "hunter2")
	if err != nil || proof != publicProof(hash) {
		t.Fatalf("verify correcta: proof=%q err=%v", proof, err)
	}
	// Incorrecta => ErrWrongPassword.
	if _, err := s.PublicVerify(context.Background(), "tok", "nope"); !errors.Is(err, domain.ErrWrongPassword) {
		t.Fatalf("verify incorrecta => %v", err)
	}
	// Enlace sin contraseña => prueba vacía sin error.
	fs.resolveHash = ""
	if proof, err := s.PublicVerify(context.Background(), "tok", ""); err != nil || proof != "" {
		t.Fatalf("sin contraseña: proof=%q err=%v", proof, err)
	}
}

func TestPublicBrowseFileShare(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{file: domain.File{ID: fid, StorageKey: "k", Name: "doc.pdf"}}
	blobs := &fakeBlobs{getBody: "public"}
	s := newSvc(fs, blobs, nil)
	node, err := s.PublicBrowse(context.Background(), domain.Share{FileID: &fid}, "acme", "")
	if err != nil {
		t.Fatalf("PublicBrowse: %v", err)
	}
	if node.IsDir {
		t.Fatal("enlace a fichero no es carpeta")
	}
	got, _ := io.ReadAll(node.Body)
	_ = node.Body.Close()
	if string(got) != "public" {
		t.Fatalf("contenido: %q", got)
	}
	// Un enlace a fichero no admite subruta.
	if _, err := s.PublicBrowse(context.Background(), domain.Share{FileID: &fid}, "acme", "extra"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("subruta en enlace a fichero => %v", err)
	}
}

func TestPublicBrowseFolderListingAndDownload(t *testing.T) {
	folderID := uuid.New()
	sub := domain.Folder{ID: uuid.New(), Name: "sub"}
	fs := &fakeStore{
		folder:     domain.Folder{ID: folderID, Name: "Compartida"},
		subfolders: []domain.Folder{sub},
		page:       store.FilePage{Files: []domain.File{{ID: uuid.New(), Name: "a.txt", SizeBytes: 3}}},
	}
	blobs := &fakeBlobs{getBody: "hi"}
	s := newSvc(fs, blobs, nil)
	share := domain.Share{FolderID: &folderID}

	// Raíz => listado (subcarpetas + ficheros).
	node, err := s.PublicBrowse(context.Background(), share, "acme", "")
	if err != nil {
		t.Fatalf("PublicBrowse raíz: %v", err)
	}
	if !node.IsDir || len(node.Subfolders) != 1 || len(node.Files) != 1 {
		t.Fatalf("listado inesperado: %+v", node)
	}

	// Subruta a fichero (no es subcarpeta) => descarga por nombre.
	fs.subfolders = nil
	fs.byName = domain.File{ID: uuid.New(), Name: "a.txt", StorageKey: "k"}
	node, err = s.PublicBrowse(context.Background(), share, "acme", "a.txt")
	if err != nil {
		t.Fatalf("PublicBrowse fichero: %v", err)
	}
	if node.IsDir {
		t.Fatal("a.txt debería resolver a fichero")
	}
	_ = node.Body.Close()
}

// Con una carpeta y un fichero hermanos del MISMO nombre, un enlace público
// resuelve a la CARPETA. No es un capricho: el recorrido que había miraba las
// subcarpetas primero y solo caía al fichero si no encontraba ninguna, y
// `ResolvePath` —la otra entrada, la de WebDAV— prefiere justo lo contrario.
// Las dos comparten ahora el mismo descenso en SQL, así que si alguien mueve la
// preferencia al store, una de las dos cambia sin que nadie lo pida.
func TestPublicBrowsePrefiereLaCarpetaSobreElFicheroHomonimo(t *testing.T) {
	folderID := uuid.New()
	homonima := domain.Folder{ID: uuid.New(), Name: "informe"}
	fs := &fakeStore{
		folder:     domain.Folder{ID: homonima.ID, Name: "informe"},
		subfolders: []domain.Folder{homonima},
		byName:     domain.File{ID: uuid.New(), Name: "informe", StorageKey: "k"},
		page:       store.FilePage{},
	}
	s := newSvc(fs, &fakeBlobs{getBody: "hi"}, nil)

	node, err := s.PublicBrowse(context.Background(), domain.Share{FolderID: &folderID}, "acme", "informe")
	if err != nil {
		t.Fatalf("PublicBrowse: %v", err)
	}
	if !node.IsDir {
		t.Fatal("con los dos presentes, un enlace público tiene que resolver a la carpeta")
	}
}

func TestSharedBrowseGate(t *testing.T) {
	folderID := uuid.New()
	// Sin share vigente: deny-by-default (ErrNotFound, sin filtrar existencia).
	denied := &fakeStore{shareForErr: domain.ErrNotFound}
	s := newSvc(denied, &fakeBlobs{}, nil)
	if _, _, err := s.SharedBrowse(tenantCtx(t), "bob", nil, folderID, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin share => %v, quiero ErrNotFound", err)
	}

	// Con share de carpeta (viewer): navega y devuelve el permiso concedido.
	fs := &fakeStore{
		shareForRes: domain.Share{FolderID: &folderID, Permission: domain.PermRead},
		folder:      domain.Folder{ID: folderID, Name: "Compartida"},
		subfolders:  []domain.Folder{{ID: uuid.New(), Name: "sub"}},
		page:        store.FilePage{Files: []domain.File{{ID: uuid.New(), Name: "a.txt"}}},
	}
	s = newSvc(fs, &fakeBlobs{}, nil)
	node, perm, err := s.SharedBrowse(tenantCtx(t), "bob", []string{"eng"}, folderID, "")
	if err != nil {
		t.Fatalf("SharedBrowse: %v", err)
	}
	if !node.IsDir || len(node.Subfolders) != 1 || len(node.Files) != 1 {
		t.Fatalf("listado inesperado: %+v", node)
	}
	if perm != domain.PermRead {
		t.Fatalf("permiso = %q, quiero read", perm)
	}
	if len(fs.lastGroups) != 1 || fs.lastGroups[0] != "eng" {
		t.Fatalf("los grupos del JWT deben llegar a la verja: %v", fs.lastGroups)
	}
}

func TestSharedUploadPermission(t *testing.T) {
	folderID := uuid.New()
	// Viewer: no escribe (ErrPermission), y el blob nunca se sube.
	viewer := &fakeStore{shareForRes: domain.Share{FolderID: &folderID, Permission: domain.PermRead}}
	blobs := &fakeBlobs{}
	s := newSvc(viewer, blobs, &capturePub{})
	if _, err := s.SharedUpload(tenantCtx(t), "bob", nil, folderID, "", "f.txt", "text/plain", "bob", strings.NewReader("hi")); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("viewer => %v, quiero ErrPermission", err)
	}
	if blobs.putBody != nil {
		t.Fatalf("un viewer NO debe subir nada, got %q", blobs.putBody)
	}

	// Editor: sube (subpath "" resuelve a la raíz del share).
	editor := &fakeStore{
		shareForRes: domain.Share{FolderID: &folderID, Permission: domain.PermWrite},
		quota:       domain.Quota{LimitBytes: 1 << 20},
		file:        domain.File{ID: uuid.New(), Name: "f.txt", SizeBytes: 2},
	}
	s = newSvc(editor, &fakeBlobs{}, &capturePub{})
	if _, err := s.SharedUpload(tenantCtx(t), "bob", nil, folderID, "", "f.txt", "text/plain", "bob", strings.NewReader("hi")); err != nil {
		t.Fatalf("editor SharedUpload: %v", err)
	}
	if editor.newFile == nil || editor.newFile.Name != "f.txt" {
		t.Fatalf("el editor debe crear el metadato, got %+v", editor.newFile)
	}
}

func ptr[T any](v T) *T { return &v }

// --- operaciones en lote y ZIP -------------------------------------------

func TestBatchMoveEmitsSummary(t *testing.T) {
	fs := &fakeStore{batchMoveRes: store.BatchResult{Files: 2, Folders: 1}}
	pub := &capturePub{}
	s := newSvc(fs, &fakeBlobs{}, pub)
	files := []uuid.UUID{uuid.New(), uuid.New()}
	folders := []uuid.UUID{uuid.New()}
	target := uuid.New()
	res, err := s.BatchMove(tenantCtx(t), files, folders, target)
	if err != nil {
		t.Fatalf("BatchMove: %v", err)
	}
	if res.Files != 2 || res.Folders != 1 {
		t.Fatalf("resultado = %+v, quiero 2/1", res)
	}
	// Los ids y el destino llegan al store tal cual (sin N+1: una sola llamada).
	if len(fs.lastBatchFiles) != 2 || len(fs.lastBatchFolders) != 1 || fs.lastBatchTarget != target {
		t.Fatalf("no propagado al store: files=%v folders=%v target=%v", fs.lastBatchFiles, fs.lastBatchFolders, fs.lastBatchTarget)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.batch.moved" {
		t.Fatalf("esperaba batch.moved, got %v", pub.types)
	}
	// Error del store no emite y se propaga.
	fs.batchMoveErr = domain.ErrCycle
	pub.types = nil
	if _, err := s.BatchMove(tenantCtx(t), files, folders, target); !errors.Is(err, domain.ErrCycle) {
		t.Fatalf("ciclo debe propagarse, got %v", err)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe emitir tras error")
	}
}

func TestBatchTrashSoftKeepsBlobs(t *testing.T) {
	// Soft delete en lote: NO se purga ningún blob (recuperable desde la papelera).
	fs := &fakeStore{
		batchTrashDel: []store.DeletedFile{{StorageKey: "k1"}, {StorageKey: "k2"}},
		batchTrashN:   3,
	}
	blobs := &fakeBlobs{}
	pub := &capturePub{}
	s := newSvc(fs, blobs, pub)
	res, err := s.BatchTrash(tenantCtx(t), []uuid.UUID{uuid.New()}, []uuid.UUID{uuid.New()})
	if err != nil {
		t.Fatalf("BatchTrash: %v", err)
	}
	if res.Files != 2 || res.Folders != 3 {
		t.Fatalf("resultado = %+v, quiero 2 ficheros / 3 carpetas", res)
	}
	if len(blobs.delKeys) != 0 {
		t.Fatalf("la papelera en lote NO debe purgar blobs, got %v", blobs.delKeys)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.batch.trashed" {
		t.Fatalf("esperaba batch.trashed, got %v", pub.types)
	}
}

func TestZipEntriesPassesCap(t *testing.T) {
	fs := &fakeStore{zipEntries: []store.ZipEntry{{StorageKey: "k", Path: "a.txt", Size: 1}}}
	s := newSvc(fs, &fakeBlobs{}, nil)
	entries, err := s.ZipEntries(tenantCtx(t), []uuid.UUID{uuid.New()}, nil)
	if err != nil {
		t.Fatalf("ZipEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entradas = %d, quiero 1", len(entries))
	}
	if fs.lastZipMax != maxZipFiles {
		t.Fatalf("cap pasado = %d, quiero %d", fs.lastZipMax, maxZipFiles)
	}
	// El error de tamaño se propaga (para responder HTTP antes de escribir el zip).
	fs.zipErr = domain.ErrBatchTooLarge
	if _, err := s.ZipEntries(tenantCtx(t), nil, []uuid.UUID{uuid.New()}); !errors.Is(err, domain.ErrBatchTooLarge) {
		t.Fatalf("ErrBatchTooLarge debe propagarse, got %v", err)
	}
}

func TestStreamZipStreamsTreeAndSanitizes(t *testing.T) {
	blobs := &fakeBlobs{getBody: "DATA"}
	s := newSvc(&fakeStore{}, blobs, nil)
	entries := []ZipEntry{
		{StorageKey: "k1", Path: "Docs/sub/a.txt", Size: 4},   // estructura preservada
		{StorageKey: "k2", Path: "../../etc/passwd", Size: 4}, // traversal → saneado
		{StorageKey: "k3", Path: "b.txt", Size: 4},
	}
	var buf bytes.Buffer
	if err := s.StreamZip(tenantCtx(t), &buf, entries); err != nil {
		t.Fatalf("StreamZip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip inválido: %v", err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		// Anti path traversal: ninguna entrada absoluta ni con "..".
		if strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "..") {
			t.Fatalf("ruta insegura en el zip: %q", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("abrir entrada %q: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		got[f.Name] = string(b)
	}
	if len(got) != 3 {
		t.Fatalf("entradas del zip = %v, quiero 3", got)
	}
	if got["Docs/sub/a.txt"] != "DATA" || got["etc/passwd"] != "DATA" || got["b.txt"] != "DATA" {
		t.Fatalf("contenido/nombres inesperados: %v", got)
	}
}

func TestStreamZipBlobError(t *testing.T) {
	blobs := &fakeBlobs{getErr: errBoom}
	s := newSvc(&fakeStore{}, blobs, nil)
	var buf bytes.Buffer
	if err := s.StreamZip(tenantCtx(t), &buf, []ZipEntry{{StorageKey: "k", Path: "a.txt"}}); err == nil {
		t.Fatal("un blob ilegible debe devolver error")
	}
}

func TestSanitizeZipPath(t *testing.T) {
	cases := map[string]string{
		"a/b.txt":          "a/b.txt",
		"../../etc/passwd": "etc/passwd",
		"/abs/x":           "abs/x",
		`a\..\b`:           "a/b", // '\' normalizado y '..' descartado
		"./a/./b":          "a/b",
		"..":               "",
		"":                 "",
		"a//b":             "a/b",
		"a/\x00\x01/c":     "a/__/c", // control neutralizado
	}
	for in, want := range cases {
		if got := sanitizeZipPath(in); got != want {
			t.Errorf("sanitizeZipPath(%q) = %q, quiero %q", in, got, want)
		}
	}
}

func TestFileVersionsPrependsCurrent(t *testing.T) {
	fid := uuid.New()
	next := 2
	fs := &fakeStore{
		file:         domain.File{ID: fid, Version: 4, SizeBytes: 40, ModifiedBy: "editor-2"},
		versions:     []domain.FileVersion{{FileID: fid, Version: 3, SizeBytes: 30, AuthorSub: "editor-1"}},
		versionsNext: &next,
	}
	s := newSvc(fs, &fakeBlobs{}, nil)
	page, err := s.FileVersions(tenantCtx(t), fid, "", 10)
	if err != nil {
		t.Fatalf("FileVersions: %v", err)
	}
	// La primera página encabeza con la versión ACTUAL (is_current) desde el fichero.
	if len(page.Versions) != 2 {
		t.Fatalf("versiones=%d, quiero 2 (actual + 1 archivada)", len(page.Versions))
	}
	if !page.Versions[0].IsCurrent || page.Versions[0].Version != 4 || page.Versions[0].AuthorSub != "editor-2" {
		t.Fatalf("cabeza no es la versión actual: %+v", page.Versions[0])
	}
	if page.Versions[1].IsCurrent || page.Versions[1].Version != 3 {
		t.Fatalf("archivada inesperada: %+v", page.Versions[1])
	}
	if page.NextCursor != "2" {
		t.Fatalf("next_cursor = %q, quiero \"2\"", page.NextCursor)
	}
	// Página siguiente (cursor): no se antepone la versión actual, pero la verja de
	// acceso SÍ se aplica (ver el caso de abajo).
	page, err = s.FileVersions(tenantCtx(t), fid, "2", 10)
	if err != nil {
		t.Fatalf("FileVersions página 2: %v", err)
	}
	if len(page.Versions) != 1 || page.Versions[0].Version != 3 {
		t.Fatalf("con cursor no debe anteponer la actual: %+v", page.Versions)
	}
	// Cursor corrupto es entrada inválida (no un 500).
	if _, err := s.FileVersions(tenantCtx(t), fid, "!!bad!!", 10); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("cursor corrupto => %v, quiero ErrInvalidName", err)
	}
	// Fichero inexistente/ajeno en la primera página: ErrNotFound (RLS, sin fuga).
	fs2 := &fakeStore{fileErr: domain.ErrNotFound}
	if _, err := newSvc(fs2, &fakeBlobs{}, nil).FileVersions(tenantCtx(t), fid, "", 10); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("fichero ausente => %v, quiero ErrNotFound", err)
	}
}

func TestRestoreVersionPermissionGate(t *testing.T) {
	fid := uuid.New()
	// Viewer (read): NO restaura (ErrPermission) y el store NUNCA se toca.
	viewer := &fakeStore{restoreVer: domain.File{ID: fid, Version: 5}}
	pub := &capturePub{}
	s := newSvc(viewer, &fakeBlobs{}, pub)
	if _, err := s.RestoreVersion(tenantCtx(t), fid, 2, "bob", domain.PermRead); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("viewer => %v, quiero ErrPermission", err)
	}
	if viewer.lastRestoreVer != 0 {
		t.Fatalf("un viewer NO debe llegar al store, got versión %d", viewer.lastRestoreVer)
	}
	if len(pub.types) != 0 {
		t.Fatal("no debe emitir tras denegar")
	}
	// Editor (write): restaura y emite el evento.
	editor := &fakeStore{restoreVer: domain.File{ID: fid, Version: 5}}
	pub = &capturePub{}
	s = newSvc(editor, &fakeBlobs{}, pub)
	f, err := s.RestoreVersion(tenantCtx(t), fid, 2, "bob", domain.PermWrite)
	if err != nil {
		t.Fatalf("editor RestoreVersion: %v", err)
	}
	if editor.lastRestoreVer != 2 || f.Version != 5 {
		t.Fatalf("restauración: versión pedida=%d, fichero=%+v", editor.lastRestoreVer, f)
	}
	if len(pub.types) != 1 || pub.types[0] != "drive.file.version.restored" {
		t.Fatalf("esperaba version.restored, got %v", pub.types)
	}
}

func TestOpenVersionContent(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{getVersion: domain.FileVersion{FileID: fid, Version: 2, StorageKey: "k", ContentType: "text/plain"}}
	blobs := &fakeBlobs{getBody: "old-content"}
	s := newSvc(fs, blobs, nil)
	v, rc, err := s.OpenVersionContent(tenantCtx(t), fid, 2)
	if err != nil {
		t.Fatalf("OpenVersionContent: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "old-content" || v.Version != 2 {
		t.Fatalf("contenido=%q versión=%d", got, v.Version)
	}
	// Blob ausente mapea a ErrNotFound.
	blobs.getErr = objectstore.ErrNotFound
	if _, _, err := s.OpenVersionContent(tenantCtx(t), fid, 2); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("blob ausente => %v, quiero ErrNotFound", err)
	}
	// Versión inexistente/ajena: el error del store se propaga.
	fs.getVersionErr = domain.ErrNotFound
	if _, _, err := s.OpenVersionContent(tenantCtx(t), fid, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("versión ausente => %v, quiero ErrNotFound", err)
	}
}

func TestListFilesPageCursorRoundTrip(t *testing.T) {
	next := store.FileCursor{NameLower: "b.txt", ID: uuid.New()}
	fs := &fakeStore{page: store.FilePage{Files: []domain.File{{ID: uuid.New()}}, Next: &next}}
	s := newSvc(fs, &fakeBlobs{}, nil)
	page, err := s.ListFilesPage(tenantCtx(t), uuid.New(), "", 10)
	if err != nil {
		t.Fatalf("ListFilesPage: %v", err)
	}
	if page.NextCursor == "" {
		t.Fatal("esperaba cursor de continuación")
	}
	// El cursor generado se acepta como entrada de la siguiente página.
	if _, err := s.ListFilesPage(tenantCtx(t), uuid.New(), page.NextCursor, 10); err != nil {
		t.Fatalf("cursor no round-trip: %v", err)
	}
	// Cursor corrupto es entrada inválida.
	if _, err := s.ListFilesPage(tenantCtx(t), uuid.New(), "!!!not-base64!!!", 10); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("cursor corrupto: %v", err)
	}
}

func TestRecentCursorRoundTrip(t *testing.T) {
	next := store.RecentCursor{UpdatedAt: time.Now().UTC().Truncate(time.Microsecond), ID: uuid.New()}
	fs := &fakeStore{
		recentEntries: []store.Entry{{ID: uuid.New(), Name: "a.txt", Path: "/a.txt"}},
		recentNext:    &next,
	}
	s := newSvc(fs, &fakeBlobs{}, nil)
	page, err := s.Recent(tenantCtx(t), "", 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if page.NextCursor == "" {
		t.Fatal("esperaba cursor de continuación")
	}
	// El cursor generado se acepta como entrada de la página siguiente.
	if _, err := s.Recent(tenantCtx(t), page.NextCursor, 10); err != nil {
		t.Fatalf("cursor no round-trip: %v", err)
	}
	// Cursor corrupto es entrada inválida (no un 500).
	if _, err := s.Recent(tenantCtx(t), "!!!not-base64!!!", 10); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("cursor corrupto: %v", err)
	}
}

func TestSetFavoriteValidation(t *testing.T) {
	fs := &fakeStore{}
	s := newSvc(fs, &fakeBlobs{}, nil)
	ctx := tenantCtx(t)
	fid := uuid.New()
	folderID := uuid.New()
	// Ni file ni folder.
	if err := s.SetFavorite(ctx, "u", nil, nil, true); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("sin objetivo: %v", err)
	}
	// Ambos a la vez.
	if err := s.SetFavorite(ctx, "u", &fid, &folderID, true); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("ambos objetivos: %v", err)
	}
	// Un objetivo válido se propaga al store.
	if err := s.SetFavorite(ctx, "u", &fid, nil, true); err != nil {
		t.Fatalf("SetFavorite válido: %v", err)
	}
	if fs.lastFavFile == nil || !fs.lastFavOn {
		t.Fatalf("no propagado al store: file=%v on=%v", fs.lastFavFile, fs.lastFavOn)
	}
}

func TestQuotaMethods(t *testing.T) {
	fs := &fakeStore{quota: domain.Quota{LimitBytes: 500}}
	s := newSvc(fs, &fakeBlobs{}, nil)
	if _, err := s.Quota(tenantCtx(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetQuotaLimit(tenantCtx(t), 1000); err != nil {
		t.Fatal(err)
	}
}

func TestRootAndPassthroughs(t *testing.T) {
	fs := &fakeStore{
		root:   domain.Folder{ID: uuid.New()},
		folder: domain.Folder{ID: uuid.New()},
		file:   domain.File{ID: uuid.New()},
		share:  domain.Share{ID: uuid.New()},
	}
	s := newSvc(fs, &fakeBlobs{}, nil)
	ctx := tenantCtx(t)
	if _, err := s.Root(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFolder(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ListChildren(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameFolder(ctx, uuid.New(), "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveFolder(ctx, uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFile(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameFile(ctx, uuid.New(), "y"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveFile(ctx, uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(ctx, "q", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListFileShares(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListFolderShares(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeShare(ctx, uuid.New()); err != nil {
		t.Fatal(err)
	}
}

// TestVersionHistoryGateAppliesToEveryPage: la verja del historial vivía DENTRO del
// `if before == 0`, así que pedir la página dos —con un cursor cualquiera— la saltaba
// entera y listaba las versiones de un fichero que no se puede abrir. La autorización
// descansaba en la RLS de `file_versions`, que solo aísla por TENANT y dentro de la
// organización no separa nada.
func TestVersionHistoryGateAppliesToEveryPage(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{
		fileErr:  domain.ErrNotFound, // el fichero no es accesible para quien pregunta
		versions: []domain.FileVersion{{FileID: fid, Version: 3}},
	}
	s := newSvc(fs, &fakeBlobs{}, nil)

	for _, cursor := range []string{"", "2", "99"} {
		if _, err := s.FileVersions(tenantCtx(t), fid, cursor, 10); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("cursor %q: err = %v, quiero ErrNotFound (la verja va en TODAS las páginas)", cursor, err)
		}
	}
}

// TestOpenVersionContentNeedsAccessToTheFile: descargar el blob de una versión archivada
// se gateaba con la RLS de `file_versions` —solo tenant—, así que quien conociera un id
// de fichero y un número de versión se llevaba el contenido de un fichero al que no tiene
// acceso: un adjunto de canal, el fichero de otra persona. La verja es el fichero PADRE.
func TestOpenVersionContentNeedsAccessToTheFile(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{
		fileErr: domain.ErrNotFound,
	}
	s := newSvc(fs, &fakeBlobs{}, nil)
	if _, _, err := s.OpenVersionContent(tenantCtx(t), fid, 3); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound: sin acceso al fichero no hay blob", err)
	}
}
