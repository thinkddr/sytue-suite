// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de los handlers HTTP de UNIDADES DE EQUIPO con el fake del servicio (sin
// PostgreSQL ni S3). Cubren crear/listar/obtener/renombrar/archivar, la gestión de
// miembros y el contenido, y —lo esencial— el mapeo deny-by-default: un no-miembro
// (ErrNotFound → 404), un rol insuficiente (ErrPermission → 403) y el último gestor
// (ErrLastManager → 409). La verja REAL (RLS + rol) se prueba por integración en el
// store; aquí se fija el CONTRATO HTTP.
package drivehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (%s)", err, rec.Body.String())
	}
}

// --- crear / listar / obtener -------------------------------------------

func TestCreateTeamDriveOK(t *testing.T) {
	svc := &fakeSvc{teamDrive: domain.TeamDrive{ID: uuid.New(), Name: "Marketing"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.CreateTeamDrive(rec, mkReq(http.MethodPost, "/api/v1/team-drives", `{"name":"Marketing"}`, nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d, quiero 201 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	decodeJSON(t, rec, &got)
	if got.Role != "manager" {
		t.Fatalf("el creador debe ser manager, rol=%q", got.Role)
	}
}

func TestCreateTeamDriveMissingName(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.CreateTeamDrive(rec, mkReq(http.MethodPost, "/x", `{"name":"  "}`, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("nombre vacío => %d, quiero 400", rec.Code)
	}
}

func TestListTeamDrives(t *testing.T) {
	svc := &fakeSvc{teamDrives: []service.TeamDriveMembership{
		{TeamDrive: domain.TeamDrive{ID: uuid.New(), Name: "A"}, Role: domain.TeamManager},
		{TeamDrive: domain.TeamDrive{ID: uuid.New(), Name: "B"}, Role: domain.TeamViewer},
	}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.ListTeamDrives(rec, mkReq(http.MethodGet, "/api/v1/team-drives", "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	var got struct {
		Items []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"items"`
	}
	decodeJSON(t, rec, &got)
	if len(got.Items) != 2 || got.Items[0].Role != "manager" || got.Items[1].Role != "viewer" {
		t.Fatalf("items inesperados: %+v", got.Items)
	}
}

// AISLAMIENTO: un no-miembro no distingue «no existe» de «no eres miembro» (404).
func TestGetTeamDriveNonMember(t *testing.T) {
	h := newHandlers(&fakeSvc{teamDriveErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.GetTeamDrive(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no-miembro => %d, quiero 404", rec.Code)
	}
}

// --- renombrar / archivar (solo manager) --------------------------------

func TestUpdateTeamDriveRename(t *testing.T) {
	svc := &fakeSvc{teamRenamed: domain.TeamDrive{ID: uuid.New(), Name: "Nuevo"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.UpdateTeamDrive(rec, mkReq(http.MethodPatch, "/x", `{"name":"Nuevo"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestUpdateTeamDriveArchive(t *testing.T) {
	svc := &fakeSvc{teamArchived: domain.TeamDrive{ID: uuid.New(), Name: "X"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.UpdateTeamDrive(rec, mkReq(http.MethodPatch, "/x", `{"archived":true}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if !svc.lastTeamArchived {
		t.Fatal("no propagó archived=true al servicio")
	}
}

func TestUpdateTeamDriveBothFieldsRejected(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.UpdateTeamDrive(rec, mkReq(http.MethodPatch, "/x", `{"name":"a","archived":true}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("name+archived juntos => %d, quiero 400", rec.Code)
	}
}

// Un rol insuficiente (no-manager) al renombrar → 403 (deny-by-default).
func TestUpdateTeamDriveInsufficientRole(t *testing.T) {
	h := newHandlers(&fakeSvc{teamRenameErr: domain.ErrPermission})
	rec := httptest.NewRecorder()
	h.UpdateTeamDrive(rec, mkReq(http.MethodPatch, "/x", `{"name":"a"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rol insuficiente => %d, quiero 403", rec.Code)
	}
}

// --- miembros -----------------------------------------------------------

func TestListTeamDriveMembersForbidden(t *testing.T) {
	// Un no-manager (o no-miembro con rol insuficiente) no ve la lista de miembros.
	h := newHandlers(&fakeSvc{teamMembersErr: domain.ErrPermission})
	rec := httptest.NewRecorder()
	h.ListTeamDriveMembers(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rol insuficiente => %d, quiero 403", rec.Code)
	}
}

func TestPutTeamDriveMemberOK(t *testing.T) {
	svc := &fakeSvc{teamMember: domain.TeamMember{Subject: "user-2", Role: domain.TeamEditor}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.PutTeamDriveMember(rec, mkReq(http.MethodPut, "/x", `{"subject":"user-2","role":"editor"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastTeamMemberSub != "user-2" || svc.lastTeamMemberRol != domain.TeamEditor {
		t.Fatalf("no propagó subject/role: %q %q", svc.lastTeamMemberSub, svc.lastTeamMemberRol)
	}
}

func TestPutTeamDriveMemberInvalidRole(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.PutTeamDriveMember(rec, mkReq(http.MethodPut, "/x", `{"subject":"user-2","role":"admin"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rol inválido => %d, quiero 400", rec.Code)
	}
}

func TestPutTeamDriveMemberMissingSubject(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.PutTeamDriveMember(rec, mkReq(http.MethodPut, "/x", `{"subject":"","role":"editor"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sujeto vacío => %d, quiero 400", rec.Code)
	}
}

// Degradar/quitar al último gestor → 409 (la unidad no puede quedar sin gestor).
func TestPutTeamDriveMemberLastManager(t *testing.T) {
	h := newHandlers(&fakeSvc{teamPutErr: domain.ErrLastManager})
	rec := httptest.NewRecorder()
	h.PutTeamDriveMember(rec, mkReq(http.MethodPut, "/x", `{"subject":"user-1","role":"viewer"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("último gestor => %d, quiero 409", rec.Code)
	}
}

func TestRemoveTeamDriveMemberOK(t *testing.T) {
	svc := &fakeSvc{}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.RemoveTeamDriveMember(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": uuid.New().String(), "subject": "user-2"}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
	if svc.lastTeamRemoveSub != "user-2" {
		t.Fatalf("no propagó el sujeto a quitar: %q", svc.lastTeamRemoveSub)
	}
}

func TestRemoveTeamDriveMemberLastManager(t *testing.T) {
	h := newHandlers(&fakeSvc{teamRemoveErr: domain.ErrLastManager})
	rec := httptest.NewRecorder()
	h.RemoveTeamDriveMember(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{"id": uuid.New().String(), "subject": "user-1"}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("último gestor => %d, quiero 409", rec.Code)
	}
}

// --- contenido ----------------------------------------------------------

func TestListTeamDriveChildrenOK(t *testing.T) {
	svc := &fakeSvc{teamListing: service.TeamListing{
		Subfolders: []domain.Folder{{ID: uuid.New(), Name: "sub"}},
		Files:      []domain.File{{ID: uuid.New(), Name: "f.txt"}},
		Role:       domain.TeamEditor,
	}}
	h := newHandlers(svc)
	folderID := uuid.New()
	rec := httptest.NewRecorder()
	h.ListTeamDriveChildren(rec, mkReq(http.MethodGet, "/x?folder="+folderID.String(), "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Role     string `json:"role"`
		CanWrite bool   `json:"can_write"`
		Folders  []any  `json:"folders"`
		Files    []any  `json:"files"`
	}
	decodeJSON(t, rec, &got)
	if got.Role != "editor" || !got.CanWrite || len(got.Folders) != 1 || len(got.Files) != 1 {
		t.Fatalf("respuesta inesperada: %+v", got)
	}
	if svc.lastTeamFolderID != folderID {
		t.Fatalf("no propagó ?folder=: %v", svc.lastTeamFolderID)
	}
}

func TestListTeamDriveChildrenNonMember(t *testing.T) {
	h := newHandlers(&fakeSvc{teamChildErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.ListTeamDriveChildren(rec, mkReq(http.MethodGet, "/x", "", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no-miembro => %d, quiero 404", rec.Code)
	}
}

func TestCreateTeamDriveFolderViewerForbidden(t *testing.T) {
	// Un viewer no puede crear carpetas (el servicio devuelve ErrPermission).
	h := newHandlers(&fakeSvc{teamFolderErr: domain.ErrPermission})
	rec := httptest.NewRecorder()
	h.CreateTeamDriveFolder(rec, mkReq(http.MethodPost, "/x", `{"name":"docs"}`, map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer crea carpeta => %d, quiero 403", rec.Code)
	}
}

func TestUploadTeamDriveFileOK(t *testing.T) {
	svc := &fakeSvc{teamUpload: domain.File{ID: uuid.New(), Name: "doc.txt"}}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.UploadTeamDriveFile(rec, mkReq(http.MethodPost, "/x?name=doc.txt", "hello", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("código = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastUploadName != "doc.txt" {
		t.Fatalf("no propagó el nombre: %q", svc.lastUploadName)
	}
}

func TestUploadTeamDriveFileMissingName(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.UploadTeamDriveFile(rec, mkReq(http.MethodPost, "/x", "hello", map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin nombre => %d, quiero 400", rec.Code)
	}
}

func TestDownloadTeamDriveFileOK(t *testing.T) {
	svc := &fakeSvc{
		teamOpenFile: domain.File{Name: "d.txt", ContentType: "text/plain", SizeBytes: 4, Checksum: "abc"},
		teamOpenBody: "data",
	}
	h := newHandlers(svc)
	rec := httptest.NewRecorder()
	h.DownloadTeamDriveFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{
		"id": uuid.New().String(), "fileID": uuid.New().String(),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("falta nosniff en la descarga")
	}
	if rec.Body.String() != "data" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
}

func TestDownloadTeamDriveFileNonMember(t *testing.T) {
	h := newHandlers(&fakeSvc{teamOpenErr: domain.ErrNotFound})
	rec := httptest.NewRecorder()
	h.DownloadTeamDriveFile(rec, mkReq(http.MethodGet, "/x", "", map[string]string{
		"id": uuid.New().String(), "fileID": uuid.New().String(),
	}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no-miembro => %d, quiero 404", rec.Code)
	}
}

func TestDeleteTeamDriveFileViewerForbidden(t *testing.T) {
	h := newHandlers(&fakeSvc{teamDelFileErr: domain.ErrPermission})
	rec := httptest.NewRecorder()
	h.DeleteTeamDriveFile(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{
		"id": uuid.New().String(), "fileID": uuid.New().String(),
	}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer borra => %d, quiero 403", rec.Code)
	}
}

func TestDeleteTeamDriveFolderOK(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rec := httptest.NewRecorder()
	h.DeleteTeamDriveFolder(rec, mkReq(http.MethodDelete, "/x", "", map[string]string{
		"id": uuid.New().String(), "folderID": uuid.New().String(),
	}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204", rec.Code)
	}
}
