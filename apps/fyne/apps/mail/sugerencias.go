// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"image/color"
	"slices"
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
)

// # El autocompletado de Para, CC y CCO (`AddressField.tsx` + `/api/contacts`)
//
// Con dos letras escritas, a los 200 ms, se le pregunta a Sytue Contacts y se enseñan
// hasta ocho direcciones —una fila por dirección, la principal de cada contacto
// primero— bajo el campo. Las flechas señalan, Intro elige, Escape cierra y el ratón
// elige al PULSAR, antes de que el campo pierda el foco. Si Contacts no contesta, no
// pasa nada: se sigue escribiendo a mano.

// Sugerencia es un `ContactSuggestion`.
type Sugerencia struct{ Nombre, Correo string }

// Ficha es cómo entra en el campo: «Nombre <correo>» o el correo solo.
func (s Sugerencia) Ficha() string {
	if s.Nombre != "" {
		return s.Nombre + " <" + s.Correo + ">"
	}
	return s.Correo
}

// Buscador es lo que el autocompletado le pide a Contacts. Lo cumple *api.Contacts.
type Buscador interface {
	Search(ctx context.Context, q string, limite int) ([]api.Contact, error)
}

var _ Buscador = (*api.Contacts)(nil)

// Lo que fija la web.
const (
	MaxSugerencias    = 8                      // `MAX_SUGGESTIONS` del BFF
	EsperaSugerencias = 200 * time.Millisecond // el `setTimeout` del campo
	minSugerencia     = 2                      // `token.length < 2`
)

// SugerenciasDe es lo que el BFF hace con la respuesta de Contacts: una entrada por
// dirección, la principal de cada contacto primero, sin repetir (sin mayúsculas) y
// como mucho ocho.
func SugerenciasDe(cs []api.Contact) []Sugerencia {
	var out []Sugerencia
	visto := map[string]bool{}
	for _, c := range cs {
		correos := slices.Clone(c.Emails)
		slices.SortStableFunc(correos, func(a, b api.ContactEmail) int {
			switch {
			case a.IsPrimary == b.IsPrimary:
				return 0
			case a.IsPrimary:
				return -1
			}
			return 1
		})
		for _, e := range correos {
			correo := strings.TrimSpace(e.Email)
			if correo == "" || visto[strings.ToLower(correo)] {
				continue
			}
			visto[strings.ToLower(correo)] = true
			out = append(out, Sugerencia{Nombre: strings.TrimSpace(c.FormattedName), Correo: correo})
			if len(out) >= MaxSugerencias {
				return out
			}
		}
	}
	return out
}

// ConContactos engancha el autocompletado de los campos de direcciones.
func (b *Buzon) ConContactos(c Buscador) *Buzon {
	b.Contactos = c
	return b
}

// sugeridor es el `useEffect` del campo: espera, pregunta y descarta las respuestas
// que llegan cuando ya se ha escrito otra cosa.
func (b *Buzon) sugeridor(c *CampoDirecciones) func(string) {
	var parar func()
	turno := 0
	return func(q string) {
		if parar != nil {
			parar()
			parar = nil
		}
		turno++
		if q == "" || b.Contactos == nil {
			return
		}
		mio := turno
		parar = b.Esperar(EsperaSugerencias, func() {
			pedir(b, func(ctx context.Context) ([]api.Contact, error) {
				return b.Contactos.Search(ctx, q, MaxSugerencias)
			}, func(cs []api.Contact, err error) {
				if mio != turno || err != nil {
					return
				}
				c.PonerSugerencias(SugerenciasDe(cs))
			})
		})
	}
}

// ---- La lista bajo el campo ----

// Medidas de la lista: `border bg-surface-raised rounded-lg py-1 shadow-md max-h-60
// mt-1`, y cada fila `px-3 py-1.5 text-sm`.
const (
	margenYLista    float32 = 1 * 0.25 * tema.Rem
	maxAltoLista    float32 = 60 * 0.25 * tema.Rem
	margenXSugeren  float32 = 3 * 0.25 * tema.Rem
	margenYSugeren  float32 = 1.5 * 0.25 * tema.Rem
	separacionLista float32 = 1 * 0.25 * tema.Rem
)

// AltoSugerencia es una fila: una línea si solo hay correo y dos con nombre.
func AltoSugerencia(s Sugerencia) float32 {
	lineas := float32(1)
	if s.Nombre != "" {
		lineas = 2
	}
	return 2*margenYSugeren + lineas*tema.TextoSM.Linea
}

// PonerSugerencias enseña la lista, o la quita si viene vacía. Lo que llegue con el
// campo ya sin foco o con menos de dos letras se descarta.
func (c *CampoDirecciones) PonerSugerencias(xs []Sugerencia) {
	_, actual := PartirDirecciones(c.Valor)
	if !c.enfocado || len([]rune(strings.TrimSpace(actual))) < minSugerencia || len(xs) == 0 {
		c.cerrarSugerencias()
		return
	}
	c.sugerencias, c.activa = xs, -1
	c.mostrarSugerencias()
}

// SugerenciasAbiertas dice si la lista está a la vista.
func (c *CampoDirecciones) SugerenciasAbiertas() bool { return c.panel != nil }

func (c *CampoDirecciones) cerrarSugerencias() {
	c.sugerencias, c.activa, c.filasSug = nil, -1, nil
	if c.panel != nil {
		c.panel = nil
		if c.AlSugerir != nil {
			c.AlSugerir(nil)
		}
	}
}

// mostrarSugerencias arma el panel y se lo da a quien aloja el campo (AlSugerir), que
// lo coloca debajo, ENCIMA de lo que haya. No es una capa del lienzo a propósito: en
// Fyne cada capa lleva su propio foco, y con la lista en una el teclado dejaba de
// llegar al campo en el que se está escribiendo.
func (c *CampoDirecciones) mostrarSugerencias() {
	if c.AlSugerir == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filas := make([]fyne.CanvasObject, 0, len(c.sugerencias))
	c.filasSug = c.filasSug[:0]
	alto := 2 * margenYLista
	for i, s := range c.sugerencias {
		f := nuevaFilaSugerencia(c, i, s, v)
		c.filasSug = append(c.filasSug, f)
		filas = append(filas, f)
		alto += AltoSugerencia(s)
	}
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioLG)
	lista := container.NewVScroll(container.New(ui.Margen{Arr: margenYLista, Abj: margenYLista}, container.New(ui.Columna{}, filas...)))
	panel := container.New(&enCaja{caja: caja}, append(caja.Objetos(), lista)...)
	sombra := ui.NuevaSombra(tema.SombraMD, tema.RadioLG)
	c.panel = container.New(&conSombra{s: sombra, v: v, alto: min(alto, maxAltoLista) + 2*tema.Borde}, sombra.Objeto(), panel)
	c.AlSugerir(c.panel)
}

// ColocarSugerencias pone el panel bajo el campo (`absolute mt-1 w-full`): `campo` es
// dónde está el campo en las coordenadas de quien lo aloja.
func (c *CampoDirecciones) ColocarSugerencias(campo fyne.Position) {
	if c.panel == nil {
		return
	}
	c.panel.Move(campo.AddXY(0, c.Size().Height+separacionLista))
	c.panel.Resize(fyne.NewSize(c.Size().Width, c.panel.MinSize().Height))
}

// conSombra pone la sombra del panel detrás de él, a su mismo tamaño.
type conSombra struct {
	s    *ui.Sombra
	v    tema.Variante
	alto float32
}

func (c *conSombra) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }

func (c *conSombra) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.s.Ajustar(fyne.NewPos(0, 0), t, c.v)
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(t)
}

// senalar mueve la fila activa (`ArrowDown`/`ArrowUp`) y repinta la lista.
func (c *CampoDirecciones) senalar(i int) {
	c.activa = i
	for _, f := range c.filasSug {
		f.Refresh()
	}
}

// pedirSugerencias avisa al buzón de lo que se está escribiendo: con dos letras y el
// foco, que busque; si no, que no, y la lista se va.
func (c *CampoDirecciones) pedirSugerencias() {
	_, actual := PartirDirecciones(c.Valor)
	q := strings.TrimSpace(actual)
	if !c.enfocado || len([]rune(q)) < minSugerencia {
		q = ""
		c.cerrarSugerencias()
	}
	if c.Buscar != nil {
		c.Buscar(q)
	}
}

// teclaSugerencias son las flechas, Intro y Escape con la lista abierta.
func (c *CampoDirecciones) teclaSugerencias(k *fyne.KeyEvent) bool {
	if !c.SugerenciasAbiertas() || len(c.sugerencias) == 0 {
		return false
	}
	switch k.Name {
	case fyne.KeyDown:
		c.senalar(min(c.activa+1, len(c.sugerencias)-1))
	case fyne.KeyUp:
		c.senalar(max(c.activa-1, 0))
	case fyne.KeyReturn, fyne.KeyEnter:
		if c.activa < 0 {
			return false
		}
		c.elegir(c.sugerencias[c.activa])
	case fyne.KeyEscape:
		c.cerrarSugerencias()
	default:
		return false
	}
	return true
}

// elegir es `choose`: la sugerencia entra como ficha y el campo queda listo para la
// siguiente dirección, con el foco donde estaba.
func (c *CampoDirecciones) elegir(s Sugerencia) {
	fichas, _ := PartirDirecciones(c.Valor)
	c.cerrarSugerencias()
	c.editor.OnChanged = nil
	c.editor.PonerTexto("")
	c.editor.AlCambiar(c.teclear)
	c.cambiar(UnirDireccionesCampo(append(fichas, s.Ficha()), ""))
	if c.Buscar != nil {
		c.Buscar("")
	}
}

// filaSugerencia es una opción: el nombre en `text` y el correo en `text-subtle`
// debajo, o el correo solo. La activa, en `bg-selected`.
type filaSugerencia struct {
	widget.BaseWidget
	c *CampoDirecciones
	i int
	s Sugerencia
	v tema.Variante
}

var (
	_ desktop.Mouseable = (*filaSugerencia)(nil)
	_ desktop.Hoverable = (*filaSugerencia)(nil)
)

func nuevaFilaSugerencia(c *CampoDirecciones, i int, s Sugerencia, v tema.Variante) *filaSugerencia {
	f := &filaSugerencia{c: c, i: i, s: s, v: v}
	f.ExtendBaseWidget(f)
	return f
}

// MouseDown elige: es el `onMouseDown` con `preventDefault` de la web, que elige
// ANTES de que el campo pierda el foco y cierre lo escrito como ficha suelta. El foco
// vuelve al campo en cuanto el clic termina.
func (f *filaSugerencia) MouseDown(*desktop.MouseEvent) {
	c := f.c
	c.elegir(f.s)
	fyne.Do(func() {
		if l := fyne.CurrentApp().Driver().CanvasForObject(c); l != nil {
			l.Focus(c.editor)
		}
	})
}

func (f *filaSugerencia) MouseUp(*desktop.MouseEvent)    {}
func (f *filaSugerencia) MouseIn(*desktop.MouseEvent)    { f.c.senalar(f.i) }
func (f *filaSugerencia) MouseMoved(*desktop.MouseEvent) {}
func (f *filaSugerencia) MouseOut()                      {}
func (f *filaSugerencia) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (f *filaSugerencia) MinSize() fyne.Size { return fyne.NewSize(0, AltoSugerencia(f.s)) }

func (f *filaSugerencia) CreateRenderer() fyne.WidgetRenderer {
	r := &filaSugerenciaRenderer{f: f, fondo: canvas.NewRectangle(color.NRGBA{})}
	arriba := f.s.Correo
	if f.s.Nombre != "" {
		arriba = f.s.Nombre
		r.abajo = ui.Texto(f.s.Correo, tema.TextoSM, ui.Normal, tema.Color("text-subtle", f.v))
	}
	r.arriba = ui.Texto(arriba, tema.TextoSM, ui.Normal, tema.Color("text", f.v))
	r.textoArriba = arriba
	r.Refresh()
	return r
}

type filaSugerenciaRenderer struct {
	f             *filaSugerencia
	fondo         *canvas.Rectangle
	arriba, abajo *canvas.Text
	textoArriba   string
}

func (r *filaSugerenciaRenderer) MinSize() fyne.Size { return r.f.MinSize() }
func (r *filaSugerenciaRenderer) Objects() []fyne.CanvasObject {
	if r.abajo == nil {
		return []fyne.CanvasObject{r.fondo, r.arriba}
	}
	return []fyne.CanvasObject{r.fondo, r.arriba, r.abajo}
}
func (r *filaSugerenciaRenderer) Destroy() {}

func (r *filaSugerenciaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	cabe := max(t.Width-2*margenXSugeren, 0)
	ui.Truncar(r.arriba, r.textoArriba, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.arriba, tema.TextoSM, margenXSugeren, margenYSugeren)
	if r.abajo != nil {
		ui.Truncar(r.abajo, r.f.s.Correo, tema.TextoSM, ui.Normal, cabe)
		ui.ColocarTexto(r.abajo, tema.TextoSM, margenXSugeren, margenYSugeren+tema.TextoSM.Linea)
	}
}

func (r *filaSugerenciaRenderer) Refresh() {
	r.fondo.FillColor = color.NRGBA{}
	if r.f.c.activa == r.f.i {
		r.fondo.FillColor = tema.Color("selected", r.f.v)
	}
	r.fondo.Refresh()
	r.Layout(r.f.Size())
}
