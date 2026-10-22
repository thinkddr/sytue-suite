// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
)

// El cable de las calidades, en snake_case y sin null: la web y el escritorio
// leen estas claves tal cual.
func TestCalidadesEnElCable(t *testing.T) {
	w, h := int32(1280), int32(720)
	v := domain.Video{
		Kind: domain.KindUpload, Status: domain.StatusReady, Width: &w, Height: &h,
		HlsKey: "x/hls/master.m3u8", WebmKey: "x/webm/720p.webm",
	}
	b, err := json.Marshal(calidadesDTOde(v))
	if err != nil {
		t.Fatal(err)
	}
	// Un vídeo de antes: HLS lo que permitía el original; WebM solo 720.
	if got := string(b); got != `{"qualities":[720,360],"hls_qualities":[720,360],"webm_qualities":[720]}` {
		t.Fatalf("vídeo antiguo: %s", got)
	}
	// Uno sin nada generado aún: listas vacías, no null.
	b, _ = json.Marshal(calidadesDTOde(domain.Video{Kind: domain.KindUpload, Qualities: []int32{720}}))
	if got := string(b); got != `{"qualities":[720],"hls_qualities":[],"webm_qualities":[]}` {
		t.Fatalf("recién creado: %s", got)
	}
	// Y van dentro del vídeo, al mismo nivel que el resto de campos.
	b, _ = json.Marshal(videoDTO{calidadesDTO: calidadesDTOde(v)})
	if !strings.Contains(string(b), `"webm_qualities":[720]`) || strings.Contains(string(b), "calidadesDTO") {
		t.Fatalf("videoDTO: %s", b)
	}
}

func TestEstimarPorHTTP(t *testing.T) {
	h := &Handlers{}
	pedir := func(q string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		h.Estimar(rec, httptest.NewRequest(http.MethodGet, "/api/v1/videos:estimate?"+q, http.NoBody))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	code, out := pedir("size_bytes=500000000&duration_ms=60000&width=1280&height=720")
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	opciones, _ := out["options"].([]any)
	if len(opciones) != 3 || out["approximate"] != false || out["duration_ms"] != float64(60000) {
		t.Fatalf("estimación: %v", out)
	}
	primera, _ := opciones[0].(map[string]any)
	if primera["quality"] != float64(1080) || primera["allowed"] != false || primera["bytes"] != float64(0) {
		t.Fatalf("1080 de un 720p no se permite: %v", primera)
	}
	segunda, _ := opciones[1].(map[string]any)
	if segunda["quality"] != float64(720) || segunda["allowed"] != true || segunda["recommended"] != true || segunda["bytes"] != float64(domain.BytesDe(720, 60000)) {
		t.Fatalf("720: %v", segunda)
	}
	// Solo con el tamaño: aproximada.
	if code, out := pedir("size_bytes=60000000"); code != http.StatusOK || out["approximate"] != true {
		t.Fatalf("solo tamaño: %d %v", code, out)
	}
	for _, q := range []string{"", "size_bytes=-1", "size_bytes=abc", "size_bytes=1&width=x", "size_bytes=1&duration_ms=-5"} {
		if code, _ := pedir(q); code != http.StatusBadRequest {
			t.Errorf("%q: %d, se esperaba 400", q, code)
		}
	}
}
