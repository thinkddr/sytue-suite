// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image"
	"io"
	"net/url"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/LinkPreview.tsx (`LinkPreview` y
// `MeetingLinkCard`), `extractLinks` de MessageBody.tsx y lib/enlacesExternos.ts.
//
// Bajo un mensaje van las tarjetas de sus dos primeros enlaces: la vista previa
// (OpenGraph, pedida al servicio, que la cachea y la protege de SSRF) o, si es una
// reunión de Zoom, Meet, Teams o Webex, la tarjeta de «Unirse». Una vista previa sin
// título ni imagen no se pinta.

// EnlacesDe es `extractLinks`: los dos primeros enlaces distintos del cuerpo, también
// dentro de títulos, citas y listas.
func EnlacesDe(cuerpo string, tope int) []string {
	var out []string
	var andar func([]Nodo)
	andar = func(ns []Nodo) {
		for _, n := range ns {
			if len(out) >= tope {
				return
			}
			switch n.Tipo {
			case NodoEnlace:
				if !contiene(out, n.Enlace) {
					out = append(out, n.Enlace)
				}
			case NodoTitulo, NodoCita:
				andar(n.Hijos)
			case NodoLista, NodoNumerada:
				for _, it := range n.Items {
					andar(it)
				}
			case NodoTexto, NodoNegrita, NodoCursiva, NodoTachado, NodoCodigo, NodoBloque:
			}
		}
	}
	andar(ParsearCuerpo(cuerpo))
	return out
}

// Los proveedores de reunión externos (son marcas: no se traducen).
var nombresProveedor = map[string]string{"zoom": "Zoom", "meet": "Google Meet", "teams": "Microsoft Teams", "webex": "Webex"}

var (
	reZoom  = regexp.MustCompile(`/(j|my|s|w)/`)
	reMeet  = regexp.MustCompile(`^/[a-z]{3}-[a-z]{4}-[a-z]{3}$`)
	reWebex = regexp.MustCompile(`/(meet|join|j\.php)`)
)

// ProveedorDeReunion es `proveedorDeReunion`: solo https y solo los hosts conocidos,
// con la forma de ruta de una reunión.
func ProveedorDeReunion(crudo string) string {
	u, err := url.Parse(strings.TrimSpace(crudo))
	if err != nil || u.Scheme != "https" {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	switch {
	case h == "zoom.us" || strings.HasSuffix(h, ".zoom.us"):
		if reZoom.MatchString(u.Path) {
			return "zoom"
		}
	case h == "meet.google.com":
		if reMeet.MatchString(u.Path) {
			return "meet"
		}
	case h == "teams.microsoft.com" || h == "teams.live.com":
		if strings.Contains(u.Path, "/meetup-join/") || strings.HasPrefix(u.Path, "/meet/") {
			return "teams"
		}
	case h == "webex.com" || strings.HasSuffix(h, ".webex.com"):
		if reWebex.MatchString(u.Path) {
			return "webex"
		}
	}
	return ""
}

// Las medidas de LinkPreview.tsx. HOJA.
var (
	// AnchoTarjetaEnlace es el `max-w-xl` (36 rem), RellenoTarjetaEnlace su `p-2.5`,
	// HuecoTarjetaEnlace su `gap-3` y AireTarjetaEnlace su `mt-1.5`.
	AnchoTarjetaEnlace   float32 = 36 * tema.Rem
	RellenoTarjetaEnlace         = tema.Espacio(2.5)
	HuecoTarjetaEnlace           = tema.Espacio(3)
	AireTarjetaEnlace            = tema.Espacio(1.5)
	// LadoImagenEnlace es el `h-16 w-16` de la imagen.
	LadoImagenEnlace = tema.Espacio(16)
	// escalonSnug es `text-xs leading-snug` de la descripción.
	escalonSnug = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: tema.TextoXS.Tam * 1.375}
)

// TarjetaEnlace es una tarjeta bajo el mensaje: la vista previa de un enlace o, con
// Proveedor, la de una reunión externa.
type TarjetaEnlace struct {
	ui.Pulsable
	T *i18n.Translator
	// Vista es la vista previa; Proveedor, el de una reunión ("" si no lo es).
	Vista     api.RoomsUnfurl
	Proveedor string
	Imagen    image.Image
	objs      []fyne.CanvasObject
}

var _ desktop.Hoverable = (*TarjetaEnlace)(nil)

// NuevaTarjetaEnlace crea la tarjeta; `alPulsar` abre el enlace.
func NuevaTarjetaEnlace(t *i18n.Translator, v api.RoomsUnfurl, proveedor string, img image.Image, alPulsar func()) *TarjetaEnlace {
	c := &TarjetaEnlace{T: t, Vista: v, Proveedor: proveedor, Imagen: img}
	c.Nombre = t.T("link.preview", nil)
	switch {
	case proveedor != "":
		c.Nombre = t.T("link.join", nil) + ": " + c.tituloReunion()
	case v.Title != "":
		c.Nombre = t.T("link.previewOf", i18n.Vars{"title": v.Title})
	}
	c.AlPulsar = alPulsar
	c.ExtendBaseWidget(c)
	return c
}

func (c *TarjetaEnlace) tituloReunion() string {
	return c.T.T("link.joinMeeting", i18n.Vars{"provider": nombresProveedor[c.Proveedor]})
}

// sitio es `siteName || hostOf(url)`.
func (c *TarjetaEnlace) sitio() string {
	if c.Vista.SiteName != "" {
		return c.Vista.SiteName
	}
	if u, err := url.Parse(c.Vista.URL); err == nil {
		return strings.TrimPrefix(u.Hostname(), "www.")
	}
	return ""
}

// lineasDescripcion es la descripción partida en su `line-clamp-2`.
func (c *TarjetaEnlace) lineasDescripcion(cabe float32) []string {
	if c.Vista.Description == "" {
		return nil
	}
	ls := ui.Repartir(c.Vista.Description, escalonSnug, ui.Normal, cabe)
	if len(ls) > 2 {
		ls = ls[:2]
		ls[1] += "…"
	}
	return ls
}

// MedidaTarjetaEnlace es lo que ocupa a ese ancho.
func MedidaTarjetaEnlace(c *TarjetaEnlace, ancho float32) fyne.Size {
	ancho = min(ancho, AnchoTarjetaEnlace)
	borde := 2*tema.Borde + 2*RellenoTarjetaEnlace
	if c.Proveedor != "" {
		return fyne.NewSize(ancho, borde+CajaFichero)
	}
	alto := float32(0)
	if c.sitio() != "" {
		alto += escalonMeta.Linea
	}
	if c.Vista.Title != "" {
		alto += tema.Espacio(0.5) + tema.TextoSM.Linea
	}
	if ls := c.lineasDescripcion(c.cabeTexto(ancho)); len(ls) > 0 {
		alto += tema.Espacio(0.5) + float32(len(ls))*escalonSnug.Linea
	}
	if c.Imagen != nil {
		alto = max(alto, LadoImagenEnlace)
	}
	return fyne.NewSize(ancho, borde+alto)
}

func (c *TarjetaEnlace) cabeTexto(ancho float32) float32 {
	cabe := min(ancho, AnchoTarjetaEnlace) - 2*tema.Borde - 2*RellenoTarjetaEnlace
	if c.Imagen != nil {
		cabe -= LadoImagenEnlace + HuecoTarjetaEnlace
	}
	return max(cabe, 0)
}

func (c *TarjetaEnlace) CreateRenderer() fyne.WidgetRenderer { return &tarjetaEnlaceRenderer{c: c} }

type tarjetaEnlaceRenderer struct{ c *TarjetaEnlace }

func (r *tarjetaEnlaceRenderer) MinSize() fyne.Size {
	return MedidaTarjetaEnlace(r.c, r.c.Size().Width)
}
func (r *tarjetaEnlaceRenderer) Objects() []fyne.CanvasObject { return r.c.objs }
func (r *tarjetaEnlaceRenderer) Destroy()                     {}
func (r *tarjetaEnlaceRenderer) Refresh()                     { r.Layout(r.c.Size()) }

func (r *tarjetaEnlaceRenderer) Layout(t fyne.Size) {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t.Width = min(t.Width, AnchoTarjetaEnlace)
	fondo := "surface"
	if c.Encima() {
		fondo = "bg-subtle"
	}
	caja := ui.NuevaCaja(tema.Color(fondo, v), tema.Color("border", v), tema.RadioMD)
	ui.MoverCaja(caja, fyne.Position{}, t)
	c.objs = caja.Objetos()
	x, y := tema.Borde+RellenoTarjetaEnlace, tema.Borde+RellenoTarjetaEnlace
	if c.Proveedor != "" {
		ic := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v), tema.RadioMD)
		ui.MoverCaja(ic, fyne.NewPos(x, y), fyne.NewSize(CajaFichero, CajaFichero))
		g := dibujo(glifoVideo, 18, "text-muted", v).Objeto()
		g.Move(fyne.NewPos(x+(CajaFichero-18)/2, y+(CajaFichero-18)/2))
		g.Resize(fyne.NewSize(18, 18))
		c.objs = append(c.objs, ic.Objetos()...)
		c.objs = append(c.objs, g)
		unirse := c.T.T("link.join", nil)
		anchoUnirse := ui.Medir(unirse, tema.TextoSM, ui.SemiNegrita).Width
		der := t.Width - tema.Borde - RellenoTarjetaEnlace
		u := ui.Texto(unirse, tema.TextoSM, ui.SemiNegrita, tema.Color("primary-text", v))
		ui.ColocarTexto(u, tema.TextoSM, der-anchoUnirse, (t.Height-tema.TextoSM.Linea)/2)
		tx := ui.Texto("", tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
		xt := x + CajaFichero + HuecoTarjetaEnlace
		ui.Truncar(tx, c.tituloReunion(), tema.TextoSM, ui.SemiNegrita, max(der-anchoUnirse-HuecoTarjetaEnlace-xt, 0))
		ui.ColocarTexto(tx, tema.TextoSM, xt, (t.Height-tema.TextoSM.Linea)/2)
		c.objs = append(c.objs, tx, u)
	} else {
		cabe := c.cabeTexto(t.Width)
		linea := func(s string, e tema.Escalon, p ui.Peso, token string) {
			tx := ui.Texto("", e, p, tema.Color(token, v))
			ui.Truncar(tx, s, e, p, cabe)
			ui.ColocarTexto(tx, e, x, y)
			c.objs = append(c.objs, tx)
			y += e.Linea
		}
		if s := c.sitio(); s != "" {
			linea(s, escalonMeta, ui.Normal, "text-subtle")
		}
		if c.Vista.Title != "" {
			y += tema.Espacio(0.5)
			linea(c.Vista.Title, tema.TextoSM, ui.SemiNegrita, "text")
		}
		if ls := c.lineasDescripcion(cabe); len(ls) > 0 {
			y += tema.Espacio(0.5)
			for _, l := range ls {
				linea(l, escalonSnug, ui.Normal, "text-muted")
			}
		}
		if c.Imagen != nil {
			im := canvas.NewImageFromImage(c.Imagen)
			im.FillMode = canvas.ImageFillCover // `object-cover`
			im.CornerRadius = tema.RadioSM
			im.Move(fyne.NewPos(t.Width-tema.Borde-RellenoTarjetaEnlace-LadoImagenEnlace, tema.Borde+RellenoTarjetaEnlace))
			im.Resize(fyne.NewSize(LadoImagenEnlace, LadoImagenEnlace))
			c.objs = append(c.objs, im)
		}
	}
	aro := ui.NuevoAro(tema.RadioMD, ui.SinHueco, v)
	aro.Colocar(t)
	aro.Ver(c.Enfocado())
	c.objs = append(c.objs, aro.Objetos()...)
}

// ---- En la Sala ----

// vistaEnlace es lo que se sabe de un enlace: la vista (nil si aún no ha llegado o no
// tiene nada que pintar) y su imagen.
type vistaEnlace struct {
	vista  *api.RoomsUnfurl
	imagen image.Image
}

// Enlazador es lo que Rooms pide para las vistas previas; lo cumple *api.Rooms.
type Enlazador interface {
	Unfurl(ctx context.Context, enlace string) (api.RoomsUnfurl, error)
	UnfurlImage(ctx context.Context, ruta string) (io.ReadCloser, error)
}

// tarjetasEnlace son las del mensaje: la de reunión en el acto, y la vista previa si ya
// ha llegado; la que falta se pide UNA vez y, al llegar, se rehace el historial.
func (sa *Sala) tarjetasEnlace(m Mensaje) []fyne.CanvasObject {
	if m.Enviando || m.Fallido || m.Cuerpo == "" {
		return nil
	}
	var out []fyne.CanvasObject
	for _, e := range EnlacesDe(m.Cuerpo, 2) {
		abrir := func() {
			if u, err := url.Parse(e); err == nil && sa.AbrirEnlace != nil {
				_ = sa.AbrirEnlace(u)
			}
		}
		if p := ProveedorDeReunion(e); p != "" {
			out = append(out, NuevaTarjetaEnlace(sa.P.T, api.RoomsUnfurl{URL: e}, p, nil, abrir))
			continue
		}
		ve, pedida := sa.enlaces[e]
		if !pedida {
			sa.pedirVista(e)
			continue
		}
		if ve.vista != nil {
			out = append(out, NuevaTarjetaEnlace(sa.P.T, *ve.vista, "", ve.imagen, abrir))
		}
	}
	return out
}

// pedirVista pide la vista previa y su imagen; lo que no tiene título ni imagen se queda
// sin tarjeta, en silencio, como la web.
func (sa *Sala) pedirVista(e string) {
	sa.enlaces[e] = vistaEnlace{}
	en, ok := sa.S.(Enlazador)
	if !ok {
		return
	}
	sa.Lanzar(func() {
		v, err := en.Unfurl(sa.ctx, e)
		if err != nil || (strings.TrimSpace(v.Title) == "" && v.ImageURL == "") {
			return
		}
		var img image.Image
		if v.ImageURL != "" {
			if rc, err := en.UnfurlImage(sa.ctx, v.ImageURL); err == nil {
				img, _, _ = image.Decode(rc)
				_ = rc.Close()
			}
		}
		fyne.Do(func() {
			sa.enlaces[e] = vistaEnlace{vista: &v, imagen: img}
			sa.Esperar(0, sa.P.Canal.Historial.Refresh)
		})
	})
}
