// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoomsFreeGifsUsesCatalogueAndProxy(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodPost {
			var query map[string]string
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
				t.Error(err)
			}
			if query["query"] != "dance" || query["cursor"] != "24" {
				t.Errorf("búsqueda perdió sus términos/cursor: %v", query)
			}
			_, _ = io.WriteString(w, `{"items":[{"id":7,"title":"Dance","license":"CC0 1.0","width":4,"height":4,"size":100}],"next":"48"}`)
			return
		}
		_, _ = io.WriteString(w, "GIF89a")
	}))
	defer srv.Close()
	c := NewRooms(srv.URL, nil, nil)
	ctx := context.Background()
	page, err := c.FreeGifs(ctx, "dance", "24")
	if err != nil || len(page.Items) != 1 || page.Next != "48" {
		t.Fatalf("catálogo: %+v %v", page, err)
	}
	for _, preview := range []bool{true, false} {
		rc, err := c.FreeGifContent(ctx, 7, preview)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil || string(data) != "GIF89a" {
			t.Fatalf("proxy: %q %v", data, err)
		}
	}
	want := "POST /api/v1/gifs/search\nGET /api/v1/gifs/7?preview=1\nGET /api/v1/gifs/7"
	if got := strings.Join(requests, "\n"); got != want {
		t.Errorf("rutas nativas:\n%s", got)
	}
}
