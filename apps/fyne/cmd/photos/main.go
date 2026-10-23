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
package main

import (
	"bytes"
	"context"
	"io"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Ventana inicial: la misma con la que se midió la web (1280×800).
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")
	w := a.NewWindow("Sytue Photos")
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	tr := traductor(s.Idioma())
	cli := api.NewPhotos(s.URL("photos"), s.Sesion, nil)
	teca, grande := photos.NuevaFototeca(), photos.NuevaFototeca()
	grande.Variante = photos.VarianteGrande

	v := photos.Vista{
		T: tr, Teca: teca, TecaGrande: grande, Ventana: anchoInicial,
		Variante: tema.De(a.Settings().ThemeVariant()),
	}
	p := photos.Nueva(cli, v, w)
	p.Drive = api.NewDrive(s.URL("drive"), s.Sesion, nil)
	p.IA = api.NewIA(s.URL("ai"), s.Sesion, nil)
	// Compartir un álbum con alguien y decir quién me compartió uno: Sytue ID.
	p.Directorio = api.NewDirectorio(s.URL("id"), s.Sesion, nil)
	p.AlEntrar = func() { go func() { _ = s.Entrar(context.Background()) }() }
	p.AbrirWeb = func(destino string) {
		_ = api.AbrirFuera(a, destino)
	}
	// Solo compone el enlace del álbum PÚBLICO (`/p/<token>`), que va sin org a
	// propósito (ADR 0056): es para quien no tiene sesión, como en la web.
	p.WebDe = func(ruta string) string { return api.AppURL(s.Apex(), "photos", "", ruta) }
	// La vuelta a Drive desde el editor, en cambio, es de la sesión: canónica con org.
	org := s.Sesion.Organizacion
	p.AbrirApp = func(producto, ruta string) { _ = api.AbrirEn(a, s.Apex(), producto, org(), ruta) }
	p.Elegir = func() ([]string, error) { return ficheros.ElegirImagenes(tr.T("gallery.add", nil)) }
	p.Guardar = func(nombre string, datos []byte) (string, error) {
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, bytes.NewReader(datos))
	}

	// Las dos cachés piden por la misma puerta; la grande, con la huella de la receta
	// de las fotos que se han girado en esta sesión.
	teca.Pedir = func(ctx context.Context, id, variante string) ([]byte, error) {
		return leer(cli.Variant(ctx, id, variante))
	}
	grande.Pedir = func(ctx context.Context, id, variante string) ([]byte, error) {
		return leer(cli.VariantDe(ctx, id, variante, p.Huella(id)))
	}

	// Entrar o salir en esta app o en otra de las doce vuelve a pedir la pantalla: la
	// que había era de otra sesión (o de ninguna).
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() != dentro {
			dentro = e.Dentro()
			p.Ir(p.Ruta())
		}
	})

	// Un álbum compartido llega como `sytue-photos:/p/{token}` (o su dirección web, si el
	// sistema se la da a la app): se enseña aquí, sin sesión, como la página `/p/` de la
	// web. Es ruta PROPIA: no es la página «no encontrada». Un token que no existe, sí.
	p.NoEncontrado = s.NoEncontrado
	s.RutaPropia(func(u *url.URL) bool {
		return photos.TokenDeEnlace(u) != "" || photos.AlbumDeEnlace(u) != "" || photos.AlbumPropioDeEnlace(u) != ""
	})
	s.AlAbrirURL(func(u *url.URL) {
		if token := photos.TokenDeEnlace(u); token != "" {
			fyne.Do(func() { p.AbrirPublico(token) })
		}
		// El aviso «X ha compartido un álbum contigo» (`photos.shared`).
		if album := photos.AlbumDeEnlace(u); album != "" {
			fyne.Do(func() { p.AbrirCompartido(album) })
		}
		// «X ha añadido fotos a tu álbum» (`photos.album.added`).
		if album := photos.AlbumPropioDeEnlace(u); album != "" {
			fyne.Do(func() { p.AbrirAlbum(album) })
		}
	})

	// Una imagen pulsada en Drive llega como `sytue-photos:/photos/abrir?file=…&volver=…`:
	// se indexa (es idempotente y no copia nada: el original sigue en Drive) y se abre en
	// el EDITOR, que es a donde lleva `/abrir` en la web; su «Volver» devuelve a la
	// carpeta de Drive. Si algo falla, la sesión la manda a la web; si el servicio dice
	// que no está, a la página «no encontrada».
	//nolint:contextcheck // el editor vive más que la petición que lo abrió
	s.AlAbrirFichero("photos", func(ctx context.Context, ap api.Apertura) error {
		f, err := cli.Indexar(ctx, ap.Ref.ID)
		if err != nil {
			return err
		}
		fyne.Do(func() { p.AbrirEditor(f, ap.Volver) })
		return nil
	})

	// «Suelta para añadir» mientras se arrastran ficheros por encima de la ventana.
	soltar.AlArrastrar(w, p.Arrastrando)

	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Photos).Montar(p.Objeto()))
	s.AlFondo(p.AlFondo)
	s.AlVolver(p.AlVolver)
	w.SetOnClosed(p.AlFondo)
	p.Arrancar()
	s.Arrancar(context.Background())
	w.ShowAndRun()
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
