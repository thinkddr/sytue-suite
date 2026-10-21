// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func TestDriveClientResolveNoBearer(t *testing.T) {
	c := NewDriveClient("http://drive.internal")
	if _, err := c.Resolve(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("sin bearer debe devolver forbidden, got %v", err)
	}
}

func TestDriveClientResolveOK(t *testing.T) {
	fid := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("propaga el bearer, got %q", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v1/files/"+fid.String()) {
			t.Errorf("URL inesperada: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + fid.String() + `","name":"doc.pdf","content_type":"application/pdf","size_bytes":42}`))
	}))
	defer srv.Close()

	c := NewDriveClient(srv.URL + "/")
	ctx := WithBearer(context.Background(), "tok")
	att, err := c.Resolve(ctx, uuid.Nil, fid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if att.FileID != fid || att.Name != "doc.pdf" || att.ContentType != "application/pdf" || att.SizeBytes != 42 {
		t.Fatalf("attachment inesperado: %+v", att)
	}
}

// Con canal, Resolve va por la ruta de ÁMBITO DE CANAL: es la única que ve un
// adjunto subido al canal y la que exige membresía (verja de la Ola 3A).
func TestDriveClientResolveChannelScope(t *testing.T) {
	cid, fid := uuid.New(), uuid.New()
	want := "/api/v1/channels/" + cid.String() + "/files/" + fid.String()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != want {
			t.Errorf("URL = %s, quiero %s", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + fid.String() + `","name":"a.png","content_type":"image/png","size_bytes":7}`))
	}))
	defer srv.Close()

	att, err := NewDriveClient(srv.URL).Resolve(WithBearer(context.Background(), "tok"), cid, fid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if att.FileID != fid || att.Name != "a.png" {
		t.Fatalf("attachment inesperado: %+v", att)
	}
}

func TestDriveClientResolveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := NewDriveClient(srv.URL)
	_, err := c.Resolve(WithBearer(context.Background(), "tok"), uuid.Nil, uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("404 debe mapear a ErrNotFound, got %v", err)
	}
}

func TestDriveClientResolveForbiddenIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewDriveClient(srv.URL)
	_, err := c.Resolve(WithBearer(context.Background(), "tok"), uuid.Nil, uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("403 debe mapear a ErrNotFound, got %v", err)
	}
}

func TestDriveClientResolveServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewDriveClient(srv.URL)
	if _, err := c.Resolve(WithBearer(context.Background(), "tok"), uuid.Nil, uuid.New()); err == nil {
		t.Fatal("500 debe devolver error")
	}
}

func TestDriveClientResolveBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()
	c := NewDriveClient(srv.URL)
	if _, err := c.Resolve(WithBearer(context.Background(), "tok"), uuid.Nil, uuid.New()); err == nil {
		t.Fatal("JSON inválido debe devolver error")
	}
}
