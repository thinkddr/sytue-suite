// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cast es Sytue Cast de Sytue Office: un visor de vídeos de este equipo y un
// conversor entre formatos, sin cuenta, sin cifrado y sin servidor
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.10, D-26 y D-27).
//
// Comparado con `cmd/cast`: no hay `sesion.Abrir` ni `cifrado.NewPrivateApplication`.
// La fachada `api.Cast` va enchufada a `local/videoteca` (la carpeta `<Datos>/Vídeos`
// más un índice JSON) y monta las MISMAS pantallas de Inicio, Explorar y Ver (D-04); lo
// que necesita servicio (subir, emitir, directo, chat, comentarios, compartir,
// moderación, calidades) no se monta. «Subir» es importar un fichero del equipo, y
// «publicar» lo enseña en Explorar de esta app en este equipo; no se sirve a la red.
//
// El reproductor propio lee WebM con VP9 y Opus. El resto de formatos (MP4, MOV, MKV,
// AVI, HEVC…) se ve y se convierte con el ffmpeg que la persona TENGA instalado, lanzado
// como un proceso aparte (`local/conversor`): la app no lo trae ni lo descarga
// (CONVENTIONS §12). Sin ffmpeg, lo demás se ve dentro de la app por el descodificador
// del sistema (`video/sistema`, CA-08): GStreamer en el escritorio si está instalado y
// MediaCodec en Android; si no hay o no sabe el formato, se abre con el reproductor del
// sistema.
package main

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/apps/cast"
	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada/altavoz"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/fyne/video/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

// Ventana inicial: la misma que la edición conectada (1280×800).
const (
	anchoInicial = 1280
	altoInicial  = 800
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "cast")
	if err != nil {
		slog.Error("cast", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	tr, err := l.Traductor(catalogos.Cast())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	t := textosDe(l.Idioma())
	movil := a.Driver().Device().IsMobile()

	// «Seguir donde lo dejé» en la carpeta de ajustes de Office, no en la de la suite.
	cast.CarpetaReanudar(func() (string, error) { return l.Espacio.Rutas.Ajustes, nil })

	herr := nuevasHerramientas(l.Espacio.Ajuste("cast.json"))
	// Los descodificadores del sistema (CA-08): GStreamer si está instalado (Linux, macOS),
	// Media Foundation en Windows (si no es un Windows N) y MediaCodec en Android (el fichero
	// que el teléfono no sepa leer lo dice al abrirlo). Sin ellos, lo que no es WebM sigue
	// necesitando ffmpeg o el reproductor del sistema.
	if (!movil || runtime.GOOS == "android") && sistema.Registrar() {
		slog.Info("cast: descodificador del sistema disponible para ver lo que no es WebM", "sistema", runtime.GOOS)
	}
	leeSistema := &lectorSistema{}
	vigia := &vigiaPosters{}
	autor := l.Espacio.Perfil.Nombre
	if autor == "" {
		autor = t("yo")
	}
	var p *cast.App
	var teca *videoteca.Videoteca
	teca, err = videoteca.Abrir(videoteca.Opciones{
		Carpeta:      videoteca.CarpetaEn(l.Espacio.Rutas.Datos),
		Datos:        l.Espacio.Rutas.Producto("cast"),
		Cache:        filepath.Join(l.Espacio.Rutas.Cache, "cast"),
		Autor:        autor,
		Canal:        t("canal"),
		Herramientas: herr.Actuales,
		Sondear:      herr.sondear,
		LeeElSistema: leeSistema.lee,
		Fotograma:    fotogramaJPEG,
		AlCambiar: func(id string) {
			fyne.Do(func() {
				if p == nil {
					return
				}
				// Un póster nuevo (lo haya puesto quien sea): fuera el viejo de la caché y,
				// fuera de la página del vídeo (que no lo enseña y no se debe cortar), se
				// vuelve a pedir la lista, que trae su dirección nueva.
				if teca != nil && vigia.cambio(teca, id) {
					p.TirarPoster(id)
					if p.Ruta() != cast.RutaVer {
						p.Recargar()
						return
					}
				}
				p.Repintar()
			})
		},
	})
	if err != nil {
		slog.Error("no se pudo abrir la videoteca", "error", err)
		os.Exit(1)
	}
	vigia.conocer(teca)
	fachada, err := api.NewPrivateCast(teca)
	if err != nil {
		slog.Error("cast", "error", err)
		os.Exit(1)
	}

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(anchoInicial, altoInicial))

	posters := photos.NuevaFototeca()
	v := cast.Vista{
		T: tr, Ventana: anchoInicial, Variante: tema.De(a.Settings().ThemeVariant()), Teca: posters, Movil: movil,
		Secciones: []cast.Seccion{cast.SecInicio, cast.SecExplorar, cast.SecSubir},
		// Lo que en la conectada dice «Subir», «No listado» o «visualizaciones» y aquí
		// no es verdad (tabla propia, textos.go).
		Propios: propiosDe(l.Idioma()),
	}
	var c *controlador
	v.Destinos = []cast.Destino{{Rotulo: t("colecciones"), Icono: iconos.Rejilla, Al: func() { c.colecciones() }}}
	p = cast.Nueva(fachada, v, w)
	p.Movil = movil
	posters.Pedir = p.PedirPoster
	posters.AlLlegar = func(string) { fyne.Do(p.Repintar) }
	// No hay cuenta: quien usa el equipo es siempre la dueña de todo.
	p.ConSesion = func() bool { return true }
	p.Yo = func() string { return videoteca.Yo }
	p.SinComentarios = true
	// El reproductor lee los ficheros de la videoteca directamente (con su ruta): así lo
	// que no es WebM lo puede abrir el descodificador del sistema (CA-08).
	p.AbrirMedia = func(ctx context.Context, ruta string) (cast.Fuente, error) { return teca.OpenMedia(ctx, ruta) }
	p.Guardar = func(nombre string, r io.Reader) (string, error) {
		carpeta, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(carpeta, nombre, r)
	}
	// El altavoz del sistema existe en todos los destinos nativos; la edición local no
	// compila la llamada (`edicionlocal`), así que no se mira `llamada.Disponible`.
	p.Audio = altavoz.Sistema{}
	c = &controlador{l: l, w: w, p: p, teca: teca, herr: herr, t: t, movil: movil}
	p.Importar = c.importar
	p.MandosVer = c.mandos
	p.AvisoVer = c.aviso
	c.barra = container.NewStack()
	c.ponerBarra()
	// El ffmpeg del equipo, también en Ajustes (su pestaña de Cast).
	if !movil {
		l.AjustesDeLaApp(c.t("ffmpeg.titulo"), c.seccionFFmpeg)
	}

	// Lo que llega de fuera («Abrir con», argumentos, soltar, la nube): se apunta en la
	// videoteca sin copiarlo y se abre.
	l.AlAbrirFichero(c.abrirApertura, func(err error) { c.fallo(t("abrir.error"), err) })

	w.SetContent(l.Montar(w, iconos.Cast, container.NewBorder(nil, c.barra, nil, nil, p.Objeto())))
	p.Arrancar()
	// Después de Arrancar, que pone su propio «soltar» (el de subir al servicio).
	l.AceptarFicheros(w)
	// Un vídeo de la videoteca (`?clase=video&id=…`), en el reproductor.
	l.AlAbrirElemento(func(clase, id string) {
		if clase == "video" {
			p.AbrirVideo(id)
		}
	})
	w.ShowAndRun()
}

// abrirApertura abre un fichero del equipo en la app.
func (c *controlador) abrirApertura(ctx context.Context, ap api.Apertura) error {
	if ap.Local == "" {
		return local.ErrSinFichero
	}
	u, err := url.Parse(ap.Local)
	if err != nil {
		return err
	}
	var e videoteca.Entrada
	if u.Scheme == "file" {
		e, err = c.teca.Apuntar(ctx, u.Path)
	} else {
		// Un `content://` del teléfono no tiene ruta que apuntar: se copia a la videoteca.
		e, err = c.copiarDeURI(ctx, ap.Local)
	}
	if err != nil {
		return err
	}
	fyne.Do(func() { c.p.AbrirVideo(e.ID) }) //nolint:contextcheck // el reproductor vive lo que la ventana, no la apertura que trajo el vídeo
	return nil
}

// copiarDeURI copia un vídeo que solo se puede leer por su URI (Android) a la carpeta de
// la videoteca y lo importa.
func (c *controlador) copiarDeURI(ctx context.Context, uri string) (videoteca.Entrada, error) {
	nombre, datos, err := local.LeerLocal(uri)
	if err != nil {
		return videoteca.Entrada{}, err
	}
	tmp, err := os.MkdirTemp(c.l.Espacio.Rutas.Cache, "llegado-")
	if err != nil {
		return videoteca.Entrada{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	ruta := filepath.Join(tmp, filepath.Base(nombre))
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		return videoteca.Entrada{}, err
	}
	return c.teca.Importar(ctx, ruta)
}

// ponerBarra enseña abajo, si no hay ffmpeg, qué se puede ver y cómo arreglarlo.
func (c *controlador) ponerBarra() {
	if _, err := c.herr.Actuales(); err == nil {
		c.barra.Objects = nil
		c.barra.Refresh()
		return
	}
	texto := c.t("sinffmpeg.barra")
	if video.HaySistema() {
		texto = c.t("sinffmpeg.gst")
	}
	if video.HaySistema() && runtime.GOOS == "windows" {
		texto = c.t("sinffmpeg.mf") // Media Foundation, no GStreamer
	}
	var boton fyne.CanvasObject
	switch {
	case c.conSistemaMovil():
		texto = c.t("movil.sistema")
	case c.movil:
		texto = c.t("sinffmpeg.movil")
	default:
		boton = widget.NewButton(c.t("ffmpeg"), c.ajustesFFmpeg)
	}
	etiqueta := widget.NewLabel(texto)
	etiqueta.Wrapping = fyne.TextWrapWord
	fila := container.NewBorder(nil, nil, nil, boton, etiqueta)
	if boton == nil {
		fila = container.NewStack(etiqueta)
	}
	c.barra.Objects = []fyne.CanvasObject{container.NewVBox(widget.NewSeparator(), container.NewPadded(fila))}
	c.barra.Refresh()
}
