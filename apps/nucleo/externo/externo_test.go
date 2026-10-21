// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package externo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pngDe es un PNG de verdad de w×h.
func pngDe(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// pngQueMiente es un PNG cuya cabecera dice 100 000 × 100 000: unos bytes que, sin mirar
// las dimensiones antes, pedirían 40 GB al decodificarse.
func pngQueMiente(t *testing.T) []byte {
	t.Helper()
	b := pngDe(t, 1, 1)
	ihdr := b[8+8 : 8+8+13] // firma, largo y tipo del trozo, y sus 13 bytes
	binary.BigEndian.PutUint32(ihdr[0:4], 100_000)
	binary.BigEndian.PutUint32(ihdr[4:8], 100_000)
	binary.BigEndian.PutUint32(b[8+8+13:], crc32.ChecksumIEEE(b[8+4:8+8+13]))
	return b
}

// TestLaImagenDeFueraConSusTopes: una imagen buena se lee; una página, un cuerpo mayor
// que el tope, una cabecera que pide gigas, una dirección que no es web y una
// redirección fuera de la web, no. Y a un tercero no le llega ninguna cookie.
//
// VISTO EN ROJO (2026-09-23, en el frontal): decodificando sin `DecodeConfig`, el PNG
// que miente no da ErrGrande; y sin el `CheckRedirect`, la redirección a `file:` se
// sigue hasta que falla el transporte con otro error.
func TestLaImagenDeFueraConSusTopes(t *testing.T) {
	buena, miente := pngDe(t, 3, 2), pngQueMiente(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Cookies()) > 0 || r.Header.Get("Authorization") != "" {
			t.Errorf("a un tercero le llegan credenciales: %v", r.Header)
		}
		switch r.URL.Path {
		case "/buena.png":
			_, _ = w.Write(buena)
		case "/miente.png":
			_, _ = w.Write(miente)
		case "/pagina":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		case "/enorme":
			_, _ = w.Write(make([]byte, MaxBytes+10))
		case "/salta":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	img, err := Imagen(ctx, srv.URL+"/buena.png")
	if err != nil || img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
		t.Fatalf("la imagen buena da %v, %v", img, err)
	}
	for ruta, quiero := range map[string]error{
		"/miente.png": ErrGrande,
		"/pagina":     ErrNoImagen,
		"/enorme":     ErrGrande,
		"/salta":      ErrNoWeb,
	} {
		if _, err := Imagen(ctx, srv.URL+ruta); !errors.Is(err, quiero) {
			t.Errorf("%s da %v, y tenía que dar %v", ruta, err, quiero)
		}
	}
	for _, mala := range []string{"javascript:alert(1)", "file:///etc/passwd", "data:image/png;base64,AA==", "notas"} {
		if _, _, err := Bajar(ctx, mala, MaxBytes); !errors.Is(err, ErrNoWeb) {
			t.Errorf("%q da %v", mala, err)
		}
	}
	if _, err := Imagen(ctx, srv.URL+"/no"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("un 404 da %v", err)
	}
}
