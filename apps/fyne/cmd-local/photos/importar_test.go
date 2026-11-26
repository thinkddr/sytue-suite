// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/bmp"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
)

// abrirFototecaDePrueba abre en dir la fototeca de SQLite como Photos (con sus formatos).
func abrirFototecaDePrueba(t *testing.T, dir string) *fototeca.Fototeca {
	t.Helper()
	repo, err := fototeca.Abrir(context.Background(), dir, "instalacion-1", fototeca.Opciones{Formatos: formatosAdmitidos})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	return repo
}

func fototecaDePrueba(t *testing.T) (*fototeca.Fototeca, *api.Photos) {
	t.Helper()
	repo := abrirFototecaDePrueba(t, t.TempDir())
	cli, err := api.NewPhotos(sinRed, nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo, cli
}

func jpegDe(t *testing.T, r uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for x := range 40 {
		img.Set(x, 3, color.NRGBA{R: r, A: 255})
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Importar una carpeta con el motor de la copia: cada foto entra una vez, lo que ya
// estaba (por su huella, aunque se llame distinto) no se duplica, los vídeos y lo que no
// es imagen se quedan fuera, y una segunda ronda no trae nada.
func TestImportarUnaCarpetaCadaFotoUnaVez(t *testing.T) {
	repo, cli := fototecaDePrueba(t)
	dir := t.TempDir()
	escribir := func(rel string, datos []byte) {
		t.Helper()
		r := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(r), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(r, datos, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, b := jpegDe(t, 10), jpegDe(t, 200)
	escribir("playa.jpg", a)
	escribir("2024/Viaje/montaña.jpg", b)
	escribir("2024/Viaje/copia de playa.jpg", a) // la misma foto con otro nombre
	escribir("video.mp4", []byte("no es una foto"))
	escribir(".oculta/x.jpg", jpegDe(t, 90))
	escribir("notas.txt", []byte("hola"))
	// Una que ya estaba en la fototeca, importada a mano.
	if _, err := cli.ImportarFoto(t.Context(), "11111111-2222-4333-8444-555555555555", "ya.jpg", "image/jpeg", bytes.NewReader(b), int64(len(b))); err != nil {
		t.Fatal(err)
	}

	g := soloImportables{GaleriaSistema: galeriaCarpetas{carpetas: func() []string { return []string{dir} }}}
	c := photos.NuevaCopia(g, "")
	p, err := c.Ronda(t.Context(), "local", nuevoDestinoLocal(repo, cli), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Total != 3 || p.Subidas != 1 || p.Fallidas != 0 {
		t.Fatalf("ronda: %+v", p)
	}
	linea, _, err := cli.Timeline(t.Context(), "", 100, false, "")
	if err != nil || len(linea) != 2 {
		t.Fatal("la fototeca debe tener las dos fotos distintas", len(linea), err)
	}
	// Otra ronda no trae nada; tampoco con un registro nuevo (por las huellas).
	if p, err := c.Ronda(t.Context(), "local", nuevoDestinoLocal(repo, cli), nil); err != nil || p.Total != 0 {
		t.Fatalf("segunda ronda: %+v %v", p, err)
	}
	otra := photos.NuevaCopia(g, "")
	if p, err := otra.Ronda(t.Context(), "local", nuevoDestinoLocal(repo, cli), nil); err != nil || p.Subidas != 0 {
		t.Fatalf("con registro nuevo: %+v %v", p, err)
	}
	// Abrir solo lo que está en las carpetas.
	if _, _, err := (galeriaCarpetas{carpetas: func() []string { return []string{dir} }}).Abrir(t.Context(),
		carrete.Elemento{ID: filepath.Join(t.TempDir(), "fuera.jpg")}); err == nil {
		t.Fatal("abrió un fichero de fuera de las carpetas")
	}
}

// La HEIC solo se importa si este equipo sabe convertirla (las órdenes, en
// `local/imagenes`).
func TestHEICSoloSiHayConQue(t *testing.T) {
	if importable("image/heic", false) || !importable("image/heic", true) || !importable("image/webp", false) || importable("video/mp4", true) {
		t.Fatal("importable")
	}
}

// bmpDe es una captura BMP opaca de 64×48.
func bmpDe(t *testing.T, r uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = r
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	var b bytes.Buffer
	if err := bmp.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Un BMP (una captura de Windows, un dibujo viejo) entra en la fototeca como cualquier
// foto, por las tres puertas: enlazado desde una carpeta (con la extensión en mayúsculas),
// copiado del carrete, que lo puede llamar `image/x-ms-bmp`, y elegido con «Añadir fotos»
// (el tipo por la extensión, que es el que mide la fototeca en su cabecera). Se mide, se
// derivan sus miniaturas y su original vuelve tal cual.
func TestUnBMPEntraEnLaFototeca(t *testing.T) {
	repo, cli := fototecaDePrueba(t)
	dir := t.TempDir()
	enlazado := bmpDe(t, 40)
	if err := os.WriteFile(filepath.Join(dir, "Pantallazo.BMP"), enlazado, 0o644); err != nil {
		t.Fatal(err)
	}
	carpetas := func() []string { return []string{dir} }
	d := nuevoDestinoLocal(repo, cli)
	d.carpetas = carpetas
	p, err := photos.NuevaCopia(soloImportables{GaleriaSistema: galeriaCarpetas{carpetas: carpetas}}, "").Ronda(t.Context(), "local", d, nil)
	if err != nil || p.Total != 1 || p.Subidas != 1 || p.Fallidas != 0 {
		t.Fatalf("de la carpeta: %+v %v", p, err)
	}

	delCarrete := bmpDe(t, 90)
	g := &carreteFalso{
		elementos: []carrete.Elemento{{ID: "1", Nombre: "dibujo.bmp", Tipo: "image/x-ms-bmp", Tomada: time.Now()}},
		datos:     map[string][]byte{"1": delCarrete},
	}
	p, err = photos.NuevaCopia(soloImportables{GaleriaSistema: g}, "").Ronda(t.Context(), "carrete", nuevoDestinoLocal(repo, cli), nil)
	if err != nil || p.Total != 1 || p.Subidas != 1 || p.Fallidas != 0 {
		t.Fatalf("del carrete: %+v %v", p, err)
	}

	elegido := bmpDe(t, 160)
	tipo := mime.TypeByExtension(".bmp")
	if _, err := cli.ImportarFoto(t.Context(), "11111111-2222-4333-8444-555555555555", "icono.bmp", tipo, bytes.NewReader(elegido), int64(len(elegido))); err != nil {
		t.Fatalf("elegido (%s): %v", tipo, err)
	}

	linea, _, err := cli.Timeline(t.Context(), "", 100, false, "")
	if err != nil || len(linea) != 3 {
		t.Fatal("la galería debe tener los tres BMP:", len(linea), err)
	}
	originales := map[string][]byte{"Pantallazo.BMP": enlazado, "icono.bmp": elegido}
	for _, f := range linea {
		if f.ContentType != "image/bmp" || f.Width != 64 || f.Height != 48 {
			t.Errorf("%s: %s %dx%d", f.Name, f.ContentType, f.Width, f.Height)
		}
		r, err := cli.Variant(t.Context(), f.ID, api.PhotoThumb)
		if err != nil {
			t.Fatalf("%s: miniatura: %v", f.Name, err)
		}
		mini, _ := io.ReadAll(r)
		_ = r.Close()
		if _, _, err := image.Decode(bytes.NewReader(mini)); err != nil {
			t.Errorf("%s: la miniatura no se lee: %v", f.Name, err)
		}
		if quiero, ok := originales[f.Name]; ok {
			var original bytes.Buffer
			if err := repo.Original(t.Context(), f.ID, &original); err != nil || !bytes.Equal(original.Bytes(), quiero) {
				t.Errorf("%s: el original no vuelve igual: %v", f.Name, err)
			}
		}
	}
}
