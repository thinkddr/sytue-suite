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

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type privatePhotosFixture struct{ PhotosData }

func (privatePhotosFixture) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
}

type privatePhotosMediaFixture struct {
	privatePhotosFixture
	open func(context.Context, string, string, string) (io.ReadCloser, error)
}

func (p privatePhotosMediaFixture) VariantDe(ctx context.Context, id, variant, fingerprint string) (io.ReadCloser, error) {
	return p.open(ctx, id, variant, fingerprint)
}

func TestPrivatePhotosMediaNeverFallsBackToLegacyHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "plaintext legacy media")
	}))
	defer server.Close()
	original := NewPhotos(server.URL, nil, nil)
	failure := errors.New("encrypted media unavailable")
	media := privatePhotosMediaFixture{open: func(ctx context.Context, id, variant, fingerprint string) (io.ReadCloser, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if id != "photo" || variant != "miniatura" || fingerprint != "retained" {
			return nil, ErrPrivatePhotosOperation
		}
		return io.NopCloser(strings.NewReader("authenticated private media")), nil
	}}
	private, err := original.WithPrivateData(media)
	if err != nil {
		t.Fatal(err)
	}
	body, err := private.VariantDe(t.Context(), "photo", "miniatura", "retained")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "authenticated private media" {
		t.Fatal("private variant", err)
	}
	media.open = func(context.Context, string, string, string) (io.ReadCloser, error) { return nil, failure }
	private, err = original.WithPrivateData(media)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Variant(t.Context(), "photo", "miniatura"); !errors.Is(err, failure) {
		t.Fatal("provider failure did not remain private", err)
	}
	unsupported, err := original.WithPrivateData(privatePhotosFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unsupported.Variant(t.Context(), "photo", "miniatura"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("missing private operation used plaintext", err)
	}
	if _, err := unsupported.Export(t.Context(), "photo", "png"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported export used plaintext", err)
	}
	if _, err := unsupported.Albums(t.Context()); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported album list used plaintext", err)
	}
	if _, err := unsupported.Album(t.Context(), "album"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported album read used plaintext", err)
	}
	if _, err := unsupported.AlbumPhotos(t.Context(), "album"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported album photos used plaintext", err)
	}
	if _, err := unsupported.CreateAlbum(t.Context(), "album"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported album creation used plaintext", err)
	}
	if err := unsupported.RenameAlbum(t.Context(), "album", "title"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported album rename used plaintext", err)
	}
	if err := unsupported.DeleteAlbum(t.Context(), "album"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported album deletion used plaintext", err)
	}
	if _, err := unsupported.AddToAlbum(t.Context(), "album", []string{"photo"}); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported membership write used plaintext", err)
	}
	if err := unsupported.RemoveFromAlbum(t.Context(), "album", "photo"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported membership removal used plaintext", err)
	}
	if requests.Load() != 0 {
		t.Fatal("private media reached the legacy server", requests.Load())
	}
}

type privatePhotosExportFixture struct {
	privatePhotosFixture
	export func(context.Context, string, string) (io.ReadCloser, error)
}

func (p privatePhotosExportFixture) Export(ctx context.Context, id, format string) (io.ReadCloser, error) {
	return p.export(ctx, id, format)
}

type privatePhotosIngestFixture struct {
	privatePhotosFixture
	ingest func(context.Context, string) (Photo, error)
}

func (p privatePhotosIngestFixture) Indexar(ctx context.Context, file string) (Photo, error) {
	return p.ingest(ctx, file)
}

func TestPrivatePhotosIndexarUsesDeviceAndNeverLegacyHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{"id":"legacy"}`)
	}))
	defer server.Close()
	retained := Photo{ID: "photo", Name: "playa.png", ContentType: "image/png", SizeBytes: 42, Width: 6, Height: 4, DriveFileID: "drive-file"}
	fixture := privatePhotosIngestFixture{ingest: func(ctx context.Context, file string) (Photo, error) {
		if ctx.Err() != nil {
			return Photo{}, ctx.Err()
		}
		if file != "drive-file" {
			return Photo{}, ErrPrivatePhotosOperation
		}
		return retained, nil
	}}
	original := NewPhotos(server.URL, nil, server.Client())
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	photo, err := private.Indexar(t.Context(), "drive-file")
	if err != nil || photo.ID != retained.ID || photo.Name != retained.Name || photo.DriveFileID != retained.DriveFileID {
		t.Fatal("private indexar did not use its device", err, photo)
	}
	failure := errors.New("private source unavailable")
	fixture.ingest = func(context.Context, string) (Photo, error) { return Photo{}, failure }
	private, err = original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Indexar(t.Context(), "drive-file"); !errors.Is(err, failure) {
		t.Fatal("failed private indexar fell back", err)
	}
	unsupported, err := original.WithPrivateData(privatePhotosFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unsupported.Indexar(t.Context(), "drive-file"); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("unsupported indexar used plaintext", err)
	}
	if requests.Load() != 0 {
		t.Fatal("private indexar reached legacy HTTP", requests.Load())
	}
}

func TestPrivatePhotosExportUsesDeviceAndNeverLegacyHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "legacy clear export")
	}))
	defer server.Close()
	fixture := privatePhotosExportFixture{export: func(ctx context.Context, id, format string) (io.ReadCloser, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if id != "photo" || format != "png" {
			return nil, ErrPrivatePhotosOperation
		}
		return io.NopCloser(strings.NewReader("authenticated device export")), nil
	}}
	original := NewPhotos(server.URL, nil, server.Client())
	private, err := original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := private.Export(t.Context(), "photo", "png")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil || string(raw) != "authenticated device export" {
		t.Fatal("private export did not use its device", err)
	}
	failure := errors.New("private source unavailable")
	fixture.export = func(context.Context, string, string) (io.ReadCloser, error) { return nil, failure }
	private, err = original.WithPrivateData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Export(t.Context(), "photo", "png"); !errors.Is(err, failure) {
		t.Fatal("failed private export fell back", err)
	}
	if requests.Load() != 0 {
		t.Fatal("private export reached legacy HTTP", requests.Load())
	}
}
