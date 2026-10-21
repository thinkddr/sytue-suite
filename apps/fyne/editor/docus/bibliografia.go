// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"encoding/json"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El grupo «Citas» de Referencias (`CitasBibliografiaGroup` de Bibliografia.tsx): el
// gestor de fuentes, «Insertar cita» y «Insertar o actualizar bibliografía». El HTML es
// de `rico` (bibliografia.go).

// Las órdenes de las citas; en la web no pasan por `exec`.
const (
	cmdFuentes      editor.Comando = "bibSources" // valor: las fuentes, en JSON
	cmdCitar        editor.Comando = "bibCite"    // valor: el id de la fuente
	cmdBibliografia editor.Comando = "bibUpdate"
)

// Anchos de los dos desplegables: `w-80` y `w-64`.
var (
	AnchoFuentes = tema.Espacio(80)
	AnchoCitar   = tema.Espacio(64)
)

// ordenBibliografia ejecuta las órdenes de las citas. Devuelve false si no es una.
func (f *Folio) ordenBibliografia(c editor.Comando, valor string) bool {
	switch c {
	case cmdFuentes:
		var fs []rico.Fuente
		if json.Unmarshal([]byte(valor), &fs) != nil {
			return true
		}
		f.antesDeCambiar(false)
		f.doc.PonerFuentes(fs)
		f.reestructurado()
	case cmdCitar:
		for _, fu := range f.doc.Fuentes() {
			if fu.ID != valor {
				continue
			}
			f.antesDeCambiar(false)
			f.borrarSeleccion()
			e := f.edicion(f.cur.b)
			e.InsertarCita(f.cur.p, fu)
			f.irA(e.Lugar, f.cur.p+1)
			f.reestructurado()
		}
	case cmdBibliografia:
		f.antesDeCambiar(false)
		n := f.doc.GenerarBibliografia(f.traducir("bib.heading"), f.traducir("bib.empty"))
		f.reestructurado()
		if f.ctx != nil && f.ctx.Notificar != nil {
			titulo := f.traducir("bib.updated.none")
			if n > 0 {
				titulo = f.ctx.Traducir("bib.updated", i18n.Vars{"count": n})
			}
			f.ctx.Notificar(ui.AvisoExito, titulo)
		}
	default:
		return false
	}
	return true
}

// grupoCitas es el grupo «Citas» de Referencias.
func (in *inicio) grupoCitas(c *editor.Contexto) fyne.CanvasObject {
	f, _ := c.Lienzo.(*Folio)
	var fuentes []rico.Fuente
	if f != nil {
		fuentes = f.doc.Fuentes()
	}
	gestor := editor.NuevoBotonEmergente(iconos.EdLibro, c.Traducir("bib.sources"), func(cerrar func()) fyne.CanvasObject {
		return nuevoGestorFuentes(c, f).objeto
	})
	gestor.Activo, gestor.Inerte = len(fuentes) > 0, c.SoloLectura || f == nil
	citar := editor.NuevoBotonEmergente(iconos.EdCita, c.Traducir("bib.cite"), func(cerrar func()) fyne.CanvasObject {
		return panelCitar(c, f, cerrar)
	})
	citar.Inerte = c.SoloLectura || len(fuentes) == 0
	actualizar := editor.NuevoBotonCinta(iconos.EdBibliografia, c.Traducir("bib.update"), func() { c.Mandar(cmdBibliografia, "") })
	actualizar.Inerte = c.SoloLectura
	return editor.NuevoGrupo(c.Traducir("bib.group"), gestor, citar, actualizar)
}

// panelCitar es `InsertarCitaPopover`: las fuentes, y la elegida va al cursor.
func panelCitar(c *editor.Contexto, f *Folio, cerrar func()) fyne.CanvasObject {
	fuentes := f.doc.Fuentes()
	filas := make([]fyne.CanvasObject, 0, 1+len(fuentes))
	filas = append(filas, container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1), tema.Espacio(1), 0), // `px-1 pb-1`
		ui.NuevoRotulo(c.Traducir("bib.cite.atCursor"), tema.Texto2XS, ui.Normal, "text-subtle")))
	for _, fu := range fuentes {
		titulo := fu.Titulo
		if titulo == "" {
			titulo = c.Traducir("bib.source.untitled")
		}
		filas = append(filas, nuevaFilaDoble(titulo, rico.TextoCita(fu), AnchoCitar-2*tema.Espacio(2),
			c.Traducir("bib.cite.source", i18n.Vars{"name": nombreFuente(fu)}), func() {
				cerrar()
				c.Mandar(cmdCitar, fu.ID)
			}))
	}
	return container.NewVScroll(container.New(ui.Columna{}, filas...))
}

func nombreFuente(fu rico.Fuente) string {
	if fu.Titulo != "" {
		return fu.Titulo
	}
	return fu.Autor
}

// gestorFuentes es `FuentesPopover`: la lista de fuentes —pulsar una la edita, la papelera
// la borra— y el formulario de una nueva o de la que se edita.
type gestorFuentes struct {
	c        *editor.Contexto
	f        *Folio
	borrador rico.Fuente
	cuerpo   *fyne.Container
	objeto   fyne.CanvasObject
}

func nuevoGestorFuentes(c *editor.Contexto, f *Folio) *gestorFuentes {
	g := &gestorFuentes{c: c, f: f, borrador: rico.Fuente{Tipo: "libro"}}
	g.cuerpo = container.New(ui.Columna{})
	g.objeto = container.New(anchoFijo{AnchoFuentes - 2*tema.Espacio(2)}, g.cuerpo)
	g.pintar()
	return g
}

// guardar es `guardar`: la fuente nueva (con el siguiente id) o la editada; sin título, nada.
func (g *gestorFuentes) guardar() {
	if strings.TrimSpace(g.borrador.Titulo) == "" {
		return
	}
	fs := g.f.doc.Fuentes()
	d := g.borrador
	if d.Tipo != "web" {
		d.URL = ""
	}
	if d.ID == "" {
		d.ID = rico.SiguienteFuente(fs)
		fs = append(fs, d)
	} else {
		for i := range fs {
			if fs[i].ID == d.ID {
				fs[i] = d
			}
		}
	}
	g.mandar(fs)
	g.borrador = rico.Fuente{Tipo: "libro"}
	g.pintar()
}

func (g *gestorFuentes) borrar(id string) {
	var fs []rico.Fuente
	for _, fu := range g.f.doc.Fuentes() {
		if fu.ID != id {
			fs = append(fs, fu)
		}
	}
	g.mandar(fs)
	if g.borrador.ID == id {
		g.borrador = rico.Fuente{Tipo: "libro"}
	}
	g.pintar()
}

func (g *gestorFuentes) mandar(fs []rico.Fuente) {
	datos, err := json.Marshal(fs)
	if err == nil {
		g.c.Mandar(cmdFuentes, string(datos))
	}
}

// pintar rehace el panel: `text-xs font-medium` con el título, la lista con su raya de
// abajo y el formulario (`grid gap-1.5`).
func (g *gestorFuentes) pintar() {
	c := g.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := AnchoFuentes - 2*tema.Espacio(2)
	objs := []fyne.CanvasObject{
		container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1), 0, 0), ui.NuevoRotulo(c.Traducir("bib.sources"), tema.TextoXS, ui.Medio, "text")),
	}
	fuentes := g.f.doc.Fuentes()
	if len(fuentes) > 0 {
		filas := make([]fyne.CanvasObject, 0, len(fuentes))
		for _, fu := range fuentes {
			titulo := fu.Titulo
			if titulo == "" {
				titulo = c.Traducir("bib.source.untitled")
			}
			tipo := map[string]string{"libro": "bib.type.book", "articulo": "bib.type.article", "web": "bib.type.web"}[fu.Tipo]
			editar := nuevaFilaDoble(titulo, rico.TextoCita(fu)+" · "+c.Traducir(tipo), ancho-tema.Espacio(1.5)-ui.LadoIconoSM,
				c.Traducir("bib.source.edit", i18n.Vars{"name": nombreFuente(fu)}), func() { g.borrador = fu; g.pintar() })
			papelera := ui.NuevoBotonIcono(iconos.Ninguno, c.Traducir("bib.source.delete", i18n.Vars{"name": nombreFuente(fu)}), func() { g.borrar(fu.ID) })
			papelera.Glifo, papelera.Lado, papelera.TamIcono, papelera.TrazoEncima = iconos.TrazoEd(iconos.EdPapelera, 16, "text-muted", v), ui.LadoIconoSM, 16, "danger-text"
			filas = append(filas, container.New(filaCentro{hueco: tema.Espacio(1.5)}, editar, papelera))
		}
		raya := canvas.NewRectangle(tema.Color("border", v))
		raya.SetMinSize(fyne.NewSize(ancho, tema.Borde))
		lista := container.NewVScroll(container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filas...))
		lista.SetMinSize(fyne.NewSize(ancho, min(float32(len(filas))*(tema.TextoXS.Linea+tema.Texto2XS.Linea+2*tema.Espacio(1)), tema.Espacio(40))))
		objs = append(objs, lista, container.New(layout.NewCustomPaddedLayout(tema.Espacio(1.5), tema.Espacio(2), 0, 0), raya))
	}
	claveForm := "bib.form.add"
	if g.borrador.ID != "" {
		claveForm = "bib.form.edit"
	}
	objs = append(objs, container.New(layout.NewCustomPaddedLayout(0, tema.Espacio(1), 0, 0),
		ui.NuevoRotulo(c.Traducir(claveForm), tema.Texto2XS, ui.Normal, "text-subtle")), g.formulario(ancho))
	g.cuerpo.Objects = objs
	g.cuerpo.Refresh()
}

// formulario es el `grid gap-1.5` del borrador: tipo, autor, título, año y editorial (o
// revista, o sitio), la URL de una web, y los botones.
func (g *gestorFuentes) formulario(ancho float32) fyne.CanvasObject {
	c := g.c
	d := &g.borrador
	tipos := make([]editor.Opcion, 0, len(rico.TiposFuente))
	for _, t := range rico.TiposFuente {
		tipos = append(tipos, editor.Opcion{Valor: t, Rotulo: c.Traducir(map[string]string{"libro": "bib.type.book", "articulo": "bib.type.article", "web": "bib.type.web"}[t])})
	}
	tipo := editor.NuevoSelectorCinta(c.Traducir("bib.type"), ancho, tipos, d.Tipo, func(v string) { d.Tipo = v; g.pintar() })
	campo := func(marca, valor string, poner func(string)) *ui.Entrada {
		e := ui.NuevaEntrada(marca).Con(ui.CampoSM)
		e.PonerTexto(valor)
		e.AlCambiar(poner)
		return e
	}
	editorial := map[string]string{"libro": "bib.field.publisher", "articulo": "bib.field.journal", "web": "bib.field.site"}[d.Tipo]
	var boton *botonPequeno
	autor := campo(c.Traducir("bib.field.author.placeholder"), d.Autor, func(s string) { d.Autor = s })
	titulo := campo(c.Traducir("bib.field.title"), d.Titulo, func(s string) {
		d.Titulo = s
		if boton != nil {
			boton.Inerte = strings.TrimSpace(s) == ""
			boton.Refresh()
		}
	})
	anio := campo(c.Traducir("bib.field.year"), d.Anio, func(s string) { d.Anio = s })
	edit := campo(c.Traducir(editorial), d.Editorial, func(s string) { d.Editorial = s })
	autor.Ancho, titulo.Ancho = ancho, ancho
	mitad := (ancho - tema.Espacio(1.5)) / 2
	anio.Ancho, edit.Ancho = mitad, mitad
	filas := []fyne.CanvasObject{tipo, autor, titulo, container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, anio, edit)}
	if d.Tipo == "web" {
		url := campo("URL", d.URL, func(s string) { d.URL = s })
		url.Ancho = ancho
		filas = append(filas, url)
	}
	claveGuardar := "bib.form.add"
	var botones []fyne.CanvasObject
	if d.ID != "" {
		claveGuardar = "bib.form.update"
		botones = append(botones, nuevoBotonAccion(c.Traducir("action.cancel"), func() { g.borrador = rico.Fuente{Tipo: "libro"}; g.pintar() }))
	}
	boton = nuevoBotonAccion(c.Traducir(claveGuardar), g.guardar)
	boton.Inerte = strings.TrimSpace(d.Titulo) == ""
	botones = append(botones, boton)
	filas = append(filas, container.New(layout.NewCustomPaddedLayout(tema.Espacio(0.5), 0, 0, 0), container.New(repartoIgual{hueco: tema.Espacio(1.5)}, botones...)))
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, filas...)
}

// anchoFijo da a su hijo un ancho fijo (el `w-80` de un panel).
type anchoFijo struct{ ancho float32 }

func (a anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.ancho, objs[0].MinSize().Height)
}

func (a anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// repartoIgual es `flex gap-N` con hijos `w-full`: se reparten el ancho a partes iguales.
type repartoIgual struct{ hueco float32 }

func (r repartoIgual) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(0, h)
}

func (r repartoIgual) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) == 0 {
		return
	}
	w := (t.Width - r.hueco*float32(len(objs)-1)) / float32(len(objs))
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i)*(w+r.hueco), 0))
		o.Resize(fyne.NewSize(w, o.MinSize().Height))
	}
}

// filaDoble es una fila pulsable de dos líneas: `rounded-sm px-2 py-1 text-left
// hover:bg-bg-subtle`, con la primera en `text-xs` y la segunda en `sytue-meta-dense`, las
// dos recortadas.
type filaDoble struct {
	widget.BaseWidget
	arriba, abajo, rotulo string
	ancho                 float32
	alPulsar              func()
	encima, enfocado      bool
}

var (
	_ fyne.Tappable     = (*filaDoble)(nil)
	_ fyne.Focusable    = (*filaDoble)(nil)
	_ desktop.Hoverable = (*filaDoble)(nil)
	_ fyne.Accessible   = (*filaDoble)(nil)
)

func nuevaFilaDoble(arriba, abajo string, ancho float32, rotulo string, alPulsar func()) *filaDoble {
	f := &filaDoble{arriba: arriba, abajo: abajo, ancho: ancho, rotulo: rotulo, alPulsar: alPulsar}
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaDoble) Tapped(*fyne.PointEvent)        { f.alPulsar() }
func (f *filaDoble) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *filaDoble) MouseMoved(*desktop.MouseEvent) {}
func (f *filaDoble) MouseOut()                      { f.encima = false; f.Refresh() }
func (f *filaDoble) FocusGained()                   { f.enfocado = true; f.Refresh() }
func (f *filaDoble) FocusLost()                     { f.enfocado = false; f.Refresh() }
func (f *filaDoble) TypedRune(r rune) {
	if r == ' ' {
		f.alPulsar()
	}
}

func (f *filaDoble) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(f, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		f.alPulsar()
	}
}
func (f *filaDoble) AccessibilityLabel() string             { return f.rotulo }
func (f *filaDoble) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (f *filaDoble) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	alto := tema.TextoXS.Linea + 2*tema.Espacio(1)
	if f.abajo != "" {
		alto += tema.Texto2XS.Linea
	}
	return fyne.NewSize(f.ancho, alto)
}

func (f *filaDoble) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaDobleRenderer{
		f: f, caja: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM), aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		arriba: ui.Texto(f.arriba, tema.TextoXS, ui.Normal, tema.Color("text", v)),
		abajo:  ui.Texto(f.abajo, tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.arriba, r.abajo)...)
	return r
}

type filaDobleRenderer struct {
	f             *filaDoble
	caja          *ui.Caja
	aro           *ui.Aro
	arriba, abajo *canvas.Text
	objs          []fyne.CanvasObject
}

func (r *filaDobleRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaDobleRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *filaDobleRenderer) Destroy()                     {}

func (r *filaDobleRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	x, y := tema.Espacio(2), tema.Espacio(1)
	ui.Truncar(r.arriba, r.f.arriba, tema.TextoXS, ui.Normal, max(t.Width-2*x, 0))
	ui.ColocarTexto(r.arriba, tema.TextoXS, x, y)
	ui.Truncar(r.abajo, r.f.abajo, tema.Texto2XS, ui.Normal, max(t.Width-2*x, 0))
	ui.ColocarTexto(r.abajo, tema.Texto2XS, x, y+tema.TextoXS.Linea)
}

func (r *filaDobleRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	if r.f.encima {
		fondo = tema.Color("bg-subtle", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.aro.Pintar(v)
	r.aro.Ver(r.f.enfocado)
	if s := r.f.Size(); s.Width > 0 {
		r.Layout(s)
	}
}
