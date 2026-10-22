// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package miniatura_test

import (
	"bytes"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
)

func pngDe(t *testing.T, ancho, alto int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, ancho, alto))
	for y := range alto {
		for x := range ancho {
			img.Set(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("codificar png de prueba: %v", err)
	}
	return buf.Bytes()
}

// TestRenderReduceYNoAmplia fija las dos mitades del contrato de tamaño.
func TestRenderReduceYNoAmplia(t *testing.T) {
	grande := pngDe(t, 800, 400)
	salida, err := miniatura.Render(grande, 200, "image/png")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(salida))
	if err != nil {
		t.Fatalf("releer la miniatura: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 100 {
		t.Errorf("se esperaba 200x100 conservando el aspecto, salió %dx%d", cfg.Width, cfg.Height)
	}
	if len(salida) >= len(grande) {
		t.Errorf("la miniatura (%d B) no es más pequeña que el origen (%d B): "+
			"el ahorro de ancho de banda es la razón de proxiar", len(salida), len(grande))
	}

	// Una imagen ya pequeña se conserva: ampliar solo gasta bytes y se ve peor.
	pequena := pngDe(t, 40, 30)
	salida, err = miniatura.Render(pequena, 200, "image/png")
	if err != nil {
		t.Fatalf("render de la pequeña: %v", err)
	}
	cfg, _, _ = image.DecodeConfig(bytes.NewReader(salida))
	if cfg.Width != 40 || cfg.Height != 30 {
		t.Errorf("no debe ampliar: se esperaba 40x30, salió %dx%d", cfg.Width, cfg.Height)
	}
}

// TestRenderRechazaLaBombaSinDecodificarla es la razón de que este paquete sea
// compartido en vez de estar copiado en cada servicio.
//
// Un PNG de pocos KB puede declarar unas dimensiones enormes en su cabecera. Si
// se decodifica antes de mirarlas, el proceso reserva el búfer de píxeles del
// ORIGEN y se queda sin memoria: una imagen de una vista previa de enlace la
// elige quien pega el enlace, así que el origen es hostil por definición.
func TestRenderRechazaLaBombaSinDecodificarla(t *testing.T) {
	// Cabecera PNG válida que declara 60.000×60.000 = 3.600 millones de píxeles,
	// muy por encima de MaxPixeles, sin traer los datos.
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := []byte{
		0, 0, 0, 13, 'I', 'H', 'D', 'R',
		0, 0, 0xEA, 0x60, // ancho: 60000 px
		0, 0, 0xEA, 0x60, // alto: 60000 px
		8, 6, 0, 0, 0, // 8 bits, RGBA
		0, 0, 0, 0, // CRC (irrelevante para DecodeConfig)
	}
	buf.Write(ihdr)

	if _, err := miniatura.Render(buf.Bytes(), 200, "image/jpeg"); err == nil {
		t.Fatal("una imagen que declara 3.600 millones de píxeles debe rechazarse")
	}
}

// TestRenderNoRevientaConBasura: lo que llega de un host cualquiera puede no ser
// una imagen. Debe ser un error de entrada, nunca un pánico ni un 500.
func TestRenderNoRevientaConBasura(t *testing.T) {
	for nombre, datos := range map[string][]byte{
		"vacío":            {},
		"texto":            []byte("<html>esto no es una imagen</html>"),
		"png truncado":     pngDe(t, 100, 100)[:20],
		"solo la cabecera": {0x89, 'P', 'N', 'G'},
	} {
		t.Run(nombre, func(t *testing.T) {
			if _, err := miniatura.Render(datos, 200, "image/jpeg"); err == nil {
				t.Error("se esperaba error de medio no soportado")
			}
		})
	}
}

// cabeceraPNG es una cabecera PNG válida (sin datos) de esas dimensiones y
// profundidad de bits, RGBA.
func cabeceraPNG(ancho, alto uint32, bits byte) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13})
	trozo := []byte{
		'I', 'H', 'D', 'R',
		byte(ancho >> 24), byte(ancho >> 16), byte(ancho >> 8), byte(ancho),
		byte(alto >> 24), byte(alto >> 16), byte(alto >> 8), byte(alto),
		bits, 6, 0, 0, 0,
	}
	buf.Write(trozo)
	crc := crc32.ChecksumIEEE(trozo)
	buf.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})
	return buf.Bytes()
}

// TestElTopeEsDeMemoriaNoDePixeles (FILE-07): un PNG de 16 bits de 6000×6000
// está por debajo de MaxPixeles, pero decodificarlo y reducirlo pide ~450 MB;
// se rechaza por la cabecera. El mismo tamaño en gris de 8 bits sí cabe.
func TestElTopeEsDeMemoriaNoDePixeles(t *testing.T) {
	cfg16, _, err := image.DecodeConfig(bytes.NewReader(cabeceraPNG(6000, 6000, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if 6000*6000 > miniatura.MaxPixeles || miniatura.Cabe(cfg16) {
		t.Fatalf("un PNG de 16 bits de 36 MP (coste %d MB) no cabe", miniatura.Coste(cfg16)>>20)
	}
	if _, err := miniatura.Render(cabeceraPNG(6000, 6000, 16), 200, "image/jpeg"); err == nil {
		t.Fatal("Render lo rechaza antes de decodificar")
	}
	if _, err := miniatura.Decodificar(cabeceraPNG(6000, 6000, 16)); err == nil {
		t.Fatal("Decodificar lo rechaza antes de decodificar")
	}
	gris := image.Config{ColorModel: color.GrayModel, Width: 6000, Height: 6000}
	if !miniatura.Cabe(gris) {
		t.Fatalf("una imagen gris de 36 MP cabe (coste %d MB)", miniatura.Coste(gris)>>20)
	}
}
