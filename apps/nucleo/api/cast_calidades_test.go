// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// El cable de las calidades tal como lo pintan services/cast/internal/http
// (calidades.go y playDTO): la escalera WebM, la estimación y la elección.
func TestCastCalidadesHablaElCable(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.RequestURI()+" "+string(cuerpo))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/videos/v1/play":
			_, _ = io.WriteString(w, `{"kind":"upload","hls_url":"/media/v1/tok.9/hls/master.m3u8","webm_url":"/media/v1/tok.9/webm/720p.webm","webm_qualities":[1080,720,360],"poster_url":null,"duration_ms":125000}`)
		case "GET /api/v1/videos:estimate":
			_, _ = io.WriteString(w, `{"options":[{"quality":1080,"allowed":false,"recommended":false,"bytes":0},{"quality":720,"allowed":true,"recommended":true,"bytes":34000000},{"quality":360,"allowed":true,"recommended":true,"bytes":11430000}],"recommended":[720,360],"original_bytes":60000000,"duration_ms":60000,"approximate":true}`)
		case "GET /api/v1/videos/v1/qualities", "PUT /api/v1/videos/v1/qualities":
			_, _ = io.WriteString(w, `{"options":[{"quality":720,"allowed":true,"recommended":true,"bytes":34000000}],"recommended":[720],"original_bytes":1,"duration_ms":60000,"approximate":false,"qualities":[720],"hls_qualities":[720,360],"webm_qualities":[720],"pending":true}`)
		case "POST /api/v1/videos":
			_, _ = io.WriteString(w, `{"id":"v1","qualities":[720],"hls_qualities":[],"webm_qualities":[]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewCast(srv.URL, nil, nil), context.Background()

	play, err := c.Play(ctx, "v1")
	if err != nil || !slices.Equal(play.Calidades(), []int{1080, 720, 360}) {
		t.Fatalf("play: %+v %v", play, err)
	}
	if got := play.WebMURLDe(1080); got != "/media/v1/tok.9/webm/1080p.webm" {
		t.Errorf("URL de 1080: %q", got)
	}
	if got := play.WebMURLDe(360); got != "/media/v1/tok.9/webm/360p.webm" {
		t.Errorf("URL de 360: %q", got)
	}
	if got := play.WebMURLDe(480); got != "" {
		t.Errorf("una calidad que no hay no tiene URL: %q", got)
	}

	e, err := c.Estimate(ctx, 60_000_000, 0, 1280, 720)
	if err != nil || !e.Approximate || len(e.Options) != 3 || e.Options[0].Allowed || e.Bytes([]int{720, 360}) != 45_430_000 {
		t.Fatalf("estimación: %+v %v", e, err)
	}

	q, err := c.Qualities(ctx, "v1")
	if err != nil || !q.Pending || !slices.Equal(q.WebMQualities, []int{720}) || !slices.Equal(q.HLSQualities, []int{720, 360}) || len(q.Options) != 1 {
		t.Fatalf("calidades del vídeo: %+v %v", q, err)
	}
	if _, err := c.SetQualities(ctx, "v1", []int{720}); err != nil {
		t.Fatal(err)
	}
	v, err := c.Create(ctx, CastNew{Title: "Con calidades", Qualities: []int{720}})
	if err != nil || !slices.Equal(v.Qualities, []int{720}) || v.WebMQualities == nil {
		t.Fatalf("crear: %+v %v", v, err)
	}

	// La consulta de la estimación y los cuerpos, exactos.
	quiere := []string{
		"GET /api/v1/videos/v1/play ",
		"GET /api/v1/videos:estimate?height=720&size_bytes=60000000&width=1280 ",
		"GET /api/v1/videos/v1/qualities ",
		`PUT /api/v1/videos/v1/qualities {"qualities":[720]}`,
		`POST /api/v1/videos {"title":"Con calidades","qualities":[720]}`,
	}
	if !slices.Equal(vistas, quiere) {
		t.Fatalf("peticiones:\n%s\nse esperaban:\n%s", strings.Join(vistas, "\n"), strings.Join(quiere, "\n"))
	}
}

// Un servicio de antes de la escalera no manda webm_qualities: solo hay el WebM
// de webm_url, que es el de 720p.
func TestCastPlayDeAntesDeLaEscalera(t *testing.T) {
	var p CastPlay
	if err := json.Unmarshal([]byte(`{"kind":"upload","webm_url":"/media/v1/t/webm/720p.webm"}`), &p); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Calidades(), []int{720}) || p.WebMURLDe(720) != p.WebMURL || p.WebMURLDe(360) != "" {
		t.Fatalf("antiguo: %v %q", p.Calidades(), p.WebMURLDe(720))
	}
	if (CastPlay{}).Calidades() != nil {
		t.Fatal("sin WebM (directo), ninguna")
	}
}
