// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package miniatura_test

import (
	"bytes"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
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

// webpVP8X fabrica la cabecera de un WebP extendido (VP8X) que DECLARA ancho×alto,
// con o sin alfa. Basta para DecodeConfig: es lo que usa la guarda antes de decodificar.
func webpVP8X(ancho, alto int, alfa bool) []byte {
	var flags byte
	if alfa {
		flags = 1 << 4
	}
	w, h := ancho-1, alto-1
	chunk := []byte{flags, 0, 0, 0, byte(w), byte(w >> 8), byte(w >> 16), byte(h), byte(h >> 8), byte(h >> 16)}
	var b bytes.Buffer
	b.WriteString("RIFF")
	tam := 4 + 8 + len(chunk)
	b.Write([]byte{byte(tam), byte(tam >> 8), byte(tam >> 16), byte(tam >> 24)})
	b.WriteString("WEBPVP8X")
	b.Write([]byte{byte(len(chunk)), 0, 0, 0})
	b.Write(chunk)
	return b.Bytes()
}

// TestRenderAceptaWebP: el decodificador WebP de x/image está registrado y la
// miniatura conserva el alfa al salir en PNG (D-DRIVE-03).
func TestRenderAceptaWebP(t *testing.T) {
	datos, err := os.ReadFile("testdata/rosa-con-alfa.webp")
	if err != nil {
		t.Fatal(err)
	}
	out, err := miniatura.Render(datos, 128, "image/png")
	if err != nil {
		t.Fatalf("Render(webp): %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("la miniatura no es PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 128 || b.Dy() > 128 {
		t.Fatalf("miniatura %dx%d, quiero 128 de lado mayor", b.Dx(), b.Dy())
	}
	transparente := false
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y && !transparente; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xffff {
				transparente = true
				break
			}
		}
	}
	if !transparente {
		t.Fatal("el alfa del WebP se ha perdido en la miniatura")
	}
}

// TestElWebPConAlfaCuestaLoMedido: un WebP con alfa pide ~7 B/px al decodificar
// (el alfa pasa por un NRGBA intermedio). Con la tarifa del JPEG (3 B/px), uno de
// 20 MP cabía en MaxBytes y pedía el doble; se rechaza por la cabecera, sin decodificar.
func TestElWebPConAlfaCuestaLoMedido(t *testing.T) {
	if _, err := miniatura.Render(webpVP8X(5000, 4000, true), 512, "image/png"); !errors.Is(err, miniatura.ErrMedioNoSoportado) {
		t.Fatalf("WebP con alfa de 20 MP: err=%v, quiero ErrMedioNoSoportado", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(webpVP8X(5000, 4000, false)))
	if err != nil || !miniatura.Cabe(cfg) {
		t.Fatalf("WebP opaco de 20 MP debe caber (err=%v)", err)
	}
	if _, err := miniatura.Render(webpVP8X(20000, 20000, false), 512, "image/png"); !errors.Is(err, miniatura.ErrMedioNoSoportado) {
		t.Fatalf("bomba WebP de 400 MP: err=%v, quiero ErrMedioNoSoportado", err)
	}
}

// TestElWebPExtendidoSinAlfaCuestaComoSinPerdida: un VP8X sin la marca de alfa se
// anuncia como YCbCr, pero puede llevar dentro un VP8L sin pérdida que decodifica a
// NRGBA (4 B/px). Uno de 25 MP cabía a 3 B/px y pedía más que MaxBytes.
func TestElWebPExtendidoSinAlfaCuestaComoSinPerdida(t *testing.T) {
	cfg, formato, err := miniatura.Cabecera(webpVP8X(5000, 5000, false))
	if err != nil || formato != "webp" || cfg.ColorModel != color.NRGBAModel {
		t.Fatalf("Cabecera(VP8X opaco) = %v, %q, %v; quiero NRGBA", cfg.ColorModel, formato, err)
	}
	if _, err := miniatura.Render(webpVP8X(5000, 5000, false), 512, "image/png"); !errors.Is(err, miniatura.ErrMedioNoSoportado) {
		t.Fatalf("WebP VP8X opaco de 25 MP: err=%v, quiero ErrMedioNoSoportado", err)
	}
	// El JPEG del mismo tamaño no cambia de tarifa.
	if !miniatura.Cabe(image.Config{ColorModel: color.YCbCrModel, Width: 5000, Height: 5000}) {
		t.Fatal("un JPEG de 25 MP debe seguir cabiendo")
	}
}

// TestLaMiniaturaJPEGDeUnaImagenConAlfaSaleSobreBlanco: el JPEG no tiene alfa; lo
// transparente de un PNG o WebP se aplana sobre blanco, no sobre negro.
func TestLaMiniaturaJPEGDeUnaImagenConAlfaSaleSobreBlanco(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64)) // todo transparente
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatal(err)
	}
	out, err := miniatura.Render(in.Bytes(), 32, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(16, 16).RGBA(); r>>8 < 0xf0 || g>>8 < 0xf0 || b>>8 < 0xf0 {
		t.Fatalf("lo transparente sale (%d,%d,%d), quiero blanco", r>>8, g>>8, b>>8)
	}
}
