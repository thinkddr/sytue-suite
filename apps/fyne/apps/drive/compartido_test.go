// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioCompartidoConmigo sirve `GET /shared/{id}/content` de dos raíces y apunta qué
// contenidos se pidieron (raíz y subcamino).
type servicioCompartidoConmigo struct {
	mu        sync.Mutex
	pedidos   []string
	contenido map[string][]byte
}

func (s *servicioCompartidoConmigo) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/v1/shared/") || !strings.HasSuffix(r.URL.Path, "/content") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raiz := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/shared/"), "/content")
		clave := raiz + ":" + r.URL.Query().Get("subpath")
		s.mu.Lock()
		s.pedidos = append(s.pedidos, clave)
		datos, ok := s.contenido[clave]
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(datos)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *servicioCompartidoConmigo) pedido(clave string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.pedidos, clave)
}

func jpegDePrueba(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := range 40 {
		for y := range 20 {
			img.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// vistaAbierta espera a que el diálogo de la vista previa tenga su contenido y lo
// devuelve; nil si no se abrió ninguno.
func vistaAbierta(t *testing.T, p *Pantalla, listo func(fyne.CanvasObject) bool) fyne.CanvasObject {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		var top fyne.CanvasObject
		fyne.DoAndWait(func() { top = p.win.Canvas().Overlays().Top() })
		if top != nil && listo(top) {
			return top
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func contiene[T any](o fyne.CanvasObject) bool {
	hay := false
	recorrer(o, func(x fyne.CanvasObject) {
		if _, ok := x.(T); ok {
			hay = true
		}
	})
	return hay
}

func botonDescargarDe(p *Pantalla, o fyne.CanvasObject) *ui.Boton {
	var b *ui.Boton
	recorrer(o, func(x fyne.CanvasObject) {
		if y, ok := x.(*ui.Boton); ok && y.Texto == p.tr.T("action.download", nil) {
			b = y
		}
	})
	return b
}

func textoVistaDe(o fyne.CanvasObject) string {
	var lineas []string
	recorrer(o, func(x fyne.CanvasObject) {
		if c, ok := x.(*fyne.Container); ok {
			if tv, ok := c.Layout.(*textoVista); ok {
				lineas = tv.lineas
			}
		}
	})
	return strings.Join(lineas, "\n")
}

// TestCompartidoConmigoEnseñaTextoEImagenYDescargaLoDemas (DRIVE-12): un `.txt` o un
// `.jpg` compartido se ven en la vista previa —traídos de `GET /shared/{id}/content`—
// con su «Descargar» en el pie; un `.zip` se sigue descargando sin diálogo. Antes de la
// pieza abrir CUALQUIER fichero compartido lo descargaba.
func TestCompartidoConmigoEnseñaTextoEImagenYDescargaLoDemas(t *testing.T) {
	// Las descargas van a `~/Downloads` de una casa de usar y tirar.
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	descargas := filepath.Join(casa, "Downloads")

	s := &servicioCompartidoConmigo{contenido: map[string][]byte{
		"r1:actas/notas.txt": []byte("orden del día\nuno"),
		"r1:fotos/playa.jpg": jpegDePrueba(t),
		"r1:copia.zip":       []byte("PK"),
		"r2:":                []byte("fichero raíz"),
	}}
	p := pantallaDePrueba(t, s.servidor(t))
	c := p.sec.compartido
	c.Raiz = &api.Compartido{File: api.File{ID: "r1", Name: "Proyecto"}, IsDir: true, Permission: "view"}

	cerrar := func() {
		fyne.DoAndWait(func() {
			for p.win.Canvas().Overlays().Top() != nil {
				p.win.Canvas().Overlays().Remove(p.win.Canvas().Overlays().Top())
			}
		})
	}

	// Un texto de dentro de la carpeta: el `<pre>` con su contenido.
	c.AlAbrir(api.Compartido{File: api.File{Name: "notas.txt", Path: "actas/notas.txt", SizeBytes: 17}})
	d := vistaAbierta(t, p, func(o fyne.CanvasObject) bool { return textoVistaDe(o) != "" })
	if d == nil {
		t.Fatal("abrir un .txt compartido no enseña la vista previa")
	}
	if got := textoVistaDe(d); got != "orden del día\nuno" {
		t.Errorf("la vista previa enseña %q", got)
	}
	if botonDescargarDe(p, d) == nil {
		t.Error("la vista previa no trae «Descargar»")
	}
	cerrar()

	// Una foto: en tu unidad iría a SyPhotos, pero SyPhotos no abre lo de otra persona.
	c.AlAbrir(api.Compartido{File: api.File{Name: "playa.jpg", Path: "fotos/playa.jpg", SizeBytes: 900}})
	d = vistaAbierta(t, p, contiene[*canvas.Image])
	if d == nil {
		t.Fatal("abrir un .jpg compartido no enseña la imagen")
	}
	// «Descargar» del pie baja ese mismo fichero.
	test.Tap(botonDescargarDe(p, d))
	esperarFichero(t, filepath.Join(descargas, "playa.jpg"))
	cerrar()

	// Un .zip no tiene vista previa: se descarga, sin diálogo.
	c.AlAbrir(api.Compartido{File: api.File{Name: "copia.zip", Path: "copia.zip", SizeBytes: 2}})
	esperarFichero(t, filepath.Join(descargas, "copia.zip"))
	var top fyne.CanvasObject
	fyne.DoAndWait(func() { top = p.win.Canvas().Overlays().Top() })
	if top != nil {
		t.Error("un .zip compartido abre un diálogo en vez de descargarse")
	}

	// Una raíz que ES el fichero: se pide sin subcamino.
	c.AlAbrirRaiz(api.Compartido{File: api.File{ID: "r2", Name: "léeme.md", SizeBytes: 12}, Permission: "view"})
	d = vistaAbierta(t, p, func(o fyne.CanvasObject) bool { return textoVistaDe(o) != "" })
	if d == nil || textoVistaDe(d) != "fichero raíz" || !s.pedido("r2:") {
		t.Fatal("abrir una raíz compartida de texto no enseña su vista previa")
	}
	cerrar()
}

// TestTipoVistaSinAppIncluyeLasFotosCompartido: con `sinApp` (lo compartido contigo) las fotos que
// abriría SyPhotos también tienen vista previa; sin él, siguen yendo a su app.
func TestTipoVistaSinAppIncluyeLasFotosCompartido(t *testing.T) {
	for _, nombre := range []string{"a.jpg", "a.JPEG", "a.png", "a.gif", "a.webp", "a.avif"} {
		if got := TipoVista(Elemento{Nombre: nombre, Tamano: 10}, true); got != "imagen" {
			t.Errorf("%s sin app: %q", nombre, got)
		}
	}
	for _, nombre := range []string{"a.jpg", "a.png", "a.gif"} {
		if got := TipoVista(Elemento{Nombre: nombre, Tamano: 10}, false); got != "" {
			t.Errorf("%s con app: %q, tiene que ir a SyPhotos", nombre, got)
		}
	}
	for _, nombre := range []string{"logo.svg", "a.heic", "a.pdf", "a.zip"} {
		if got := TipoVista(Elemento{Nombre: nombre, Tamano: 10}, true); got != "" {
			t.Errorf("%s sin app: %q, tiene que descargarse", nombre, got)
		}
	}
}

func esperarFichero(t *testing.T, ruta string) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		if b, err := os.ReadFile(ruta); err == nil && len(b) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no se descargó %s", ruta)
}
