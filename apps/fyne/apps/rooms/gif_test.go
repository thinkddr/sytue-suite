// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conEmoji es el doble con el catálogo de la organización.
type conEmoji struct {
	*servicioFalso
	catalogo []api.RoomsCustomEmoji
}

func (s *conEmoji) CustomEmoji(context.Context) ([]api.RoomsCustomEmoji, error) {
	s.apuntar("CustomEmoji")
	return s.catalogo, nil
}

func (s *conEmoji) CreateCustomEmoji(_ context.Context, nombre, fichero string) (api.RoomsCustomEmoji, error) {
	s.apuntar("CreateCustomEmoji", nombre, fichero)
	e := api.RoomsCustomEmoji{Name: nombre, FileID: fichero, ContentType: "image/gif"}
	s.catalogo = append(s.catalogo, e)
	return e, nil
}

// driveDelTenant es el doble de Drive con la raíz, la subida y las miniaturas.
type driveDelTenant struct{ *ficherosFalso }

func (d *driveDelTenant) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "raiz"}, nil
}

func (d *driveDelTenant) Delete(_ context.Context, kind api.DriveKind, id string) error {
	d.llamadas = append(d.llamadas, "borrar "+string(kind)+" "+id)
	return nil
}

func (d *driveDelTenant) Upload(_ context.Context, carpeta, nombre, tipo string, r io.Reader, _ int64) (api.File, error) {
	_, _ = io.Copy(io.Discard, r)
	d.llamadas = append(d.llamadas, "subir "+carpeta+" "+nombre+" "+tipo)
	return api.File{ID: "f-" + nombre}, nil
}

func (d *driveDelTenant) Thumbnail(_ context.Context, id string) (io.ReadCloser, error) {
	d.llamadas = append(d.llamadas, "mini "+id)
	var b bytes.Buffer
	_ = gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, 40, 20), color.Palette{color.Black, color.White}), nil)
	return io.NopCloser(&b), nil
}

// Download es el GIF original: dos fotogramas, que es lo que distingue un GIF animado de
// su miniatura.
func (d *driveDelTenant) Download(_ context.Context, id string) (io.ReadCloser, error) {
	d.llamadas = append(d.llamadas, "bajar "+id)
	pal := color.Palette{color.Black, color.White}
	g := &gif.GIF{Image: []*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 4, 4), pal), image.NewPaletted(image.Rect(0, 0, 4, 4), pal),
	}, Delay: []int{10, 10}}
	var b bytes.Buffer
	_ = gif.EncodeAll(&b, g)
	return io.NopCloser(&b), nil
}

// «GIF» del «+»: solo los GIF del catálogo, con su miniatura; buscar filtra; elegir
// escribe `:nombre:` en la caja; subir va a la raíz de Drive con el nombre del fichero
// como atajo, y lo que no es un GIF no entra.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): sin filtrar por
// `image/gif` en Gifs, el logotipo PNG sale en la rejilla.
func TestElSelectorDeGif(t *testing.T) {
	for nombre, quiero := range map[string]string{
		"Baile Loco.GIF": "baile-loco", "###.gif": "", "a_b-c.gif": "a_b-c",
		strings.Repeat("x", 80) + ".gif": strings.Repeat("x", 64),
	} {
		if got := SlugGif(nombre); got != quiero {
			t.Errorf("SlugGif(%q) = %q; quiero %q", nombre, got, quiero)
		}
	}
	base := servicioConDos()
	s := &conEmoji{servicioFalso: base, catalogo: []api.RoomsCustomEmoji{
		{Name: "baile", FileID: "f1", ContentType: "image/gif"},
		{Name: "logo", FileID: "f2", ContentType: "image/png"},
	}}
	sa, _, w := salaEn(t, base, 1280)
	dr := &driveDelTenant{ficherosFalso: &ficherosFalso{}}
	sa.S, sa.Ficheros = s, dr
	dir := t.TempDir()
	elegido := filepath.Join(dir, "Nuevo Baile.gif")
	sa.ElegirFicheros = func() ([]string, error) { return []string{elegido}, nil }
	for _, f := range []string{elegido, filepath.Join(dir, "foto.png")} {
		if err := os.WriteFile(f, []byte("GIF89a"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sa.Arrancar(context.Background())
	tr := sa.P.T

	sa.MenuMas(sa.P.Canal.Compositor)
	var rotulos []string
	for _, e := range sa.menuMas.Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok {
			rotulos = append(rotulos, it.Texto)
		}
	}
	if got := strings.Join(rotulos, " | "); !strings.HasSuffix(got, tr.T("card.newTask", nil)+" | "+tr.T("gif.title", nil)) {
		t.Errorf("el menú «+» lleva %s", got)
	}
	sa.menuMas.Cerrar()

	sa.AbrirGif()
	g := sa.gif
	if g == nil || len(g.Gifs()) != 1 || g.Gifs()[0].Name != "baile" || g.Imagenes["f1"] == nil {
		t.Fatalf("el selector abre con %+v", g)
	}
	if a := g.Animados["f1"]; a == nil || len(a.Image) != 2 {
		t.Fatalf("el GIF del catálogo no llega entero para animarse: %+v · %v", a, dr.llamadas)
	}
	test.WidgetRenderer(g)
	if dir := os.Getenv("SYTUE_CAPTURA_SALA"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "sala-gif-1280.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	g.Buscar().PonerTexto("zzz")
	if len(g.Gifs()) != 0 {
		t.Error("buscar «zzz» deja GIF")
	}
	g.Buscar().PonerTexto("")

	g.AlSubir()
	if !s.pidio("CreateCustomEmoji(nuevo-baile,f-Nuevo Baile.gif)") || len(g.Gifs()) != 2 || g.Subiendo || g.FalloSubida != "" {
		t.Errorf("subir: %v · %v · %q", s.llamadas, dr.llamadas, g.FalloSubida)
	}
	elegido = filepath.Join(dir, "foto.png")
	g.AlSubir()
	if g.FalloSubida != tr.T("gif.uploadFailed", nil) {
		t.Errorf("un PNG deja %q", g.FalloSubida)
	}

	g.AlElegir(":baile:")
	if !strings.Contains(sa.P.Canal.Compositor.Texto(), ":baile:") {
		t.Errorf("la caja dice %q", sa.P.Canal.Compositor.Texto())
	}
}
