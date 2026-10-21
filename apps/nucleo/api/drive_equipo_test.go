// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// TestUnidadesDeEquipo afirma las rutas de las unidades de equipo, que van por las suyas
// y no por las de «Mi unidad»: el contenido está aislado por unidad.
func TestUnidadesDeEquipo(t *testing.T) {
	var mu sync.Mutex
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		mu.Lock()
		pedidas = append(pedidas, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+string(cuerpo)))
		mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/team-drives":
			_, _ = io.WriteString(w, `{"items":[{"id":"u1","name":"Ventas","role":"editor","archived_at":"2026-09-01T10:00:00Z"}]}`)
		case "POST /api/v1/team-drives", "PATCH /api/v1/team-drives/u1":
			_, _ = io.WriteString(w, `{"id":"u1","name":"Ventas","role":"manager"}`)
		case "GET /api/v1/team-drives/u1/members":
			_, _ = io.WriteString(w, `{"members":[{"subject":"ana","role":"manager"}]}`)
		case "GET /api/v1/team-drives/u1/children":
			_, _ = io.WriteString(w, `{"role":"viewer","can_write":false,"folders":[{"id":"c1","name":"Ofertas"}],"files":[{"id":"f1","name":"a.pdf","size_bytes":3}],"next_cursor":"p2"}`)
		case "POST /api/v1/team-drives/u1/folders", "POST /api/v1/team-drives/u1/files":
			_, _ = io.WriteString(w, `{"id":"x","name":"x"}`)
		case "GET /api/v1/team-drives/u1/files/f1/content":
			_, _ = io.WriteString(w, "PDF")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	d := NewDrive(srv.URL, &sesionFalsa{tokens: []string{"tok"}}, nil)

	if u, err := d.TeamDrives(ctx); err != nil || len(u) != 1 || !u[0].PuedeEscribir() || u[0].ArchivedAt == nil {
		t.Fatalf("TeamDrives = %+v, %v", u, err)
	}
	if u, err := d.CreateTeamDrive(ctx, "Ventas"); err != nil || u.Role != RolGestionar {
		t.Fatalf("CreateTeamDrive = %+v, %v", u, err)
	}
	if _, err := d.RenameTeamDrive(ctx, "u1", "Comercial"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ArchiveTeamDrive(ctx, "u1", false); err != nil {
		t.Fatal(err)
	}
	if m, err := d.TeamDriveMembers(ctx, "u1"); err != nil || len(m) != 1 || m[0].Role != RolGestionar {
		t.Fatalf("TeamDriveMembers = %+v, %v", m, err)
	}
	if err := d.PutTeamDriveMember(ctx, "u1", "luis", RolEditar); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveTeamDriveMember(ctx, "u1", "luis"); err != nil {
		t.Fatal(err)
	}
	if c, err := d.TeamDriveChildren(ctx, "u1", "c1", "p1"); err != nil || c.CanWrite || len(c.Folders) != 1 || c.NextCursor != "p2" {
		t.Fatalf("TeamDriveChildren = %+v, %v", c, err)
	}
	if _, err := d.CreateTeamDriveFolder(ctx, "u1", "", "Nueva"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UploadTeamDrive(ctx, "u1", "c1", "a b.txt", "text/plain", strings.NewReader("hola"), 4); err != nil {
		t.Fatal(err)
	}
	if r, err := d.DownloadTeamDrive(ctx, "u1", "f1"); err != nil {
		t.Fatal(err)
	} else if b, _ := io.ReadAll(r); string(b) != "PDF" || r.Close() != nil {
		t.Fatalf("DownloadTeamDrive = %q", b)
	}
	if err := d.DeleteTeamDriveItem(ctx, "u1", DriveFolder, "c1"); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/team-drives", `POST /api/v1/team-drives {"name":"Ventas"}`,
		`PATCH /api/v1/team-drives/u1 {"name":"Comercial"}`, `PATCH /api/v1/team-drives/u1 {"archived":false}`,
		"GET /api/v1/team-drives/u1/members", `PUT /api/v1/team-drives/u1/members {"role":"editor","subject":"luis"}`,
		"DELETE /api/v1/team-drives/u1/members/luis", "GET /api/v1/team-drives/u1/children?cursor=p1&folder=c1",
		`POST /api/v1/team-drives/u1/folders {"name":"Nueva"}`, "POST /api/v1/team-drives/u1/files?folder=c1&name=a+b.txt hola",
		"GET /api/v1/team-drives/u1/files/f1/content", "DELETE /api/v1/team-drives/u1/folders/c1",
	}
	if !slices.Equal(pedidas, quiero) {
		t.Fatalf("al servicio ha llegado\n%s\ny se esperaba\n%s", strings.Join(pedidas, "\n"), strings.Join(quiero, "\n"))
	}
}
