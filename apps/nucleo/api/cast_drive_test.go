// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCastSubeDesdeDrive: la previsualización va por GET con el id en la consulta, la
// copia por POST con lo que se eligió (y sin los campos vacíos, que son los de por
// omisión del servicio), y un 507 dice cuánto queda.
func TestCastSubeDesdeDrive(t *testing.T) {
	const id = "0b6f7a2e-3c1d-4e5f-8a9b-0c1d2e3f4a5b"
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.RequestURI()+" "+string(cuerpo))
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("file_id") == id:
			_, _ = io.WriteString(w, `{"file_id":"`+id+`","name":"Demo.mp4","size_bytes":1048576,"content_type":"video/mp4","title":"Demo"}`)
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusInsufficientStorage)
			_, _ = io.WriteString(w, `{"error":{"code":"quota_exceeded","message":"sin sitio","details":{"available_bytes":512}}}`)
		default:
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, videoCast)
		}
	}))
	defer srv.Close()
	c := NewCast(srv.URL, nil, nil)
	ctx := context.Background()

	f, err := c.DriveFile(ctx, id)
	if err != nil || f.Name != "Demo.mp4" || f.SizeBytes != 1<<20 || f.ContentType != "video/mp4" || f.Title != "Demo" {
		t.Fatalf("DriveFile = %+v, %v", f, err)
	}
	v, err := c.FromDrive(ctx, CastFromDrive{FileID: id, Title: "Demo", Visibility: CastOrg})
	if err != nil || v.ID != "v1" {
		t.Fatalf("FromDrive = %+v, %v", v, err)
	}
	if _, err := c.DriveFile(ctx, "otro"); err == nil {
		t.Fatal("un 507 no dio error")
	} else if quedan, ok := CastQuotaAvailable(err); !ok || quedan != 512 {
		t.Errorf("el 507 dice %d, %v", quedan, ok)
	}
	quiero := []string{
		"GET /api/v1/videos:from-drive?file_id=" + id + " ",
		`POST /api/v1/videos:from-drive {"file_id":"` + id + `","title":"Demo","visibility":"org"}`,
	}
	for i, q := range quiero {
		if vistas[i] != q {
			t.Errorf("petición %d:\n  hay    %q\n  quiero %q", i, vistas[i], q)
		}
	}
}
