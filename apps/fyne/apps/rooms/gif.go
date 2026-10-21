// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"image"
	"image/gif"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/GifPickerModal.tsx.
//
// «GIF»: biblioteca CC0 servida por Rooms y GIF de la ORGANIZACIÓN. Es el catálogo del tenant que
// sostiene los emoji propios, porque un GIF es una imagen con nombre. Elegir uno escribe
// su atajo `:nombre:` en la caja (no es un adjunto: el catálogo sirve igual en todos los
// canales); subir uno lo deja para todo el equipo.
//
// La vista previa SE MUEVE, como en la web: la tesela baja el GIF original de Drive (la
// miniatura es solo el primer fotograma) y lo anima con ui.Gif, que se para fuera de la
// pantalla y con «menos movimiento».

// EmojiPropio es lo que el selector pide a Rooms; lo cumple *api.Rooms.
type EmojiPropio interface {
	CustomEmoji(ctx context.Context) ([]api.RoomsCustomEmoji, error)
	CreateCustomEmoji(ctx context.Context, nombre, fichero string) (api.RoomsCustomEmoji, error)
}

// DriveOrganizacion es lo que el selector pide a Drive: la raíz del tenant, donde se sube
// la imagen, y las miniaturas; lo cumple *api.Drive.
type DriveOrganizacion interface {
	Root(ctx context.Context) (api.Folder, error)
	Upload(ctx context.Context, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (api.File, error)
	Thumbnail(ctx context.Context, fichero string) (io.ReadCloser, error)
	Delete(ctx context.Context, kind api.DriveKind, id string) error
}

// DescargaDrive es bajar el fichero original; lo cumple *api.Drive. Es aparte porque solo
// la usa la tesela animada: sin ella se queda la miniatura quieta.
type DescargaDrive interface {
	Download(ctx context.Context, fichero string) (io.ReadCloser, error)
}

var (
	_ EmojiPropio       = (*api.Rooms)(nil)
	_ DriveOrganizacion = (*api.Drive)(nil)
	_ DescargaDrive     = (*api.Drive)(nil)
)

var (
	extensionGif = regexp.MustCompile(`\.[a-z0-9]+$`)
	noSlugGif    = regexp.MustCompile(`[^a-z0-9_-]+`)
)

// SlugGif es `slugify`: el nombre del fichero como atajo del catálogo, con el mismo
// invariante que el CHECK de la migración 0013 (minúsculas, cifras, `_` y `-`, 64 como
// mucho).
func SlugGif(nombre string) string {
	s := extensionGif.ReplaceAllString(strings.ToLower(nombre), "")
	s = strings.Trim(noSlugGif.ReplaceAllString(s, "-"), "-")
	return s[:min(len(s), 64)]
}

// Medidas de GifPickerModal.
var (
	// AltoCajaGif es el `h-64` de la caja con la rejilla, y AltoImagenGif el `h-24` de
	// cada imagen.
	AltoCajaGif   float32 = 64 * 0.25 * tema.Rem
	AltoImagenGif float32 = 24 * 0.25 * tema.Rem
)

// SelectorGif es el cuerpo del diálogo: el buscador, la caja con su estado o la rejilla
// de tres, y «Subir GIF» con su ayuda.
type SelectorGif struct {
	widget.BaseWidget
	T *i18n.Translator

	// Todos es el catálogo entero, nil mientras carga; Fallo, que no se pudo cargar.
	Todos []api.RoomsCustomEmoji
	Fallo bool
	// Subiendo es el `loading` del botón; FalloSubida, el texto rojo de debajo.
	Subiendo    bool
	FalloSubida string
	// Imagenes son las miniaturas que ya han llegado, por fichero, y Animados los GIF
	// originales ya descodificados, que mandan sobre la miniatura.
	Imagenes map[string]image.Image
	Animados map[string]*gif.GIF

	AlElegir     func(atajo string)
	AlReintentar func()
	AlSubir      func()

	buscar         *ui.Entrada
	columna        *fyne.Container
	libre          *SelectorGifLibre
	opciones       *ui.Segmentado
	libreActiva    bool
	alLibre        func()
	alCerrarLibre  func()
	alOrganizacion func()
}

var _ fyne.Widget = (*SelectorGif)(nil)

// NuevoSelectorGif crea el cuerpo, cargando.
func NuevoSelectorGif(t *i18n.Translator) *SelectorGif {
	s := &SelectorGif{T: t, Imagenes: map[string]image.Image{}, Animados: map[string]*gif.GIF{}}
	s.ExtendBaseWidget(s)
	s.buscar = ui.NuevaEntrada(t.T("gif.search", nil))
	s.buscar.AlCambiar(func(string) { s.Refresh() })
	return s
}

// Buscar es el campo, para los guardianes.
func (s *SelectorGif) Buscar() *ui.Entrada { return s.buscar }

// Gifs es `gifs`: solo lo que se mueve —el catálogo mezcla logotipos PNG y caritas— y
// cuyo nombre contiene lo buscado.
func (s *SelectorGif) Gifs() []api.RoomsCustomEmoji {
	q := strings.ToLower(strings.TrimSpace(s.buscar.Texto()))
	var out []api.RoomsCustomEmoji
	for _, e := range s.Todos {
		if e.ContentType == "image/gif" && buscar.Contiene(e.Name, q) {
			out = append(out, e)
		}
	}
	return out
}

// CreateRenderer monta la columna (`flex flex-col gap-3`).
func (s *SelectorGif) CreateRenderer() fyne.WidgetRenderer {
	s.columna = container.New(ui.Columna{Hueco: tema.Espacio(3)})
	r := &selectorGifRenderer{s: s}
	r.Refresh()
	return r
}

type selectorGifRenderer struct{ s *SelectorGif }

func (r *selectorGifRenderer) MinSize() fyne.Size           { return r.s.columna.MinSize() }
func (r *selectorGifRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.s.columna} }
func (r *selectorGifRenderer) Destroy()                     {}
func (r *selectorGifRenderer) Layout(t fyne.Size)           { r.s.columna.Resize(t) }

func (r *selectorGifRenderer) Refresh() {
	s, t := r.s, r.s.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if s.libre != nil && s.libreActiva {
		s.columna.Objects = []fyne.CanvasObject{s.opciones, s.libre}
		s.columna.Refresh()
		return
	}
	subir := ui.NuevoBoton(t.T("gif.upload", nil), func() {
		if s.AlSubir != nil {
			s.AlSubir()
		}
	}).Con(ui.Secundario, ui.TallaSM)
	subir.Cargando, subir.Inerte = s.Subiendo, s.Subiendo
	pie := container.New(filaCentradaRooms{hueco: tema.Espacio(2)}, subir,
		ui.NuevoRotulo(t.T("gif.help", nil), tema.TextoSM, ui.Normal, "text-subtle"))
	hijos := []fyne.CanvasObject{s.buscar, s.caja(v), pie}
	if s.opciones != nil {
		hijos = append([]fyne.CanvasObject{s.opciones}, hijos...)
	}
	if s.FalloSubida != "" {
		hijos = append(hijos, ui.NuevoParrafo(s.FalloSubida, tema.TextoSM, ui.Normal, "danger-text", v))
	}
	s.columna.Objects = hijos
	s.columna.Refresh()
}

// caja es `border-border h-64 overflow-y-auto rounded-md border p-2` con lo que toque:
// la rueda, el fallo con «Reintentar», el vacío o la rejilla (`grid-cols-3 gap-2`).
func (s *SelectorGif) caja(v tema.Variante) fyne.CanvasObject {
	t := s.T
	var dentro fyne.CanvasObject
	gifs := s.Gifs()
	switch {
	case s.Todos == nil && !s.Fallo:
		dentro = container.NewCenter(ui.NuevaRueda(20, "text-muted", v))
	case s.Fallo:
		vac := ui.NuevoVacio(t.T("gif.loadFailed", nil), t.T("error.checkConnection", nil))
		aviso := iconos.Trazo(iconos.Aviso, 36, "danger-text", v)
		aviso.Colocar(0, 0, 36)
		vac.Ilustracion = container.New(tamFijo{fyne.NewSize(36, 36)}, aviso.Objeto())
		vac.Accion = ui.NuevoBoton(t.T("action.retry", nil), s.AlReintentar).Con(ui.Secundario, ui.TallaSM)
		dentro = container.NewCenter(vac)
	case len(gifs) == 0:
		dentro = container.NewCenter(ui.NuevoRotulo(t.T("gif.empty", nil), tema.TextoBase, ui.Normal, "text-muted"))
	default:
		teselas := make([]fyne.CanvasObject, len(gifs))
		for i, g := range gifs {
			tesela := NuevaTeselaGif(g.Name, s.Imagenes[g.FileID], func() {
				if s.AlElegir != nil {
					s.AlElegir(":" + g.Name + ":")
				}
			})
			tesela.Animado = s.Animados[g.FileID]
			teselas[i] = tesela
		}
		dentro = container.NewVScroll(container.New(ui.Margen{Izq: tema.Espacio(2), Arr: tema.Espacio(2), Der: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(ui.Rejilla{Fijas: 3, Hueco: tema.Espacio(2)}, teselas...)))
	}
	// Sin fondo propio: se compone el del diálogo, que un relleno transparente con borde
	// dejaría ver el color del borde por dentro (ver ui.Caja).
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	return container.New(altoFijo(AltoCajaGif), container.New(enCaja{caja}, append(caja.Objetos(), dentro)...))
}

// TeselaGif es un GIF de la rejilla: `border-border hover:border-border-strong rounded-md
// border overflow-hidden`, la imagen (`h-24 object-contain bg-surface-sunken`) y el
// nombre debajo (`px-1.5 py-1 text-xs text-muted truncate`). Su pista es el atajo.
type TeselaGif struct {
	ui.Pulsable
	Rotulo string
	Imagen image.Image
	// Animado es el GIF entero; si está, se pinta él y no Imagen.
	Animado *gif.GIF
}

var _ fyne.Widget = (*TeselaGif)(nil)

// NuevaTeselaGif crea la tesela; sin imagen todavía, el hueco hundido.
func NuevaTeselaGif(nombre string, img image.Image, alPulsar func()) *TeselaGif {
	g := &TeselaGif{Rotulo: nombre, Imagen: img}
	g.Nombre, g.AlPulsar = ":"+nombre+":", alPulsar
	g.ExtendBaseWidget(g)
	return g
}

// AltoTeselaGif es la imagen, la línea del nombre con su `py-1` y los dos bordes.
func AltoTeselaGif() float32 {
	return AltoImagenGif + tema.TextoXS.Linea + 2*tema.Espacio(1) + 2*tema.Borde
}

// CreateRenderer compone la caja, la imagen y el nombre.
func (g *TeselaGif) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &teselaGifRenderer{
		g:      g,
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD),
		fondo:  canvas.NewRectangle(tema.Color("surface-sunken", v)),
		nombre: ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
	}
	r.objs = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objs = append(r.objs, r.fondo)
	switch {
	case g.Animado != nil:
		r.img = ui.NuevoGif(g.Animado)
		r.objs = append(r.objs, r.img)
	case g.Imagen != nil:
		img := canvas.NewImageFromImage(g.Imagen)
		img.FillMode = canvas.ImageFillContain
		r.img = img
		r.objs = append(r.objs, r.img)
	}
	r.objs = append(r.objs, r.nombre)
	r.Refresh()
	return r
}

type teselaGifRenderer struct {
	g      *TeselaGif
	aro    *ui.Aro
	caja   *ui.Caja
	fondo  *canvas.Rectangle
	img    fyne.CanvasObject
	nombre *canvas.Text
	objs   []fyne.CanvasObject
}

func (r *teselaGifRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, AltoTeselaGif()) }
func (r *teselaGifRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *teselaGifRenderer) Destroy()                     {}

func (r *teselaGifRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	dentro := fyne.NewSize(max(t.Width-2*tema.Borde, 0), AltoImagenGif)
	r.fondo.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.fondo.Resize(dentro)
	if r.img != nil {
		r.img.Move(r.fondo.Position())
		r.img.Resize(dentro)
	}
	ui.Truncar(r.nombre, r.g.Rotulo, tema.TextoXS, ui.Normal, max(t.Width-2*tema.Borde-2*tema.Espacio(1.5), 0))
	ui.ColocarTexto(r.nombre, tema.TextoXS, tema.Borde+tema.Espacio(1.5), tema.Borde+AltoImagenGif+tema.Espacio(1))
}

func (r *teselaGifRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := tema.Color("border", v)
	if r.g.Encima() {
		borde = tema.Color("border-strong", v)
	}
	r.caja.Pintar(tema.Color("surface-raised", v), borde)
	r.aro.Ver(r.g.Enfocado())
	r.aro.Pintar(v)
	r.Layout(r.g.Size())
}

// ---- En la Sala ----

// AbrirGif es el «GIF» del menú «+»: el diálogo `md` con el catálogo de la
// organización. Sin Rooms que lo sirva o sin Drive, no se abre.
//
//nolint:gocognit // El diálogo encadena la carga, las miniaturas y la subida en dos pasos.
func (sa *Sala) AbrirGif() {
	ep, ok := sa.S.(EmojiPropio)
	dr, conDrive := sa.Ficheros.(DriveOrganizacion)
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if !ok || !conDrive || lienzo == nil {
		return
	}
	t := sa.P.T
	s := NuevoSelectorGif(t)
	var d *ui.Dialogo
	// Las miniaturas salen de la MISMA caché que los emoji del cuerpo: cada imagen del
	// catálogo se baja una vez por sesión.
	miniaturas := func(es []api.RoomsCustomEmoji) { sa.miniaturasGifOrganizacion(s, es) }
	s.alOrganizacion = func() { miniaturas(s.Todos) }
	cargar := func() {
		s.Todos, s.Fallo = nil, false
		s.Refresh()
		pedir(sa, ep.CustomEmoji, func(es []api.RoomsCustomEmoji, err error) {
			if err != nil {
				s.Fallo = true
				s.Refresh()
				return
			}
			s.Todos = append([]api.RoomsCustomEmoji{}, es...)
			PonerCatalogoOrg(s.Todos)
			miniaturas(es)
		})
	}
	s.AlReintentar = cargar
	s.AlElegir = func(atajo string) {
		if s.alCerrarLibre != nil {
			s.alCerrarLibre()
		}
		sa.gif = nil
		d.Cerrar()
		sa.P.Canal.Compositor.Insertar(atajo)
	}
	s.AlSubir = func() {
		if sa.ElegirFicheros == nil || s.Subiendo {
			return
		}
		pedir(sa, func(context.Context) ([]string, error) { return sa.ElegirFicheros() }, func(rutas []string, err error) {
			if err != nil || len(rutas) == 0 {
				return
			}
			ruta := rutas[0]
			nombre := SlugGif(filepath.Base(ruta))
			if nombre == "" {
				s.FalloSubida = t.T("gif.invalidName", nil)
				s.Refresh()
				return
			}
			s.Subiendo, s.FalloSubida = true, ""
			s.Refresh()
			pedir(sa, func(ctx context.Context) (struct{}, error) { return struct{}{}, subirGif(ctx, ep, dr, ruta, nombre) },
				func(_ struct{}, err error) {
					s.Subiendo = false
					if err != nil {
						s.FalloSubida = t.T("gif.uploadFailed", nil)
						s.Refresh()
						return
					}
					cargar()
				})
		})
	}
	cancelar := ui.NuevoBoton(t.T("action.cancel", nil), func() { d.Descartar() }).Con(ui.Fantasma, ui.TallaMD)
	d = ui.NuevoDialogo(t.T("gif.title", nil), s).Con(ui.DialogoMD, cancelar)
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), s.buscar
	// `AlCerrar` es la PETICIÓN de cierre: sin `d.Cerrar()` ni el aspa, ni Escape, ni el
	// velo cerraban el selector.
	d.AlCerrar = func() {
		if s.alCerrarLibre != nil {
			s.alCerrarLibre()
		}
		sa.gif = nil
		d.Cerrar()
	}
	sa.engancharGifLibre(s, ep, dr, func() {
		if s.alCerrarLibre != nil {
			s.alCerrarLibre()
		}
		sa.gif = nil
		d.Cerrar()
	})
	if s.libre != nil {
		d.Foco = s.libre.query
	}
	sa.gif = s
	d.Mostrar(lienzo)
	cargar()
	if s.alLibre != nil {
		s.alLibre()
	}
}

// TopeGif es lo más que se baja de un GIF para animarlo: el catálogo son GIF de
// reacción, y uno mayor se queda en su miniatura quieta en vez de llenar la memoria.
const TopeGif = 16 << 20

// gifEmoji es el GIF original de un fichero del catálogo, descodificado entero; nil
// mientras llega, si Drive no lo sirve o si no cabe en TopeGif. Se baja una vez por
// sesión, como las miniaturas.
func (sa *Sala) gifEmoji(fichero string) *gif.GIF {
	dr, ok := sa.Ficheros.(DescargaDrive)
	if g := sa.gifsEmoji[fichero]; g != nil || !ok || sa.pidiendoGif[fichero] {
		return g
	}
	sa.pidiendoGif[fichero] = true
	pedir(sa, func(ctx context.Context) (*gif.GIF, error) {
		rc, err := dr.Download(ctx, fichero)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return gif.DecodeAll(io.LimitReader(rc, TopeGif))
	}, func(g *gif.GIF, err error) {
		if err != nil || g == nil || len(g.Image) == 0 {
			return // se queda pedido: no se reintenta en cada repintado
		}
		sa.gifsEmoji[fichero] = g
		if sa.gif != nil {
			sa.gif.Animados[fichero] = g
			sa.gif.Refresh()
		}
	})
	return nil
}

// errNoEsGif es subir algo que no es un GIF: la web solo deja elegir `image/gif`.
var errNoEsGif = errors.New("no es un GIF")

// subirGif sube un GIF: la web solo deja elegir `image/gif`.
func subirGif(ctx context.Context, ep EmojiPropio, dr DriveOrganizacion, ruta, nombre string) error {
	tipo := mime.TypeByExtension(strings.ToLower(filepath.Ext(ruta)))
	if tipo != "image/gif" {
		return errNoEsGif
	}
	_, err := subirImagenOrg(ctx, ep, dr, ruta, tipo, nombre)
	return err
}

// subirImagenOrg sube la imagen a la raíz de Drive —un activo del tenant, no de un
// canal— y la liga al atajo. La usan «GIF» y el «Subir» del selector de emoji.
func subirImagenOrg(ctx context.Context, ep EmojiPropio, dr DriveOrganizacion, ruta, tipo, nombre string) (api.RoomsCustomEmoji, error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: la ruta la eligió la persona en el selector del sistema
	if err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	raiz, err := dr.Root(ctx)
	if err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	subido, err := dr.Upload(ctx, raiz.ID, filepath.Base(ruta), tipo, f, info.Size())
	if err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	return ep.CreateCustomEmoji(ctx, nombre, subido.ID)
}

func (sa *Sala) miniaturasGifOrganizacion(s *SelectorGif, es []api.RoomsCustomEmoji) {
	if s.libreActiva {
		return
	}
	for _, e := range es {
		if e.ContentType != "image/gif" {
			continue
		}
		if img := sa.imagenEmoji(e.FileID); img != nil {
			s.Imagenes[e.FileID] = img
		}
		if g := sa.gifEmoji(e.FileID); g != nil {
			s.Animados[e.FileID] = g
		}
	}
	s.Refresh()
}
