// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command photos es Sytue Photos de escritorio y móvil.
//
// Es SU PROPIA aplicación (`docs/CONVENTIONS.md` §13.6): habla con syphotos en directo
// y con el token de la sesión transversal, sin pasar por el BFF de la web, y a Drive
// —donde viven los originales— para subir.
//
// Para VERLA hace falta una máquina con pantalla. Lo que corre en cualquier sitio, sin
// CGO y sin servidor gráfico, es `go test ./apps/photos/...`, que pinta las mismas
// pantallas con el pintor de software y comprueba los píxeles —y que con
// `SYTUE_CAPTURA=/ruta` deja las capturas en disco—.
// paridad-web: apps/frontend/apps/photos/src/pages/auth/matrix/callback.astro
package main

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Ventana inicial: la misma con la que se midió la web (1280×800).
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.NewWithID("eu.sytue.photos")
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	// El cromo «papel» de toda la suite (ADR 0080). Aquí y no en `sesion.Abrir`: las
	// pruebas que abren sesión comprueban también el cromo de color.
	tema.ElegirCromo(tema.CromoPapel)
	w := a.NewWindow("Sytue Photos")
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	tr := traductor(s.Idioma())
	// El widget nativo lo lee su propia Fuente, sin depender de la pantalla, y se
	// vuelve a publicar tras cada escritura del cliente de arriba (ClienteConAviso).
	s.PublicadorWidget(s.FuenteWidget(tr))

	host, err := newPhotosStorage(s, a, w, tr)
	if err != nil {
		w.SetContent(ui.NuevoVacio(tr.T("matrix.failed", nil), ""))
	} else {
		s.RutaPropia(func(u *url.URL) bool {
			return photos.TokenDeEnlace(u) != "" || photos.AlbumDeEnlace(u) != "" || photos.AlbumPropioDeEnlace(u) != ""
		})
		s.AlAbrirURL(func(u *url.URL) {
			fyne.Do(func() {
				current := host.presenter.Load()
				if current == nil {
					return
				}
				if token := photos.TokenDeEnlace(u); token != "" {
					current.app.AbrirPublico(token)
				}
				if album := photos.AlbumDeEnlace(u); album != "" {
					current.app.AbrirCompartido(album)
				}
				if album := photos.AlbumPropioDeEnlace(u); album != "" {
					current.app.AbrirAlbum(album)
				}
			})
		})
		s.AlAbrirFichero("photos", host.OpenDriveFile)
		soltar.AlArrastrar(w, func(active bool) {
			if current := host.presenter.Load(); current != nil {
				current.app.Arrastrando(active)
			}
		})
		s.AlFondo(func() {
			if current := host.presenter.Load(); current != nil {
				current.app.AlFondo()
			}
		})
		s.AlVolver(func() {
			if current := host.presenter.Load(); current != nil {
				current.app.AlVolver()
			}
		})
		w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Photos).Montar(host.slot))
		w.SetOnClosed(host.Close)
		// Con interfaz: lanzada por iOS solo para el widget no se pide la fototeca, ni se
		// toca la galería del sistema.
		s.CuandoHayaEscena(func() {
			host.photos.Start()
		})
	}
	s.Arrancar(context.Background())
	w.ShowAndRun()
}

// guardarEnGaleria deja los bytes en un fichero temporal y de ahí en la galería del
// sistema, que pide un fichero.
func guardarEnGaleria(p *photos.App, nombre string, datos []byte) error {
	f, err := os.CreateTemp("", "sytue-guardar-*"+filepath.Ext(nombre))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, err = f.Write(datos)
	if cerrar := f.Close(); err == nil {
		err = cerrar
	}
	if err != nil {
		return err
	}
	c, fin := context.WithTimeout(context.Background(), 2*time.Minute)
	defer fin()
	return p.GuardarEnGaleria(c, f.Name(), nombre)
}

// leer trae entera una variante: la Fototeca guarda los bytes, no el flujo.
func leer(r io.ReadCloser, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// traductor carga el catálogo en el idioma que ya ha decidido `sesion.Suite`, el mismo
// para las trece apps.
func traductor(l i18n.Locale) *i18n.Translator {
	t, err := photos.Traductor(l)
	if err != nil {
		// El inglés está embebido y no puede faltar: si falla, es que el catálogo
		// se ha roto al generarse, y arrancar en blanco sería peor.
		panic(err)
	}
	return t
}
