// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/gif"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// CatalogoGifLibre usa el mismo catálogo comprobado y servido por Rooms que la web.
type CatalogoGifLibre interface {
	FreeGifs(context.Context, string, string) (api.RoomsFreeGifPage, error)
	FreeGifContent(context.Context, int64, bool) (io.ReadCloser, error)
}

var _ CatalogoGifLibre = (*api.Rooms)(nil)

// SelectorGifLibre solo conserva una página y sus miniaturas quietas. Abrir una
// tesela muestra la animación elegida, con el control común de menos movimiento.
type SelectorGifLibre struct {
	widget.BaseWidget
	T            *i18n.Translator
	Page         *api.RoomsFreeGifPage
	Failed       bool
	Busy         bool
	ImportFailed bool
	Images       map[int64]image.Image
	Cursors      []string
	Search       func(string, string)
	Choose       func(api.RoomsFreeGif)
	query        *ui.Entrada
	column       *fyne.Container
}

func nuevoSelectorGifLibre(t *i18n.Translator) *SelectorGifLibre {
	s := &SelectorGifLibre{T: t, Images: map[int64]image.Image{}, Cursors: []string{""}}
	s.ExtendBaseWidget(s)
	s.query = ui.NuevaEntrada(t.T("gif.search", nil))
	s.query.AlCambiar(func(string) {
		s.Cursors = []string{""}
		if s.Search != nil {
			s.Search(s.query.Texto(), "")
		}
	})
	return s
}

// CreateRenderer compone búsqueda, licencia, rejilla y navegación paginada.
func (s *SelectorGifLibre) CreateRenderer() fyne.WidgetRenderer {
	s.column = container.New(ui.Columna{Hueco: tema.Espacio(3)})
	r := &selectorGifLibreRenderer{s: s}
	r.Refresh()
	return r
}

type selectorGifLibreRenderer struct{ s *SelectorGifLibre }

func (r *selectorGifLibreRenderer) MinSize() fyne.Size { return r.s.column.MinSize() }
func (r *selectorGifLibreRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.s.column}
}
func (r *selectorGifLibreRenderer) Destroy()              {}
func (r *selectorGifLibreRenderer) Layout(size fyne.Size) { r.s.column.Resize(size) }
func (r *selectorGifLibreRenderer) Refresh() {
	s, t := r.s, r.s.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	back := ui.NuevoBoton(t.T("action.back", nil), func() {
		s.Cursors = s.Cursors[:len(s.Cursors)-1]
		s.Search(s.query.Texto(), s.Cursors[len(s.Cursors)-1])
	}).Con(ui.Fantasma, ui.TallaSM)
	back.Inerte = len(s.Cursors) < 2 || s.Page == nil || s.Busy
	next := ui.NuevoBoton(t.T("action.loadMore", nil), func() { s.Cursors = append(s.Cursors, s.Page.Next); s.Search(s.query.Texto(), s.Page.Next) }).Con(ui.Secundario, ui.TallaSM)
	next.Inerte = s.Page == nil || s.Page.Next == "" || s.Busy
	hijos := []fyne.CanvasObject{s.query, ui.NuevoParrafo(t.T("gif.freeHelp", nil), tema.TextoSM, ui.Normal, "text-muted", v), s.grid(v), container.New(filaCentradaRooms{hueco: tema.Espacio(2)}, back, next)}
	if s.ImportFailed {
		hijos = append(hijos, ui.NuevoParrafo(t.T("gif.uploadFailed", nil), tema.TextoSM, ui.Normal, "danger-text", v))
	}
	s.column.Objects = hijos
	s.column.Refresh()
}

func (s *SelectorGifLibre) grid(v tema.Variante) fyne.CanvasObject {
	var body fyne.CanvasObject
	switch {
	case s.Failed:
		body = container.NewCenter(ui.NuevoBoton(s.T.T("action.retry", nil), func() { s.Search(s.query.Texto(), s.Cursors[len(s.Cursors)-1]) }).Con(ui.Secundario, ui.TallaSM))
	case s.Page == nil:
		body = container.NewCenter(ui.NuevaRueda(20, "text-muted", v))
	case len(s.Page.Items) == 0:
		body = container.NewCenter(ui.NuevoRotulo(s.T.T("search.noResults", nil), tema.TextoBase, ui.Normal, "text-muted"))
	default:
		tiles := make([]fyne.CanvasObject, 0, len(s.Page.Items))
		for _, g := range s.Page.Items {
			tile := NuevaTeselaGif(g.Title, s.Images[g.ID], func() {
				if !s.Busy && s.Choose != nil {
					s.Choose(g)
				}
			})
			tile.Nombre = s.T.T("gif.preview", nil) + ": " + g.Title
			tiles = append(tiles, tile)
		}
		body = container.NewVScroll(container.New(ui.Rejilla{Fijas: 3, Hueco: tema.Espacio(2)}, tiles...))
	}
	return container.New(altoFijo(AltoCajaGif), body)
}

// engancharGifLibre usa los clientes existentes de Rooms/Drive para importar al
// catálogo del tenant. Las respuestas tardías se descartan al buscar/cerrar.
func (sa *Sala) engancharGifLibre(parent *SelectorGif, ep EmojiPropio, dr DriveOrganizacion, closePicker func()) {
	client, ok := sa.S.(CatalogoGifLibre)
	if !ok {
		return
	}
	pickerCtx, cancelPicker := context.WithCancel(sa.ctx)
	s := nuevoSelectorGifLibre(parent.T)
	parent.libre = s
	var sequence uint64
	var timer *time.Timer
	var cancel context.CancelFunc
	s.Search = func(query, cursor string) {
		sequence++
		current := sequence
		s.Page = nil
		s.Failed = false
		s.Images = map[int64]image.Image{}
		s.Refresh()
		if timer != nil {
			timer.Stop()
		}
		if cancel != nil {
			cancel()
		}
		ctx, stop := context.WithCancel(sa.ctx)
		cancel = stop
		timer = time.AfterFunc(300*time.Millisecond, func() {
			fyne.Do(func() {
				if sa.gif != parent || sequence != current {
					return
				}
				pedirEn(ctx, sa, func(ctx context.Context) (api.RoomsFreeGifPage, error) { return client.FreeGifs(ctx, query, cursor) }, func(page api.RoomsFreeGifPage, err error) {
					if sa.gif != parent || sequence != current {
						return
					}
					if err != nil {
						s.Failed = true
						s.Refresh()
						return
					}
					s.Page = &page
					s.Refresh()
					sa.miniaturasLibres(ctx, parent, s, client, page.Items, current, func() uint64 { return sequence })
				})
			})
		})
	}
	s.Choose = func(g api.RoomsFreeGif) { sa.previewGifLibre(pickerCtx, parent, s, client, ep, dr, g, closePicker) }
	parent.alCerrarLibre = func() { cancelPicker(); sequence++; pararBusquedaGif(timer, cancel) }
	parent.alLibre = func() {
		if s.Page == nil && !s.Failed {
			s.Search(s.query.Texto(), s.Cursors[len(s.Cursors)-1])
		}
	}
	parent.libreActiva = true
	parent.opciones = ui.NuevoSegmentado(parent.T.T("gif.title", nil), []string{parent.T.T("gif.free", nil), parent.T.T("gif.organization", nil)}, func(index int) {
		parent.libreActiva = index == 0
		parent.Refresh()
		if parent.libreActiva {
			parent.alLibre()
		} else if parent.alOrganizacion != nil {
			parent.alOrganizacion()
		}
	})
}

func (sa *Sala) miniaturasLibres(ctx context.Context, parent *SelectorGif, s *SelectorGifLibre, client CatalogoGifLibre, items []api.RoomsFreeGif, current uint64, sequence func() uint64) {
	// Una tanda secuencial evita 24 descargas simultáneas al abrir la página.
	pedirEn(ctx, sa, func(ctx context.Context) (map[int64]image.Image, error) {
		images := make(map[int64]image.Image, len(items))
		for _, g := range items {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			rc, err := client.FreeGifContent(ctx, g.ID, true)
			if err != nil {
				continue
			}
			img, _, err := image.Decode(io.LimitReader(rc, 1<<20))
			_ = rc.Close()
			if err != nil {
				continue
			}
			images[g.ID] = img
		}
		return images, nil
	}, func(images map[int64]image.Image, _ error) {
		if sa.gif == parent && sequence() == current {
			s.Images = images
			s.Refresh()
		}
	})
}

type gifLibrePreview struct {
	data      []byte
	animation *gif.GIF
}

func cargarGifLibre(ctx context.Context, client CatalogoGifLibre, id int64) (gifLibrePreview, error) {
	rc, err := client.FreeGifContent(ctx, id, false)
	if err != nil {
		return gifLibrePreview{}, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, TopeGif+1))
	if err != nil {
		return gifLibrePreview{}, err
	}
	if len(data) > TopeGif {
		return gifLibrePreview{}, io.ErrShortBuffer
	}
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return gifLibrePreview{}, err
	}
	return gifLibrePreview{data: data, animation: animation}, nil
}

func (sa *Sala) previewGifLibre(ctx context.Context, parent *SelectorGif, s *SelectorGifLibre, client CatalogoGifLibre, ep EmojiPropio, dr DriveOrganizacion, g api.RoomsFreeGif, closePicker func()) {
	s.Busy = true
	s.ImportFailed = false
	s.Refresh()
	parentCanvas := fyne.CurrentApp().Driver().CanvasForObject(parent)
	pedirEn(ctx, sa, func(ctx context.Context) (gifLibrePreview, error) { return cargarGifLibre(ctx, client, g.ID) }, func(loaded gifLibrePreview, err error) {
		if ctx.Err() != nil || sa.gif != parent {
			return
		}
		s.Busy = false
		s.Refresh()
		if err != nil {
			s.ImportFailed = true
			s.Refresh()
			return
		}
		var preview *ui.Dialogo
		cancel := ui.NuevoBoton(parent.T.T("action.cancel", nil), func() { preview.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
		add := ui.NuevoBoton(parent.T.T("action.add", nil), func() {
			preview.Cerrar()
			s.Busy = true
			s.Refresh()
			pedirEn(ctx, sa, func(ctx context.Context) (api.RoomsCustomEmoji, error) {
				return importarGifLibre(ctx, ep, dr, g, loaded.data)
			}, func(e api.RoomsCustomEmoji, err error) {
				if ctx.Err() != nil || sa.gif != parent {
					return
				}
				s.Busy = false
				if err != nil {
					s.ImportFailed = true
					s.Refresh()
					return
				}
				parent.recordarGifLibre(e)
				closePicker()
				sa.P.Canal.Compositor.Insertar(":" + e.Name + ":")
			})
		}).Con(ui.Primario, ui.TallaMD)
		origin := ui.NuevoEnlace(g.License+" · Wikimedia Commons", tema.TextoSM, ui.Normal, "primary-text", func() {
			if sa.AbrirEnlace != nil {
				u, parseErr := url.Parse(g.Source)
				if parseErr == nil {
					_ = sa.AbrirEnlace(u)
				}
			}
		})
		body := container.New(ui.Columna{Hueco: tema.Espacio(2)}, container.New(altoFijo(AltoCajaGif), ui.NuevoGif(loaded.animation)), origin)
		preview = ui.NuevoDialogo(g.Title, body).Con(ui.DialogoSM, container.New(filaCentradaRooms{hueco: tema.Espacio(2)}, cancel, add))
		preview.AlCerrar = preview.Cerrar
		preview.Mostrar(parentCanvas)
	})
}

func (s *SelectorGif) recordarGifLibre(e api.RoomsCustomEmoji) {
	for _, known := range s.Todos {
		if known.Name == e.Name {
			return
		}
	}
	s.Todos = append(s.Todos, e)
	PonerCatalogoOrg(s.Todos)
}

func gifLibreExistente(ctx context.Context, ep EmojiPropio, name string) (api.RoomsCustomEmoji, bool, error) {
	if err := ctx.Err(); err != nil {
		return api.RoomsCustomEmoji{}, false, err
	}
	es, err := ep.CustomEmoji(ctx)
	if err != nil {
		return api.RoomsCustomEmoji{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return api.RoomsCustomEmoji{}, false, err
	}
	for _, e := range es {
		if e.Name == name {
			return e, true, nil
		}
	}
	return api.RoomsCustomEmoji{}, false, nil
}

func importarGifLibre(ctx context.Context, ep EmojiPropio, dr DriveOrganizacion, g api.RoomsFreeGif, data []byte) (api.RoomsCustomEmoji, error) {
	name := "commons-" + strconv.FormatInt(g.ID, 10)
	existing, found, err := gifLibreExistente(ctx, ep, name)
	if err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	if found {
		return existing, nil
	}
	root, err := dr.Root(ctx)
	if err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	if err := ctx.Err(); err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	// Drive reemplaza por nombre. Un UUID evita tocar un archivo previo del equipo.
	file, err := dr.Upload(ctx, root.ID, name+"-"+uuid.NewString()+".gif", "image/gif", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		existing, found, listErr := gifLibreExistente(ctx, ep, name)
		if listErr == nil && found {
			return existing, nil
		}
		return api.RoomsCustomEmoji{}, err
	}
	if err := ctx.Err(); err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	e, err := ep.CreateCustomEmoji(ctx, name, file.ID)
	if ctx.Err() != nil {
		return api.RoomsCustomEmoji{}, ctx.Err()
	}
	var conflict *api.Error
	if !errors.As(err, &conflict) || conflict.Status != http.StatusConflict {
		return e, err
	}
	existing, found, listErr := gifLibreExistente(ctx, ep, name)
	if listErr != nil {
		return api.RoomsCustomEmoji{}, listErr
	}
	if !found {
		return e, err
	}
	// Solo retiramos la subida propia perdedora; nunca el activo reutilizado.
	if existing.FileID != file.ID {
		_ = dr.Delete(ctx, api.DriveFile, file.ID)
	}
	if err := ctx.Err(); err != nil {
		return api.RoomsCustomEmoji{}, err
	}
	return existing, nil
}

func pararBusquedaGif(timer *time.Timer, cancel context.CancelFunc) {
	if timer != nil {
		timer.Stop()
	}
	if cancel != nil {
		cancel()
	}
}
