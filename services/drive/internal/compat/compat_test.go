// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la traducción a los dialectos de Google Drive v3 y Microsoft Graph.
// Todo es puro: sin red, sin BD, sin contexto.
package compat

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

var (
	fileID   = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	folderID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	parentID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	ts       = time.Date(2026, 7, 27, 10, 30, 0, 0, time.FixedZone("CEST", 2*3600))
)

func sampleFile() domain.File {
	return domain.File{
		ID:          fileID,
		FolderID:    folderID,
		Name:        "informe.pdf",
		SizeBytes:   2048,
		ContentType: "application/pdf",
		Checksum:    "abc123",
		CreatedAt:   ts,
		UpdatedAt:   ts,
	}
}

func sampleFolder() domain.Folder {
	p := parentID
	return domain.Folder{ID: folderID, ParentID: &p, Name: "Proyectos", CreatedAt: ts, UpdatedAt: ts}
}

// --- Google ---------------------------------------------------------------

func TestGoogleTranslation(t *testing.T) {
	root := domain.Folder{ID: folderID, Name: "Mi unidad", IsRoot: true, UpdatedAt: ts}
	empty := domain.File{ID: fileID, FolderID: folderID, Name: "vacio.bin", UpdatedAt: ts}

	tests := []struct {
		name string
		got  GoogleFile
		want GoogleFile
	}{
		{
			name: "fichero",
			got:  GoogleFromFile(sampleFile()),
			want: GoogleFile{
				Kind: "drive#file", ID: fileID.String(), Name: "informe.pdf",
				MimeType: "application/pdf", Size: "2048",
				CreatedTime: "2026-07-27T08:30:00Z", ModifiedTime: "2026-07-27T08:30:00Z",
				Parents: []string{folderID.String()},
			},
		},
		{
			name: "carpeta lleva el mimeType de carpeta y sin size",
			got:  GoogleFromFolder(sampleFolder()),
			want: GoogleFile{
				Kind: "drive#file", ID: folderID.String(), Name: "Proyectos",
				MimeType: FolderMIME, CreatedTime: "2026-07-27T08:30:00Z",
				ModifiedTime: "2026-07-27T08:30:00Z", Parents: []string{parentID.String()},
			},
		},
		{
			name: "raíz sin padre no emite parents",
			got:  GoogleFromFolder(root),
			want: GoogleFile{
				Kind: "drive#file", ID: folderID.String(), Name: "Mi unidad",
				MimeType: FolderMIME, ModifiedTime: "2026-07-27T08:30:00Z",
			},
		},
		{
			name: "fichero de 0 bytes sin content-type",
			got:  GoogleFromFile(empty),
			want: GoogleFile{
				Kind: "drive#file", ID: fileID.String(), Name: "vacio.bin",
				MimeType: "application/octet-stream", Size: "0",
				ModifiedTime: "2026-07-27T08:30:00Z", Parents: []string{folderID.String()},
			},
		},
		{
			name: "entrada transversal (sin padre conocido)",
			got:  GoogleNode(fileID, "nota.txt", false, 7, "text/plain", ts),
			want: GoogleFile{
				Kind: "drive#file", ID: fileID.String(), Name: "nota.txt",
				MimeType: "text/plain", Size: "7", ModifiedTime: "2026-07-27T08:30:00Z",
			},
		},
		{
			name: "entrada transversal de carpeta",
			got:  GoogleNode(folderID, "Docs", true, 0, "", ts),
			want: GoogleFile{
				Kind: "drive#file", ID: folderID.String(), Name: "Docs",
				MimeType: FolderMIME, ModifiedTime: "2026-07-27T08:30:00Z",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if a, b := toJSON(t, tc.got), toJSON(t, tc.want); a != b {
				t.Fatalf("traducción\n got %s\nquiero %s", a, b)
			}
		})
	}
}

func TestGoogleFileListNeverNil(t *testing.T) {
	out := toJSON(t, NewGoogleFileList(nil, ""))
	if !strings.Contains(out, `"files":[]`) {
		t.Fatalf("files debe ser array vacío: %s", out)
	}
	if strings.Contains(out, "nextPageToken") {
		t.Fatalf("sin token no debe emitirse nextPageToken: %s", out)
	}
	if out := toJSON(t, NewGoogleFileList(nil, "tok")); !strings.Contains(out, `"nextPageToken":"tok"`) {
		t.Fatalf("falta nextPageToken: %s", out)
	}
}

func TestNewGoogleAboutQuota(t *testing.T) {
	q := domain.Quota{LimitBytes: 1000, UsedBytes: 250}
	a := NewGoogleAbout("sub-1", "Ada", "ada@sytue.eu", &q)
	if a.StorageQuota == nil || a.StorageQuota.Limit != "1000" || a.StorageQuota.Usage != "250" {
		t.Fatalf("cuota = %+v", a.StorageQuota)
	}
	if a.User.EmailAddress != "ada@sytue.eu" || !a.User.Me {
		t.Fatalf("usuario = %+v", a.User)
	}
	// Sin cuota conocida el campo se OMITE (no se inventa).
	if got := NewGoogleAbout("sub-1", "Ada", "", nil); got.StorageQuota != nil {
		t.Fatalf("storageQuota debe omitirse: %+v", got.StorageQuota)
	}
	if got := NewGoogleAbout("s", "A", "", &domain.Quota{}); got.StorageQuota != nil {
		t.Fatalf("límite 0 ⇒ storageQuota omitido: %+v", got.StorageQuota)
	}
	// `usage` es el total del plan; `usageInDrive`, solo lo de Drive: lo que Cast ha
	// declarado (mig 0026) cuenta en el primero y no en el segundo.
	q = domain.Quota{LimitBytes: 1000, UsedBytes: 250, ExternalUsage: map[string]int64{"cast": 100}}
	a = NewGoogleAbout("sub-1", "Ada", "ada@sytue.eu", &q)
	if a.StorageQuota.Usage != "250" || a.StorageQuota.UsageInDrive != "150" {
		t.Fatalf("con uso externo: %+v", a.StorageQuota)
	}
}

func TestGoogleErrorReason(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusBadRequest:          "badRequest",
		http.StatusNotFound:            "notFound",
		http.StatusForbidden:           "forbidden",
		http.StatusConflict:            "duplicate",
		http.StatusNotImplemented:      "notImplemented",
		http.StatusInsufficientStorage: "storageQuotaExceeded",
		http.StatusInternalServerError: "backendError",
	} {
		e := NewGoogleError(status, "mensaje")
		if e.Error.Code != status || e.Error.Errors[0].Reason != want {
			t.Fatalf("status %d ⇒ %+v, quiero reason %q", status, e.Error, want)
		}
	}
}

// --- `q` de files.list ----------------------------------------------------

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name string
		q    string
		want Query
	}{
		{name: "vacío", q: "", want: Query{}},
		{name: "solo padre", q: "'abc' in parents", want: Query{ParentID: "abc"}},
		{name: "raíz", q: "'root' in parents", want: Query{ParentID: "root"}},
		{
			name: "padre + no papelera",
			q:    "'abc' in parents and trashed = false",
			want: Query{ParentID: "abc", TrashedSet: true},
		},
		{
			name: "carpetas de un padre",
			q:    "'abc' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
			want: Query{ParentID: "abc", MimeType: FolderMIME, TrashedSet: true},
		},
		{
			name: "todo menos carpetas",
			q:    "mimeType != 'application/vnd.google-apps.folder'",
			want: Query{MimeType: FolderMIME, MimeNot: true},
		},
		{
			name: "nombre exacto con espacios y apóstrofo escapado",
			q:    `name = 'acta de la reuni\'on.pdf' and 'p1' in parents`,
			want: Query{Name: "acta de la reuni'on.pdf", ParentID: "p1"},
		},
		{
			name: "nombre que contiene la palabra and",
			q:    "name = 'ley and orden.txt'",
			want: Query{Name: "ley and orden.txt"},
		},
		{name: "papelera", q: "trashed = true", want: Query{Trashed: true, TrashedSet: true}},
		{name: "espacios sobrantes", q: "  'abc'   in   parents  ", want: Query{ParentID: "abc"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseQuery(tc.q)
			if err != nil {
				t.Fatalf("ParseQuery(%q) error: %v", tc.q, err)
			}
			if got != tc.want {
				t.Fatalf("ParseQuery(%q) = %+v, quiero %+v", tc.q, got, tc.want)
			}
		})
	}
}

func TestParseQueryUnsupported(t *testing.T) {
	// Lo que no se soporta falla CLARO: nunca se ignora devolviendo un listado sin
	// filtrar (sería un resultado silenciosamente incorrecto).
	for _, q := range []string{
		"name contains 'informe'",
		"'me' in owners",
		"fullText contains 'x'",
		"starred = true",
		"'a' in parents or 'b' in parents",
		"modifiedTime > '2026-01-01T00:00:00'",
		"('a' in parents)",
		"name = informe",        // valor sin comillas
		"trashed = quizá",       // booleano inválido
		"name != 'informe.pdf'", // != solo para mimeType
		"'a' in parents and name contains 'x'",
	} {
		if _, err := ParseQuery(q); !errors.Is(err, ErrUnsupportedQuery) {
			t.Fatalf("ParseQuery(%q) error = %v, quiero ErrUnsupportedQuery", q, err)
		}
	}
}

func TestQueryMatchesAndScope(t *testing.T) {
	file := GoogleFromFile(sampleFile())
	folder := GoogleFromFolder(sampleFolder())

	tests := []struct {
		name         string
		q            Query
		file, folder bool
		foldersOnly  bool
		filesOnly    bool
	}{
		{name: "sin filtros", q: Query{}, file: true, folder: true},
		{name: "nombre exacto", q: Query{Name: "informe.pdf"}, file: true},
		{name: "nombre sin distinguir mayúsculas", q: Query{Name: "INFORME.PDF"}, file: true},
		{name: "nombre que no está", q: Query{Name: "otro.pdf"}},
		{name: "solo carpetas", q: Query{MimeType: FolderMIME}, folder: true, foldersOnly: true},
		{name: "todo menos carpetas", q: Query{MimeType: FolderMIME, MimeNot: true}, file: true, filesOnly: true},
		{name: "tipo concreto", q: Query{MimeType: "application/pdf"}, file: true, filesOnly: true},
		{name: "tipo distinto", q: Query{MimeType: "image/png", MimeNot: true}, file: true, folder: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.q.Matches(file); got != tc.file {
				t.Fatalf("Matches(fichero) = %v, quiero %v", got, tc.file)
			}
			if got := tc.q.Matches(folder); got != tc.folder {
				t.Fatalf("Matches(carpeta) = %v, quiero %v", got, tc.folder)
			}
			if got := tc.q.FoldersOnly(); got != tc.foldersOnly {
				t.Fatalf("FoldersOnly = %v, quiero %v", got, tc.foldersOnly)
			}
			if got := tc.q.FilesOnly(); got != tc.filesOnly {
				t.Fatalf("FilesOnly = %v, quiero %v", got, tc.filesOnly)
			}
		})
	}
}

// --- Microsoft Graph ------------------------------------------------------

func TestGraphTranslation(t *testing.T) {
	three := 3
	tests := []struct {
		name string
		got  DriveItem
		want string // JSON esperado
	}{
		{
			name: "fichero: faceta file + hash + parentReference",
			got:  GraphFromFile(sampleFile()),
			want: `{"id":"` + fileID.String() + `","name":"informe.pdf","size":2048,` +
				`"createdDateTime":"2026-07-27T08:30:00Z","lastModifiedDateTime":"2026-07-27T08:30:00Z",` +
				`"file":{"mimeType":"application/pdf","hashes":{"sha256Hash":"abc123"}},` +
				`"parentReference":{"id":"` + folderID.String() + `"}}`,
		},
		{
			name: "carpeta: faceta folder sin childCount conocido",
			got:  GraphFromFolder(sampleFolder(), nil),
			want: `{"id":"` + folderID.String() + `","name":"Proyectos","size":0,` +
				`"createdDateTime":"2026-07-27T08:30:00Z","lastModifiedDateTime":"2026-07-27T08:30:00Z",` +
				`"folder":{},"parentReference":{"id":"` + parentID.String() + `"}}`,
		},
		{
			name: "carpeta con childCount conocido",
			got:  GraphFromFolder(domain.Folder{ID: folderID, Name: "Docs", UpdatedAt: ts}, &three),
			want: `{"id":"` + folderID.String() + `","name":"Docs","size":0,` +
				`"lastModifiedDateTime":"2026-07-27T08:30:00Z","folder":{"childCount":3}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := toJSON(t, tc.got); got != tc.want {
				t.Fatalf("traducción\n got %s\nquiero %s", got, tc.want)
			}
		})
	}
}

func TestNewGraphDrive(t *testing.T) {
	root := domain.Folder{ID: folderID, Name: "Mi unidad", IsRoot: true, UpdatedAt: ts}
	q := domain.Quota{LimitBytes: 100, UsedBytes: 95}
	d := NewGraphDrive(root, "sub-1", "Ada", &q)
	if d.ID != folderID.String() || d.Root == nil || d.Root.Folder == nil {
		t.Fatalf("drive = %+v", d)
	}
	if d.Quota == nil || d.Quota.Remaining != 5 || d.Quota.State != "critical" {
		t.Fatalf("cuota = %+v", d.Quota)
	}
	if got := NewGraphDrive(root, "sub-1", "Ada", nil); got.Quota != nil {
		t.Fatalf("sin cuota conocida se omite: %+v", got.Quota)
	}
}

func TestQuotaState(t *testing.T) {
	for _, tc := range []struct {
		used, limit int64
		want        string
	}{
		{10, 100, "normal"},
		{80, 100, "nearing"},
		{90, 100, "critical"},
		{100, 100, "exceeded"},
		{120, 100, "exceeded"},
	} {
		if got := quotaState(domain.Quota{UsedBytes: tc.used, LimitBytes: tc.limit}); got != tc.want {
			t.Fatalf("quotaState(%d/%d) = %q, quiero %q", tc.used, tc.limit, got, tc.want)
		}
	}
}

func TestGraphQuotaRespetaElLibreDelProveedor(t *testing.T) {
	total, used, available := int64(1000), int64(100), int64(0)
	q := (domain.Quota{LimitBytes: 500, UsedBytes: 500}).WithStorageExtension(&total, &used, &available)
	drive := NewGraphDrive(domain.Folder{}, "u-1", "Ada", &q)
	if drive.Quota == nil || drive.Quota.Total != 1500 || drive.Quota.Used != 600 ||
		drive.Quota.Remaining != 0 || drive.Quota.State != "exceeded" {
		t.Fatalf("Graph no puede anunciar espacio utilizable inexistente: %+v", drive.Quota)
	}
	// Valores admitidos por la API no pueden desbordar al calcular porcentajes.
	large := int64(1<<63 - 1)
	if got := quotaState(domain.Quota{LimitBytes: large, UsedBytes: large / 2}); got != "normal" {
		t.Fatalf("cuota grande a la mitad: %q", got)
	}
	if got := quotaState(domain.Quota{LimitBytes: large, UsedBytes: large - 1}); got != "critical" {
		t.Fatalf("cuota grande con un byte libre: %q", got)
	}
}

func TestNextLink(t *testing.T) {
	if got := NextLink("https://drive.sytue.eu/", "/compat/graph/v1.0/me/drive/root/children", 0, ""); got != "" {
		t.Fatalf("sin token no hay nextLink: %q", got)
	}
	got := NextLink("https://drive.sytue.eu/", "/compat/graph/v1.0/me/drive/root/children", 50, "tok-1")
	want := "https://drive.sytue.eu/compat/graph/v1.0/me/drive/root/children?%24skiptoken=tok-1&%24top=50"
	if got != want {
		t.Fatalf("nextLink = %q, quiero %q", got, want)
	}
}

func TestDriveItemCollectionNeverNil(t *testing.T) {
	if out := toJSON(t, NewDriveItemCollection(nil, "")); out != `{"value":[]}` {
		t.Fatalf("colección vacía = %s", out)
	}
}

func TestGraphErrorCode(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusBadRequest:          "invalidRequest",
		http.StatusUnauthorized:        "unauthenticated",
		http.StatusForbidden:           "accessDenied",
		http.StatusNotFound:            "itemNotFound",
		http.StatusConflict:            "nameAlreadyExists",
		http.StatusNotImplemented:      "notImplemented",
		http.StatusInsufficientStorage: "quotaLimitReached",
		http.StatusInternalServerError: "generalException",
	} {
		if got := NewGraphError(status, "m").Error.Code; got != want {
			t.Fatalf("status %d ⇒ code %q, quiero %q", status, got, want)
		}
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
