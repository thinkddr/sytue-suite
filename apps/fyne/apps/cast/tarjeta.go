// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"image"
	"image/color"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Medidas de VideoCard: póster 16:9 `rounded-card`, título en dos líneas y autor y
// datos en líneas distintas. Esquinas y jerarquía siguen la forma elegida.
func radioTarjeta() float32 { return tema.Forma("radius-card") }

func escalonTituloTarjeta() tema.Escalon {
	escala := tema.Forma("title-scale")
	tam := tema.TextoSM.Tam * escala
	return tema.Escalon{Tam: tam, Linea: 1.375 * tam}
}

const (
	HuecoPieTarjeta = 3 * 0.25 * tema.Rem
	margenMarcas    = 2 * 0.25 * tema.Rem
)

// AltoTarjeta es lo que mide una tarjeta de ese ancho.
func AltoTarjeta(ancho float32) float32 {
	return ancho*9/16 + HuecoPieTarjeta + 2*escalonTituloTarjeta().Linea + 2*tema.Espacio(1) + 2*tema.TextoXS.Linea
}

// Tarjeta es un vídeo en una rejilla: póster, duración o «EN DIRECTO», título y datos.
// Pulsarla abre el vídeo.
type Tarjeta struct {
	ui.Pulsable

	Video api.CastVideo
	// Propia enseña la visibilidad en vez del autor: en «Tus vídeos» el autor es uno.
	Propia bool
	// SinPie enseña solo el póster (el de «Gestionar»: el título ya está en la página).
	SinPie bool

	v    Vista
	cols atomic.Uint32 // ancho de CSS del póster, para el radio en píxeles de dispositivo
}

var _ fyne.Widget = (*Tarjeta)(nil)

// NuevaTarjeta crea la tarjeta.
func NuevaTarjeta(v Vista, video api.CastVideo, propia bool, alAbrir func(api.CastVideo)) *Tarjeta {
	t := &Tarjeta{Video: video, Propia: propia, v: v}
	t.Nombre = video.Title
	if alAbrir != nil {
		t.AlPulsar = func() { alAbrir(video) }
	}
	t.ExtendBaseWidget(t)
	return t
}

// AccessibilityRole: es un enlace a la página del vídeo.
func (t *Tarjeta) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

// AccessibilityLabel es el título y los datos, como los lee la web.
func (t *Tarjeta) AccessibilityLabel() string { return t.Video.Title + ". " + t.datos() }

// autor es quien publica; en la biblioteca propia, la visibilidad del vídeo.
func (t *Tarjeta) autor() string {
	if t.Propia {
		return t.v.Traduce(claveVisibilidad(t.Video.Visibility))
	}
	if t.Video.Owner.Name != "" {
		return t.Video.Owner.Name
	}
	return t.v.Traduce("card.untitledOwner")
}

// resumen es la línea de vistas y fecha, sin repetir quien publica.
func (t *Tarjeta) resumen() string {
	v, vid := t.v, t.Video
	var partes []string
	switch {
	case vid.EnDirecto() && vid.Live != nil:
		partes = append(partes, v.Traduce("live.viewers", i18n.Vars{"count": vid.Live.ViewersNow}))
	case vid.Status == api.CastStatusReady:
		partes = append(partes, v.Traduce("watch.views", i18n.Vars{"count": vid.Views}))
	}
	partes = append(partes, Hace(v, vid.Fecha()))
	return strings.Join(partes, " · ")
}

// datos conserva toda la información para el nombre accesible del enlace.
func (t *Tarjeta) datos() string { return t.autor() + " · " + t.resumen() }

// CreateRenderer arma la tarjeta.
func (t *Tarjeta) CreateRenderer() fyne.WidgetRenderer {
	v := t.v.Variante
	r := &tarjetaRenderer{t: t}
	r.aro = ui.NuevoAro(radioTarjeta(), ui.SinHueco, v)
	r.fondo = ui.NuevaCaja(tema.Color("surface-sunken", v), color.NRGBA{}, radioTarjeta())
	glifo := glifoVideo
	if t.Video.Kind == "live" {
		glifo = glifoDirecto
	}
	r.glifo = iconos.TrazoDeRuta(glifo, 32, tema.Color("text-subtle", v))
	r.poster = canvas.NewRaster(r.generar)
	r.poster.ScaleMode = canvas.ImageScaleSmooth
	r.titulo = ui.NuevoParrafo(t.Video.Title, escalonTituloTarjeta(), ui.PesoTitulo(4), "text", v).Recortado(2)
	r.autor = ui.Texto(t.autor(), tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	r.datos = ui.Texto(t.resumen(), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	r.objetos = append(r.objetos, r.fondo.Objetos()...)
	r.objetos = append(r.objetos, r.glifo.Objeto(), r.poster)

	vid := t.Video
	switch {
	case vid.EnDirecto():
		r.marca = InsigniaDirecto(t.v)
	case vid.Status != api.CastStatusReady && vid.Status != "":
		r.marca = ui.NuevaInsignia(t.v.Traduce(claveEstado(vid.Status)), tonoEstado(vid.Status))
	}
	if r.marca != nil {
		r.objetos = append(r.objetos, r.marca)
	}
	if vid.DurationMS > 0 && !vid.EnDirecto() {
		texto := Duracion(time.Duration(vid.DurationMS) * time.Millisecond)
		r.pildora = ui.NuevaCaja(color.NRGBA{A: 184}, color.NRGBA{}, tema.RadioSM)
		r.duracion = ui.Texto(texto, tema.TextoXS, ui.Medio, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		r.objetos = append(r.objetos, r.pildora.Objetos()...)
		r.objetos = append(r.objetos, r.duracion)
	}
	if !t.SinPie {
		r.objetos = append(r.objetos, r.titulo, r.autor, r.datos)
	}
	r.Refresh()
	return r
}

type tarjetaRenderer struct {
	t        *Tarjeta
	aro      *ui.Aro
	fondo    *ui.Caja
	glifo    *iconos.Dibujo
	poster   *canvas.Raster
	marca    *ui.Insignia
	pildora  *ui.Caja
	duracion *canvas.Text
	titulo   *ui.Parrafo
	autor    *canvas.Text
	datos    *canvas.Text
	objetos  []fyne.CanvasObject
}

// generar pide el póster a la caché: aquí solo hay un acierto o un hueco, el trabajo
// va en segundo plano (ver photos.Fototeca.Tesela).
func (r *tarjetaRenderer) generar(w, h int) image.Image {
	teca := r.t.v.Teca
	if teca == nil || r.t.Video.PosterURL == "" {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	radio := radioTarjeta()
	if caja := math.Float32frombits(r.t.cols.Load()); caja > 0 {
		radio *= float32(w) / caja
	}
	if m := teca.Tesela(r.t.Video.ID, w, h, radio); m != nil {
		return m
	}
	return image.NewNRGBA(image.Rect(0, 0, 1, 1))
}

func (r *tarjetaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(10*tema.Rem, AltoTarjeta(10*tema.Rem))
}
func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaRenderer) Destroy()                     {}

func (r *tarjetaRenderer) Layout(t fyne.Size) {
	alto := t.Width * 9 / 16
	caja := fyne.NewSize(t.Width, alto)
	r.aro.Colocar(t)
	r.aro.Ver(r.t.Enfocado())
	ui.MoverCaja(r.fondo, fyne.NewPos(0, 0), caja)
	r.poster.Move(fyne.NewPos(0, 0))
	r.poster.Resize(caja)
	r.t.cols.Store(math.Float32bits(t.Width))
	r.glifo.Colocar((t.Width-32)/2, (alto-32)/2, 32)
	if r.marca != nil {
		m := r.marca.MinSize()
		r.marca.Move(fyne.NewPos(margenMarcas, margenMarcas))
		r.marca.Resize(m)
	}
	if r.pildora != nil {
		m := ui.Medir(r.duracion.Text, tema.TextoXS, ui.Medio)
		w, h := m.Width+2*tema.Espacio(1.5), tema.TextoXS.Linea+tema.Espacio(0.5)
		x, y := t.Width-margenMarcas-w, alto-margenMarcas-h
		ui.MoverCaja(r.pildora, fyne.NewPos(x, y), fyne.NewSize(w, h))
		ui.ColocarTexto(r.duracion, tema.TextoXS, x+tema.Espacio(1.5), y+tema.Espacio(0.25))
	}
	y := alto + HuecoPieTarjeta
	r.titulo.Move(fyne.NewPos(0, y))
	r.titulo.Resize(fyne.NewSize(t.Width, 2*escalonTituloTarjeta().Linea))
	y += 2*escalonTituloTarjeta().Linea + tema.Espacio(1)
	ui.Truncar(r.autor, r.t.autor(), tema.TextoXS, ui.Normal, t.Width)
	ui.ColocarTexto(r.autor, tema.TextoXS, 0, y)
	y += tema.TextoXS.Linea + tema.Espacio(1)
	ui.Truncar(r.datos, r.t.resumen(), tema.TextoXS, ui.Normal, t.Width)
	ui.ColocarTexto(r.datos, tema.TextoXS, 0, y)
}

func (r *tarjetaRenderer) Refresh() {
	r.aro.CambiarRadio(radioTarjeta())
	r.fondo.CambiarRadio(radioTarjeta())
	r.titulo.Escalon, r.titulo.Peso = escalonTituloTarjeta(), ui.PesoTitulo(4)
	r.aro.Ver(r.t.Enfocado())
	r.poster.Refresh()
	r.titulo.Refresh()
	canvas.Refresh(r.t)
}

// ---- La rejilla ----

// ColumnasRejilla es `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4`,
// contra el ancho de la VENTANA (son media queries).
func ColumnasRejilla(ventana float32) int {
	switch {
	case ventana >= ui.CorteXL:
		return 4
	case ventana >= ui.CorteLG:
		return 3
	case ventana >= ui.CorteSM:
		return 2
	}
	return 1
}

// Rejilla coloca tarjetas (o sus esqueletos) en columnas iguales con `gap-4`, cada
// fila del alto de una tarjeta de ese ancho.
func Rejilla(v Vista, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(&rejilla{cols: ColumnasRejilla(v.Ventana)}, hijos...)
}

type rejilla struct{ cols int }

func (r *rejilla) columna(ancho float32) float32 {
	return (ancho - float32(r.cols-1)*HuecoRejilla) / float32(r.cols)
}

func (r *rejilla) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, r.AltoCon(objs, float32(r.cols)*12*tema.Rem))
}

// AltoCon es el de las filas que salen a ese ancho.
func (r *rejilla) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	filas := (len(objs) + r.cols - 1) / r.cols
	if filas == 0 {
		return 0
	}
	return float32(filas)*AltoTarjeta(r.columna(ancho)) + float32(filas-1)*HuecoRejilla
}

func (r *rejilla) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	w := r.columna(t.Width)
	h := AltoTarjeta(w)
	for i, o := range objs {
		c, f := i%r.cols, i/r.cols
		o.Move(fyne.NewPos(float32(c)*(w+HuecoRejilla), float32(f)*(h+HuecoRejilla)))
		o.Resize(fyne.NewSize(w, h))
	}
}

// Tarjetas es la rejilla de una lista de vídeos.
func Tarjetas(v Vista, videos []api.CastVideo, propias bool, alAbrir func(api.CastVideo)) *fyne.Container {
	hijos := make([]fyne.CanvasObject, len(videos))
	for i, vid := range videos {
		hijos[i] = NuevaTarjeta(v, vid, propias, alAbrir)
	}
	return Rejilla(v, hijos...)
}

// seccionVideos es una sección con título y su rejilla; nil si no hay vídeos.
func seccionVideos(v Vista, titulo string, videos []api.CastVideo, alAbrir func(api.CastVideo)) []ui.Bloque {
	if len(videos) == 0 {
		return nil
	}
	return []ui.Bloque{
		{Obj: TituloSeccion(v, titulo), Margen: HuecoTitulo},
		{Obj: Tarjetas(v, videos, false, alAbrir), Margen: HuecoSeccion},
	}
}
