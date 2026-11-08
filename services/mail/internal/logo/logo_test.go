// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package logo

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDominiosCompartidos recorre los mismos casos que la web
// (apps/frontend/apps/mail/src/lib/dominioRemitente.test.ts).
func TestDominiosCompartidos(t *testing.T) {
	datos, err := os.ReadFile("testdata/dominios.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Casos []struct {
			Correo  string  `json:"correo"`
			Dominio *string `json:"dominio"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(datos, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Casos) == 0 {
		t.Fatal("sin casos")
	}
	for _, c := range f.Casos {
		want := ""
		if c.Dominio != nil {
			want = *c.Dominio
		}
		if got := DeCorreo(c.Correo); got != want {
			t.Errorf("DeCorreo(%q) = %q, quiero %q", c.Correo, got, want)
		}
	}
}

func pngDe(t *testing.T, lado int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, lado, lado))
	for i := range img.Pix {
		img.Pix[i] = 0xc0
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func lado(t *testing.T, datos []byte) int {
	t.Helper()
	cfg, err := png.DecodeConfig(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("no es PNG: %v", err)
	}
	return max(cfg.Width, cfg.Height)
}

func TestDeclaradosOrdenaPorCalidad(t *testing.T) {
	html := `<head>
	  <link rel="icon" href="/favicon-16.png" sizes="16x16">
	  <link rel="shortcut icon" href="/favicon.svg" type="image/svg+xml">
	  <link rel="mask-icon" href="/mask.png">
	  <link rel="apple-touch-icon" href="touch.png">
	  <link rel="icon" sizes="96x96" href="https://cdn.x.com/i96.png?v=1&amp;b=2">
	  <link rel="icon" href="data:image/png;base64,AAAA">
	  <link rel="icon" href="http://x.com/inseguro.png" sizes="64x64">
	</head>`
	got := Declarados(html, "https://www.x.com/es/")
	want := []string{
		"https://cdn.x.com/i96.png?v=1&b=2",
		"https://www.x.com/es/touch.png",
		"https://www.x.com/favicon-16.png",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Declarados = %q\nquiero %q", got, want)
	}
}

func TestNormalizarReduceYDescartaLosPequenos(t *testing.T) {
	p, err := Normalizar(pngDe(t, 512))
	if err != nil {
		t.Fatal(err)
	}
	if l := lado(t, p); l != LadoMaximo {
		t.Fatalf("lado %d, quiero %d", l, LadoMaximo)
	}
	if _, err := Normalizar(pngDe(t, 16)); err == nil {
		t.Fatal("un icono de 16 px debería descartarse")
	}
	if _, err := Normalizar([]byte("<svg/>")); err == nil {
		t.Fatal("un SVG no es raster")
	}
}

// ico arma un .ico con una imagen PNG y otra DIB de 32 bits.
func ico(t *testing.T, pngLado, dibLado int) []byte {
	t.Helper()
	p := pngDe(t, pngLado)
	dib := make([]byte, 40+dibLado*dibLado*4)
	binary.LittleEndian.PutUint32(dib[0:], 40)
	binary.LittleEndian.PutUint32(dib[4:], uint32(dibLado))
	binary.LittleEndian.PutUint32(dib[8:], uint32(2*dibLado))
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	for i := 40; i < len(dib); i += 4 {
		copy(dib[i:], []byte{0x10, 0x20, 0x30, 0xff}) // BGRA
	}
	var b bytes.Buffer
	b.Write([]byte{0, 0, 1, 0, 2, 0})
	off := 6 + 2*16
	for _, img := range [][]byte{p, dib} {
		e := make([]byte, 16)
		binary.LittleEndian.PutUint32(e[8:], uint32(len(img)))
		binary.LittleEndian.PutUint32(e[12:], uint32(off))
		b.Write(e)
		off += len(img)
	}
	b.Write(p)
	b.Write(dib)
	return b.Bytes()
}

func TestNormalizarLeeIcoEnPNGYEnDIB(t *testing.T) {
	// La mayor es la DIB de 48: se queda esa, con sus colores en su sitio.
	out, err := Normalizar(ico(t, 16, 48))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if l := max(img.Bounds().Dx(), img.Bounds().Dy()); l != 48 {
		t.Fatalf("lado %d, quiero 48", l)
	}
	if c := color.NRGBAModel.Convert(img.At(3, 3)).(color.NRGBA); c != (color.NRGBA{R: 0x30, G: 0x20, B: 0x10, A: 0xff}) {
		t.Fatalf("color %v: el DIB es BGRA", c)
	}
	// Y la mayor en PNG, cuando es la PNG.
	out, err = Normalizar(ico(t, 64, 16))
	if err != nil || lado(t, out) != 64 {
		t.Fatalf("ico con PNG de 64: %v", err)
	}
}

func TestBuscarPrefiereElDeclaradoYSaltaLosPequenos(t *testing.T) {
	var pedidas []string
	traer := func(_ context.Context, u string, _ int64) (Respuesta, error) {
		pedidas = append(pedidas, u)
		switch u {
		case "https://x.com/":
			return Respuesta{
				Cuerpo: []byte(`<link rel="icon" sizes="16x16" href="/p.png"><link rel="apple-touch-icon" href="/t.png">`),
				Tipo:   "text/html", URL: "https://www.x.com/",
			}, nil
		case "https://www.x.com/t.png": // declara 180 y resulta ser de 16: fuera
			return Respuesta{Cuerpo: pngDe(t, 16), Tipo: "image/png", URL: u}, nil
		case "https://www.x.com/apple-touch-icon.png":
			return Respuesta{Cuerpo: pngDe(t, 180), Tipo: "image/png", URL: u}, nil
		}
		return Respuesta{}, errors.New("404")
	}
	out, err := Buscar(context.Background(), "x.com", traer)
	if err != nil {
		t.Fatal(err)
	}
	if l := lado(t, out); l != LadoMaximo {
		t.Fatalf("lado %d", l)
	}
	want := []string{"https://x.com/", "https://www.x.com/t.png", "https://www.x.com/p.png", "https://www.x.com/apple-touch-icon.png"}
	if !reflect.DeepEqual(pedidas, want) {
		t.Fatalf("pedidas %q\nquiero %q", pedidas, want)
	}

	nada := func(context.Context, string, int64) (Respuesta, error) { return Respuesta{}, errors.New("404") }
	if _, err := Buscar(context.Background(), "x.com", nada); !errors.Is(err, ErrSinLogo) {
		t.Fatalf("sin nada: %v", err)
	}
}

func TestTraerSeguroNoSaleDeHTTPS(t *testing.T) {
	if _, err := TraerSeguro(context.Background(), "http://x.com/", 10); err == nil {
		t.Fatal("http debería rechazarse")
	}
	if _, err := TraerSeguro(context.Background(), "https://127.0.0.1/", 10); err == nil {
		t.Fatal("loopback debería bloquearse")
	}
}

func TestCacheUnaBusquedaPorDominio(t *testing.T) {
	var llamadas atomic.Int32
	soltar := make(chan struct{})
	ahora := time.Unix(0, 0)
	c := &Cache{
		Ahora: func() time.Time { return ahora },
		Buscar: func(_ context.Context, d string) ([]byte, error) {
			llamadas.Add(1)
			<-soltar
			if d == "sin.com" {
				return nil, ErrSinLogo
			}
			return []byte("png"), nil
		},
	}
	var wg sync.WaitGroup
	res := make([][]byte, 4)
	for i := range res {
		wg.Go(func() { res[i] = c.Logo(context.Background(), "x.com") })
	}
	time.Sleep(20 * time.Millisecond)
	close(soltar)
	wg.Wait()
	for _, r := range res {
		if string(r) != "png" {
			t.Fatalf("resultado %q", r)
		}
	}
	if n := llamadas.Load(); n != 1 {
		t.Fatalf("%d búsquedas simultáneas, quiero 1", n)
	}
	c.Logo(context.Background(), "x.com")
	for range 2 {
		if c.Logo(context.Background(), "sin.com") != nil {
			t.Fatal("sin.com no tiene logo")
		}
	}
	if n := llamadas.Load(); n != 2 {
		t.Fatalf("%d búsquedas: el acierto y el fallo se recuerdan", n)
	}
	// El fallo caduca en un día; el acierto, no.
	ahora = ahora.Add(25 * time.Hour)
	c.Logo(context.Background(), "x.com")
	c.Logo(context.Background(), "sin.com")
	if n := llamadas.Load(); n != 3 {
		t.Fatalf("%d búsquedas tras un día, quiero 3", n)
	}
}
