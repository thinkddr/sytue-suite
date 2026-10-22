// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de CreateChannelModal.tsx dentro de la pantalla principal declarada
// en pantalla.go.
//
// «Crear canal»: diálogo `md` con el nombre (y el slug con el que se creará), la
// visibilidad en dos radios con su explicación, el tema y el pie de cancelar y crear.
// Los errores del nombre —ya existe, no vale, sin permiso— van JUNTO al campo y no en
// un aviso flotante.

// MaxNombreCanal es `MAX_NAME`.
const MaxNombreCanal = 100

// SlugCanal es `channelSlug`: minúsculas, letras y cifras ASCII, y un guion por cada
// tramo de espacios, guiones bajos, guiones o puntos; sin guiones en las puntas y a lo
// sumo 64. Es lo que el servicio hará con el nombre, y por eso la ayuda lo enseña.
func SlugCanal(nombre string) string {
	var b strings.Builder
	guion := false
	for _, c := range strings.ToLower(strings.TrimSpace(nombre)) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
			guion = false
		case c == ' ' || c == '_' || c == '-' || c == '.':
			if !guion && b.Len() > 0 {
				b.WriteByte('-')
				guion = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

// HuecoFormulario es el `gap-4` del cuerpo, y AireLeyenda el `mb-1.5` de la leyenda
// de la visibilidad. HOJA.
var (
	HuecoFormulario = tema.Espacio(4)
	AireLeyenda     = tema.Espacio(1.5)
	SangriaRadio    = tema.Espacio(6)
)

// CrearCanal es el cuerpo del diálogo y su estado.
type CrearCanal struct {
	widget.BaseWidget

	T *i18n.Translator
	// Privado es la visibilidad elegida; Ocupado, el envío en camino.
	Privado, Ocupado bool
	// ErrorNombre va junto al campo; Fallo es el error genérico de debajo.
	ErrorNombre string
	Fallo       bool
	// AlCrear recibe el nombre, el tema y si es privado.
	AlCrear func(nombre, asunto string, privado bool)

	nombre, temaCanal *ui.Entrada
	publico, privado  *ui.Eleccion
	crear             *ui.Boton
	caja              *fyne.Container
}

var _ fyne.Widget = (*CrearCanal)(nil)

// NuevoCrearCanal crea el formulario vacío.
func NuevoCrearCanal(t *i18n.Translator) *CrearCanal {
	c := &CrearCanal{T: t}
	c.ExtendBaseWidget(c)
	c.nombre = ui.NuevaEntrada(t.T("createChannel.namePlaceholder", nil))
	c.nombre.AlCambiar(func(s string) {
		if len([]rune(s)) > MaxNombreCanal {
			c.nombre.PonerTexto(string([]rune(s)[:MaxNombreCanal])) // `maxLength`
			return
		}
		c.ErrorNombre = ""
		c.Refresh()
	})
	c.nombre.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			c.Enviar()
			return true
		}
		return false
	}
	c.temaCanal = ui.NuevaEntrada(t.T("createChannel.topicPlaceholder", nil))
	c.publico = ui.NuevoRadio(t.T("createChannel.public", nil), func(bool) { c.Privado = false; c.Refresh() })
	c.privado = ui.NuevoRadio(t.T("createChannel.private", nil), func(bool) { c.Privado = true; c.Refresh() })
	c.crear = ui.NuevoBoton(t.T("createChannel.title", nil), c.Enviar).Con(ui.Primario, ui.TallaMD)
	return c
}

// Nombre es la caja del nombre, que recibe el foco al abrir.
func (c *CrearCanal) Nombre() *ui.Entrada { return c.nombre }

// BotonCrear es el primario del pie.
func (c *CrearCanal) BotonCrear() *ui.Boton { return c.crear }

// Enviar es `submit`: sin slug, u ocupado, no hace nada.
func (c *CrearCanal) Enviar() {
	if SlugCanal(c.nombre.Texto()) == "" || c.Ocupado || c.AlCrear == nil {
		return
	}
	c.Ocupado, c.ErrorNombre, c.Fallo = true, "", false
	c.Refresh()
	c.AlCrear(strings.TrimSpace(c.nombre.Texto()), strings.TrimSpace(c.temaCanal.Texto()), c.Privado)
}

// construir rehace la columna con la ayuda o el error de ahora.
func (c *CrearCanal) construir() {
	t := c.T
	ayuda := t.T("createChannel.nameHelp", nil)
	if slug := SlugCanal(c.nombre.Texto()); slug != "" {
		ayuda = t.T("createChannel.slugHelp", i18n.Vars{"slug": slug})
	}
	c.nombre.Invalida = c.ErrorNombre != ""
	c.publico.Marcada, c.privado.Marcada = !c.Privado, c.Privado
	explicacion := func(clave string) fyne.CanvasObject {
		return container.New(ui.Fila{}, sangria(SangriaRadio),
			ui.NuevoRotulo(t.T(clave, nil), tema.TextoXS, ui.Normal, "text-muted"))
	}
	visibilidad := container.New(ui.Columna{Hueco: tema.Espacio(2)},
		container.New(ui.Columna{Hueco: AireLeyenda},
			ui.NuevoRotulo(t.T("createChannel.visibility", nil), tema.TextoSM, ui.SemiNegrita, "text"),
			container.New(ui.Columna{}, c.publico, explicacion("createChannel.publicHelp"))),
		container.New(ui.Columna{}, c.privado, explicacion("createChannel.privateHelp")))
	hijos := []fyne.CanvasObject{
		ui.GrupoCampo(t.T("createChannel.name", nil), true, c.nombre, ayuda, c.ErrorNombre),
		visibilidad,
		ui.GrupoCampo(t.T("channel.topic", nil), false, c.temaCanal, t.T("createChannel.topicHelp", nil), ""),
	}
	if c.Fallo {
		hijos = append(hijos, ui.NuevoRotulo(t.T("createChannel.failed", nil), tema.TextoSM, ui.Normal, "danger-text"))
	}
	c.caja = container.New(ui.Columna{Hueco: HuecoFormulario}, hijos...)
	c.crear.Inerte = SlugCanal(c.nombre.Texto()) == ""
	c.crear.Cargando = c.Ocupado
	c.crear.Refresh()
}

// CreateRenderer monta la columna.
func (c *CrearCanal) CreateRenderer() fyne.WidgetRenderer {
	c.construir()
	return &crearCanalRenderer{c: c}
}

type crearCanalRenderer struct{ c *CrearCanal }

func (r *crearCanalRenderer) MinSize() fyne.Size           { return r.c.caja.MinSize() }
func (r *crearCanalRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.c.caja} }
func (r *crearCanalRenderer) Destroy()                     {}
func (r *crearCanalRenderer) Layout(t fyne.Size)           { r.c.caja.Resize(t) }

func (r *crearCanalRenderer) Refresh() {
	r.c.construir()
	r.Layout(r.c.Size())
}

// sangria es un espacio de ese ancho, para una `ui.Fila`: el `pl-6` de la explicación
// de un radio.
func sangria(ancho float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.NRGBA{})
	r.SetMinSize(fyne.NewSize(ancho, 0))
	return r
}
