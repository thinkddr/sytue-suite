// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestNubeConectarVuelveALaAppYSoloAlProveedor(t *testing.T) {
	for _, tc := range []struct {
		provider, target string
		valido           bool
	}{
		{"google-drive", "https://accounts.google.com/o/oauth2/v2/auth?state=nonce", true},
		{"onedrive", "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=nonce", true},
		{"dropbox", "https://www.dropbox.com/oauth2/authorize?state=nonce", true},
		{"dropbox", "https://accounts.google.com/o/oauth2/v2/auth?state=nonce", false},
		{"google-drive", "http://accounts.google.com/?state=nonce", false},
		{"google-drive", "https://accounts.google.com/", false},
		{"icloud", "https://www.icloud.com/?state=nonce", false},
	} {
		t.Run(tc.provider+tc.target, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in map[string]string
				_ = json.NewDecoder(r.Body).Decode(&in)
				if r.URL.Path != "/api/v1/personal-storage/connect" || in["return_to"] != "native-drive" || in["provider"] != tc.provider {
					t.Errorf("petición: %s %+v", r.URL.Path, in)
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"instance_id": "mia", "authorize_url": tc.target})
			}))
			defer srv.Close()
			gw := NewIntegraciones(srv.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
			got, err := gw.NubeConectar(context.Background(), tc.provider, "")
			if tc.valido {
				if err != nil || got.State != "nonce" || got.Section != "native-personal-storage" || NativeConsentProduct(got.Section) != "drive" {
					t.Fatalf("%+v %v", got, err)
				}
			} else if !errors.Is(err, ErrOAuthState) {
				t.Fatalf("consentimiento inseguro aceptado: %+v %v", got, err)
			}
		})
	}
}

func TestNubeRutasYLibres(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/personal-storage/accounts":
			_, _ = w.Write([]byte(`{"items":[{"instance_id":"g","provider":"google-drive","connected":true},{"instance_id":"o","provider":"onedrive","connected":false}],
				"providers":[{"provider":"google-drive","available":true},{"provider":"onedrive","available":true},{"provider":"dropbox","available":false,"disabled_by_org":true}]}`))
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v1/personal-storage/a%2Fb/items":
			if r.URL.Query().Get("folder") != "/x y" || r.URL.Query().Get("cursor") != "c&d" {
				t.Errorf("consulta: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"1","name":"f","folder":true}],"next_cursor":"n"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v1/personal-storage/a%2Fb/content":
			b, _ := io.ReadAll(r.Body)
			if r.URL.Query().Get("name") != "n.txt" || string(b) != "hola" || r.ContentLength != 4 {
				t.Errorf("subida: %s %q", r.URL.RawQuery, b)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"2","name":"n.txt","size":4}`))
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api/v1/personal-storage/a%2Fb":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("ruta inesperada %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	gw := NewIntegraciones(srv.URL, &sesionFalsa{tokens: []string{"bearer"}}, nil)
	ctx := context.Background()
	n, err := gw.NubesPersonales(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c := n.Conectadas(); len(c) != 1 || c[0].InstanceID != "g" {
		t.Fatalf("conectadas: %+v", c)
	}
	// Google ya está; OneDrive a medias se ofrece otra vez; Dropbox, apagado por la organización.
	if l := n.Libres(); !reflect.DeepEqual(l, []string{"onedrive"}) {
		t.Fatalf("libres: %v", l)
	}
	p, err := gw.NubeElementos(ctx, "a/b", "/x y", "c&d")
	if err != nil || len(p.Items) != 1 || !p.Items[0].Folder || p.NextCursor != "n" {
		t.Fatalf("%+v %v", p, err)
	}
	if e, err := gw.NubeSubir(ctx, "a/b", "", "n.txt", strings.NewReader("hola"), 4); err != nil || e.ID != "2" {
		t.Fatalf("%+v %v", e, err)
	}
	if err := gw.NubeDesconectar(ctx, "a/b"); err != nil {
		t.Fatal(err)
	}
}
