// SPDX-License-Identifier: Apache-2.0
// Copyright (c) Sytue. Licensed under Apache-2.0.

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thinkddr/astro-fyne/native/visual"
)

func TestCLIRechazaUnPixelYCreaElDiff(t *testing.T) {
	dir := t.TempDir()
	referencePath, nativePath, diffPath := filepath.Join(dir, "web.png"), filepath.Join(dir, "native.png"), filepath.Join(dir, "diff.png")
	web, native := image.NewNRGBA(image.Rect(0, 0, 2, 2)), image.NewNRGBA(image.Rect(0, 0, 2, 2))
	native.SetNRGBA(1, 1, color.NRGBA{A: 1})
	if err := writePNG(referencePath, web); err != nil {
		t.Fatal(err)
	}
	if err := writePNG(nativePath, native); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--reference", referencePath, "--native", nativePath, "--out", diffPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("el proceso debe fallar por la diferencia: %d, %s", code, stderr.String())
	}
	var result visual.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Exact || result.Accepted || result.ChangedPixels != 1 {
		t.Fatalf("informe incorrecto: %+v, %v", result, err)
	}
	if _, err := readPNG(diffPath); err != nil {
		t.Fatalf("debe guardar el diff incluso cuando falla el gate: %v", err)
	}
}

func insertPNGChunk(encoded []byte, kind string, data []byte, afterPixels bool) []byte {
	chunk := make([]byte, len(data)+12)
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(data)))
	copy(chunk[4:8], kind)
	copy(chunk[8:], data)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	position := 33 // Después de IHDR: antes de cualquier IDAT.
	if afterPixels {
		position = len(encoded) - 12 // Antes de IEND: después de todos los IDAT.
	}
	result := append([]byte{}, encoded[:position]...)
	result = append(result, chunk...)
	return append(result, encoded[position:]...)
}

func staticPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(1, 1, color.NRGBA{R: 100, G: 50, B: 25, A: 127})
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestCLINoCertificaMetadatosIgnoradosAunqueLosPixelesCoincidan(t *testing.T) {
	// Los chunks mantienen CRC válidas. image/png ignora estos metadatos y
	// decodificaría los mismos canales que el PNG estático original.
	for _, kind := range []string{"acTL", "fcTL", "fdAT", "gAMA", "iCCP", "cHRM", "sRGB", "cICP", "mDCV", "cLLI", "eXIf", "sBIT", "bKGD", "vpAg"} {
		for _, afterPixels := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-after-pixels-%v", kind, afterPixels), func(t *testing.T) {
				dir := t.TempDir()
				encoded := staticPNG(t)
				data := []byte{0, 0, 0, 2, 0, 0, 0, 0}
				marked := insertPNGChunk(encoded, kind, data, afterPixels)
				decoded, err := png.Decode(bytes.NewReader(marked))
				if err != nil {
					t.Fatalf("el decoder estándar debe ilustrar la interpretación omitida: %v", err)
				}
				plain, err := png.Decode(bytes.NewReader(encoded))
				if err != nil {
					t.Fatal(err)
				}
				result, _, err := visual.Compare(plain, decoded, visual.Tolerance{})
				if err != nil || !result.Exact {
					t.Fatalf("el caso negativo debe conservar los mismos canales antes del guard: %+v, %v", result, err)
				}
				for _, markedSide := range []string{"reference", "native"} {
					referencePath, nativePath, diffPath := filepath.Join(dir, "web.png"), filepath.Join(dir, "native.png"), filepath.Join(dir, "diff.png")
					web, native := encoded, encoded
					if markedSide == "reference" {
						web = marked
					} else {
						native = marked
					}
					if err := os.WriteFile(referencePath, web, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(nativePath, native, 0600); err != nil {
						t.Fatal(err)
					}
					var stdout, stderr bytes.Buffer
					code := run([]string{"--reference", referencePath, "--native", nativePath, "--out", diffPath}, &stdout, &stderr)
					if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), kind) {
						t.Fatalf("%s %s no debe declarar paridad: código %d, stdout %s, stderr %s", markedSide, kind, code, stdout.String(), stderr.String())
					}
					if _, err := os.Stat(diffPath); !os.IsNotExist(err) {
						t.Fatalf("el rechazo no debe producir un diff: %v", err)
					}
				}
			})
		}
	}
}

func TestCLISigueComparandoRGBAConMetadatosDescriptivos(t *testing.T) {
	dir := t.TempDir()
	encoded := staticPNG(t)
	referencePath, nativePath, diffPath := filepath.Join(dir, "web.png"), filepath.Join(dir, "native.png"), filepath.Join(dir, "diff.png")
	annotated := insertPNGChunk(encoded, "tEXt", []byte("Comment\x00Captured by CI"), false)
	if err := os.WriteFile(referencePath, annotated, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nativePath, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--reference", referencePath, "--native", nativePath, "--out", diffPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("PNG RGBA estático debe seguir permitido: %d, %s", code, stderr.String())
	}
	var result visual.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !result.Exact || !result.Accepted || result.ChangedPixels != 0 {
		t.Fatalf("metadatos descriptivos no modifican muestras RGBA: %+v, %v", result, err)
	}
}

func TestCLINoIgnoraDatosAnexosTrasIEND(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing.png")
	if err := os.WriteFile(path, append(staticPNG(t), []byte("hidden frame")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPNG(path); err == nil {
		t.Fatal("PNG con payload posterior a IEND no pertenece al contrato estático")
	}
}

func TestCLINoSobrescribeLaReferencia(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--reference", "web.png", "--native", "native.png", "--out", "web.png"}, &stdout, &stderr); code != 2 {
		t.Fatalf("sobrescribir la referencia es un error de uso: %d", code)
	}
}

func TestCLIRechaza16BitsSinDeclararParidad(t *testing.T) {
	dir := t.TempDir()
	referencePath, nativePath, diffPath := filepath.Join(dir, "web16.png"), filepath.Join(dir, "native16.png"), filepath.Join(dir, "diff.png")
	web, native := image.NewNRGBA64(image.Rect(0, 0, 1, 1)), image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	web.SetNRGBA64(0, 0, color.NRGBA64{R: 0x0101, A: 0xffff})
	native.SetNRGBA64(0, 0, color.NRGBA64{R: 0x0102, A: 0xffff})
	if err := writePNG(referencePath, web); err != nil {
		t.Fatal(err)
	}
	if err := writePNG(nativePath, native); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--reference", referencePath, "--native", nativePath, "--out", diffPath}, &stdout, &stderr); code != 2 {
		t.Fatalf("dieciséis bits deben rechazarse antes de comparar: %d, %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("un formato rechazado no puede publicar un informe de igualdad: %s", stdout.String())
	}
	if _, err := os.Stat(diffPath); !os.IsNotExist(err) {
		t.Fatalf("el formato rechazado no debe generar un diff: %v", err)
	}
}
