// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imagenes

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"mime"
	"os"
	"testing"

	"golang.org/x/image/tiff"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
)

// Con el paquete importado, `miniatura` lee también AVIF, TIFF y BMP: la cabecera, la
// guarda de tamaño y la imagen, como un PNG. Y un `.bmp` es `image/bmp` en `mime`, como
// dice su cabecera, en cualquier sistema.
func TestAVIFTIFFYBMPSeLeenComoLosDeLaWeb(t *testing.T) {
	avif, err := os.ReadFile("testdata/foto.avif")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := tiff.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 30, 20)), nil); err != nil {
		t.Fatal(err)
	}
	for formato, datos := range map[string][]byte{"avif": avif, "tiff": b.Bytes(), "bmp": bmpDePrueba(30, 20)} {
		cfg, leido, err := miniatura.Cabecera(datos)
		if err != nil || leido != formato || !miniatura.Cabe(cfg) {
			t.Fatalf("%s: la cabecera %q %+v %v", formato, leido, cfg, err)
		}
		img, err := miniatura.Decodificar(datos)
		if err != nil || img.Bounds().Dx() != cfg.Width {
			t.Fatalf("%s: la imagen: %v", formato, err)
		}
	}
	if got := mime.TypeByExtension(".BMP"); got != TipoBMP {
		t.Fatalf("el tipo de un .bmp es %q", got)
	}
	if !EsBMP("image/x-ms-bmp") || !EsBMP(TipoBMP) || EsBMP("image/png") {
		t.Fatal("EsBMP")
	}
}

// bmpDePrueba es un BMP de 24 bits de w×h hecho a mano: con `x/image/bmp` la prueba
// registraría ella misma el decodificador que se prueba.
func bmpDePrueba(w, h int) []byte {
	fila := (w*3 + 3) &^ 3
	tam := 54 + fila*h
	b := make([]byte, tam)
	copy(b, "BM")
	binary.LittleEndian.PutUint32(b[2:], uint32(tam))     //nolint:gosec // G115: una imagen de prueba pequeña
	binary.LittleEndian.PutUint32(b[10:], 54)             // donde empiezan los píxeles
	binary.LittleEndian.PutUint32(b[14:], 40)             // BITMAPINFOHEADER
	binary.LittleEndian.PutUint32(b[18:], uint32(w))      //nolint:gosec // G115: como arriba
	binary.LittleEndian.PutUint32(b[22:], uint32(h))      //nolint:gosec // G115: como arriba
	binary.LittleEndian.PutUint16(b[26:], 1)              // un plano
	binary.LittleEndian.PutUint16(b[28:], 24)             // bits por píxel
	binary.LittleEndian.PutUint32(b[34:], uint32(fila*h)) //nolint:gosec // G115: como arriba
	for i := 54; i < tam; i++ {
		b[i] = byte(i)
	}
	return b
}

// Una HEIC se reconoce por su nombre o su tipo; sin ningún conversor en el equipo no hay
// órdenes que probar, y convertirla dice que no se puede.
func TestHEICSoloSiHayConQue(t *testing.T) {
	if !EsHEIC("IMG_0001.HEIC") || !EsHEIC("image/heif") || !EsHEIC("foto.heif") || EsHEIC("foto.jpg") || EsHEIC("image/avif") {
		t.Fatal("EsHEIC")
	}
	antes := buscarEjecutable
	t.Cleanup(func() { buscarEjecutable = antes })
	buscarEjecutable = func(string) (string, error) { return "", os.ErrNotExist }
	t.Setenv("PATH", t.TempDir())
	if o := ordenesHEIC("a.heic", "a.jpg"); len(o) != 0 {
		t.Fatal("órdenes sin conversores", o)
	}
	if _, _, err := AJPEG(t.Context(), "IMG_0001.HEIC", []byte("no es una heic")); !errors.Is(err, ErrSinHEIC) {
		t.Fatalf("sin conversores: %v", err)
	}
}
