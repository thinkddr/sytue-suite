// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"image"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
)

// Un AVIF entra en la fototeca local como cualquier foto (FO-03): se mide, se derivan
// sus miniaturas (que salen en JPEG) y su original vuelve tal cual. Sin admitir el
// formato, la misma fototeca lo rechaza, que es lo que hace la conectada.
func TestLaFototecaLocalImportaAVIF(t *testing.T) {
	raw, err := os.ReadFile("../../local/imagenes/testdata/foto.avif") // la misma que lee `local/imagenes`
	if err != nil {
		t.Fatal(err)
	}
	for _, admitir := range []bool{false, true} {
		o := fototeca.Opciones{}
		if admitir {
			o.Formatos = formatosAdmitidos
		}
		repo, err := fototeca.Abrir(context.Background(), t.TempDir(), "instalacion-1", o)
		if err != nil {
			t.Fatal(err)
		}
		cli, err := api.NewPhotos(sinRed, nil, nil).WithPrivateData(repo)
		if err != nil {
			t.Fatal(err)
		}
		foto, err := cli.ImportarFoto(t.Context(), uuid.NewString(), "gato.avif", "image/avif", bytes.NewReader(raw), int64(len(raw)))
		if !admitir {
			if err == nil {
				t.Fatal("sin admitirlo, un AVIF ha entrado")
			}
		} else {
			if err != nil {
				t.Fatal("importar:", err)
			}
			if foto.Width != 96 || foto.Height != 64 || foto.ContentType != "image/avif" {
				t.Fatalf("medida %dx%d %s", foto.Width, foto.Height, foto.ContentType)
			}
			r, err := cli.Variant(t.Context(), foto.ID, api.PhotoThumb)
			if err != nil {
				t.Fatal(err)
			}
			mini, _ := io.ReadAll(r)
			_ = r.Close()
			if _, formato, err := image.Decode(bytes.NewReader(mini)); err != nil || formato != "jpeg" {
				t.Fatal("miniatura:", formato, err)
			}
			var original bytes.Buffer
			if err := repo.Original(t.Context(), foto.ID, &original); err != nil || !bytes.Equal(original.Bytes(), raw) {
				t.Fatal("el original no vuelve igual", err)
			}
		}
		repo.Close()
	}
}
