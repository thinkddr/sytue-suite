// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type directPhotosFixture struct {
	privatePhotosFixture
	importPhoto func(context.Context, string, string, string, io.Reader, int64) (Photo, error)
	resume      func(context.Context) ([]Photo, error)
}

func (p directPhotosFixture) ImportarFoto(ctx context.Context, job, name, mime string, body io.Reader, size int64) (Photo, error) {
	return p.importPhoto(ctx, job, name, mime, body, size)
}

func (p directPhotosFixture) ReanudarImportaciones(ctx context.Context) ([]Photo, error) {
	return p.resume(ctx)
}

func TestPrivatePhotosDirectImportNeverFallsBackToLegacyHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	original := NewPhotos(server.URL, nil, server.Client())
	wanted := Photo{ID: "logical-job", Name: "original.png"}
	fixture := directPhotosFixture{importPhoto: func(ctx context.Context, job, name, mime string, body io.Reader, size int64) (Photo, error) {
		raw, err := io.ReadAll(body)
		if err != nil {
			return Photo{}, err
		}
		if job != wanted.ID || name != wanted.Name || mime != "image/png" || size != 8 || string(raw) != "original" {
			t.Fatal("private facade changed its exact OS intent")
		}
		return wanted, ctx.Err()
	}, resume: func(ctx context.Context) ([]Photo, error) { return []Photo{wanted}, ctx.Err() }}
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !private.CanImportPrivatePhotos() {
		t.Fatal("private direct import capability missing")
	}
	got, err := private.ImportarFoto(t.Context(), wanted.ID, wanted.Name, "image/png", strings.NewReader("original"), 8)
	if err != nil || got.ID != wanted.ID {
		t.Fatal(got, err)
	}
	completed, err := private.ReanudarImportaciones(t.Context())
	if err != nil || len(completed) != 1 || completed[0].ID != wanted.ID {
		t.Fatal(completed, err)
	}
	unavailable, err := original.WithPrivateData(privatePhotosFixture{})
	if err != nil {
		t.Fatal(err)
	}
	for _, unsupported := range []*Photos{nil, original, unavailable} {
		if unsupported.CanImportPrivatePhotos() {
			t.Fatal("missing direct importer advertised as available")
		}
		if _, err := unsupported.ImportarFoto(t.Context(), wanted.ID, wanted.Name, "image/png", strings.NewReader("original"), 8); !errors.Is(err, ErrPrivatePhotosOperation) {
			t.Fatal("unsupported import did not fail closed", err)
		}
		if _, err := unsupported.ReanudarImportaciones(t.Context()); !errors.Is(err, ErrPrivatePhotosOperation) {
			t.Fatal("unsupported replay did not fail closed", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("private input reached legacy HTTP", requests.Load())
	}
}
