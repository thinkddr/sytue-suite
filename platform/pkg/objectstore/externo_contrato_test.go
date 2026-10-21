// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type externalIndexFixture map[string]string

func (m externalIndexFixture) PutExternalObject(_ context.Context, tenant, key, id string) error {
	m[tenant+":"+key] = id
	return nil
}

func (m externalIndexFixture) GetExternalObject(_ context.Context, tenant, key string) (string, error) {
	if id, ok := m[tenant+":"+key]; ok {
		return id, nil
	}
	return "", ErrNotFound
}

func (m externalIndexFixture) DeleteExternalObject(_ context.Context, tenant, key string) error {
	delete(m, tenant+":"+key)
	return nil
}

func TestGoogleDriveConservaElIndicePorTenant(t *testing.T) {
	index := externalIndexFixture{"acme:ext/blob": "provider-acme", "beta:ext/blob": "provider-beta"}
	g := NewGoogleDrive(func(context.Context) (string, error) { return "consent", nil }, index)
	calls := 0
	g.http = &http.Client{Transport: dropboxTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer consent" || r.Method != http.MethodGet || r.URL.Query().Get("alt") != "media" {
			t.Fatalf("petición incorrecta: %s %s", r.Method, r.URL)
		}
		return dropboxResponse(r, http.StatusOK, r.URL.Path), nil
	})}
	for _, tenant := range []string{"acme", "beta"} {
		rc, err := g.Get(context.Background(), tenancy.Tenant{Slug: tenant}, "ext/blob")
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil || string(b) != "/drive/v3/files/provider-"+tenant {
			t.Fatalf("se perdió el índice del tenant %s: %q %v", tenant, b, err)
		}
	}
	if _, err := g.Get(context.Background(), tenancy.Tenant{Slug: "otra"}, "ext/blob"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ausente: %v", err)
	}
	if calls != 2 {
		t.Fatalf("una clave ausente no debe pedir otro objeto: %d", calls)
	}
}

func TestOneDriveConservaRutaYBytesSinModificar(t *testing.T) {
	o := NewOneDrive(func(context.Context) (string, error) { return "consent", nil })
	o.http = &http.Client{Transport: dropboxTransport(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "\x00cipher\xff" || r.ContentLength != 8 || r.Header.Get("Authorization") != "Bearer consent" || r.Header.Get("Content-Type") != "application/octet-stream" || r.Method != http.MethodPut || r.URL.EscapedPath() != "/v1.0/me/drive/root:/Sytue/tenants/acme/ext/con%20espacio:/content" {
			t.Fatalf("subida alterada: %s %s length=%d bytes=%q err=%v", r.Method, r.URL, r.ContentLength, body, err)
		}
		return dropboxResponse(r, http.StatusCreated, `{}`), nil
	})}
	if err := o.Put(context.Background(), tenancy.Tenant{Slug: "acme"}, "ext/con espacio", strings.NewReader("\x00cipher\xff"), 8, PutOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestProveedoresSinFirmaConservanElContrato(t *testing.T) {
	tok := func(context.Context) (string, error) { t.Fatal("firmar no debe pedir token"); return "", nil }
	w, err := NewWebDAV("https://nas.example/", "Sytue", "user", "password")
	if err != nil {
		t.Fatal(err)
	}
	backends := []BlobStore{NewGoogleDrive(tok, externalIndexFixture{}), NewOneDrive(tok), NewDropbox(tok), w}
	for _, b := range backends {
		if _, err := b.PresignGet(context.Background(), tenancy.Tenant{Slug: "acme"}, "key", time.Minute); !errors.Is(err, ErrPresignUnsupported) {
			t.Fatalf("%T Get: %v", b, err)
		}
		if _, err := b.PresignPut(context.Background(), tenancy.Tenant{Slug: "acme"}, "key", time.Minute); !errors.Is(err, ErrPresignUnsupported) {
			t.Fatalf("%T Put: %v", b, err)
		}
	}
}
