// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Unused methods deliberately have no implementation. These tests check the API
// boundary; real repository dispatch is also exercised against SDK/Synapse.
type privateDriveDataFixture struct {
	DriveData
	scope    storage.Scope
	root     func(context.Context) (Folder, error)
	download func(context.Context, string) (io.ReadCloser, error)
}

func (f privateDriveDataFixture) PrivateStorageScope() storage.Scope       { return f.scope }
func (f privateDriveDataFixture) Root(ctx context.Context) (Folder, error) { return f.root(ctx) }
func (f privateDriveDataFixture) Download(ctx context.Context, id string) (io.ReadCloser, error) {
	return f.download(ctx, id)
}

func TestPrivateDriveFacadePreservesPrivateFailuresAndKeepsOriginalSeparate(t *testing.T) {
	ctx := t.Context()
	calls := 0
	original := NewDrive("https://drive.example", nil, &http.Client{Transport: intentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"legacy"}`)), Header: http.Header{}, Request: r}, nil
	})})
	unavailable := errors.New("private keys unavailable")
	data := privateDriveDataFixture{scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "drive"}, root: func(context.Context) (Folder, error) { return Folder{}, unavailable }, download: func(_ context.Context, id string) (io.ReadCloser, error) {
		if id != "file" {
			t.Fatal("changed file ID")
		}
		return io.NopCloser(strings.NewReader("private content")), nil
	}}
	private, err := original.WithPrivateData(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := private.Root(ctx); !errors.Is(err, unavailable) || calls != 0 {
		t.Fatal("private failure fell back to legacy", err, calls)
	}
	body, err := private.Download(ctx, "file")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(raw) != "private content" || calls != 0 {
		t.Fatal("private bytes not dispatched", err, calls)
	}
	if got, err := original.Root(ctx); err != nil || got.ID != "legacy" || calls != 1 {
		t.Fatal("facade changed original", got, err, calls)
	}
	if _, err := private.WithPrivateData(data); !errors.Is(err, ErrPrivateDriveOperation) {
		t.Fatal("nested facade accepted", err)
	}
	for _, scope := range []storage.Scope{{}, {Tenant: "org", Subject: "owner", Product: "notes"}} {
		data.scope = scope
		if _, err := original.WithPrivateData(data); !errors.Is(err, ErrPrivateDriveOperation) {
			t.Fatal("wrong private scope accepted", err)
		}
	}
	if _, err := original.WithPrivateData(nil); !errors.Is(err, ErrPrivateDriveOperation) {
		t.Fatal("nil private data accepted", err)
	}
}

func TestPrivateDriveFacadeNeverUsesLegacyHTTPForUnhandledOperations(t *testing.T) {
	calls := 0
	original := NewDrive("https://drive.example", nil, &http.Client{Transport: intentTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected network") })})
	private, err := original.WithPrivateData(privateDriveDataFixture{scope: storage.Scope{Tenant: "org", Subject: "owner", Product: "drive"}})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]func() error{
		"purge":       func() error { return private.Purge(t.Context(), DriveFile, "file") },
		"folder-copy": func() error { _, e := private.CopyFolder(t.Context(), "folder"); return e },
		"share": func() error {
			_, e := private.ShareWith(t.Context(), DriveFile, "file", "user", "other", false)
			return e
		},
		"shared-list": func() error { _, e := private.SharedList(t.Context(), "root", ""); return e },
		"shared-upload": func() error {
			_, e := private.SharedUpload(t.Context(), "root", "", "private", "text/plain", strings.NewReader("private"), 7)
			return e
		},
		"team-upload": func() error {
			_, e := private.UploadTeamDrive(t.Context(), "team", "folder", "private", "text/plain", strings.NewReader("private"), 7)
			return e
		},
		"quota":           func() error { _, e := private.Quota(t.Context()); return e },
		"account-control": func() error { return private.SaveExternalStorage(t.Context(), ConfigAlmacen{}) },
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			if err := action(); !errors.Is(err, ErrPrivateDriveOperation) || calls != 0 {
				t.Fatal("legacy operation reached transport", err, calls)
			}
		})
	}
}
