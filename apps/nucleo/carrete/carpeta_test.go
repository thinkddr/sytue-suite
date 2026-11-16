// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !(darwin && cgo)

package carrete

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// conRaiz pone la galería en una carpeta de la prueba.
func conRaiz(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	antes, antesRaices := raiz, raices
	raiz = func() (string, error) { return d, nil }
	raices = func() ([]string, error) { return []string{d}, nil }
	t.Cleanup(func() { raiz, raices = antes, antesRaices })
	return d
}

// absoluto es el ID de un fichero de la galería del escritorio: su ruta absoluta.
func absoluto(partes ...string) string { return filepath.ToSlash(filepath.Join(partes...)) }

func escribir(t *testing.T, ruta string, cuando time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(filepath.Base(ruta)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ruta, cuando, cuando); err != nil {
		t.Fatal(err)
	}
}

// TestListarLaCarpetaDeImagenes: se ven las fotos de todas las subcarpetas, de la más
// antigua a la más reciente; no se ve lo que no es una imagen, lo oculto, ni lo que
// Sytue guardó (o la copia de seguridad lo volvería a subir).
func TestListarLaCarpetaDeImagenes(t *testing.T) {
	d := conRaiz(t)
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	escribir(t, filepath.Join(d, "Álbum de cámara", "IMG_2.HEIC"), base.Add(2*time.Hour))
	escribir(t, filepath.Join(d, "IMG_1.jpg"), base)
	escribir(t, filepath.Join(d, "2024", "Vacaciones", "playa.png"), base.Add(time.Hour))
	escribir(t, filepath.Join(d, "notas.txt"), base)
	escribir(t, filepath.Join(d, ".miniaturas", "x.jpg"), base)
	escribir(t, filepath.Join(d, ".oculta.jpg"), base)
	escribir(t, filepath.Join(d, AlbumPropio, "bajada.jpg"), base)
	escribir(t, filepath.Join(d, "Álbum de cámara", "VID_1.mp4"), base.Add(3*time.Hour))

	if !Disponible() || EstadoPermiso() != Concedido {
		t.Fatalf("la carpeta debería estar disponible y sin pedir permiso: %v", EstadoPermiso())
	}
	l, err := Listar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(l))
	for _, e := range l {
		ids = append(ids, e.ID)
	}
	quiero := []string{
		absoluto(d, "IMG_1.jpg"), absoluto(d, "2024", "Vacaciones", "playa.png"),
		absoluto(d, "Álbum de cámara", "IMG_2.HEIC"), absoluto(d, "Álbum de cámara", "VID_1.mp4"),
	}
	if len(ids) != len(quiero) {
		t.Fatalf("lista %q, quiero %q", ids, quiero)
	}
	for i := range quiero {
		if ids[i] != quiero[i] {
			t.Fatalf("lista %q, quiero %q", ids, quiero)
		}
	}
	if l[2].Tipo != "image/heic" || l[2].Tamano != int64(len("IMG_2.HEIC")) || l[2].Video {
		t.Errorf("tipo o tamaño mal: %+v", l[2])
	}
	if !l[3].Video || l[3].Tipo != "video/mp4" {
		t.Errorf("el vídeo no se ve como vídeo: %+v", l[3])
	}
	if RedMedida() {
		t.Error("en el escritorio no hay datos móviles")
	}
	if _, _, err := Convertir(context.Background(), filepath.Join(d, "IMG_1.jpg")); !errors.Is(err, ErrNoDisponible) {
		t.Errorf("Convertir en el escritorio: %v", err)
	}
}

// TestGuardarYAbrir: lo guardado va a «Sytue Photos» sin pisar lo que había, y Abrir
// no sale de la carpeta aunque el ID lo intente.
func TestGuardarYAbrir(t *testing.T) {
	d := conRaiz(t)
	origen := filepath.Join(t.TempDir(), "x.jpg")
	escribir(t, origen, time.Now())
	ctx := context.Background()
	id1, err := Guardar(ctx, origen, "../../foto.jpg")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := Guardar(ctx, origen, "foto.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != absoluto(d, AlbumPropio, "foto.jpg") || id2 != absoluto(d, AlbumPropio, "foto (1).jpg") {
		t.Fatalf("ids %q y %q", id1, id2)
	}
	ruta, limpiar, err := Abrir(ctx, Elemento{ID: id1})
	limpiar()
	if err != nil || ruta != filepath.Join(d, AlbumPropio, "foto.jpg") {
		t.Fatalf("abrir: %q, %v", ruta, err)
	}
	for _, fuera := range []string{"../fuera.jpg", absoluto(d, "..", "fuera.jpg"), "relativa.jpg"} {
		if _, _, err := Abrir(ctx, Elemento{ID: fuera}); err == nil {
			t.Fatalf("Abrir(%q) ha salido de la galería", fuera)
		}
	}
}

func TestTipoDe(t *testing.T) {
	for nombre, tipo := range map[string]string{
		"a.JPG": "image/jpeg", "b.heic": "image/heic", "c.webp": "image/webp", "d.mov": "video/quicktime",
		"f.MP4": "video/mp4", "g.txt": "", "e": "",
	} {
		if hay := TipoDe(nombre); hay != tipo {
			t.Errorf("TipoDe(%q) = %q, quiero %q", nombre, hay, tipo)
		}
	}
}

// TestVariasCarpetas: en Windows la galería es la biblioteca Imágenes, de varias
// carpetas. Se recorren todas, una que está dentro de otra no saca nada dos veces, una
// que no existe (un disco quitado) no tumba la lista, y se abre de cualquiera de ellas.
func TestVariasCarpetas(t *testing.T) {
	d := conRaiz(t)
	otra := t.TempDir()
	cuando := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	escribir(t, filepath.Join(d, "Álbum de cámara", "IMG_1.jpg"), cuando)
	escribir(t, filepath.Join(otra, "IMG_2.jpg"), cuando.Add(time.Hour))
	raices = func() ([]string, error) {
		return []string{d, filepath.Join(d, "Álbum de cámara"), otra, filepath.Join(otra, "no-existe"), ""}, nil
	}
	l, err := Listar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 || l[0].ID != absoluto(d, "Álbum de cámara", "IMG_1.jpg") || l[1].ID != absoluto(otra, "IMG_2.jpg") {
		t.Fatalf("lista %+v", l)
	}
	if ruta, limpiar, err := Abrir(context.Background(), l[1]); err != nil || ruta != filepath.Join(otra, "IMG_2.jpg") {
		t.Fatalf("abrir de la segunda carpeta: %q, %v", ruta, err)
	} else {
		limpiar()
	}
	if got := carpetasLimpias([]string{otra, d, filepath.Join(d, "x"), d}); len(got) != 2 || got[0] != otra || got[1] != d {
		t.Errorf("carpetasLimpias = %q", got)
	}
	// Una que contiene a dos que no van seguidas las sustituye a las dos, una sola vez.
	if got := carpetasLimpias([]string{filepath.Join(d, "a"), otra, filepath.Join(d, "b"), d}); len(got) != 2 || got[0] != d || got[1] != otra {
		t.Errorf("carpetasLimpias con una que contiene a dos = %q", got)
	}
}
