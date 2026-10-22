// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestDrive(t *testing.T) {
	var subida struct{ nombre, tipo, cuerpo, token string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","name":"","is_root":true}`)
		case "GET /api/v1/folders/raiz/children":
			if r.URL.Query().Get("cursor") == "" {
				_, _ = io.WriteString(w, `{"folders":[{"id":"c1","name":"Contabilidad"}],"files":[],"next_cursor":"pag2"}`)
				return
			}
			_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f1","folder_id":"raiz","name":"a.pdf","size_bytes":1536,"content_type":"application/pdf"}]}`)
		case "POST /api/v1/folders":
			_, _ = io.WriteString(w, `{"id":"c2","parent_id":"raiz","name":"Nueva"}`)
		case "GET /api/v1/files/f1/content":
			_, _ = io.WriteString(w, "%PDF-1.7")
		case "GET /api/v1/files/f1/thumbnail":
			_, _ = io.WriteString(w, "\x89PNG")
		case "POST /api/v1/folders/raiz/files":
			cuerpo, _ := io.ReadAll(r.Body)
			subida.nombre, subida.tipo, subida.cuerpo = r.URL.Query().Get("name"), r.Header.Get("Content-Type"), string(cuerpo)
			subida.token = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `{"id":"f2","folder_id":"raiz","name":"`+subida.nombre+`","size_bytes":4}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	d := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	raiz, err := d.Root(ctx)
	if err != nil || raiz.ID != "raiz" || !raiz.IsRoot {
		t.Fatalf("Root = %+v, %v", raiz, err)
	}
	p1, err := d.Children(ctx, raiz.ID, "")
	if err != nil || len(p1.Folders) != 1 || p1.NextCursor != "pag2" {
		t.Fatalf("página 1 = %+v, %v", p1, err)
	}
	p2, err := d.Children(ctx, raiz.ID, p1.NextCursor)
	if err != nil || len(p2.Files) != 1 || p2.Files[0].SizeBytes != 1536 || p2.NextCursor != "" {
		t.Fatalf("página 2 = %+v, %v", p2, err)
	}
	if c, err := d.CreateFolder(ctx, "raiz", "Nueva"); err != nil || c.ID != "c2" {
		t.Fatalf("CreateFolder = %+v, %v", c, err)
	}
	cuerpo, err := d.Download(ctx, "f1")
	if err != nil {
		t.Fatal(err)
	}
	contenido, _ := io.ReadAll(cuerpo)
	_ = cuerpo.Close()
	if string(contenido) != "%PDF-1.7" {
		t.Fatalf("descarga = %q", contenido)
	}
	if mini, err := d.Thumbnail(ctx, "f1"); err != nil {
		t.Fatal(err)
	} else if datos, _ := io.ReadAll(mini); string(datos) != "\x89PNG" || mini.Close() != nil {
		t.Fatalf("miniatura = %q", datos)
	}
	// Un nombre con eñe, espacios y raya viaja en la query, no en una cabecera.
	nombre := "Factura nº 3 — señal.pdf"
	f, err := d.Upload(ctx, "raiz", nombre, "application/pdf", strings.NewReader("hola"), 4)
	if err != nil || f.ID != "f2" || subida.nombre != nombre || subida.tipo != "application/pdf" || subida.cuerpo != "hola" || subida.token != "Bearer tok" {
		t.Fatalf("Upload = %+v, %v; llegó %+v", f, err, subida)
	}
}

// Lo que viaja al servicio al organizar: qué método, a qué ruta y con qué cuerpo. Mover
// un fichero y mover una carpeta NO llevan el mismo campo.
func TestDriveOrganizar(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		if r.Method == "QUERY" { // la búsqueda viaja en el cuerpo, no en la URL
			if r.URL.RawQuery != "" {
				t.Errorf("la búsqueda ha dejado su texto en la URL: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"files":[{"id":"f9","name":"iva.pdf","is_directory":false,"path":"/Contabilidad/iva.pdf","size_bytes":7},{"id":"c9","name":"IVA","is_directory":true,"path":"/IVA"}]}`)
			pedidas = append(pedidas, r.Method+" "+r.URL.Path+" "+string(cuerpo))
			return
		}
		pedidas = append(pedidas, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(cuerpo)))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/trash":
			_, _ = io.WriteString(w, `{"folders":[{"id":"c1","name":"Vieja","deleted_at":"2026-09-01T10:00:00Z"}],"files":[],"retention_seconds":2592000}`)
		case "GET /api/v1/quota":
			_, _ = io.WriteString(w, `{"tenant":"acme","limit_bytes":1000,"used_bytes":250,"available_bytes":750,"external_storage":true}`)
		case "GET /api/v1/folders/c1/shares":
			_, _ = io.WriteString(w, `{"shares":[{"id":"s1","kind":"link","permission":"read","has_password":true,"expires_at":"2026-10-01T21:59:59Z"}]}`)
		case "POST /api/v1/shares":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"share":{"id":"s2","kind":"link","permission":"write","token":"secreto"},"url":"https://drive.example/public/secreto"}`)
		case "GET /api/v1/files/f1/versions":
			_, _ = io.WriteString(w, `{"versions":[{"file_id":"f1","version":3,"size_bytes":9,"is_current":true},{"file_id":"f1","version":2,"size_bytes":7,"is_current":false,"author_sub":"ana"}],"next_cursor":"v1"}`)
		case "GET /api/v1/files/f1/versions/2/content":
			_, _ = io.WriteString(w, "la de antes")
		case "GET /api/v1/recent":
			if r.URL.Query().Get("cursor") == "mas" {
				_, _ = io.WriteString(w, `{"items":[]}`)
				return
			}
			_, _ = io.WriteString(w, `{"items":[{"id":"f7","name":"acta.md","is_directory":false,"path":"/Actas/acta.md"}],"next_cursor":"mas"}`)
		case "GET /api/v1/favorites":
			_, _ = io.WriteString(w, `{"items":[{"id":"c7","name":"Actas","is_directory":true,"path":"/Actas"},{"id":"f7","name":"acta.md","is_directory":false,"path":"/Actas/acta.md"}]}`)
		case "POST /api/v1/favorites":
			_, _ = io.WriteString(w, `{"favorite":true}`)
		case "GET /api/v1/folders/c7":
			_, _ = io.WriteString(w, `{"id":"c7","parent_id":"raiz","name":"Actas"}`)
		case "POST /api/v1/batch/move":
			_, _ = io.WriteString(w, `{"moved_files":2,"moved_folders":1}`)
		case "POST /api/v1/batch/trash":
			_, _ = io.WriteString(w, `{"trashed_files":1,"trashed_folders":0}`)
		case "GET /api/v1/batch/zip":
			w.Header().Set("X-Zip-Content-Bytes", "1536")
			_, _ = io.WriteString(w, "PK\x03\x04")
		case "POST /api/v1/folders/c1/copy":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"c3","parent_id":"raiz","name":"Copia de Contabilidad"}`)
		case "POST /api/v1/files/f1/copy":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"f2","folder_id":"raiz","name":"Copia de a.pdf"}`)
		case "PATCH /api/v1/files/f1", "PATCH /api/v1/folders/c1":
			if strings.Contains(string(cuerpo), "repetido") {
				w.WriteHeader(http.StatusConflict)
				return
			}
			_, _ = io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	d := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	pasos := []error{
		d.Rename(ctx, DriveFile, "f1", "b.pdf"), d.Rename(ctx, DriveFolder, "c1", "Nueva"),
		d.Move(ctx, DriveFile, "f1", "c1"), d.Move(ctx, DriveFolder, "c1", "c2"),
		d.Delete(ctx, DriveFile, "f1"), d.Restore(ctx, DriveFolder, "c1"), d.Purge(ctx, DriveFile, "f1"),
	}
	for i, err := range pasos {
		if err != nil {
			t.Fatalf("paso %d: %v", i, err)
		}
	}
	var e *Error
	if err := d.Rename(ctx, DriveFile, "f1", "repetido"); !errors.As(err, &e) || e.Status != http.StatusConflict {
		t.Fatalf("un nombre repetido tiene que llegar como 409: %v", err)
	}
	if c, err := d.CopyFile(ctx, "f1"); err != nil || c.Name != "Copia de a.pdf" {
		t.Fatalf("CopyFile = %+v, %v", c, err)
	}
	if c, err := d.CopyFolder(ctx, "c1"); err != nil || c.Name != "Copia de Contabilidad" {
		t.Fatalf("CopyFolder = %+v, %v", c, err)
	}
	if p, err := d.Trash(ctx); err != nil || len(p.Folders) != 1 || p.Folders[0].DeletedAt == nil || p.RetentionSeconds != 2592000 {
		t.Fatalf("Trash = %+v, %v", p, err)
	}
	if q, err := d.Quota(ctx); err != nil || q.UsedBytes != 250 || q.LimitBytes != 1000 || !q.ExternalStorage {
		t.Fatalf("Quota = %+v, %v", q, err)
	}
	if c, err := d.Shares(ctx, DriveFolder, "c1"); err != nil || len(c) != 1 || !c[0].HasPassword || c[0].ExpiresAt == nil {
		t.Fatalf("Shares = %+v, %v", c, err)
	}
	// La caducidad es el FINAL del día elegido y viaja en RFC3339, que es lo que el
	// servicio exige.
	fin := time.Date(2026, 10, 1, 21, 59, 59, 0, time.UTC)
	if c, dir, err := d.ShareLink(ctx, DriveFolder, "c1", true, "clave", fin); err != nil || c.ID != "s2" || dir != "https://drive.example/public/secreto" {
		t.Fatalf("ShareLink = %+v, %q, %v", c, dir, err)
	}
	if _, _, err := d.ShareLink(ctx, DriveFile, "f1", false, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ShareWith(ctx, DriveFile, "f1", "user", "sub-ana", true); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ShareWith(ctx, DriveFolder, "c1", "group", "Ventas", false); err != nil {
		t.Fatal(err)
	}
	if err := d.Revoke(ctx, "s2"); err != nil {
		t.Fatal(err)
	}
	if h, sig, err := d.Versions(ctx, "f1", ""); err != nil || len(h) != 2 || !h[0].IsCurrent || h[1].Version != 2 || h[1].SizeBytes != 7 || h[1].AuthorSub != "ana" || sig != "v1" {
		t.Fatalf("Versions = %+v, %q, %v", h, sig, err)
	}
	if cuerpo, err := d.DownloadVersion(ctx, "f1", 2); err != nil {
		t.Fatal(err)
	} else if datos, _ := io.ReadAll(cuerpo); string(datos) != "la de antes" || cuerpo.Close() != nil {
		t.Fatalf("DownloadVersion = %q", datos)
	}
	if err := d.RestoreVersion(ctx, "f1", 2); err != nil {
		t.Fatal(err)
	}
	if r, sig, err := d.Recent(ctx, ""); err != nil || len(r) != 1 || r[0].Path != "/Actas/acta.md" || sig != "mas" {
		t.Fatalf("Recent = %+v, %q, %v", r, sig, err)
	}
	// La segunda página se pide con el cursor de la primera, y la última no trae otro.
	if r, sig, err := d.Recent(ctx, "mas"); err != nil || len(r) != 0 || sig != "" {
		t.Fatalf("Recent(mas) = %+v, %q, %v", r, sig, err)
	}
	// Los destacados llegan en el orden del servicio, carpeta y fichero MEZCLADOS.
	if r, err := d.Favorites(ctx); err != nil || len(r) != 2 || r[0].ID != "c7" || !r[0].IsDir || r[1].IsDir {
		t.Fatalf("Favorites = %+v, %v", r, err)
	}
	if err := errors.Join(d.SetFavorite(ctx, DriveFile, "f7", true), d.SetFavorite(ctx, DriveFolder, "c7", false)); err != nil {
		t.Fatal(err)
	}
	if c, err := d.Folder(ctx, "c7"); err != nil || c.ParentID == nil || *c.ParentID != "raiz" {
		t.Fatalf("Folder = %+v, %v", c, err)
	}
	if l, err := d.BatchMove(ctx, []string{"f1", "f2"}, []string{"c1"}, "c9"); err != nil || l.Total() != 3 {
		t.Fatalf("BatchMove = %+v, %v", l, err)
	}
	if l, err := d.BatchTrash(ctx, []string{"f1"}, nil); err != nil || l.Ficheros != 1 || l.Carpetas != 0 {
		t.Fatalf("BatchTrash = %+v, %v", l, err)
	}
	if cuerpo, contenido, err := d.Zip(ctx, []string{"f1", "f2"}, []string{"c1"}, "Selección.zip"); err != nil {
		t.Fatal(err)
	} else if datos, _ := io.ReadAll(cuerpo); string(datos) != "PK\x03\x04" || cuerpo.Close() != nil || contenido != 1536 {
		t.Fatalf("Zip = %q, %d", datos, contenido)
	}
	r, err := d.Search(ctx, "iva")
	if err != nil || len(r.Files) != 1 || r.Files[0].Path != "/Contabilidad/iva.pdf" || len(r.Folders) != 1 || r.Folders[0].Path != "/IVA" {
		t.Fatalf("Search = %+v, %v", r, err)
	}
	quiero := []string{
		`PATCH /api/v1/files/f1 {"name":"b.pdf"}`, `PATCH /api/v1/folders/c1 {"name":"Nueva"}`,
		`PATCH /api/v1/files/f1 {"new_folder_id":"c1"}`, `PATCH /api/v1/folders/c1 {"new_parent_id":"c2"}`,
		"DELETE /api/v1/files/f1", "POST /api/v1/folders/c1/restore", "DELETE /api/v1/files/f1/purge",
		`PATCH /api/v1/files/f1 {"name":"repetido"}`, "POST /api/v1/files/f1/copy", "POST /api/v1/folders/c1/copy", "GET /api/v1/trash", "GET /api/v1/quota",
		"GET /api/v1/folders/c1/shares",
		`POST /api/v1/shares {"expires_at":"2026-10-01T21:59:59Z","folder_id":"c1","kind":"link","password":"clave","permission":"write"}`,
		`POST /api/v1/shares {"file_id":"f1","kind":"link","permission":"read"}`,
		`POST /api/v1/shares {"file_id":"f1","kind":"user","permission":"write","subject":"sub-ana"}`,
		`POST /api/v1/shares {"folder_id":"c1","kind":"group","permission":"read","subject":"Ventas"}`,
		"DELETE /api/v1/shares/s2",
		"GET /api/v1/files/f1/versions", "GET /api/v1/files/f1/versions/2/content", "POST /api/v1/files/f1/versions/2/restore",
		"GET /api/v1/recent", "GET /api/v1/recent", "GET /api/v1/favorites", `POST /api/v1/favorites {"favorite":true,"file_id":"f7"}`,
		`POST /api/v1/favorites {"favorite":false,"folder_id":"c7"}`, "GET /api/v1/folders/c7",
		`POST /api/v1/batch/move {"file_ids":["f1","f2"],"folder_ids":["c1"],"target_folder_id":"c9"}`,
		`POST /api/v1/batch/trash {"file_ids":["f1"],"folder_ids":null}`, "GET /api/v1/batch/zip",
		"QUERY /api/v1/search q=iva",
	}
	if !slices.Equal(pedidas, quiero) {
		t.Fatalf("al servicio ha llegado\n%s\ny se esperaba\n%s", strings.Join(pedidas, "\n"), strings.Join(quiero, "\n"))
	}
}

// TestDriveCompartidoConmigo afirma las CUATRO rutas de «Compartido conmigo» y, sobre
// todo, que el subcamino viaja por query y no dentro del path: con barras en el path el
// enrutador del servicio lo partiría y la carpeta de dentro no se encontraría nunca.
func TestDriveCompartidoConmigo(t *testing.T) {
	var subida struct{ nombre, subcamino, cuerpo string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := r.URL.Query().Get("subpath")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/shared-with-me":
			_, _ = io.WriteString(w, `{"items":[
				{"id":"r1","name":"Legal","is_directory":true,"path":"/Ana/Legal","updated_at":"2026-09-20T10:00:00Z","permission":"edit","shared_by":"ana@sytue.eu"},
				{"id":"r2","name":"acta.pdf","is_directory":false,"path":"/Ana/acta.pdf","size_bytes":12,"content_type":"application/pdf","updated_at":"2026-09-19T10:00:00Z","permission":"view","shared_by":"ana@sytue.eu"}]}`)
		case "GET /api/v1/shared/r1/list":
			if sub != "" && sub != "2026" {
				t.Errorf("subcamino: %q", sub)
			}
			if sub == "" {
				_, _ = io.WriteString(w, `{"permission":"edit","is_dir":true,"more":false,"items":[
					{"id":"c1","name":"2026","is_directory":true,"path":"2026","updated_at":"2026-09-20T10:00:00Z"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"permission":"edit","is_dir":true,"more":false,"items":[
				{"id":"f1","name":"acta.pdf","is_directory":false,"path":"2026/acta.pdf","size_bytes":12,"content_type":"application/pdf","updated_at":"2026-09-20T10:00:00Z"}]}`)
		case "GET /api/v1/shared/r1/content":
			if sub != "2026/acta.pdf" {
				t.Errorf("subcamino de contenido: %q", sub)
			}
			_, _ = io.WriteString(w, "PDF")
		case "POST /api/v1/shared/r1/files":
			cuerpo, _ := io.ReadAll(r.Body)
			subida.nombre, subida.subcamino, subida.cuerpo = r.URL.Query().Get("name"), sub, string(cuerpo)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"f2","name":"nuevo.txt","size_bytes":3}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d, ctx := NewDrive(srv.URL, nil, nil), context.Background()

	raices, err := d.SharedWithMe(ctx)
	if err != nil || len(raices) != 2 {
		t.Fatalf("raíces: %+v, %v", raices, err)
	}
	if !raices[0].IsDir || raices[0].Permission != "edit" || raices[0].SharedBy != "ana@sytue.eu" || raices[0].Name != "Legal" {
		t.Errorf("raíz carpeta: %+v", raices[0])
	}
	if raices[1].IsDir || raices[1].SizeBytes != 12 || raices[1].Permission != "view" {
		t.Errorf("raíz fichero: %+v", raices[1])
	}

	lista, err := d.SharedList(ctx, "r1", "")
	if err != nil || !lista.IsDir || !lista.Editable() || len(lista.Items) != 1 || lista.Items[0].Path != "2026" {
		t.Fatalf("raíz: %+v, %v", lista, err)
	}
	dentro, err := d.SharedList(ctx, "r1", "2026")
	if err != nil || len(dentro.Items) != 1 || dentro.Items[0].Path != "2026/acta.pdf" || dentro.Items[0].IsDir {
		t.Fatalf("dentro: %+v, %v", dentro, err)
	}

	cuerpo, err := d.SharedContent(ctx, "r1", "2026/acta.pdf")
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := io.ReadAll(cuerpo)
	_ = cuerpo.Close()
	if string(bytes) != "PDF" {
		t.Errorf("contenido: %q", bytes)
	}

	if _, err := d.SharedUpload(ctx, "r1", "2026", "nuevo.txt", "", strings.NewReader("abc"), 3); err != nil {
		t.Fatal(err)
	}
	if subida.nombre != "nuevo.txt" || subida.subcamino != "2026" || subida.cuerpo != "abc" {
		t.Errorf("subida: %+v", subida)
	}

	// Sin permiso de escritura el listado no miente: solo "edit" es editable.
	if (CompartidoListado{Permission: "view"}).Editable() {
		t.Error("«view» no es editable")
	}
}

func TestLosCamposDeDriveSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/drive/internal/domain/domain.go")
	for _, campo := range []string{
		`json:"size_bytes"`, `json:"content_type"`, `json:"folder_id"`, `json:"is_root"`, `json:"parent_id,omitempty"`, `json:"deleted_at,omitempty"`,
		`json:"is_current"`, `json:"label,omitempty"`, `json:"block_external_share"`, `json:"has_password,omitempty"`, `json:"expires_at,omitempty"`, `ShareLink  ShareKind = "link"`,
		`PermRead  Permission = "read"`, `PermWrite Permission = "write"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/drive ya no declara %s: revisar internal/api/drive.go", campo)
		}
	}
	manejadores := webref.Read(t, "services/drive/internal/http/handlers.go")
	for _, literal := range []string{
		`json:"next_cursor,omitempty"`, `json:"folders"`, `json:"files"`, "?name=",
		`json:"name,omitempty"`, `json:"new_folder_id,omitempty"`, `json:"new_parent_id,omitempty"`,
		`json:"retention_seconds"`, `json:"is_directory"`, `json:"path"`, `"files": files`,
		`"limit_bytes"`, `"used_bytes"`, `"external_storage"`,
		`"versions": versions`, `"items": items`, `json:"file_id,omitempty"`, `json:"favorite"`,
		`json:"kind"`, `json:"permission,omitempty"`, `json:"password,omitempty"`, `"shares": shares`, `"share": sh`, `resp["url"]`,
		`json:"file_ids"`, `json:"folder_ids"`, `json:"target_folder_id,omitempty"`, `"moved_files"`, `"moved_folders"`, `"trashed_files"`, `"trashed_folders"`,
		// «Compartido conmigo»: la forma de sharedNodeView, el permiso traducido a
		// view/edit y el subcamino por query (no en el path: lleva barras).
		`json:"shared_by,omitempty"`, `Query().Get("subpath")`, `"permission": permissionLabel(perm)`, `"is_dir":     node.IsDir`,
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/drive/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/drive/internal/http/server.go")
	for _, ruta := range []string{
		`api.Patch("/folders/{id}"`, `api.Patch("/files/{id}"`, `api.Delete("/folders/{id}"`, `api.Delete("/files/{id}"`,
		`api.Post("/files/{id}/copy"`, `api.Get("/files/{id}/thumbnail"`, `api.Get("/trash"`, `api.Post("/files/{id}/restore"`, `api.Post("/folders/{id}/restore"`,
		`api.Get("/files/{id}/versions"`, `api.Get("/files/{id}/versions/{version}/content"`, `api.Post("/files/{id}/versions/{version}/restore"`,
		`api.Get("/recent"`, `api.Get("/favorites"`, `api.Post("/favorites"`, `api.Get("/folders/{id}"`,
		`api.Post("/shares"`, `api.Get("/files/{id}/shares"`, `api.Get("/folders/{id}/shares"`, `api.Delete("/shares/{id}"`,
		`api.Delete("/files/{id}/purge"`, `api.Delete("/folders/{id}/purge"`, `GetYQuery(api, "/search"`, `api.Get("/quota"`,
		`api.Post("/batch/move"`, `api.Post("/batch/trash"`, `api.Get("/batch/zip"`,
		`api.Get("/shared-with-me"`, `api.Get("/shared/{id}/list"`, `api.Get("/shared/{id}/content"`, `api.Post("/shared/{id}/files"`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/drive/internal/http/server.go ya no monta %s", ruta)
		}
	}
}

// TestDirectorioResuelveEnLosDosSentidos: de un correo al `sub` para compartir, y de un
// `sub` guardado al correo para pintar con quién está compartido. Un 404 es «nadie de tu
// organización usa esa dirección» y tiene que llegar como tal.
func TestDirectorioResuelveEnLosDosSentidos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/directory:resolve" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		switch string(cuerpo) {
		case `{"email":"ana@acme.test"}`, `{"user_id":"sub-ana"}`:
			_, _ = io.WriteString(w, `{"user_id":"sub-ana","email":"ana@acme.test","name":"Ana"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d := NewDirectorio(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)
	ctx := context.Background()
	if p, err := d.PorCorreo(ctx, "ana@acme.test"); err != nil || p.ID != "sub-ana" {
		t.Fatalf("PorCorreo = %+v, %v", p, err)
	}
	if p, err := d.PorID(ctx, "sub-ana"); err != nil || p.Email != "ana@acme.test" || p.Nombre != "Ana" {
		t.Fatalf("PorID = %+v, %v", p, err)
	}
	var e *Error
	if _, err := d.PorCorreo(ctx, "nadie@fuera.test"); !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatalf("una dirección de fuera tiene que ser 404: %v", err)
	}
}
