// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// paridad-web: apps/frontend/apps/rooms/src/pages/share.astro
//
// «Compartir en Rooms»: la pantalla a la que llega otro producto de la suite —Drive,
// Mail, Calendar— para mandar algo a un canal.
//
// En nativo el camino de entrada cambia y el de la web no se copia: allí es una URL
// con `?source=drive`, y aquí es el ESQUEMA de la aplicación
// (`sytue-rooms://share?source=drive`), que es lo que dice §13.6 sobre cómo se llaman
// entre sí los productos. La pantalla es la misma.
//
// Medido en `rooms/compartir.ancho.json` (1280×800, rem 14): 69 elementos, la columna
// centrada a `max-w-xl` con `p-6` y `gap-4`.
var (
	// AnchoCompartir es el `max-w-xl` de la columna: 36rem con el rem de 14.
	AnchoCompartir float32 = 36 * tema.Rem
	// CanalonCompartir es su `p-6` y HuecoCompartir su `gap-4`.
	CanalonCompartir = tema.Espacio(6)
	HuecoCompartir   = tema.Espacio(4)
	// AltoDestino es la fila de un destino: `px-2 py-2 gap-3` con un avatar de 32.
	AltoDestino = 32 + 2*tema.Espacio(2)
	// HuecoDestinos es el `gap-0.5` entre filas.
	HuecoDestinos = tema.Espacio(0.5)
)

// Compartir es la pantalla de compartir, en sus seis estados.
type Compartir struct {
	widget.BaseWidget

	T      *i18n.Translator
	Locale i18n.Locale

	// Origen es de qué producto llega lo que se comparte, con la clave de
	// `share.source.*`: `drive`, `mail`, `calendar`, `contacts` o `generic`.
	Origen string
	Estado Estado
	// Canales son los destinos que se ofrecen, ya filtrados por la búsqueda.
	Canales []Canal
	// Enviando es el destino al que se está mandando; vacío, ninguno.
	Enviando string
	// Texto es el mensaje propuesto por quien comparte (el nombre y el enlace del
	// recurso), ya escrito en la caja y editable.
	Texto string

	AlEscribir   func(string)
	AlBuscar     func(string)
	AlCompartir  func(Canal)
	AlReintentar func()

	consulta string
	fondo    *canvas.Rectangle
	cuerpo   *fyne.Container
	objetos  []fyne.CanvasObject
}

var _ fyne.Widget = (*Compartir)(nil)

// NuevoCompartir crea la pantalla con su traductor.
func NuevoCompartir(t *i18n.Translator, l i18n.Locale, origen string) *Compartir {
	c := &Compartir{T: t, Locale: l, Origen: origen}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el título de la pantalla.
func (c *Compartir) AccessibilityLabel() string { return c.T.T("share.title", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (c *Compartir) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// claveOrigen es `SOURCE_KEY`: de dónde viene lo compartido. Un origen desconocido cae
// en el genérico, como en la web.
func claveOrigen(origen string) string {
	switch origen {
	case "drive", "mail", "calendar", "contacts":
		return "share.source." + origen
	}
	return "share.source.generic"
}

// CreateRenderer compone la columna.
func (c *Compartir) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := c.T
	c.fondo = canvas.NewRectangle(tema.Color("bg", v))

	titulo := ui.NuevoRotulo(t.T("share.title", nil), tema.TextoXL, ui.SemiNegrita, "text")
	intro := ui.NuevoRotulo(
		t.T("share.intro", i18n.Vars{"what": t.T(claveOrigen(c.Origen), nil)}),
		tema.TextoBase, ui.Normal, "text-muted")

	mensaje := ui.NuevaAreaTexto(t.T("share.messageAria", nil))
	mensaje.Filas = 3 // `rows={3}` de la web
	mensaje.PonerTexto(c.Texto)
	mensaje.AlCambiar(func(s string) {
		if r := []rune(s); len(r) > TopeCompartir {
			mensaje.PonerTexto(string(r[:TopeCompartir]))
			return
		}
		vacio := strings.TrimSpace(c.Texto) == ""
		c.Texto = s
		if vacio != (strings.TrimSpace(s) == "") {
			c.Refresh() // la lista se apaga o se enciende con el mensaje
		}
		if c.AlEscribir != nil {
			c.AlEscribir(s)
		}
	})
	buscar := ui.NuevaEntrada(t.T("search.channelOrPerson", nil)).Con(ui.CampoMD)
	buscar.AlCambiar(func(s string) {
		c.consulta = s
		if c.AlBuscar != nil {
			c.AlBuscar(s)
		}
	})

	c.cuerpo = container.New(ui.Columna{Hueco: HuecoCompartir},
		titulo, intro, mensaje, buscar, c.destinos())
	c.objetos = []fyne.CanvasObject{c.fondo, c.cuerpo}
	r := &compartirRenderer{c: c}
	r.Refresh()
	return r
}

// destinos es la lista, en sus estados. Son cuatro pinturas distintas y no una lista
// vacía cuatro veces.
func (c *Compartir) destinos() fyne.CanvasObject {
	t := c.T
	switch c.Estado {
	case Cargando:
		return ui.NuevoRotulo(t.T("share.loadingChannels", nil), tema.TextoSM, ui.Normal, "text-muted")
	case Error, SinConexion:
		vac := ui.NuevoVacio(t.T("browse.loadFailed", nil), t.T("error.checkConnection", nil))
		vac.Accion = ui.NuevoBoton(t.T("action.retry", nil), c.AlReintentar).
			Con(ui.Secundario, ui.TallaSM)
		return vac
	case Vacio, SinPermiso:
		return ui.NuevoRotulo(t.T("share.noChannels", nil), tema.TextoSM, ui.Normal, "text-muted")
	case Listo:
	}
	filas := make([]fyne.CanvasObject, 0, len(c.Canales)+1)
	sinTexto := strings.TrimSpace(c.Texto) == ""
	if len(c.Canales) == 0 {
		clave := "share.noChannels"
		if strings.TrimSpace(c.consulta) != "" {
			clave = "forward.emptyQuery"
		}
		filas = append(filas, container.New(ui.Margen{Arr: tema.Espacio(8), Abj: tema.Espacio(8)},
			container.NewCenter(ui.NuevoRotulo(t.T(clave, nil), tema.TextoSM, ui.Normal, "text-muted"))))
	}
	for _, canal := range c.Canales {
		// `disabled={sendingId !== null || !canShare}`, y la rueda en el que se envía.
		enviando := c.Enviando
		if sinTexto && enviando == "" {
			enviando = "-"
		}
		f := NuevaFilaDestino(canal, enviando, func(k Canal) {
			if c.Enviando == "" && c.AlCompartir != nil {
				c.AlCompartir(k)
			}
		})
		f.AlfaInerte = 0.5 // `disabled:opacity-50`, y no el 60 de «Reenviar»
		filas = append(filas, f)
	}
	if sinTexto {
		filas = append(filas, ui.NuevoRotulo(t.T("share.writeSomething", nil), escalonMeta, ui.Normal, "text-subtle"))
	}
	return container.New(ui.Columna{Hueco: HuecoDestinos}, filas...)
}

// TopeCompartir es el `MAX_BODY` de la pantalla.
const TopeCompartir = 8000

type compartirRenderer struct{ c *Compartir }

// MinSize es la columna con su `p-6`.
func (r *compartirRenderer) MinSize() fyne.Size {
	return fyne.NewSize(AnchoCompartir+2*CanalonCompartir,
		r.c.cuerpo.MinSize().Height+2*CanalonCompartir)
}

func (r *compartirRenderer) Objects() []fyne.CanvasObject { return r.c.objetos }
func (r *compartirRenderer) Destroy()                     {}

// Layout centra la columna (`mx-auto max-w-xl p-6`).
func (r *compartirRenderer) Layout(t fyne.Size) {
	c := r.c
	c.fondo.Resize(t)
	ancho := min(AnchoCompartir, max(t.Width-2*CanalonCompartir, 0))
	c.cuerpo.Move(fyne.NewPos((t.Width-ancho)/2, CanalonCompartir))
	c.cuerpo.Resize(fyne.NewSize(ancho, max(t.Height-2*CanalonCompartir, 0)))
}

// Refresh repinta el fondo, rehace la lista de destinos —cambia con la búsqueda, con la
// carga y con el mensaje— y rehace la disposición.
func (r *compartirRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if n := len(r.c.cuerpo.Objects); n > 0 {
		r.c.cuerpo.Objects[n-1] = r.c.destinos()
		r.c.cuerpo.Refresh()
	}
	r.c.fondo.FillColor = tema.Color("bg", v)
	r.c.fondo.Refresh()
	r.Layout(r.c.Size())
}
