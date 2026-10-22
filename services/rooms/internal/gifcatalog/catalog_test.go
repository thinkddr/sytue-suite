// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package gifcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func eligible(id int64) commonsPage {
	return commonsPage{ID: id, Title: "File:Motion.gif", Index: int(id), Images: []imageInfo{{
		URL: imageOrigin + "/wikipedia/commons/test.gif", Mime: "image/gif", Duration: 1,
		Size: 300, Width: 4, Height: 4, Metadata: map[string]metadata{
			"LicenseShortName": {Value: "CC0"}, "LicenseUrl": {Value: "https://creativecommons.org/publicdomain/zero/1.0/deed.en"},
			"AttributionRequired": {Value: "false"}, "Restrictions": {},
		},
	}}}
}

func jsonResponse(t *testing.T, v any) *http.Response {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}
}

func hermeticCatalog(f roundTripFunc) *Catalog {
	return &Catalog{http: &http.Client{Transport: f}, items: make(map[int64]cached)}
}

// La búsqueda de Commons contiene licencias distintas; ni su filtro de índice
// ni «public domain» autorizan por sí solos a reutilizar el original.
func TestSearchFailsClosedAndPreservesPagination(t *testing.T) {
	pages := make([]commonsPage, 1, 8)
	pages[0] = eligible(1)
	for id, change := range map[int64]func(*imageInfo){
		2: func(i *imageInfo) { i.Metadata["LicenseShortName"] = metadata{Value: "CC BY-SA 4.0"} },
		3: func(i *imageInfo) { delete(i.Metadata, "LicenseUrl") },
		4: func(i *imageInfo) { i.Duration = 0 },
		5: func(i *imageInfo) { i.URL = "https://127.0.0.1/private.gif" },
		6: func(i *imageInfo) { i.Size = MaxBytes + 1 },
		7: func(i *imageInfo) { i.Metadata["AttributionRequired"] = metadata{Value: "true"} },
		8: func(i *imageInfo) { delete(i.Metadata, "Restrictions") },
	} {
		p := eligible(id)
		change(&p.Images[0])
		pages = append(pages, p)
	}
	c := hermeticCatalog(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("filtró identidad a Commons")
		}
		if r.Header.Get("User-Agent") != userAgent || !strings.Contains(userAgent, "https://sytue.eu") {
			t.Error("User-Agent sin contacto")
		}
		q := r.URL.Query()
		search := q.Get("gsrsearch")
		if !strings.Contains(search, `filemime:"image/gif"`) || !strings.Contains(search, `"OR" "filemime:video"`) {
			t.Errorf("filtros retirables o términos no literales: %q", search)
		}
		if q.Get("gsroffset") != "24" || q.Get("gsrlimit") != "24" {
			t.Errorf("página no acotada: %v", q)
		}
		var res response
		res.Query.Pages = pages
		res.Continue.Offset = 48
		return jsonResponse(t, res), nil
	})
	p, err := c.Search(context.Background(), "OR filemime:video", "24")
	if err != nil || len(p.Items) != 1 || p.Items[0].ID != 1 || p.Next != "48" {
		t.Fatalf("filtrado/paginación: %+v %v", p, err)
	}
	if _, err = c.Search(context.Background(), "", "not a cursor"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cursor inválido: %v", err)
	}
}

func animated(t *testing.T) []byte {
	t.Helper()
	g := &gif.GIF{Image: []*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}),
		image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}),
	}, Delay: []int{10, 10}}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, g); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestContentRechecksLicenseAndActualAnimation(t *testing.T) {
	data := animated(t)
	downloads := 0
	c := hermeticCatalog(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "commons.wikimedia.org" {
			p := eligible(1)
			if r.URL.Query().Get("pageids") == "2" {
				p.ID = 2
				p.Images[0].Metadata["LicenseShortName"] = metadata{Value: "Public domain"}
			}
			var res response
			res.Query.Pages = []commonsPage{p}
			return jsonResponse(t, res), nil
		}
		downloads++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	if _, err := c.Content(context.Background(), 2); !errors.Is(err, ErrUnavailable) || downloads != 0 {
		t.Fatalf("bajó un original sin CC0: %v descargas=%d", err, downloads)
	}
	if got, err := c.Content(context.Background(), 1); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("animación segura: %v", err)
	}
	data = []byte("GIF89a")
	if _, err := c.Content(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatal("aceptó solo la cabecera pese a la duración en metadatos")
	}
}

func TestRedirectCannotChangeHostOrReachPrivateOrigin(t *testing.T) {
	c := New()
	original, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, http.NoBody)
	for _, target := range []string{imageOrigin + "/file.gif", "https://127.0.0.1/private", "http://commons.wikimedia.org/w/api.php"} {
		r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
		if err := c.http.CheckRedirect(r, []*http.Request{original}); err == nil {
			t.Errorf("permitió redirección a %s", target)
		}
	}
}
