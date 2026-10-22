// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type dropboxTransport func(*http.Request) (*http.Response, error)

func (f dropboxTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func dropboxResponse(r *http.Request, status int, body string) *http.Response {
	w := httptest.NewRecorder()
	w.WriteHeader(status)
	_, _ = w.WriteString(body)
	res := w.Result()
	res.Request = r
	return res
}

func fakeDropbox(t *testing.T, handle dropboxTransport) *Dropbox {
	t.Helper()
	d := NewDropbox(func(context.Context) (string, error) { return "consent-token", nil })
	d.http = &http.Client{Transport: dropboxTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer consent-token" {
			t.Fatalf("el acceso debe usar POST y el token del vault: %s %v", r.Method, r.Header)
		}
		return handle(r)
	})}
	return d
}

func TestDropboxSubeSinRenombrarNiMezclarOrganizaciones(t *testing.T) {
	tenant := tenancy.Tenant{Slug: "acme"}
	d := fakeDropbox(t, func(r *http.Request) (*http.Response, error) {
		var args struct {
			Path       string `json:"path"`
			Mode       string `json:"mode"`
			Autorename bool   `json:"autorename"`
		}
		if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &args); err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(r.Body)
		if r.URL.String() != dropboxContent+"/files/upload" || args.Path != "/Sytue/tenants/acme/ext/files/x" ||
			args.Mode != "overwrite" || args.Autorename || string(body) != "bytes" || r.ContentLength != 5 {
			t.Fatalf("subida incorrecta: url=%s args=%+v length=%d body=%s", r.URL, args, r.ContentLength, body)
		}
		return dropboxResponse(r, http.StatusOK, `{}`), nil
	})
	if err := d.Put(context.Background(), tenant, "ext/files/x", strings.NewReader("bytes"), 5, PutOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestDropboxSubidaGrandeConservaCadaOffset(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 2*dropboxTrozo+3)
	calls := 0
	d := fakeDropbox(t, func(r *http.Request) (*http.Response, error) {
		var args struct {
			Cursor struct {
				ID     string `json:"session_id"`
				Offset int64  `json:"offset"`
			} `json:"cursor"`
			Commit struct {
				Path string `json:"path"`
			} `json:"commit"`
		}
		if err := json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &args); err != nil {
			t.Fatal(err)
		}
		offset := calls * dropboxTrozo
		length := min(dropboxTrozo, len(payload)-offset)
		body, _ := io.ReadAll(r.Body)
		if r.ContentLength != int64(length) || !bytes.Equal(body, payload[offset:offset+length]) {
			t.Fatalf("trozo %d perdido o duplicado: length=%d body=%d", calls, r.ContentLength, len(body))
		}
		routes := []string{"/2/files/upload_session/start", "/2/files/upload_session/append_v2", "/2/files/upload_session/finish"}
		if r.URL.Path != routes[calls] {
			t.Fatalf("trozo %d: ruta %s", calls, r.URL.Path)
		}
		if calls > 0 && (args.Cursor.ID != "session" || args.Cursor.Offset != int64(offset)) {
			t.Fatalf("cursor incorrecto: %+v", args.Cursor)
		}
		if calls == 2 && args.Commit.Path != "/Sytue/tenants/acme/ext/large" {
			t.Fatalf("se perdió la ruta al confirmar: %+v", args.Commit)
		}
		calls++
		return dropboxResponse(r, http.StatusOK, `{"session_id":"session"}`), nil
	})
	if err := d.Put(context.Background(), tenancy.Tenant{Slug: "acme"}, "ext/large", bytes.NewReader(payload), int64(len(payload)), PutOptions{}); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("subida incompleta: %d llamadas", calls)
	}
}

func TestDropboxDistingueAusenciaDeOtrosConflictos(t *testing.T) {
	tenant := tenancy.Tenant{Slug: "acme"}
	d := fakeDropbox(t, func(r *http.Request) (*http.Response, error) {
		return dropboxResponse(r, http.StatusConflict, `{"error_summary":"path/not_found/abc"}`), nil
	})
	if _, err := d.Get(context.Background(), tenant, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("descarga ausente: %v", err)
	}
	if exists, err := d.Existe(context.Background(), tenant, "missing"); exists || err != nil {
		t.Fatalf("Exists: %v %v", exists, err)
	}
	if err := d.Delete(context.Background(), tenant, "missing"); err != nil {
		t.Fatalf("borrar sigue siendo idempotente: %v", err)
	}
	d = fakeDropbox(t, func(r *http.Request) (*http.Response, error) {
		return dropboxResponse(r, http.StatusConflict, `{"error_summary":"path/locked/abc"}`), nil
	})
	if err := d.Delete(context.Background(), tenant, "locked"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("un conflicto real no se puede ocultar: %v", err)
	}
}

func TestDropboxLeeYCompruebaSinPedirElPerfil(t *testing.T) {
	d := fakeDropbox(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/2/files/download":
			return dropboxResponse(r, http.StatusOK, "content"), nil
		case "/2/files/get_metadata":
			return dropboxResponse(r, http.StatusOK, `{"size":7}`), nil
		case "/2/files/list_folder":
			return dropboxResponse(r, http.StatusOK, `{"entries":[]}`), nil
		default:
			t.Fatalf("ruta inesperada: %s", r.URL)
			return nil, errors.New("ruta inesperada")
		}
	})
	tenant := tenancy.Tenant{Slug: "acme"}
	if err := d.Probar(context.Background()); err != nil {
		t.Fatal(err)
	}
	n, err := d.Stat(context.Background(), tenant, "ext/x")
	if err != nil || n != 7 {
		t.Fatalf("metadatos: %d %v", n, err)
	}
	rc, err := d.Get(context.Background(), tenant, "ext/x")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if string(body) != "content" {
		t.Fatalf("contenido incorrecto: %q", body)
	}
}
