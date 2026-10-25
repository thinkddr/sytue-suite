// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// En la web: `startEditTopic`, `commitTopic` y la línea del tema de la cabecera de
// ChatApp.tsx.
//
// Quien administra el equipo edita el tema EN LA BANDA: el tema es un botón (subrayado
// al pasar) o, sin tema, «✎ Añadir un tema»; al pulsarlo, un campo SIN CAJA con su
// filete de abajo (`border-b border-header-fg/40 bg-transparent text-xs`). Intro guarda,
// Escape cancela y perder el foco también guarda.

// TopeTema es el `maxLength={280}` del campo.
const TopeTema = 280

// BotonTema es la línea del tema cuando se puede editar: el tema en `text-header-fg/80`
// o «Añadir un tema» con el lápiz de 12 en `text-header-fg/60`.
type BotonTema struct {
	ui.Pulsable
	Tema, Anadir string

	texto *canvas.Text
	raya  *canvas.Rectangle
	objs  []fyne.CanvasObject
}

var _ desktop.Hoverable = (*BotonTema)(nil)

// NuevoBotonTema crea la línea; `rotulo` es su nombre (`channel.editTopic`).
func NuevoBotonTema(temaCanal, anadir, rotulo string, al func()) *BotonTema {
	b := &BotonTema{Tema: temaCanal, Anadir: anadir}
	b.Nombre, b.AlPulsar = rotulo, al
	b.ExtendBaseWidget(b)
	return b
}

// GlifoTema es el `EditGlyph size={12}` de «Añadir un tema».
const GlifoTema float32 = 12

func (b *BotonTema) rotuloVisible() string {
	if b.Tema != "" {
		return b.Tema
	}
	return b.Anadir
}

// MinSize es el texto a la caja de línea de `text-xs`, con el lápiz y su `gap-1` si
// no hay tema.
func (b *BotonTema) MinSize() fyne.Size {
	ancho := ui.Medir(b.rotuloVisible(), tema.TextoXS, ui.Normal).Width
	if b.Tema == "" {
		ancho += GlifoTema + tema.Espacio(1)
	}
	return fyne.NewSize(ancho, tema.TextoXS.Linea)
}

func (b *BotonTema) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.texto = ui.Texto(b.rotuloVisible(), tema.TextoXS, ui.Normal, atenuado("header-fg", v, 0.8))
	b.raya = canvas.NewRectangle(atenuado("header-fg", v, 1))
	b.objs = []fyne.CanvasObject{b.texto, b.raya}
	if b.Tema == "" {
		g := dibujo(glifoLapiz, GlifoTema, "header-fg", v)
		b.objs = append(b.objs, g.Objeto())
	}
	return &botonTemaRenderer{b: b}
}

type botonTemaRenderer struct{ b *BotonTema }

func (r *botonTemaRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonTemaRenderer) Objects() []fyne.CanvasObject { return r.b.objs }
func (r *botonTemaRenderer) Destroy()                     {}

func (r *botonTemaRenderer) Layout(t fyne.Size) {
	b := r.b
	x := float32(0)
	if b.Tema == "" {
		g := b.objs[2]
		g.Move(fyne.NewPos(0, (t.Height-GlifoTema)/2))
		g.Resize(fyne.NewSize(GlifoTema, GlifoTema))
		x = GlifoTema + tema.Espacio(1)
	}
	ancho := ui.Truncar(b.texto, b.rotuloVisible(), tema.TextoXS, ui.Normal, max(t.Width-x, 0))
	ui.ColocarTexto(b.texto, tema.TextoXS, x, 0)
	// `hover:underline` en el tema; «Añadir un tema» no se subraya, se aclara.
	b.raya.Move(fyne.NewPos(x, (t.Height+tema.TextoXS.Tam)/2))
	b.raya.Resize(fyne.NewSize(ancho, 1))
}

func (r *botonTemaRenderer) Refresh() {
	b := r.b
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	alfa := float32(0.8)
	if b.Tema == "" {
		alfa = 0.6
	}
	if b.Encima() || b.Enfocado() {
		alfa = 1
	}
	b.texto.Color = atenuado("header-fg", v, alfa)
	b.texto.Refresh()
	b.raya.FillColor = atenuado("header-fg", v, 1)
	b.raya.Hidden = b.Tema == "" || !b.Encima()
	b.raya.Refresh()
	r.Layout(b.Size())
}

// montarTema pone la pieza de la línea del tema que toque: el campo si se está
// editando, el botón si se puede editar, o nada (se queda el rótulo de siempre).
func (c *Cabecera) montarTema(v tema.Variante) {
	anterior := c.editorTema
	c.editorTema, c.botonTema, c.lineaTema = nil, nil, nil
	t := c.T
	switch {
	case c.EditandoTema:
		if anterior != nil {
			c.editorTema = anterior
			anterior.Tinta(tema.Color("header-fg", v), tema.Color("header-fg", v))
			c.lineaTema = canvas.NewRectangle(atenuado("header-fg", v, 0.4))
			c.objetos = append(c.objetos, anterior.Objeto(), c.lineaTema)
			break
		}
		var ed *ui.EditorPlano
		ed = ui.NuevoEditorPlano(tema.TextoXS, func(foco bool) {
			// Solo el campo VIGENTE: uno ya retirado que pierde el foco al enfocar el
			// nuevo no debe cerrar el que se acaba de abrir.
			if !foco && c.EditandoTema && c.editorTema == ed && c.AlGuardarTema != nil {
				c.AlGuardarTema(ed.Texto())
			}
		})
		ed.PonerTexto(c.Canal.Tema)
		ed.SetPlaceHolder(t.T("channel.topicPlaceholder", nil))
		ed.Tinta(tema.Color("header-fg", v), tema.Color("header-fg", v))
		ed.Tecla = func(k *fyne.KeyEvent) bool {
			switch k.Name {
			case fyne.KeyReturn, fyne.KeyEnter:
				if c.AlGuardarTema != nil {
					c.AlGuardarTema(ed.Texto())
				}
				return true
			case fyne.KeyEscape:
				if c.AlCancelarTema != nil {
					c.AlCancelarTema()
				}
				return true
			default:
				return false
			}
		}
		c.editorTema = ed
		c.lineaTema = canvas.NewRectangle(atenuado("header-fg", v, 0.4))
		c.objetos = append(c.objetos, ed.Objeto(), c.lineaTema)
	case c.PuedeEditarTema && c.AlEditarTema != nil:
		c.botonTema = NuevoBotonTema(c.Canal.Tema, t.T("channel.addTopic", nil), t.T("channel.editTopic", nil), c.AlEditarTema)
		c.objetos = append(c.objetos, c.botonTema)
	}
}

// conLineaTema dice si la cabecera lleva segunda línea: el tema, su campo o su botón,
// o la presencia del otro en un directo.
func (c *Cabecera) conLineaTema() bool {
	return c.lineaDeDebajo() != "" || c.editorTema != nil || c.botonTema != nil
}

// lineaDeDebajo es la línea bajo el nombre: el tema de un canal; en un directo 1:1, la
// presencia del otro y su estado («En línea · Comiendo»); en uno de grupo, cuántos son.
func (c *Cabecera) lineaDeDebajo() string {
	k := c.Canal
	switch {
	case k.Clase != Directo:
		return k.Tema
	case k.Personas > 0:
		return c.T.T("dm.peopleCount", i18n.Vars{"count": k.Personas})
	}
	s := c.T.T(clavePresencia(ponerDesconectado(k.Presencia)), nil)
	if k.EstadoPar != "" {
		s += " · " + k.EstadoPar
	}
	return s
}

// ponerDesconectado es el `?? 'offline'` de la web: sin presencia sabida, sin conexión.
func ponerDesconectado(p ui.Presencia) ui.Presencia {
	if p == ui.SinPresencia {
		return ui.Desconectado
	}
	return p
}

// colocarTema pone la segunda línea en (x, y) con `cabe` de ancho.
func (c *Cabecera) colocarTema(x, y, cabe float32) {
	switch {
	case c.editorTema != nil:
		c.rotuloTema.Hide()
		// `w-full border-b`: el campo ocupa lo que haya y su filete va debajo.
		c.editorTema.Colocar(fyne.NewPos(x, y+(tema.TextoXS.Linea-c.editorTema.AltoFuente())/2),
			fyne.NewSize(cabe, c.editorTema.AltoFuente()))
		c.lineaTema.Move(fyne.NewPos(x, y+tema.TextoXS.Linea))
		c.lineaTema.Resize(fyne.NewSize(cabe, tema.Borde))
	case c.botonTema != nil:
		c.rotuloTema.Hide()
		m := c.botonTema.MinSize()
		c.botonTema.Move(fyne.NewPos(x, y))
		c.botonTema.Resize(fyne.NewSize(min(m.Width, cabe), m.Height))
	case c.lineaDeDebajo() != "":
		c.rotuloTema.Show()
		ui.Truncar(c.rotuloTema, c.lineaDeDebajo(), tema.TextoXS, ui.Normal, cabe)
		ui.ColocarTexto(c.rotuloTema, tema.TextoXS, x, y)
	default:
		c.rotuloTema.Hide()
	}
}

// ---- En la Sala ----

// EditarTema es `startEditTopic`: la línea del tema pasa a ser su campo, con el foco.
func (sa *Sala) EditarTema() {
	cab := sa.P.Canal.Cabecera
	if !cab.PuedeEditarTema {
		return
	}
	cab.EditandoTema = true
	cab.Refresh()
	if lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto()); lienzo != nil && cab.editorTema != nil {
		lienzo.Focus(cab.editorTema)
	}
}

// GuardarTema es `commitTopic`: cierra el campo y, si el tema cambió, lo guarda. Si el
// servicio dice que no, vuelve el de antes en silencio, como la web.
func (sa *Sala) GuardarTema(texto string) {
	cab := sa.P.Canal.Cabecera
	if !cab.EditandoTema {
		return
	}
	cab.EditandoTema = false
	r := []rune(strings.TrimSpace(texto))
	nuevo := string(r[:min(len(r), TopeTema)])
	if nuevo == cab.Canal.Tema {
		cab.Refresh()
		return
	}
	sa.cambiarTema(nuevo, false, "")
}

// CancelarTema es el Escape: el campo se cierra sin tocar nada.
func (sa *Sala) CancelarTema() {
	cab := sa.P.Canal.Cabecera
	cab.EditandoTema = false
	cab.Refresh()
}
