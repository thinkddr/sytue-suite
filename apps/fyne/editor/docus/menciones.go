// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"cmp"
	"context"
	"image/color"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// El selector de menciones de los comentarios, `Menciones.tsx` de la web: al escribir
// «@» sale la gente de la organización y, al elegir, se escribe `@ana@empresa.es`, que
// es la forma que el servicio reconoce para avisar a esa persona.
//
// Se elige con el ratón o con el teclado sin salir del campo, como en la web: flechas
// arriba/abajo mueven la opción activa, Intro la escribe y Escape cierra la lista hasta
// la siguiente tecla. La lista se pinta como la de la web (`useMenciones`): panel
// `sytue-menu-surface border-border rounded-md border p-1 shadow-md` y cada opción
// `rounded-sm px-2 py-1`, la activa en `bg-bg-subtle` (el `menu-hover` del menú), el nombre en `text-sm
// font-medium text-text` y el correo debajo en `text-xs text-text-muted`. Lo vigila
// TestLaListaDeMencionesPintaComoLaWeb, que lee esas clases del fuente.

var reMencion = regexp.MustCompile(`(?:^|\s)@([^\s@]*)$`)

// consultaMencion es lo que se lleva escrito tras la última «@» suelta.
func consultaMencion(texto string) (string, bool) {
	m := reMencion.FindStringSubmatch(texto)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// ponerMencion sustituye la mención a medias por la dirección elegida.
func ponerMencion(texto, email string) string {
	i := strings.LastIndex(texto, "@")
	return texto[:i] + "@" + email + " "
}

// candidatosMencion son quienes casan por nombre o dirección, seis como mucho.
func candidatosMencion(gente []api.Miembro, q string) []api.Miembro {
	var out []api.Miembro
	for _, p := range gente {
		if len(out) == 6 {
			break
		}
		if buscar.Contiene(p.Email, q) || buscar.Contiene(p.Nombre, q) {
			out = append(out, p)
		}
	}
	return out
}

// moverActiva es adónde va la opción activa con una flecha: da la vuelta en los dos
// extremos. Otra tecla la deja donde estaba. La misma regla que `moverActiva` de la web.
func moverActiva(activa, total int, tecla fyne.KeyName) int {
	switch {
	case total == 0:
		return 0
	case tecla == fyne.KeyDown:
		return (activa + 1) % total
	case tecla == fyne.KeyUp:
		return (activa - 1 + total) % total
	}
	return activa
}

// menciones cuelga de un área de texto y se repinta con lo que se escribe.
type menciones struct {
	a      *App
	campo  *ui.AreaTexto
	caja   *fyne.Container
	gente  []api.Miembro
	pedida bool
	// lista son los candidatos a la vista y activa, el que se escribe con Intro.
	lista  []api.Miembro
	activa int
	q      string
	// cerradaEn es el texto en el que se pulsó Escape: la lista no vuelve hasta que cambie.
	cerradaEn *string
}

func nuevasMenciones(a *App, campo *ui.AreaTexto) *menciones {
	m := &menciones{a: a, campo: campo, caja: container.New(ui.Columna{})}
	campo.AlTeclear = m.teclear
	return m
}

// teclear ve la tecla antes que el área: con la lista abierta, las flechas, Intro y
// Escape son suyas; el resto sigue escribiendo.
func (m *menciones) teclear(k *fyne.KeyEvent) bool {
	if len(m.lista) == 0 {
		return false
	}
	switch k.Name {
	case fyne.KeyDown, fyne.KeyUp:
		m.activa = moverActiva(m.activa, len(m.lista), k.Name)
		m.pintar()
	case fyne.KeyReturn, fyne.KeyEnter:
		m.elegir(m.lista[m.activa])
	case fyne.KeyEscape:
		t := m.campo.Texto()
		m.cerradaEn = &t
		m.lista = nil
		m.pintar()
	default:
		return false
	}
	return true
}

func (m *menciones) elegir(p api.Miembro) {
	m.campo.PonerTexto(ponerMencion(m.campo.Texto(), p.Email))
}

// cambio se llama con cada tecla; la gente se pide UNA vez, al primer «@».
func (m *menciones) cambio(texto string) {
	q, ok := consultaMencion(texto)
	if ok && !m.pedida && m.a.Directorio != nil {
		m.pedida = true
		m.a.lanzar(func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelar()
			gente, err := m.a.Directorio.Agenda(ctx, 500)
			fyne.Do(func() {
				if err != nil {
					m.pedida = false // se reintenta en la siguiente arroba
				}
				m.gente = gente
				m.cambio(m.campo.Texto())
			})
		})
	}
	if m.cerradaEn != nil && *m.cerradaEn != texto {
		m.cerradaEn = nil
	}
	if q != m.q {
		m.q, m.activa = q, 0
	}
	m.lista = nil
	if ok && m.cerradaEn == nil {
		m.lista = candidatosMencion(m.gente, q)
	}
	m.activa = min(m.activa, max(len(m.lista)-1, 0))
	m.pintar()
}

func (m *menciones) pintar() {
	m.caja.Objects = nil
	if len(m.lista) > 0 {
		filas := container.New(ui.Columna{})
		for i, p := range m.lista {
			filas.Add(nuevaOpcionMencion(p, i == m.activa, func() { m.elegir(p) }, func() {
				if m.activa != i {
					m.activa = i
					m.pintar()
				}
			}))
		}
		m.caja.Add(nuevoPanelMenciones(filas))
	}
	m.caja.Refresh()
}

// Los tokens de la lista, los de las clases de la web (ver el comentario de arriba).
const (
	fondoPanelMenciones   = "menu-surface"
	bordePanelMenciones   = "border"
	fondoOpcionActiva     = "menu-hover"
	textoNombreMencion    = "text"
	textoCorreoMencion    = "text-muted"
	rellenoPanelMenciones = 1 // p-1
	rellenoXOpcion        = 2 // px-2
	rellenoYOpcion        = 1 // py-1
)

// panelMenciones es el `<ul>`: caja con borde, sombra media y `p-1` alrededor.
type panelMenciones struct {
	widget.BaseWidget
	filas fyne.CanvasObject
}

func nuevoPanelMenciones(filas fyne.CanvasObject) *panelMenciones {
	p := &panelMenciones{filas: filas}
	p.ExtendBaseWidget(p)
	return p
}

func (p *panelMenciones) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &panelMencionesRenderer{
		p:      p,
		caja:   ui.NuevaCaja(tema.Color(fondoPanelMenciones, v), tema.Color(bordePanelMenciones, v), tema.RadioMD),
		sombra: ui.NuevaSombra(tema.SombraMD, tema.RadioMD),
	}
	r.objetos = append(append([]fyne.CanvasObject{r.sombra.Objeto()}, r.caja.Objetos()...), p.filas)
	return r
}

type panelMencionesRenderer struct {
	p       *panelMenciones
	caja    *ui.Caja
	sombra  *ui.Sombra
	objetos []fyne.CanvasObject
}

func (r *panelMencionesRenderer) MinSize() fyne.Size {
	d := 2 * tema.Espacio(rellenoPanelMenciones)
	return r.p.filas.MinSize().AddWidthHeight(d, d)
}

func (r *panelMencionesRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	d := tema.Espacio(rellenoPanelMenciones)
	r.p.filas.Move(fyne.NewPos(d, d))
	r.p.filas.Resize(t.SubtractWidthHeight(2*d, 2*d))
}

func (r *panelMencionesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color(fondoPanelMenciones, v), tema.Color(bordePanelMenciones, v))
	r.Layout(r.p.Size())
}

func (r *panelMencionesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *panelMencionesRenderer) Destroy()                     {}

// opcionMencion es un `<li role="option">`: nombre y, si lo hay, el correo debajo.
type opcionMencion struct {
	widget.BaseWidget
	p                  api.Miembro
	activa             bool
	alElegir, alEntrar func()
}

var _ desktop.Hoverable = (*opcionMencion)(nil)

func nuevaOpcionMencion(p api.Miembro, activa bool, alElegir, alEntrar func()) *opcionMencion {
	o := &opcionMencion{p: p, activa: activa, alElegir: alElegir, alEntrar: alEntrar}
	o.ExtendBaseWidget(o)
	return o
}

// colores son el fondo, el nombre y el correo de la opción.
func (o *opcionMencion) colores(v tema.Variante) (fondo, nombre, correo color.NRGBA) {
	if o.activa {
		fondo = tema.Color(fondoOpcionActiva, v)
	}
	return fondo, tema.Color(textoNombreMencion, v), tema.Color(textoCorreoMencion, v)
}

func (o *opcionMencion) Tapped(*fyne.PointEvent)        { o.alElegir() }
func (o *opcionMencion) MouseIn(*desktop.MouseEvent)    { o.alEntrar() } // onMouseEnter
func (o *opcionMencion) MouseMoved(*desktop.MouseEvent) {}
func (o *opcionMencion) MouseOut()                      {}

func (o *opcionMencion) MinSize() fyne.Size {
	o.ExtendBaseWidget(o)
	alto := 2*tema.Espacio(rellenoYOpcion) + tema.TextoSM.Linea
	if o.p.Nombre != "" {
		alto += tema.TextoXS.Linea
	}
	return fyne.NewSize(0, alto)
}

func (o *opcionMencion) CreateRenderer() fyne.WidgetRenderer {
	fondo, nombre, correo := o.colores(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r := &opcionMencionRenderer{
		o:      o,
		caja:   ui.NuevaCaja(fondo, color.NRGBA{}, tema.RadioSM),
		nombre: ui.Texto("", tema.TextoSM, ui.Medio, nombre),
		correo: ui.Texto("", tema.TextoXS, ui.Normal, correo),
	}
	r.objetos = append(r.caja.Objetos(), r.nombre, r.correo)
	return r
}

type opcionMencionRenderer struct {
	o              *opcionMencion
	caja           *ui.Caja
	nombre, correo *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *opcionMencionRenderer) MinSize() fyne.Size { return r.o.MinSize() }

func (r *opcionMencionRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	x, y := tema.Espacio(rellenoXOpcion), tema.Espacio(rellenoYOpcion)
	cabe := max(t.Width-2*x, 0)
	ui.Truncar(r.nombre, cmp.Or(r.o.p.Nombre, r.o.p.Email), tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoSM, x, y)
	r.correo.Hidden = r.o.p.Nombre == ""
	ui.Truncar(r.correo, r.o.p.Email, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.correo, tema.TextoXS, x, y+tema.TextoSM.Linea)
}

func (r *opcionMencionRenderer) Refresh() {
	fondo, nombre, correo := r.o.colores(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.caja.Pintar(fondo, color.NRGBA{})
	r.nombre.Color, r.correo.Color = nombre, correo
	r.Layout(r.o.Size())
	r.nombre.Refresh()
	r.correo.Refresh()
}

func (r *opcionMencionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *opcionMencionRenderer) Destroy()                     {}
