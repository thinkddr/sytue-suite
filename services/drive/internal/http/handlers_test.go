// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de los handlers HTTP de Sytue Drive con un fake del servicio, sin
// PostgreSQL ni S3. Ejercitan rutas OK, validación de entrada, mapeo de errores
// de dominio a códigos HTTP, el sobre JSON canónico y las cabeceras de descarga.
package drivehttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// --- fake service --------------------------------------------------------

type fakeSvc struct {
	labelSet     *domain.Label // último snapshot recibido por SetFileLabel
	labelPerm    domain.Permission
	labelErr     error
	root         domain.Folder
	folder       domain.Folder
	folderErr    error
	rootErr      error
	childFold    []domain.Folder
	childErr     error
	page         service.FilePage
	pageErr      error
	created      domain.Folder
	createErr    error
	renamed      domain.Folder
	renameErr    error
	moved        domain.Folder
	moveErr      error
	delFldErr    error
	file         domain.File
	fileErr      error
	uploaded     domain.File
	uploadErr    error
	presignUp    service.PresignResult
	presignUpErr error
	lastPresign  string
	finalizeFile domain.File
	finalizeErr  error
	lastHandle   string
	openFile     domain.File
	openBody     string
	openErr      error
	thumbBody    string
	thumbType    string
	thumbETag    string
	thumbErr     error
	presignURL   *url.URL
	presignErr   error
	renFile      domain.File
	renFileErr   error
	movFile      domain.File
	movFileErr   error
	delFileErr   error
	copyFile     domain.File
	copyFileEr   error
	copyFold     domain.Folder
	copyFoldEr   error
	lastCopyID   uuid.UUID
	lastCopyDs   uuid.UUID
	lastCopyPr   domain.Permission
	trashFold    []domain.Folder
	trashFile    []domain.File
	trashErr     error
	restFile     domain.File
	restFerr     error
	restFold     domain.Folder
	restFldErr   error
	purgeFErr    error
	purgeFldEr   error
	search       []service.Entry
	filePath     string
	filePathErr  error
	searchErr    error
	quota        domain.Quota
	quotaErr     error
	setQuota     domain.Quota
	setQErr      error
	share        domain.Share
	shareErr     error
	fileShares   []domain.Share
	fileShErr    error
	foldShares   []domain.Share
	foldShErr    error
	revokeErr    error

	recent    service.RecentPage
	recentErr error
	favs      []service.Entry
	favErr    error
	setFavErr error
	lastFavOn bool

	// público (enlace)
	pubShare      domain.Share
	pubTenant     string
	pubResolveErr error
	pubProof      string
	pubVerifyErr  error
	pubNode       service.PublicNode
	pubBrowseErr  error

	// compartido conmigo
	sharedEntries []service.SharedEntry
	sharedErr     error
	sharedNode    service.PublicNode
	sharedPerm    domain.Permission
	sharedErr2    error
	sharedUpload  domain.File
	sharedUpErr   error

	// versiones
	versions        service.VersionPage
	versionsErr     error
	openVer         domain.FileVersion
	openVerBody     string
	openVerErr      error
	restoreVer      domain.File
	restoreVerErr   error
	lastRestoreVer  int
	lastRestorePerm domain.Permission

	// lote y zip
	batchMove        service.BatchResult
	batchMoveErr     error
	batchTrash       service.BatchResult
	batchTrashErr    error
	zipEntries       []service.ZipEntry
	zipEntriesErr    error
	zipStreamErr     error
	zipBody          string
	lastBatchFiles   []uuid.UUID
	lastBatchFolders []uuid.UUID
	lastBatchTarget  uuid.UUID

	// captura
	lastUploadName string
	lastShareInput service.CreateShareInput
	lastVerifyPass string
	lastGroups     []string

	// unidades de equipo
	teamDrive      domain.TeamDrive
	teamDriveErr   error
	teamRole       domain.TeamRole
	teamDrives     []service.TeamDriveMembership
	teamListErr    error
	teamRenamed    domain.TeamDrive
	teamRenameErr  error
	teamArchived   domain.TeamDrive
	teamArchiveErr error
	teamMembers    []domain.TeamMember
	teamMembersErr error
	teamMember     domain.TeamMember
	teamPutErr     error
	teamRemoveErr  error
	teamListing    service.TeamListing
	teamChildErr   error
	teamFolder     domain.Folder
	teamFolderErr  error
	teamUpload     domain.File
	teamUploadErr  error
	teamOpenFile   domain.File
	teamOpenBody   string
	teamOpenErr    error
	teamDelFldErr  error
	teamDelFileErr error
	// adjuntos de canal de Rooms
	chanFile domain.File
	chanBody string
	chanErr  error
	// captura de unidades de equipo
	lastTeamArchived  bool
	lastTeamMemberSub string
	lastTeamMemberRol domain.TeamRole
	lastTeamRemoveSub string
	lastTeamFolderID  uuid.UUID
	lastChannelID     uuid.UUID
}

func (f *fakeSvc) Root(context.Context) (domain.Folder, error) { return f.root, f.rootErr }
func (f *fakeSvc) GetFolder(context.Context, uuid.UUID) (domain.Folder, error) {
	return f.folder, f.folderErr
}

func (f *fakeSvc) ListSubfolders(context.Context, uuid.UUID) ([]domain.Folder, error) {
	return f.childFold, f.childErr
}

func (f *fakeSvc) ListFilesPage(context.Context, uuid.UUID, string, int) (service.FilePage, error) {
	return f.page, f.pageErr
}

func (f *fakeSvc) CreateFolder(context.Context, uuid.UUID, string, string, bool) (domain.Folder, error) {
	return f.created, f.createErr
}

func (f *fakeSvc) RenameFolder(context.Context, uuid.UUID, string) (domain.Folder, error) {
	return f.renamed, f.renameErr
}

func (f *fakeSvc) MoveFolder(context.Context, uuid.UUID, uuid.UUID) (domain.Folder, error) {
	return f.moved, f.moveErr
}
func (f *fakeSvc) DeleteFolder(context.Context, uuid.UUID) error { return f.delFldErr }
func (f *fakeSvc) UploadFile(_ context.Context, _ uuid.UUID, name, _, _ string, body io.Reader) (domain.File, error) {
	f.lastUploadName = name
	_, _ = io.Copy(io.Discard, body)
	return f.uploaded, f.uploadErr
}

func (f *fakeSvc) PresignUpload(_ context.Context, path string, size int64, _, _ string) (service.PresignResult, error) {
	f.lastPresign = path
	return f.presignUp, f.presignUpErr
}

func (f *fakeSvc) FinalizeUpload(_ context.Context, handle string) (domain.File, error) {
	f.lastHandle = handle
	return f.finalizeFile, f.finalizeErr
}

func (f *fakeSvc) GetFile(context.Context, uuid.UUID) (domain.File, error) { return f.file, f.fileErr }

// FilePath: el fake compone la ruta con lo que ya tiene. Un fallo al resolverla NO debe
// tumbar GetFile (el metadato sigue siendo válido), y eso es lo que ejercita filePathErr.
func (f *fakeSvc) FilePath(context.Context, uuid.UUID) (string, error) {
	if f.filePathErr != nil {
		return "", f.filePathErr
	}
	return f.filePath, nil
}

func (f *fakeSvc) OpenFileContent(context.Context, uuid.UUID) (domain.File, io.ReadCloser, error) {
	if f.openErr != nil {
		return domain.File{}, nil, f.openErr
	}
	return f.openFile, io.NopCloser(strings.NewReader(f.openBody)), nil
}

func (f *fakeSvc) Thumbnail(context.Context, uuid.UUID) (io.ReadCloser, string, string, error) {
	if f.thumbErr != nil {
		return nil, "", "", f.thumbErr
	}
	return io.NopCloser(strings.NewReader(f.thumbBody)), f.thumbType, f.thumbETag, nil
}

func (f *fakeSvc) PresignDownload(context.Context, uuid.UUID) (*url.URL, error) {
	return f.presignURL, f.presignErr
}

func (f *fakeSvc) RenameFile(context.Context, uuid.UUID, string) (domain.File, error) {
	return f.renFile, f.renFileErr
}

func (f *fakeSvc) SetFileLabel(_ context.Context, _ uuid.UUID, label domain.Label, perm domain.Permission) (domain.File, error) {
	f.labelPerm = perm
	f.labelSet = &label
	if f.labelErr != nil {
		return domain.File{}, f.labelErr
	}
	out := f.renFile
	if label.ID == "" {
		out.Label = nil
	} else {
		l := label
		out.Label = &l
	}
	return out, nil
}

func (f *fakeSvc) MoveFile(context.Context, uuid.UUID, uuid.UUID) (domain.File, error) {
	return f.movFile, f.movFileErr
}
func (f *fakeSvc) DeleteFile(context.Context, uuid.UUID) error { return f.delFileErr }

func (f *fakeSvc) CopyFile(_ context.Context, srcID, destFolderID uuid.UUID, _ string, perm domain.Permission) (domain.File, error) {
	f.lastCopyID, f.lastCopyDs, f.lastCopyPr = srcID, destFolderID, perm
	return f.copyFile, f.copyFileEr
}

func (f *fakeSvc) CopyFolder(_ context.Context, srcID uuid.UUID, _ *uuid.UUID, _ string, perm domain.Permission) (domain.Folder, error) {
	f.lastCopyID, f.lastCopyPr = srcID, perm
	return f.copyFold, f.copyFoldEr
}

func (f *fakeSvc) Recent(context.Context, string, int) (service.RecentPage, error) {
	return f.recent, f.recentErr
}

func (f *fakeSvc) Favorites(context.Context, string) ([]service.Entry, error) {
	return f.favs, f.favErr
}

func (f *fakeSvc) SetFavorite(_ context.Context, _ string, _, _ *uuid.UUID, on bool) error {
	f.lastFavOn = on
	return f.setFavErr
}

func (f *fakeSvc) ListTrash(context.Context) ([]domain.Folder, []domain.File, error) {
	return f.trashFold, f.trashFile, f.trashErr
}

func (f *fakeSvc) RestoreFile(context.Context, uuid.UUID) (domain.File, error) {
	return f.restFile, f.restFerr
}

func (f *fakeSvc) RestoreFolder(context.Context, uuid.UUID) (domain.Folder, error) {
	return f.restFold, f.restFldErr
}
func (f *fakeSvc) PurgeFile(context.Context, uuid.UUID) error   { return f.purgeFErr }
func (f *fakeSvc) PurgeFolder(context.Context, uuid.UUID) error { return f.purgeFldEr }
func (f *fakeSvc) Search(context.Context, string, int) ([]service.Entry, error) {
	return f.search, f.searchErr
}
func (f *fakeSvc) Quota(context.Context) (domain.Quota, error) { return f.quota, f.quotaErr }
func (f *fakeSvc) SetQuotaLimit(context.Context, int64) (domain.Quota, error) {
	return f.setQuota, f.setQErr
}

func (f *fakeSvc) CreateShare(_ context.Context, in service.CreateShareInput) (domain.Share, error) {
	f.lastShareInput = in
	return f.share, f.shareErr
}

func (f *fakeSvc) ListFileShares(context.Context, uuid.UUID) ([]domain.Share, error) {
	return f.fileShares, f.fileShErr
}

func (f *fakeSvc) ListFolderShares(context.Context, uuid.UUID) ([]domain.Share, error) {
	return f.foldShares, f.foldShErr
}
func (f *fakeSvc) RevokeShare(context.Context, uuid.UUID) error { return f.revokeErr }
func (f *fakeSvc) PublicResolve(context.Context, string, string) (domain.Share, string, error) {
	return f.pubShare, f.pubTenant, f.pubResolveErr
}

func (f *fakeSvc) PublicVerify(_ context.Context, _, password string) (string, error) {
	f.lastVerifyPass = password
	return f.pubProof, f.pubVerifyErr
}

func (f *fakeSvc) PublicBrowse(context.Context, domain.Share, string, string) (service.PublicNode, error) {
	return f.pubNode, f.pubBrowseErr
}

func (f *fakeSvc) SharedWithMe(_ context.Context, _ string, groups []string) ([]service.SharedEntry, error) {
	f.lastGroups = groups
	return f.sharedEntries, f.sharedErr
}

func (f *fakeSvc) SharedBrowse(_ context.Context, _ string, groups []string, _ uuid.UUID, _ string) (service.PublicNode, domain.Permission, error) {
	f.lastGroups = groups
	return f.sharedNode, f.sharedPerm, f.sharedErr2
}

func (f *fakeSvc) SharedUpload(_ context.Context, _ string, _ []string, _ uuid.UUID, _, name, _, _ string, body io.Reader) (domain.File, error) {
	f.lastUploadName = name
	_, _ = io.Copy(io.Discard, body)
	return f.sharedUpload, f.sharedUpErr
}

func (f *fakeSvc) FileVersions(context.Context, uuid.UUID, string, int) (service.VersionPage, error) {
	return f.versions, f.versionsErr
}

func (f *fakeSvc) OpenVersionContent(_ context.Context, _ uuid.UUID, version int) (domain.FileVersion, io.ReadCloser, error) {
	if f.openVerErr != nil {
		return domain.FileVersion{}, nil, f.openVerErr
	}
	f.lastRestoreVer = version
	return f.openVer, io.NopCloser(strings.NewReader(f.openVerBody)), nil
}

func (f *fakeSvc) RestoreVersion(_ context.Context, _ uuid.UUID, version int, _ string, perm domain.Permission) (domain.File, error) {
	f.lastRestoreVer = version
	f.lastRestorePerm = perm
	return f.restoreVer, f.restoreVerErr
}

func (f *fakeSvc) BatchMove(_ context.Context, fileIDs, folderIDs []uuid.UUID, target uuid.UUID) (service.BatchResult, error) {
	f.lastBatchFiles, f.lastBatchFolders, f.lastBatchTarget = fileIDs, folderIDs, target
	return f.batchMove, f.batchMoveErr
}

func (f *fakeSvc) BatchTrash(_ context.Context, fileIDs, folderIDs []uuid.UUID) (service.BatchResult, error) {
	f.lastBatchFiles, f.lastBatchFolders = fileIDs, folderIDs
	return f.batchTrash, f.batchTrashErr
}

func (f *fakeSvc) ZipEntries(_ context.Context, fileIDs, folderIDs []uuid.UUID) ([]service.ZipEntry, error) {
	f.lastBatchFiles, f.lastBatchFolders = fileIDs, folderIDs
	return f.zipEntries, f.zipEntriesErr
}

func (f *fakeSvc) StreamZip(_ context.Context, w io.Writer, _ []service.ZipEntry) error {
	if f.zipStreamErr != nil {
		return f.zipStreamErr
	}
	_, _ = io.WriteString(w, f.zipBody)
	return nil
}

func (f *fakeSvc) CreateTeamDrive(context.Context, string, string) (domain.TeamDrive, error) {
	return f.teamDrive, f.teamDriveErr
}

func (f *fakeSvc) ListTeamDrives(context.Context, string) ([]service.TeamDriveMembership, error) {
	return f.teamDrives, f.teamListErr
}

func (f *fakeSvc) GetTeamDrive(context.Context, uuid.UUID, string) (domain.TeamDrive, domain.TeamRole, error) {
	return f.teamDrive, f.teamRole, f.teamDriveErr
}

func (f *fakeSvc) RenameTeamDrive(_ context.Context, _ uuid.UUID, _, _ string) (domain.TeamDrive, error) {
	return f.teamRenamed, f.teamRenameErr
}

func (f *fakeSvc) SetTeamDriveArchived(_ context.Context, _ uuid.UUID, _ string, archived bool) (domain.TeamDrive, error) {
	f.lastTeamArchived = archived
	return f.teamArchived, f.teamArchiveErr
}

func (f *fakeSvc) ListTeamDriveMembers(context.Context, uuid.UUID, string) ([]domain.TeamMember, error) {
	return f.teamMembers, f.teamMembersErr
}

func (f *fakeSvc) PutTeamDriveMember(_ context.Context, _ uuid.UUID, _, subject string, role domain.TeamRole) (domain.TeamMember, error) {
	f.lastTeamMemberSub, f.lastTeamMemberRol = subject, role
	return f.teamMember, f.teamPutErr
}

func (f *fakeSvc) RemoveTeamDriveMember(_ context.Context, _ uuid.UUID, _, subject string) error {
	f.lastTeamRemoveSub = subject
	return f.teamRemoveErr
}

func (f *fakeSvc) TeamListChildren(_ context.Context, _ uuid.UUID, _ string, folderID uuid.UUID, _ string, _ int) (service.TeamListing, error) {
	f.lastTeamFolderID = folderID
	return f.teamListing, f.teamChildErr
}

func (f *fakeSvc) TeamCreateFolder(_ context.Context, _ uuid.UUID, _ string, parentID uuid.UUID, _ string) (domain.Folder, error) {
	f.lastTeamFolderID = parentID
	return f.teamFolder, f.teamFolderErr
}

func (f *fakeSvc) TeamUploadFile(_ context.Context, _ uuid.UUID, _ string, folderID uuid.UUID, name, _ string, body io.Reader) (domain.File, error) {
	f.lastUploadName = name
	f.lastTeamFolderID = folderID
	_, _ = io.Copy(io.Discard, body)
	return f.teamUpload, f.teamUploadErr
}

func (f *fakeSvc) TeamOpenFileContent(context.Context, uuid.UUID, string, uuid.UUID) (domain.File, io.ReadCloser, error) {
	if f.teamOpenErr != nil {
		return domain.File{}, nil, f.teamOpenErr
	}
	return f.teamOpenFile, io.NopCloser(strings.NewReader(f.teamOpenBody)), nil
}

func (f *fakeSvc) TeamDeleteFolder(context.Context, uuid.UUID, string, uuid.UUID) error {
	return f.teamDelFldErr
}

func (f *fakeSvc) TeamDeleteFile(context.Context, uuid.UUID, string, uuid.UUID) error {
	return f.teamDelFileErr
}

// Adjuntos de canal de Rooms: el fake reproduce la verja (chanErr = no-miembro o
// fichero de otro canal ⇒ ErrNotFound); su enforcement REAL se prueba por
// integración contra Postgres (service/channelfiles_integration_test.go).
func (f *fakeSvc) ChannelUploadFile(_ context.Context, channelID uuid.UUID, _, name, _ string, body io.Reader) (domain.File, error) {
	f.lastChannelID, f.lastUploadName = channelID, name
	_, _ = io.Copy(io.Discard, body)
	return f.chanFile, f.chanErr
}

func (f *fakeSvc) ChannelFile(_ context.Context, channelID, _ uuid.UUID, _ string) (domain.File, error) {
	f.lastChannelID = channelID
	return f.chanFile, f.chanErr
}

func (f *fakeSvc) ChannelFileContent(_ context.Context, channelID, _ uuid.UUID, _ string) (domain.File, io.ReadCloser, error) {
	f.lastChannelID = channelID
	if f.chanErr != nil {
		return domain.File{}, nil, f.chanErr
	}
	return f.chanFile, io.NopCloser(strings.NewReader(f.chanBody)), nil
}

func (f *fakeSvc) ChannelThumbnail(_ context.Context, channelID, _ uuid.UUID, _ string) (io.ReadCloser, string, string, error) {
	f.lastChannelID = channelID
	if f.chanErr != nil {
		return nil, "", "", f.chanErr
	}
	return io.NopCloser(strings.NewReader(f.chanBody)), "image/jpeg", `"thumb"`, nil
}

func (f *fakeSvc) CopyChannelFileToMyDrive(_ context.Context, channelID, fileID uuid.UUID, _ string) (domain.File, error) {
	f.lastChannelID = channelID
	if f.chanErr != nil {
		return domain.File{}, f.chanErr
	}
	return domain.File{ID: uuid.New(), Name: f.chanFile.Name, SizeBytes: f.chanFile.SizeBytes}, nil
}

func (*fakeSvc) ExternalStorageEnabled(context.Context) bool { return false }
func (*fakeSvc) ForgetExternalStorage(string)                {}
func (*fakeSvc) ExternalInstances(context.Context) (service.ExternalAccounts, error) {
	return service.ExternalAccounts{}, nil
}
func (*fakeSvc) ProbeExternal(context.Context, store.ExternalStorage, string) string { return "" }
func (*fakeSvc) ConnectNAS(context.Context, service.NASRequest, string) (service.NASStatus, error) {
	return service.NASStatus{}, nil
}

func (*fakeSvc) NASStatus(context.Context) (service.NASStatus, error) {
	return service.NASStatus{}, nil
}
func (*fakeSvc) TeardownNAS(context.Context, store.ExternalStorage) error { return nil }

func (f *fakeSvc) PedirEdicion(context.Context, uuid.UUID, []string, peticiones.Peticion, string) (bool, error) {
	return false, nil
}

func (f *fakeSvc) PeticionesEdicion(context.Context, uuid.UUID, string) ([]peticiones.Peticion, error) {
	return nil, nil
}

func (f *fakeSvc) ResolverPeticion(context.Context, uuid.UUID, string, string, bool) error {
	return nil
}

func (f *fakeSvc) PedidasPor(context.Context, string, []uuid.UUID) (map[uuid.UUID]bool, error) {
	return map[uuid.UUID]bool{}, nil
}

var _ driveService = (*fakeSvc)(nil)

// --- helpers -------------------------------------------------------------

func newHandlers(svc driveService) *Handlers {
	return &Handlers{svc: svc, cfg: config.Config{PublicURL: "https://drive.sytue.eu", MaxUploadBytes: 1024}}
}

func withAuth(r *http.Request) *http.Request {
	ctx := authn.Into(r.Context(), authn.Claims{Subject: "user-1", Tenant: "acme"})
	return r.WithContext(ctx)
}

func withParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func mkReq(method, target, body string, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	r = withAuth(r)
	if len(params) > 0 {
		r = withParams(r, params)
	}
	return r
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("no es sobre de error JSON: %v (%s)", err, rec.Body.String())
	}
	return env.Error.Code
}

// --- folders -------------------------------------------------------------

func TestGetRoot(t *testing.T) {
	svc := &fakeSvc{root: domain.Folder{ID: uuid.New(), IsRoot: true}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.GetRoot(rec, mkReq(http.MethodGet, "/api/v1/folders/root", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestGetRootError(t *testing.T) {
	h := newHandlers(&fakeSvc{rootErr: domain.ErrQuotaExceeded})
	rec := httptest.NewRecorder()
	h.GetRoot(rec, mkReq(http.MethodGet, "/x", "", nil))
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("cuota => %d, quiero 507", rec.Code)
	}
}

func TestGetFolderOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{folder: domain.Folder{ID: id, Name: "docs"}})
	rec := httptest.NewRecorder()
	h.GetFolder(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
}

func TestGetFolderBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.GetFolder(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido => %d, quiero 400", rec.Code)
	}
}

func TestGetFolderNotFound(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{folderErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.GetFolder(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
	if code := errCode(t, rec); code != "not_found" {
		t.Fatalf("code del sobre = %q, quiero not_found", code)
	}
}

func TestListChildrenNilSlicesBecomeEmpty(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{}) // childFold/childFile nil
	rec := httptest.NewRecorder()
	h.ListChildren(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Folders == nil || got.Files == nil {
		t.Fatalf("slices nil no normalizados: %+v", got)
	}
}

func TestListChildrenError(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{childErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.ListChildren(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestListChildrenKeyset(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{
		childFold: []domain.Folder{{ID: uuid.New(), Name: "sub"}},
		page:      service.FilePage{Files: []domain.File{{ID: uuid.New()}}, NextCursor: "tok"},
	}
	h := newHandlers(svc)

	// Primera página: subcarpetas + ficheros + next_cursor.
	rec := httptest.NewRecorder()
	h.ListChildren(rec, mkReq(http.MethodGet, "/x?limit=1", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Folders) != 1 || got.NextCursor != "tok" {
		t.Fatalf("primera página: %+v", got)
	}

	// Página siguiente (cursor): sin subcarpetas, solo ficheros.
	rec = httptest.NewRecorder()
	h.ListChildren(rec, mkReq(http.MethodGet, "/x?cursor=tok", "", map[string]string{"id": id.String()}))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Folders) != 0 || len(got.Files) != 1 {
		t.Fatalf("página con cursor: %+v", got)
	}
}

func TestListFolderFilesWithCursor(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{page: service.FilePage{Files: []domain.File{{ID: uuid.New()}}, NextCursor: "next-tok"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.ListFolderFiles(rec, mkReq(http.MethodGet, "/x?limit=10", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["next_cursor"] != "next-tok" {
		t.Fatalf("next_cursor = %v", got["next_cursor"])
	}
}

func TestListFolderFilesNoCursor(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{page: service.FilePage{}}) // sin ficheros ni cursor
	rec := httptest.NewRecorder()
	h.ListFolderFiles(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if _, ok := got["next_cursor"]; ok {
		t.Fatal("no debe haber next_cursor sin más páginas")
	}
	if _, ok := got["files"]; !ok {
		t.Fatal("debe incluir files (vacío)")
	}
}

func TestCreateFolderOK(t *testing.T) {
	parent := uuid.New()
	svc := &fakeSvc{created: domain.Folder{ID: uuid.New(), Name: "nueva"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.CreateFolder(rec, mkReq(http.MethodPost, "/api/v1/folders", `{"parent_id":"`+parent.String()+`","name":"nueva"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201", rec.Code)
	}
}

func TestCreateFolderBadJSON(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.CreateFolder(rec, mkReq(http.MethodPost, "/x", `{bad`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

func TestCreateFolderBadParent(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.CreateFolder(rec, mkReq(http.MethodPost, "/x", `{"parent_id":"nope","name":"x"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("parent inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateFolderConflict(t *testing.T) {
	parent := uuid.New()
	h := newHandlers(&fakeSvc{createErr: domain.ErrConflict})
	rec := httptest.NewRecorder()
	h.CreateFolder(rec, mkReq(http.MethodPost, "/x", `{"parent_id":"`+parent.String()+`","name":"x"}`, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("código = %d, quiero 409", rec.Code)
	}
}

func TestUpdateFolderRename(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{renamed: domain.Folder{ID: id, Name: "r"}})
	rec := httptest.NewRecorder()
	h.UpdateFolder(rec, mkReq(http.MethodPatch, "/x", `{"name":"r"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename => %d", rec.Code)
	}
}

func TestUpdateFolderMove(t *testing.T) {
	id := uuid.New()
	np := uuid.New()
	h := newHandlers(&fakeSvc{moved: domain.Folder{ID: id}})
	rec := httptest.NewRecorder()
	h.UpdateFolder(rec, mkReq(http.MethodPatch, "/x", `{"new_parent_id":"`+np.String()+`"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("move => %d", rec.Code)
	}
}

func TestUpdateFolderMoveBadParent(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.UpdateFolder(rec, mkReq(http.MethodPatch, "/x", `{"new_parent_id":"nope"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("new_parent_id inválido => %d, quiero 400", rec.Code)
	}
}

func TestUpdateFolderAmbiguous(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	// ni name ni new_parent_id => 400.
	h.UpdateFolder(rec, mkReq(http.MethodPatch, "/x", `{}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin operación => %d, quiero 400", rec.Code)
	}
	// ambos a la vez => 400.
	rec = httptest.NewRecorder()
	h.UpdateFolder(rec, mkReq(http.MethodPatch, "/x", `{"name":"a","new_parent_id":"`+uuid.NewString()+`"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ambas operaciones => %d, quiero 400", rec.Code)
	}
}

func TestUpdateFolderCycle(t *testing.T) {
	id := uuid.New()
	np := uuid.New()
	h := newHandlers(&fakeSvc{moveErr: domain.ErrCycle})
	rec := httptest.NewRecorder()
	h.UpdateFolder(rec, mkReq(http.MethodPatch, "/x", `{"new_parent_id":"`+np.String()+`"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ciclo => %d, quiero 400", rec.Code)
	}
}

func TestDeleteFolderOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.DeleteFolder(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
}

func TestDeleteFolderRootImmutable(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{delFldErr: domain.ErrRootImmutable})
	rec := httptest.NewRecorder()
	h.DeleteFolder(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("raíz inmutable => %d, quiero 400", rec.Code)
	}
}

// --- files ---------------------------------------------------------------

func TestUploadFileOK(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{uploaded: domain.File{ID: uuid.New(), Name: "f.txt"}}
	h := newHandlers(svc)
	r := mkReq(http.MethodPost, "/x", "contenido", map[string]string{"id": id.String()})
	r.Header.Set("X-File-Name", "f.txt")
	r.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.UploadFile(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201", rec.Code)
	}
	if svc.lastUploadName != "f.txt" {
		t.Fatalf("nombre pasado = %q", svc.lastUploadName)
	}
}

func TestUploadFileMissingName(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	r := mkReq(http.MethodPost, "/x", "c", map[string]string{"id": id.String()})
	rec := httptest.NewRecorder()
	h.UploadFile(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre => %d, quiero 400", rec.Code)
	}
}

func TestUploadFileNameFromQuery(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{uploaded: domain.File{ID: uuid.New()}}
	h := newHandlers(svc)
	r := mkReq(http.MethodPost, "/x?name=q.bin", "c", map[string]string{"id": id.String()})
	rec := httptest.NewRecorder()
	h.UploadFile(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d", rec.Code)
	}
	if svc.lastUploadName != "q.bin" {
		t.Fatalf("nombre de query = %q", svc.lastUploadName)
	}
}

func TestUploadFileQuotaError(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{uploadErr: domain.ErrQuotaExceeded})
	r := mkReq(http.MethodPost, "/x?name=f", "c", map[string]string{"id": id.String()})
	rec := httptest.NewRecorder()
	h.UploadFile(rec, r)
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("cuota => %d, quiero 507", rec.Code)
	}
}

// --- subida directa a S3 (presigned) -------------------------------------

func jsonReq(target, body string) *http.Request {
	r := withAuth(httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestPresignHandlerOK(t *testing.T) {
	svc := &fakeSvc{presignUp: service.PresignResult{
		URL: "https://s3.example/bucket/key?sig=x", Handle: "payload.sig", ExpiresAt: time.Now(),
	}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.Presign(rec, jsonReq("/files/presign", `{"path":"/a/big.bin","size":123,"content_type":"application/pdf"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastPresign != "/a/big.bin" {
		t.Fatalf("ruta pasada = %q", svc.lastPresign)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta no JSON: %v", err)
	}
	if got["url"] == "" || got["handle"] == "" || got["expires_at"] == "" {
		t.Fatalf("faltan url/handle/expires_at: %v", got)
	}
}

func TestPresignHandlerMissingPath(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.Presign(rec, jsonReq("/files/presign", `{"size":1}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin ruta => %d, quiero 400", rec.Code)
	}
}

func TestPresignHandlerQuota(t *testing.T) {
	h := newHandlers(&fakeSvc{presignUpErr: domain.ErrQuotaExceeded})
	rec := httptest.NewRecorder()
	h.Presign(rec, jsonReq("/files/presign", `{"path":"/x","size":9}`))
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("cuota => %d, quiero 507", rec.Code)
	}
}

func TestFinalizeHandlerOK(t *testing.T) {
	svc := &fakeSvc{finalizeFile: domain.File{ID: uuid.New(), Name: "big.bin"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.Finalize(rec, jsonReq("/files/finalize", `{"handle":"payload.sig"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastHandle != "payload.sig" {
		t.Fatalf("handle pasado = %q", svc.lastHandle)
	}
}

func TestFinalizeHandlerInvalidHandle(t *testing.T) {
	h := newHandlers(&fakeSvc{finalizeErr: service.ErrPresignHandleInvalid})
	rec := httptest.NewRecorder()
	h.Finalize(rec, jsonReq("/files/finalize", `{"handle":"bad"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handle inválido => %d, quiero 400", rec.Code)
	}
}

func TestFinalizeHandlerUploadNotFound(t *testing.T) {
	h := newHandlers(&fakeSvc{finalizeErr: service.ErrUploadNotFound})
	rec := httptest.NewRecorder()
	h.Finalize(rec, jsonReq("/files/finalize", `{"handle":"payload.sig"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("subida ausente => %d, quiero 409", rec.Code)
	}
	if errCode(t, rec) != "upload_not_found" {
		t.Fatalf("code = %q", errCode(t, rec))
	}
}

// TestFilesRouteRegistration confirma que chi acepta las rutas estáticas
// /files/presign y /files/finalize junto a /files/{id}* (estático gana) sin
// pánico de registro y despacha cada una a su handler.
func TestFilesRouteRegistration(t *testing.T) {
	h := newHandlers(&fakeSvc{
		presignUp:    service.PresignResult{URL: "u", Handle: "x"},
		finalizeFile: domain.File{ID: uuid.New()},
		file:         domain.File{ID: uuid.New(), Name: "f"},
	})
	r := chi.NewRouter()
	r.Post("/files/presign", h.Presign)
	r.Post("/files/finalize", h.Finalize)
	r.Get("/files/{id}", h.GetFile)
	r.Get("/files/{id}/content", h.DownloadFile)
	r.Post("/files/{id}/copy", h.CopyFile)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/files/presign", http.StatusOK},
		{http.MethodPost, "/files/finalize", http.StatusCreated},
		{http.MethodGet, "/files/" + uuid.New().String(), http.StatusOK},
	} {
		rec := httptest.NewRecorder()
		body := `{"path":"/x","size":1}`
		if tc.path == "/files/finalize" {
			body = `{"handle":"h.s"}`
		}
		req := withAuth(httptest.NewRequest(tc.method, tc.path, strings.NewReader(body)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s => %d, quiero %d (%s)", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestGetFileOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{file: domain.File{ID: id, Name: "f"}})
	rec := httptest.NewRecorder()
	h.GetFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
}

// TestCopyFileInPlace: sin cuerpo, copia en la misma carpeta (dest = uuid.Nil) y
// concede PermWrite (el miembro del tenant es editor en «Mi unidad»).
func TestCopyFileInPlace(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{copyFile: domain.File{ID: uuid.New(), Name: "Copia de f.txt"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.CopyFile(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastCopyID != id || svc.lastCopyDs != uuid.Nil {
		t.Fatalf("copia: id=%v dest=%v (quiero dest nil)", svc.lastCopyID, svc.lastCopyDs)
	}
	if svc.lastCopyPr != domain.PermWrite {
		t.Fatalf("permiso = %q, quiero write", svc.lastCopyPr)
	}
}

// TestCopyFileToFolder: con cuerpo {dest_folder_id}, copia a esa carpeta.
func TestCopyFileToFolder(t *testing.T) {
	id, dst := uuid.New(), uuid.New()
	svc := &fakeSvc{copyFile: domain.File{ID: uuid.New()}}
	h := newHandlers(svc)
	body := `{"dest_folder_id":"` + dst.String() + `"}`
	rec := httptest.NewRecorder()
	h.CopyFile(rec, mkReq(http.MethodPost, "/x", body, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastCopyDs != dst {
		t.Fatalf("dest = %v, quiero %v", svc.lastCopyDs, dst)
	}
}

// TestCopyFileQuota: la cuota superada se mapea a 507.
func TestCopyFileQuota(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{copyFileEr: domain.ErrQuotaExceeded})
	rec := httptest.NewRecorder()
	h.CopyFile(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("cuota => %d, quiero 507", rec.Code)
	}
}

// TestCopyFolderOK: copia de carpeta (recursiva) delega y devuelve 201 con PermWrite.
func TestCopyFolderOK(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{copyFold: domain.Folder{ID: uuid.New(), Name: "Copia de A"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.CopyFolder(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastCopyID != id || svc.lastCopyPr != domain.PermWrite {
		t.Fatalf("copia carpeta: id=%v perm=%v", svc.lastCopyID, svc.lastCopyPr)
	}
}

func TestDownloadFileStream(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{
		openFile: domain.File{ID: id, Name: "árbol.txt", ContentType: "text/plain", SizeBytes: 5, Checksum: "abc"},
		openBody: "hello",
	}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.DownloadFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("ETag") != `"abc"` {
		t.Fatalf("ETag = %q", rec.Header().Get("ETag"))
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "UTF-8''") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("falta nosniff")
	}
}

func TestDownloadFilePresign(t *testing.T) {
	id := uuid.New()
	u, _ := url.Parse("https://s3/presigned")
	h := newHandlers(&fakeSvc{presignURL: u})
	rec := httptest.NewRecorder()
	h.DownloadFile(rec, mkReq(http.MethodGet, "/x?presign=1", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("presign => %d, quiero 307", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://s3/presigned" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestDownloadFilePresignError(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{presignErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.DownloadFile(rec, mkReq(http.MethodGet, "/x?presign=1", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestDownloadFileOpenError(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{openErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.DownloadFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

func TestThumbnailOK(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{thumbBody: "JPEGBYTES", thumbType: "image/jpeg", thumbETag: `"abc-thumb"`}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.Thumbnail(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 (%s)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "JPEGBYTES" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("ETag") != `"abc-thumb"` {
		t.Fatalf("ETag = %q", rec.Header().Get("ETag"))
	}
	// Disposición SEGURA para <img>: nosniff + inline + CSP sandbox (no ejecuta script).
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("falta nosniff")
	}
	if rec.Header().Get("Content-Disposition") != "inline" {
		t.Fatalf("Content-Disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatal("falta CSP sandbox")
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Fatal("falta Cache-Control (cacheable)")
	}
}

func TestThumbnailNotModified(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{thumbBody: "X", thumbType: "image/png", thumbETag: `"v-thumb"`}
	h := newHandlers(svc)
	r := mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()})
	r.Header.Set("If-None-Match", `"v-thumb"`)
	rec := httptest.NewRecorder()
	h.Thumbnail(rec, r)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match coincidente => %d, quiero 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("304 no debe llevar cuerpo, got %q", rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"v-thumb"` {
		t.Fatalf("304 debe incluir ETag, got %q", rec.Header().Get("ETag"))
	}
}

func TestThumbnailUnsupported(t *testing.T) {
	id := uuid.New()
	// Tipo no soportado (o bomba de descompresión rechazada) => 415.
	h := newHandlers(&fakeSvc{thumbErr: domain.ErrUnsupportedMedia})
	rec := httptest.NewRecorder()
	h.Thumbnail(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("no-imagen => %d, quiero 415", rec.Code)
	}
}

func TestThumbnailNotFound(t *testing.T) {
	id := uuid.New()
	// Sin acceso / fichero ajeno (RLS) => 404, sin filtrar existencia.
	h := newHandlers(&fakeSvc{thumbErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.Thumbnail(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("fichero inaccesible => %d, quiero 404", rec.Code)
	}
}

func TestUpdateFileRenameAndMove(t *testing.T) {
	id := uuid.New()
	nf := uuid.New()
	svc := &fakeSvc{renFile: domain.File{ID: id}, movFile: domain.File{ID: id}}
	h := newHandlers(svc)

	rec := httptest.NewRecorder()
	h.UpdateFile(rec, mkReq(http.MethodPatch, "/x", `{"name":"nuevo"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename => %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.UpdateFile(rec, mkReq(http.MethodPatch, "/x", `{"new_folder_id":"`+nf.String()+`"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("move => %d", rec.Code)
	}
}

func TestUpdateFileBadFolderID(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.UpdateFile(rec, mkReq(http.MethodPatch, "/x", `{"new_folder_id":"nope"}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("new_folder_id inválido => %d, quiero 400", rec.Code)
	}
}

func TestUpdateFileAmbiguous(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.UpdateFile(rec, mkReq(http.MethodPatch, "/x", `{}`, map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin operación => %d, quiero 400", rec.Code)
	}
}

func TestDeleteFileOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.DeleteFile(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
}

// --- versiones -----------------------------------------------------------

func TestListFileVersions(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{versions: service.VersionPage{
		Versions: []domain.FileVersion{
			{FileID: id, Version: 3, IsCurrent: true, SizeBytes: 30},
			{FileID: id, Version: 2, SizeBytes: 20, AuthorSub: "bob"},
		},
		NextCursor: "1",
	}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.ListFileVersions(rec, mkReq(http.MethodGet, "/x?limit=10", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Versions []map[string]any `json:"versions"`
		Next     string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Versions) != 2 || got.Next != "1" {
		t.Fatalf("versiones: %+v next=%q", got.Versions, got.Next)
	}
	if got.Versions[0]["is_current"] != true {
		t.Fatalf("la cabeza debe ser la versión actual: %+v", got.Versions[0])
	}
	// storage_key NUNCA se serializa (json:"-").
	if _, leaked := got.Versions[1]["storage_key"]; leaked {
		t.Fatal("storage_key no debe viajar al cliente")
	}
}

func TestListFileVersionsEmptyNotNull(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{}) // versions nil
	rec := httptest.NewRecorder()
	h.ListFileVersions(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	var got map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if string(got["versions"]) != "[]" {
		t.Fatalf("versions nil debe serializar como []: %s", got["versions"])
	}
	if _, ok := got["next_cursor"]; ok {
		t.Fatal("sin más páginas no debe haber next_cursor")
	}
}

func TestListFileVersionsNotFound(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{versionsErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.ListFileVersions(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("fichero ajeno/inexistente => %d, quiero 404", rec.Code)
	}
}

func TestDownloadFileVersionStream(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{
		file:        domain.File{ID: id, Name: "árbol.txt"},
		openVer:     domain.FileVersion{Version: 2, ContentType: "text/plain", SizeBytes: 3, Checksum: "old"},
		openVerBody: "old",
	}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.DownloadFileVersion(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String(), "version": "2"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if rec.Body.String() != "old" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
	// Disposición SEGURA: attachment + nosniff (nunca inline), como la descarga normal.
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "UTF-8''") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("falta nosniff")
	}
	if rec.Header().Get("ETag") != `"old"` {
		t.Fatalf("ETag = %q", rec.Header().Get("ETag"))
	}
}

func TestDownloadFileVersionBadVersion(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.DownloadFileVersion(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String(), "version": "0"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("versión 0 => %d, quiero 400", rec.Code)
	}
}

func TestDownloadFileVersionNotFound(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{openVerErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.DownloadFileVersion(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String(), "version": "9"}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("versión ausente => %d, quiero 404", rec.Code)
	}
}

func TestRestoreFileVersionOK(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{restoreVer: domain.File{ID: id, Version: 5}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.RestoreFileVersion(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String(), "version": "2"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastRestoreVer != 2 {
		t.Fatalf("versión pedida = %d, quiero 2", svc.lastRestoreVer)
	}
	// «Mi unidad» concede edición al miembro del tenant (puede reemplazar → restaurar).
	if svc.lastRestorePerm != domain.PermWrite {
		t.Fatalf("permiso = %q, quiero write", svc.lastRestorePerm)
	}
}

func TestRestoreFileVersionForbidden(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{restoreVerErr: domain.ErrPermission})
	rec := httptest.NewRecorder()
	h.RestoreFileVersion(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String(), "version": "2"}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer => %d, quiero 403", rec.Code)
	}
}

// --- trash ---------------------------------------------------------------

func TestListTrashNilSlicesBecomeEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{}) // trashFold/trashFile nil
	rec := httptest.NewRecorder()
	h.ListTrash(rec, mkReq(http.MethodGet, "/api/v1/trash", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Folders == nil || got.Files == nil {
		t.Fatalf("slices nil no normalizados: %+v", got)
	}
}

func TestRestoreFileOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{restFile: domain.File{ID: id, Name: "f"}})
	rec := httptest.NewRecorder()
	h.RestoreFile(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
}

func TestRestoreFolderConflict(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{restFldErr: domain.ErrConflict})
	rec := httptest.NewRecorder()
	h.RestoreFolder(rec, mkReq(http.MethodPost, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("restaurar con nombre en uso => %d, quiero 409", rec.Code)
	}
}

func TestPurgeFileOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.PurgeFile(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
}

func TestPurgeFolderNotFound(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{purgeFldEr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.PurgeFolder(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("código = %d, quiero 404", rec.Code)
	}
}

// TestSearchOK: un resultado sin RUTA no se puede abrir ni situar —una búsqueda cruza
// todo el árbol—, así que la ruta es el dato que hace útil la respuesta. La envoltura
// sigue siendo "files": el conector del Hub la decodifica por nombre y renombrarla
// dejaría la búsqueda federada sin resultados de Drive.
func TestSearchOK(t *testing.T) {
	h := newHandlers(&fakeSvc{search: []service.Entry{
		{ID: uuid.New(), Name: "factura.pdf", Path: "/Contabilidad/factura.pdf"},
	}})
	rec := httptest.NewRecorder()
	h.Search(rec, mkReq(http.MethodGet, "/x?q=factura&limit=5", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Files []struct {
			Path string `json:"path"`
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "/Contabilidad/factura.pdf" {
		t.Fatalf("files = %+v, quiero uno con su ruta resuelta", got.Files)
	}
}

// TestGetFileIncludesPath: el explorador navega por RUTA, no por id, así que sin este
// campo el enlace canónico /files/{id} —el que componen la búsqueda federada del Hub y
// «Compartir a Rooms»— no puede llevar a ninguna parte.
func TestGetFileIncludesPath(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{
		file:     domain.File{ID: id, Name: "informe.pdf"},
		filePath: "/Ventas/informe.pdf",
	})
	rec := httptest.NewRecorder()
	h.GetFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if got["path"] != "/Ventas/informe.pdf" {
		t.Fatalf("path = %v, quiero /Ventas/informe.pdf", got["path"])
	}
	if got["name"] != "informe.pdf" {
		t.Fatalf("el metadato original debe seguir ahí: %v", got)
	}
}

// TestGetFilePathFailureKeepsMetadata: no poder situar el fichero no invalida su
// metadato. Tumbar la respuesta entera por eso sería peor que devolverla sin ruta.
func TestGetFilePathFailureKeepsMetadata(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{
		file:        domain.File{ID: id, Name: "informe.pdf"},
		filePathErr: domain.ErrNotFound,
	})
	rec := httptest.NewRecorder()
	h.GetFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200 con el metadato", rec.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["name"] != "informe.pdf" {
		t.Fatalf("metadato perdido: %v", got)
	}
	if _, hay := got["path"]; hay {
		t.Fatalf("sin ruta resuelta no debe inventarse una: %v", got)
	}
}

func TestSearchMissingQuery(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.Search(rec, mkReq(http.MethodGet, "/x", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("q ausente => %d, quiero 400", rec.Code)
	}
}

// --- operaciones en lote y zip -------------------------------------------

func TestBatchMoveOK(t *testing.T) {
	f1, d1, target := uuid.New(), uuid.New(), uuid.New()
	svc := &fakeSvc{batchMove: service.BatchResult{Files: 1, Folders: 1}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	body := `{"file_ids":["` + f1.String() + `"],"folder_ids":["` + d1.String() + `"],"target_folder_id":"` + target.String() + `"}`
	h.BatchMove(rec, mkReq(http.MethodPost, "/api/v1/batch/move", body, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["moved_files"] != 1 || got["moved_folders"] != 1 {
		t.Fatalf("contadores = %+v", got)
	}
	// Ids y destino llegan al servicio (una sola llamada, sin N+1).
	if len(svc.lastBatchFiles) != 1 || len(svc.lastBatchFolders) != 1 || svc.lastBatchTarget != target {
		t.Fatalf("no propagado: files=%v folders=%v target=%v", svc.lastBatchFiles, svc.lastBatchFolders, svc.lastBatchTarget)
	}
}

func TestBatchMoveEmptyTargetIsRoot(t *testing.T) {
	// target_folder_id vacío => se resuelve a la raíz del tenant (svc.Root).
	rootID := uuid.New()
	fid := uuid.New()
	svc := &fakeSvc{root: domain.Folder{ID: rootID, IsRoot: true}, batchMove: service.BatchResult{Files: 1}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.BatchMove(rec, mkReq(http.MethodPost, "/x", `{"file_ids":["`+fid.String()+`"]}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastBatchTarget != rootID {
		t.Fatalf("destino = %v, quiero la raíz %v", svc.lastBatchTarget, rootID)
	}
}

func TestBatchMoveNoTargets(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.BatchMove(rec, mkReq(http.MethodPost, "/x", `{"file_ids":[],"folder_ids":[]}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("selección vacía => %d, quiero 400", rec.Code)
	}
}

func TestBatchMoveBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.BatchMove(rec, mkReq(http.MethodPost, "/x", `{"file_ids":["nope"],"target_folder_id":"`+uuid.NewString()+`"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido => %d, quiero 400", rec.Code)
	}
}

func TestBatchMoveCycleAndConflict(t *testing.T) {
	fid := uuid.New()
	target := uuid.New()
	body := `{"file_ids":["` + fid.String() + `"],"target_folder_id":"` + target.String() + `"}`
	// Ciclo => 400.
	rec := httptest.NewRecorder()
	newHandlers(&fakeSvc{batchMoveErr: domain.ErrCycle}).BatchMove(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ciclo => %d, quiero 400", rec.Code)
	}
	// Choque de nombre en el destino => 409 (atómico).
	rec = httptest.NewRecorder()
	newHandlers(&fakeSvc{batchMoveErr: domain.ErrConflict}).BatchMove(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflicto => %d, quiero 409", rec.Code)
	}
}

func TestBatchTrashOK(t *testing.T) {
	fid, did := uuid.New(), uuid.New()
	svc := &fakeSvc{batchTrash: service.BatchResult{Files: 2, Folders: 1}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	body := `{"file_ids":["` + fid.String() + `"],"folder_ids":["` + did.String() + `"]}`
	h.BatchTrash(rec, mkReq(http.MethodPost, "/api/v1/batch/trash", body, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["trashed_files"] != 2 || got["trashed_folders"] != 1 {
		t.Fatalf("contadores = %+v", got)
	}
}

func TestBatchZipStreams(t *testing.T) {
	f1, f2, d1 := uuid.New(), uuid.New(), uuid.New()
	svc := &fakeSvc{
		zipEntries: []service.ZipEntry{{StorageKey: "k", Path: "a.txt", Size: 3}},
		zipBody:    "PK\x03\x04zip-bytes",
	}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	target := "/api/v1/batch/zip?file_ids=" + f1.String() + "," + f2.String() + "&folder_ids=" + d1.String() + "&name=Docs"
	h.BatchZip(rec, mkReq(http.MethodGet, target, "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "Docs.zip") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("falta nosniff en el zip")
	}
	// Lo que suman sus ficheros sin comprimir: con eso la app nativa enseña el porcentaje.
	if got := rec.Header().Get(ZipBytesHeader); got != "3" {
		t.Fatalf("%s = %q, y los ficheros suman 3", ZipBytesHeader, got)
	}
	if rec.Body.String() != "PK\x03\x04zip-bytes" {
		t.Fatalf("cuerpo del zip = %q", rec.Body.String())
	}
	// Ambos ficheros y la carpeta llegan al servicio.
	if len(svc.lastBatchFiles) != 2 || len(svc.lastBatchFolders) != 1 {
		t.Fatalf("selección no propagada: files=%v folders=%v", svc.lastBatchFiles, svc.lastBatchFolders)
	}
}

func TestBatchZipDefaultName(t *testing.T) {
	fid := uuid.New()
	h := newHandlers(&fakeSvc{zipEntries: []service.ZipEntry{{StorageKey: "k", Path: "a"}}})
	rec := httptest.NewRecorder()
	h.BatchZip(rec, mkReq(http.MethodGet, "/x?file_ids="+fid.String(), "", nil))
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "sytue-drive.zip") {
		t.Fatalf("sin ?name debe usar sytue-drive.zip: %q", cd)
	}
}

func TestBatchZipNoSelection(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.BatchZip(rec, mkReq(http.MethodGet, "/api/v1/batch/zip", "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin selección => %d, quiero 400", rec.Code)
	}
}

func TestBatchZipTooLarge(t *testing.T) {
	fid := uuid.New()
	// El error de tamaño sale como HTTP ANTES de escribir el zip (400).
	h := newHandlers(&fakeSvc{zipEntriesErr: domain.ErrBatchTooLarge})
	rec := httptest.NewRecorder()
	h.BatchZip(rec, mkReq(http.MethodGet, "/x?folder_ids="+fid.String(), "", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("selección demasiado grande => %d, quiero 400", rec.Code)
	}
}

// --- recientes y favoritos -----------------------------------------------

func TestRecentWithCursor(t *testing.T) {
	svc := &fakeSvc{recent: service.RecentPage{
		Entries:    []service.Entry{{ID: uuid.New(), Name: "a.txt", Path: "/a.txt"}},
		NextCursor: "next-tok",
	}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.Recent(rec, mkReq(http.MethodGet, "/api/v1/recent?limit=1", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.NextCursor != "next-tok" {
		t.Fatalf("recientes: %+v", got)
	}
	if got.Items[0]["path"] != "/a.txt" {
		t.Fatalf("la ruta debe viajar en la respuesta: %+v", got.Items[0])
	}
}

func TestRecentEmptyItemsNotNull(t *testing.T) {
	h := newHandlers(&fakeSvc{}) // recent.Entries nil
	rec := httptest.NewRecorder()
	h.Recent(rec, mkReq(http.MethodGet, "/api/v1/recent", "", nil))
	var got map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if string(got["items"]) != "[]" {
		t.Fatalf("items nil debe serializar como []: %s", got["items"])
	}
	if _, ok := got["next_cursor"]; ok {
		t.Fatal("sin más páginas no debe haber next_cursor")
	}
}

func TestListFavorites(t *testing.T) {
	svc := &fakeSvc{favs: []service.Entry{{ID: uuid.New(), Name: "Docs", IsDir: true, Path: "/Docs"}}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.ListFavorites(rec, mkReq(http.MethodGet, "/api/v1/favorites", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0]["is_directory"] != true {
		t.Fatalf("favoritos: %+v", got.Items)
	}
}

func TestListFavoritesEmptyNotNull(t *testing.T) {
	h := newHandlers(&fakeSvc{}) // favs nil
	rec := httptest.NewRecorder()
	h.ListFavorites(rec, mkReq(http.MethodGet, "/api/v1/favorites", "", nil))
	var got map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if string(got["items"]) != "[]" {
		t.Fatalf("items nil debe serializar como []: %s", got["items"])
	}
}

func TestSetFavoriteOK(t *testing.T) {
	fid := uuid.New()
	svc := &fakeSvc{}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.SetFavorite(rec, mkReq(http.MethodPost, "/api/v1/favorites", `{"file_id":"`+fid.String()+`","favorite":true}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if !svc.lastFavOn {
		t.Fatal("favorite=true no propagado al servicio")
	}
}

func TestSetFavoriteBadTargets(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	// Ni file ni folder.
	rec := httptest.NewRecorder()
	h.SetFavorite(rec, mkReq(http.MethodPost, "/x", `{"favorite":true}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin objetivo => %d, quiero 400", rec.Code)
	}
	// Ambos a la vez.
	rec = httptest.NewRecorder()
	body := `{"file_id":"` + uuid.NewString() + `","folder_id":"` + uuid.NewString() + `","favorite":true}`
	h.SetFavorite(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ambos objetivos => %d, quiero 400", rec.Code)
	}
	// file_id inválido.
	rec = httptest.NewRecorder()
	h.SetFavorite(rec, mkReq(http.MethodPost, "/x", `{"file_id":"nope","favorite":false}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("file_id inválido => %d, quiero 400", rec.Code)
	}
}

// --- quota ---------------------------------------------------------------

func TestGetQuota(t *testing.T) {
	// used_bytes YA incluye lo externo (lo compone el store): el desglose va aparte.
	h := newHandlers(&fakeSvc{quota: domain.Quota{Tenant: "acme", LimitBytes: 100, UsedBytes: 40, ExternalUsage: map[string]int64{"cast": 15}}})
	rec := httptest.NewRecorder()
	h.GetQuota(rec, mkReq(http.MethodGet, "/api/v1/quota", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Used      int64            `json:"used_bytes"`
		Available int64            `json:"available_bytes"`
		External  map[string]int64 `json:"external_usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Used != 40 || got.Available != 60 {
		t.Fatalf("used = %d, available = %d; quiero 40 y 60", got.Used, got.Available)
	}
	if got.External["cast"] != 15 || len(got.External) != 1 {
		t.Fatalf("external_usage = %v, quiero {cast: 15}", got.External)
	}
}

// Sin nada declarado, external_usage va igualmente, vacío: el cliente no distingue
// «nadie ha declarado» de «este Drive no lo manda».
func TestGetQuotaSinUsoExterno(t *testing.T) {
	h := newHandlers(&fakeSvc{quota: domain.Quota{Tenant: "acme", LimitBytes: 100, UsedBytes: 40}})
	rec := httptest.NewRecorder()
	h.GetQuota(rec, mkReq(http.MethodGet, "/api/v1/quota", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got["external_usage"]) != "{}" {
		t.Fatalf("external_usage = %s, quiero {}", got["external_usage"])
	}
}

func TestSetQuotaOK(t *testing.T) {
	h := newHandlers(&fakeSvc{setQuota: domain.Quota{LimitBytes: 200}})
	rec := httptest.NewRecorder()
	h.SetQuota(rec, mkReq(http.MethodPut, "/x", `{"limit_bytes":200}`, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
}

func TestSetQuotaNegative(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.SetQuota(rec, mkReq(http.MethodPut, "/x", `{"limit_bytes":-1}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negativo => %d, quiero 400", rec.Code)
	}
}

func TestSetQuotaBadJSON(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.SetQuota(rec, mkReq(http.MethodPut, "/x", `{bad`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("código = %d, quiero 400", rec.Code)
	}
}

// --- shares --------------------------------------------------------------

func TestCreateShareLinkReturnsURL(t *testing.T) {
	fid := uuid.New()
	svc := &fakeSvc{share: domain.Share{ID: uuid.New(), Kind: domain.ShareLink, Token: "secret-token"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	body := `{"file_id":"` + fid.String() + `","kind":"link"}`
	h.CreateShare(rec, mkReq(http.MethodPost, "/api/v1/shares", body, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if u, _ := got["url"].(string); !strings.HasSuffix(u, "/public/secret-token") {
		t.Fatalf("url = %v", got["url"])
	}
	if svc.lastShareInput.FileID == nil {
		t.Fatal("FileID no propagado al servicio")
	}
}

func TestCreateShareBadKind(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", `{"kind":"invalido"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kind inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateShareBadPermission(t *testing.T) {
	fid := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	body := `{"file_id":"` + fid.String() + `","kind":"user","permission":"exec"}`
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("permission inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateShareNeitherTarget(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", `{"kind":"user"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin objetivo => %d, quiero 400", rec.Code)
	}
}

func TestCreateShareBothTargets(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	body := `{"kind":"user","file_id":"` + uuid.NewString() + `","folder_id":"` + uuid.NewString() + `"}`
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ambos objetivos => %d, quiero 400", rec.Code)
	}
}

func TestCreateShareBadFileID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", `{"kind":"user","file_id":"nope"}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("file_id inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateShareBadExpires(t *testing.T) {
	fid := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	body := `{"kind":"user","file_id":"` + fid.String() + `","expires_at":"ayer"}`
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expires_at inválido => %d, quiero 400", rec.Code)
	}
}

func TestCreateShareWithExpires(t *testing.T) {
	fid := uuid.New()
	svc := &fakeSvc{share: domain.Share{ID: uuid.New(), Kind: domain.ShareUser}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	exp := time.Now().Add(time.Hour).Format(time.RFC3339)
	body := `{"kind":"user","subject":"bob","file_id":"` + fid.String() + `","expires_at":"` + exp + `"}`
	h.CreateShare(rec, mkReq(http.MethodPost, "/x", body, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastShareInput.ExpiresAt == nil {
		t.Fatal("ExpiresAt no propagado")
	}
}

func TestListFileShares(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{}) // nil => se normaliza a []
	rec := httptest.NewRecorder()
	h.ListFileShares(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if string(got["shares"]) != "[]" {
		t.Fatalf("shares nil debe serializar como []: %s", got["shares"])
	}
}

func TestListFolderShares(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{foldShares: []domain.Share{{ID: uuid.New()}}})
	rec := httptest.NewRecorder()
	h.ListFolderShares(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
}

func TestRevokeShareOK(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.RevokeShare(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
}

func TestRevokeShareBadID(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.RevokeShare(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido => %d, quiero 400", rec.Code)
	}
}

// --- public link (fichero, carpeta, contraseña) --------------------------

func publicReq(method, target, token, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	return withParams(r, map[string]string{"token": token})
}

func TestPublicEntryFileDownload(t *testing.T) {
	fid := uuid.New()
	svc := &fakeSvc{
		pubShare:  domain.Share{FileID: &fid},
		pubTenant: "acme",
		pubNode: service.PublicNode{
			File: domain.File{Name: "doc.pdf", ContentType: "application/pdf", SizeBytes: 4},
			Body: io.NopCloser(strings.NewReader("data")),
		},
	}
	rec := httptest.NewRecorder()
	newHandlers(svc).PublicEntry(rec, publicReq(http.MethodGet, "/public/tok", "tok", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if rec.Body.String() != "data" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q (debe ser attachment)", cd)
	}
}

func TestPublicEntryFolderListing(t *testing.T) {
	folderID := uuid.New()
	svc := &fakeSvc{
		pubShare:  domain.Share{FolderID: &folderID},
		pubTenant: "acme",
		pubNode: service.PublicNode{
			IsDir:      true,
			Folder:     domain.Folder{Name: "Proyecto"},
			Subfolders: []domain.Folder{{Name: "informes"}},
			Files:      []domain.File{{Name: "plan.pdf", SizeBytes: 2048}},
		},
	}
	rec := httptest.NewRecorder()
	newHandlers(svc).PublicEntry(rec, publicReq(http.MethodGet, "/public/tok", "tok", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"Proyecto", "informes", "plan.pdf", "/public/tok/informes", "/public/tok/plan.pdf"} {
		if !strings.Contains(body, want) {
			t.Fatalf("el listado no contiene %q\n%s", want, body)
		}
	}
}

func TestPublicEntryMissingToken(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeSvc{}).PublicEntry(rec, publicReq(http.MethodGet, "/public/", "", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("token vacío => %d, quiero 404", rec.Code)
	}
}

func TestPublicEntryExpired(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeSvc{pubResolveErr: domain.ErrShareExpired}).
		PublicEntry(rec, publicReq(http.MethodGet, "/public/tok", "tok", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("enlace expirado => %d, quiero 404", rec.Code)
	}
}

func TestPublicEntryPasswordPrompt(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeSvc{pubResolveErr: domain.ErrPasswordRequired}).
		PublicEntry(rec, publicReq(http.MethodGet, "/public/tok", "tok", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("prompt => %d, quiero 401", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `type="password"`) {
		t.Fatalf("no muestra el formulario de contraseña:\n%s", body)
	}
}

func TestPublicUnlockOK(t *testing.T) {
	svc := &fakeSvc{pubProof: "proof-value"}
	rec := httptest.NewRecorder()
	newHandlers(svc).PublicUnlock(rec, publicReq(http.MethodPost, "/public/tok", "tok", "password=hunter2"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unlock => %d, quiero 303", rec.Code)
	}
	if svc.lastVerifyPass != "hunter2" {
		t.Fatalf("contraseña no propagada: %q", svc.lastVerifyPass)
	}
	if sc := rec.Header().Get("Set-Cookie"); !strings.Contains(sc, publicCookiePrefix) || !strings.Contains(sc, "proof-value") {
		t.Fatalf("no fija cookie de prueba: %q", sc)
	}
	if loc := rec.Header().Get("Location"); loc != "/public/tok" {
		t.Fatalf("Location = %q", loc)
	}
}

func TestPublicUnlockWrong(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeSvc{pubVerifyErr: domain.ErrWrongPassword}).
		PublicUnlock(rec, publicReq(http.MethodPost, "/public/tok", "tok", "password=nope"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("contraseña incorrecta => %d, quiero 401", rec.Code)
	}
	if sc := rec.Header().Get("Set-Cookie"); sc != "" {
		t.Fatalf("no debe fijar cookie con contraseña incorrecta: %q", sc)
	}
	if !strings.Contains(rec.Body.String(), "incorrecta") {
		t.Fatal("debe indicar que la contraseña es incorrecta")
	}
}

// --- compartido conmigo --------------------------------------------------

func TestSharedWithMe(t *testing.T) {
	svc := &fakeSvc{sharedEntries: []service.SharedEntry{{
		Entry:      service.Entry{ID: uuid.New(), Name: "Docs", IsDir: true, Path: "/equipo/Docs"},
		Permission: domain.PermWrite, SharedBy: "alice",
	}}}
	rec := httptest.NewRecorder()
	newHandlers(svc).SharedWithMe(rec, mkReq(http.MethodGet, "/api/v1/shared-with-me", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %+v", got.Items)
	}
	if got.Items[0]["permission"] != "edit" || got.Items[0]["shared_by"] != "alice" {
		t.Fatalf("permiso/dueño no viajan: %+v", got.Items[0])
	}
	if got.Items[0]["path"] != "/equipo/Docs" {
		t.Fatalf("ruta absoluta esperada: %+v", got.Items[0])
	}
}

// TestFlatViewsCarryLabel fija la incoherencia que arregla esta ola: un fichero
// etiquetado sale con su etiqueta por las TRES vistas planas (recientes, favoritos
// y compartido conmigo), con la misma forma que en el listado principal, para que
// el diálogo de compartir avise ANTES en vez de tras el 403.
func TestFlatViewsCarryLabel(t *testing.T) {
	entry := service.Entry{
		ID: uuid.New(), Name: "nomina.pdf", Path: "/nomina.pdf",
		Label: &domain.Label{
			ID: "22222222-2222-2222-2222-222222222222", Name: "Confidencial",
			Rank: 30, BlockExternalShare: true,
		},
	}
	h := newHandlers(&fakeSvc{
		recent:        service.RecentPage{Entries: []service.Entry{entry}},
		favs:          []service.Entry{entry},
		sharedEntries: []service.SharedEntry{{Entry: entry, Permission: domain.PermRead, SharedBy: "alice"}},
	})
	vistas := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		target  string
	}{
		{"recientes", h.Recent, "/api/v1/recent"},
		{"favoritos", h.ListFavorites, "/api/v1/favorites"},
		{"compartido conmigo", h.SharedWithMe, "/api/v1/shared-with-me"},
	}
	for _, v := range vistas {
		t.Run(v.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			v.handler(rec, mkReq(http.MethodGet, v.target, "", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
			}
			var got struct {
				Items []struct {
					Label *domain.Label `json:"label"`
				} `json:"items"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got.Items) != 1 || got.Items[0].Label == nil {
				t.Fatalf("la etiqueta no viaja en %s: %s", v.target, rec.Body.String())
			}
			if got.Items[0].Label.Name != "Confidencial" || !got.Items[0].Label.BlockExternalShare {
				t.Fatalf("snapshot incompleto: %+v", got.Items[0].Label)
			}
		})
	}
}

// Un fichero SIN etiqueta no añade el campo: la respuesta no cambia de forma para
// quien no la use (el campo es aditivo y opcional).
func TestFlatViewsOmitAbsentLabel(t *testing.T) {
	svc := &fakeSvc{favs: []service.Entry{{ID: uuid.New(), Name: "a.txt", Path: "/a.txt"}}}
	rec := httptest.NewRecorder()
	newHandlers(svc).ListFavorites(rec, mkReq(http.MethodGet, "/api/v1/favorites", "", nil))
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %+v", got.Items)
	}
	if _, ok := got.Items[0]["label"]; ok {
		t.Fatalf("sin etiqueta el campo no debe aparecer: %+v", got.Items[0])
	}
}

func TestSharedWithMeEmptyNotNull(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeSvc{}).SharedWithMe(rec, mkReq(http.MethodGet, "/api/v1/shared-with-me", "", nil))
	var got map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if string(got["items"]) != "[]" {
		t.Fatalf("items nil debe serializar como []: %s", got["items"])
	}
}

func TestSharedListFolder(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{
		sharedNode: service.PublicNode{
			IsDir:      true,
			Subfolders: []domain.Folder{{ID: uuid.New(), Name: "informes"}},
			Files:      []domain.File{{ID: uuid.New(), Name: "plan.pdf", SizeBytes: 3}},
		},
		sharedPerm: domain.PermRead,
	}
	rec := httptest.NewRecorder()
	newHandlers(svc).SharedList(rec, mkReq(http.MethodGet, "/x?subpath=sub", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Permission string           `json:"permission"`
		Items      []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Permission != "view" || len(got.Items) != 2 {
		t.Fatalf("permiso/listado: %+v", got)
	}
	// Rutas RELATIVAS a la raíz del share (subpath + nombre).
	if got.Items[0]["path"] != "sub/informes" {
		t.Fatalf("ruta relativa esperada sub/informes: %+v", got.Items[0])
	}
}

func TestSharedListDenied(t *testing.T) {
	id := uuid.New()
	// Sin share vigente: deny-by-default (404, sin fugas).
	h := newHandlers(&fakeSvc{sharedErr2: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.SharedList(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sin share => %d, quiero 404", rec.Code)
	}
}

func TestSharedContentFile(t *testing.T) {
	id := uuid.New()
	svc := &fakeSvc{sharedNode: service.PublicNode{
		File: domain.File{Name: "doc.pdf", ContentType: "application/pdf", SizeBytes: 4},
		Body: io.NopCloser(strings.NewReader("data")),
	}}
	rec := httptest.NewRecorder()
	newHandlers(svc).SharedContent(rec, mkReq(http.MethodGet, "/x?subpath=doc.pdf", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if rec.Body.String() != "data" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("falta nosniff")
	}
}

func TestSharedContentIsDir(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{sharedNode: service.PublicNode{IsDir: true}})
	rec := httptest.NewRecorder()
	h.SharedContent(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("carpeta como contenido => %d, quiero 400", rec.Code)
	}
}

func TestSharedUploadEditorAndViewer(t *testing.T) {
	id := uuid.New()
	// Editor: 201.
	svc := &fakeSvc{sharedUpload: domain.File{ID: uuid.New(), Name: "f.txt"}}
	r := mkReq(http.MethodPost, "/x?subpath=", "contenido", map[string]string{"id": id.String()})
	r.Header.Set("X-File-Name", "f.txt")
	r.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	newHandlers(svc).SharedUpload(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("editor => %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUploadName != "f.txt" {
		t.Fatalf("nombre = %q", svc.lastUploadName)
	}

	// Viewer: 403 (ErrPermission).
	r = mkReq(http.MethodPost, "/x", "c", map[string]string{"id": id.String()})
	r.Header.Set("X-File-Name", "f.txt")
	rec = httptest.NewRecorder()
	newHandlers(&fakeSvc{sharedUpErr: domain.ErrPermission}).SharedUpload(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer => %d, quiero 403", rec.Code)
	}
}

func TestSharedUploadMissingName(t *testing.T) {
	id := uuid.New()
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.SharedUpload(rec, mkReq(http.MethodPost, "/x", "c", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre => %d, quiero 400", rec.Code)
	}
}

func TestJWTGroups(t *testing.T) {
	// Payload JWT (segmento central) con el claim "groups": el token ya lo verificó
	// RequireAuth; jwtGroups solo lo re-lee para un claim que authn no expone.
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"bob","groups":["eng","ops"]}`))
	r := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	r.Header.Set("Authorization", "Bearer header."+payload+".sig")
	got := jwtGroups(r)
	if len(got) != 2 || got[0] != "eng" || got[1] != "ops" {
		t.Fatalf("jwtGroups = %v, quiero [eng ops]", got)
	}
	// Sin token: nil (los shares de grupo no conceden acceso; deny-by-default).
	if g := jwtGroups(httptest.NewRequest(http.MethodGet, "/x", http.NoBody)); g != nil {
		t.Fatalf("sin token => %v, quiero nil", g)
	}
	// Claim ausente (Sytue ID aún no lo emite): nil.
	p2 := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"bob"}`))
	r2 := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	r2.Header.Set("Authorization", "Bearer h."+p2+".s")
	if g := jwtGroups(r2); g != nil {
		t.Fatalf("sin claim groups => %v, quiero nil", g)
	}
}

// --- helpers puros -------------------------------------------------------

func TestToAPIError(t *testing.T) {
	cases := []struct {
		in   error
		want int
	}{
		{nil, 0},
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrConflict, http.StatusConflict},
		{domain.ErrInvalidName, http.StatusBadRequest},
		{domain.ErrInvalidParent, http.StatusBadRequest},
		{domain.ErrQuotaExceeded, http.StatusInsufficientStorage},
		{domain.ErrCycle, http.StatusBadRequest},
		{domain.ErrRootImmutable, http.StatusBadRequest},
		{domain.ErrShareExpired, http.StatusNotFound},
		{domain.ErrPermission, http.StatusForbidden},
		{service.ErrExternalStorageOff, http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		got := toAPIError(c.in)
		if c.in == nil {
			if got != nil {
				t.Errorf("toAPIError(nil) = %v", got)
			}
			continue
		}
		var he *httpx.Error
		if !errors.As(got, &he) {
			t.Fatalf("toAPIError(%v) no es *httpx.Error: %v", c.in, got)
		}
		if he.Status != c.want {
			t.Errorf("toAPIError(%v) => %d, quiero %d", c.in, he.Status, c.want)
		}
	}
}

func TestSubFromCtx(t *testing.T) {
	r := mkReq(http.MethodGet, "/x", "", nil)
	if got := subFromCtx(r); got != "user-1" {
		t.Fatalf("subFromCtx = %q", got)
	}
	// Sin claims => vacío.
	r2 := httptest.NewRequest(http.MethodGet, "/x", http.NoBody)
	if got := subFromCtx(r2); got != "" {
		t.Fatalf("subFromCtx sin claims = %q", got)
	}
}

func TestEscapeFilename(t *testing.T) {
	if got := escapeFilename("árbol de tareas.txt"); !strings.Contains(got, "%") {
		t.Fatalf("escapeFilename no codifica: %q", got)
	}
}

// EL NOMBRE DEL FICHERO SOBREVIVE A LOS ACENTOS. Es el fallo que dejaba «Sytue
// PDF» sin poder crear un PDF en blanco y Drive sin poder subir: `fetch` manda
// una cabecera en Latin-1, así que «PDF sin título.pdf» llegaba con la í como un
// byte suelto, Postgres rechazaba el insert y la subida se caía con un 500 mudo.
// En castellano; en inglés el nombre por defecto no lleva tilde y nadie lo vio.
func TestNombreDeFicheroConAcentos(t *testing.T) {
	const quiero = "PDF sin título.pdf"

	// Por el query, que es como se manda ahora: llega intacto.
	r := httptest.NewRequest(http.MethodPost, "/?name="+url.QueryEscape(quiero), http.NoBody)
	if got := fileNameFromRequest(r); got != quiero {
		t.Fatalf("por ?name= = %q, quiero %q", got, quiero)
	}

	// Por la cabecera, como la mandan los clientes ya instalados: llega rota y se
	// repara. Lo que NO puede pasar es que se guarde inválida.
	r = httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	r.Header.Set("X-File-Name", string([]byte("PDF sin t\xedtulo.pdf")))
	got := fileNameFromRequest(r)
	if !utf8.ValidString(got) {
		t.Fatalf("el nombre no es UTF-8 válido: %q", got)
	}
	if got != quiero {
		t.Fatalf("por cabecera = %q, quiero %q", got, quiero)
	}

	// Con las dos, manda el query: es el único que no puede venir estropeado.
	r = httptest.NewRequest(http.MethodPost, "/?name="+url.QueryEscape(quiero), http.NoBody)
	r.Header.Set("X-File-Name", "otro.pdf")
	if got := fileNameFromRequest(r); got != quiero {
		t.Fatalf("con ambos = %q, quiero el del query %q", got, quiero)
	}
}

func TestUnavailableExternalStorageHasSafeActionableError(t *testing.T) {
	const privateCause = "private provider configuration"
	err := toAPIError(errors.Join(service.ErrExternalStorageOff, errors.New(privateCause)))
	var response *httpx.Error
	if !errors.As(err, &response) || response.Status != http.StatusServiceUnavailable || response.Code != "external_storage_unavailable" || strings.Contains(response.Message, privateCause) {
		t.Fatalf("unavailable storage returned an unsafe or unclassified response: %v", err)
	}
}
