// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de EmojiPicker.tsx y lib/emoji.ts dentro de la pantalla de canal
// declarada en pantalla.go.
//
// El selector de emoji: la caja de buscar, la rejilla de ocho columnas y el nombre del
// activo. Lo usan la caja de escribir (inserta el emoji) y la barra de un mensaje
// (reacciona con cualquiera, no solo con las cuatro rápidas).
//
// Los emoji de la organización y su «Subir» van en sus secciones, debajo (emojiorg.go).
//
// ponytail: el panel es el de `ui.Menu` (su capa, su borde y su sombra),
// que no deja fijar los 300 de ancho de `POP_W`: la rejilla lleva sus ocho columnas de
// `h-8 w-8` y el panel mide lo que ellas.

// EntradaEmoji es `EmojiEntry`: un emoji con todos sus shortcodes.
type EntradaEmoji struct {
	Glifo   string
	Nombres []string
}

// entradasEmoji es `EMOJI_ENTRIES`: emojiLista sin repetir emoji, en su orden, con
// todos los nombres de cada uno (👍 es :thumbsup: y :+1:).
var entradasEmoji = func() []EntradaEmoji {
	var out []EntradaEmoji
	donde := map[string]int{}
	for _, e := range emojiLista {
		if i, hay := donde[e.glifo]; hay {
			out[i].Nombres = append(out[i].Nombres, e.nombre)
			continue
		}
		donde[e.glifo] = len(out)
		out = append(out, EntradaEmoji{Glifo: e.glifo, Nombres: []string{e.nombre}})
	}
	return out
}()

// BuscarEmoji es `searchEmoji`: los que tienen un nombre que CONTIENE la consulta, sin
// distinguir mayúsculas; sin consulta, todos.
func BuscarEmoji(q string) []EntradaEmoji {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return entradasEmoji
	}
	var out []EntradaEmoji
	for _, e := range entradasEmoji {
		for _, n := range e.Nombres {
			if buscar.Contiene(n, q) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// Las medidas del selector. HOJA.
var (
	// ColumnasEmoji es `COLS`; LadoCeldaEmoji, el `h-8 w-8` de cada una, y
	// HuecoCeldaEmoji su `gap-0.5`.
	ColumnasEmoji   = 8
	LadoCeldaEmoji  = tema.Espacio(8)
	HuecoCeldaEmoji = tema.Espacio(0.5)
	// AltoRejillaEmoji es el `max-h-56` de la rejilla.
	AltoRejillaEmoji = tema.Espacio(56)
	// El driver de pruebas es de escritorio; el guardián simula el puntero
	// manteniendo la misma pregunta compartida que usa producción.
	tactilEnEmojis = ui.EsTactil
)

// SelectorEmoji es el `role="dialog"` del selector.
type SelectorEmoji struct {
	widget.BaseWidget

	T *i18n.Translator
	// AlElegir recibe el emoji elegido: el glifo, o `:nombre:` si es de la organización.
	AlElegir func(string)
	// EnHoja: va en la hoja del teléfono; la rejilla no lleva tope de alto.
	EnHoja bool
	// AlElegirImagen abre el selector del sistema y da la ruta elegida; AlSubir sube un
	// emoji de la organización y avisa al terminar. Sin ellos no hay «Subir».
	AlElegirImagen func(func(ruta string))
	AlSubir        func(nombre, ruta string, luego func(error))

	propias         []*celdaPropia
	subida          subidaPropia
	desplazaPropios *container.Scroll

	caja       *ui.Entrada
	resultados []EntradaEmoji
	activo     int
	celdas     []*celdaEmoji
	rejilla    *fyne.Container
	desplaza   *container.Scroll
	vacio      *TextoCentrado
	nombre     *canvas.Text
	columna    *fyne.Container
	r          *selectorEmojiRenderer
}

// NuevoSelectorEmoji crea el selector con todos los emoji.
func NuevoSelectorEmoji(t *i18n.Translator, alElegir func(string)) *SelectorEmoji {
	s := &SelectorEmoji{T: t, AlElegir: alElegir}
	s.ExtendBaseWidget(s)
	s.caja = ui.NuevaEntrada(t.T("emoji.searchPlaceholder", nil))
	s.caja.AlCambiar(s.Buscar)
	s.caja.AlTeclear = s.tecla
	s.rejilla = container.New(&rejillaEmoji{})
	s.desplaza = container.NewVScroll(s.rejilla)
	s.vacio = NuevoTextoCentrado(t.T("search.noResults", nil))
	s.nombre = ui.Texto("", escalonMeta, ui.Normal, color.NRGBA{})
	s.columna = container.New(ui.Columna{})
	s.Buscar("")
	return s
}

// Caja es la de buscar, que se lleva el foco al abrir con puntero fino.
func (s *SelectorEmoji) Caja() *ui.Entrada { return s.caja }

// Resultados son los emoji que se ven.
func (s *SelectorEmoji) Resultados() []EntradaEmoji { return s.resultados }

// Activo es el índice del resaltado (`aria-activedescendant`).
func (s *SelectorEmoji) Activo() int { return s.activo }

// AccessibilityLabel es el del botón que lo abre.
func (s *SelectorEmoji) AccessibilityLabel() string { return s.T.T("emoji.label", nil) }

// AccessibilityRole implementa fyne.Accessible.
func (s *SelectorEmoji) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Buscar filtra la rejilla y vuelve el activo al primero.
func (s *SelectorEmoji) Buscar(q string) {
	s.resultados, s.activo = BuscarEmoji(q), 0
	s.celdas = s.celdas[:0]
	objs := make([]fyne.CanvasObject, 0, len(s.resultados))
	for i, e := range s.resultados {
		c := &celdaEmoji{s: s, i: i, glifo: e.Glifo}
		c.ExtendBaseWidget(c)
		s.celdas = append(s.celdas, c)
		objs = append(objs, c)
	}
	s.rejilla.Objects = objs
	s.rejilla.Refresh()
	s.montar()
}

// montar pone la caja, la rejilla (o «Sin resultados») y el nombre del activo.
func (s *SelectorEmoji) montar() {
	cosas := []fyne.CanvasObject{s.caja, hueco(tema.Espacio(2))} // `mb-2`
	// La rejilla, a su alto hasta el `max-h-56`: `container.Scroll` pide muy poco.
	// En la hoja, sin tope (`max-md:max-h-none`): la desplaza el diálogo entero, no un
	// segundo desplazador dentro.
	tope := AltoRejillaEmoji
	if s.EnHoja {
		tope = rejillaEmoji{}.MinSize(s.rejilla.Objects).Height
	}
	s.desplaza.SetMinSize(fyne.NewSize(0, min(rejillaEmoji{}.MinSize(s.rejilla.Objects).Height, tope)))
	if len(s.resultados) == 0 {
		cosas = append(cosas, s.vacio)
	} else {
		cosas = append(cosas, s.desplaza, hueco(tema.Espacio(1)), s.nombre) // `mt-1`
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cosas = append(cosas, s.seccionPropios(v)...)
	if s.AlSubir != nil {
		cosas = append(cosas, s.seccionSubir(v)...)
	}
	s.columna.Objects = cosas
	s.pintarActivo()
	s.columna.Refresh()
	s.Refresh()
}

// pintarActivo es el `bg-selected` del activo y su «:nombre:» debajo.
func (s *SelectorEmoji) pintarActivo() {
	s.nombre.Text = ""
	if s.activo >= 0 && s.activo < len(s.resultados) {
		s.nombre.Text = ":" + s.resultados[s.activo].Nombres[0] + ":"
	}
	s.nombre.Color = tema.Color("text-subtle", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	s.nombre.Refresh()
	for _, c := range s.celdas {
		c.Refresh()
	}
}

// Activar mueve el resaltado, sin salirse de la lista, y lo trae a la vista
// (`scrollIntoView({ block: 'nearest' })`).
func (s *SelectorEmoji) Activar(i int) {
	if len(s.resultados) == 0 {
		return
	}
	s.activo = min(max(i, 0), len(s.resultados)-1)
	fila := float32(s.activo / ColumnasEmoji)
	y := fila * (altoCeldaEmoji() + HuecoCeldaEmoji)
	desplaza := s.desplaza
	if s.r != nil && s.r.recortado {
		desplaza = s.r.desplaza
		y += s.rejilla.Position().Y
	}
	if off := desplaza.Offset.Y; y < off {
		desplaza.Offset.Y = y
	} else if vista := desplaza.Size().Height; vista > 0 && y+altoCeldaEmoji() > off+vista {
		desplaza.Offset.Y = y + altoCeldaEmoji() - vista
	}
	desplaza.Refresh()
	s.pintarActivo()
}

// Elegir es `pick`.
func (s *SelectorEmoji) Elegir(i int) {
	if i < 0 || i >= len(s.resultados) || s.AlElegir == nil {
		return
	}
	s.AlElegir(s.resultados[i].Glifo)
}

// tecla es el `onKeyDown` del buscador: las flechas recorren la rejilla, Intro elige.
// Escape lo recoge el menú que lo aloja.
func (s *SelectorEmoji) tecla(k *fyne.KeyEvent) bool {
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		s.Elegir(s.activo)
	case fyne.KeyRight:
		s.Activar(s.activo + 1)
	case fyne.KeyLeft:
		s.Activar(s.activo - 1)
	case fyne.KeyDown:
		s.Activar(s.activo + ColumnasEmoji)
	case fyne.KeyUp:
		s.Activar(s.activo - ColumnasEmoji)
	default:
		return false
	}
	return true
}

// CreateRenderer monta la columna sin que SimpleRenderer fuerce su alto natural
// por debajo del borde del menú. En poca altura, un solo Scroll deja TODO accesible.
func (s *SelectorEmoji) CreateRenderer() fyne.WidgetRenderer {
	r := &selectorEmojiRenderer{s: s, objetos: []fyne.CanvasObject{s.columna}}
	s.r = r
	return r
}

type selectorEmojiRenderer struct {
	s         *SelectorEmoji
	entero    *fyne.Container
	fluido    *fyne.Container
	desplaza  *container.Scroll
	ancho     float32
	recortado bool
	objetos   []fyne.CanvasObject
}

func (r *selectorEmojiRenderer) MinSize() fyne.Size           { return r.s.columna.MinSize() }
func (r *selectorEmojiRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *selectorEmojiRenderer) Destroy()                     {}

func (r *selectorEmojiRenderer) Layout(t fyne.Size) {
	s := r.s
	r.recortado = t.Height < s.columna.MinSize().Height
	if !r.recortado {
		r.objetos = []fyne.CanvasObject{s.columna}
		s.columna.Move(fyne.Position{})
		s.columna.Resize(t)
		s.columna.Refresh()
		return
	}
	if r.entero == nil {
		r.entero = container.New(ui.Columna{})
	}
	// Solo se aplanan las dos rejillas que pertenecen a este selector. Dejarlas
	// dentro de otro Scroll haría que el interior se quedase el arrastre del dedo.
	r.entero.Objects = nil
	for _, o := range s.columna.Objects {
		switch o {
		case s.desplaza:
			o = s.rejilla
		case s.desplazaPropios:
			o = s.desplazaPropios.Content
		}
		r.entero.Objects = append(r.entero.Objects, o)
	}
	if r.fluido == nil || r.ancho != t.Width {
		r.ancho = t.Width
		r.fluido = ui.AlAnchoFluido(t.Width, r.entero)
		if r.desplaza == nil {
			r.desplaza = container.NewVScroll(r.fluido)
		} else {
			r.desplaza.Content = r.fluido
		}
	}
	r.objetos = []fyne.CanvasObject{r.desplaza}
	r.entero.Refresh()
	r.desplaza.Move(fyne.Position{})
	r.desplaza.Resize(t)
	r.desplaza.Refresh()
}

func (r *selectorEmojiRenderer) Refresh() { r.Layout(r.s.Size()) }

// ---- La rejilla y sus celdas ----

// rejillaEmoji es el `grid grid-cols-8 gap-0.5`.
type rejillaEmoji struct{}

// altoCeldaEmoji es el `h-8` de una celda; con el dedo, `pointer-coarse:h-11`.
func altoCeldaEmoji() float32 {
	if tactilEnEmojis() {
		return ui.SueloTactilBarra
	}
	return LadoCeldaEmoji
}

func (rejillaEmoji) MinSize(objs []fyne.CanvasObject) fyne.Size {
	filas := (len(objs) + ColumnasEmoji - 1) / ColumnasEmoji
	return fyne.NewSize(float32(ColumnasEmoji)*LadoCeldaEmoji+float32(ColumnasEmoji-1)*HuecoCeldaEmoji,
		float32(filas)*altoCeldaEmoji()+float32(max(filas-1, 0))*HuecoCeldaEmoji)
}

// Layout es `grid grid-cols-8 w-full`: las ocho columnas se reparten el ancho que haya
// (en la hoja del teléfono, el del diálogo entero).
func (rejillaEmoji) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ancho := max(LadoCeldaEmoji, (t.Width-float32(ColumnasEmoji-1)*HuecoCeldaEmoji)/float32(ColumnasEmoji))
	alto := altoCeldaEmoji()
	for i, o := range objs {
		col, fila := i%ColumnasEmoji, i/ColumnasEmoji
		o.Move(fyne.NewPos(float32(col)*(ancho+HuecoCeldaEmoji), float32(fila)*(alto+HuecoCeldaEmoji)))
		o.Resize(fyne.NewSize(ancho, alto))
	}
}

// celdaEmoji es un `role="option"`: el emoji en `text-lg`, `bg-selected` si es el
// activo y `hover:bg-bg-subtle` si no. Pasar por encima lo activa; tocarlo lo elige.
type celdaEmoji struct {
	widget.BaseWidget

	s      *SelectorEmoji
	i      int
	glifo  string
	encima bool
}

var (
	_ fyne.Tappable     = (*celdaEmoji)(nil)
	_ desktop.Hoverable = (*celdaEmoji)(nil)
)

func (c *celdaEmoji) AccessibilityLabel() string {
	return c.s.resultados[c.i].Nombres[0]
}
func (c *celdaEmoji) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (c *celdaEmoji) Tapped(*fyne.PointEvent) { c.s.Elegir(c.i) }

func (c *celdaEmoji) MouseIn(*desktop.MouseEvent) {
	c.encima = true
	c.s.activo = c.i // `onMouseEnter={() => setActive(i)}`
	c.s.pintarActivo()
}
func (c *celdaEmoji) MouseMoved(*desktop.MouseEvent) {}
func (c *celdaEmoji) MouseOut() {
	c.encima = false
	c.Refresh()
}

func (c *celdaEmoji) CreateRenderer() fyne.WidgetRenderer {
	fondo := ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM)
	// Una cara explícita de Figtree omite el fallback emoji del driver; iOS
	// tampoco busca fuentes del sistema. Pintar Y medir con la cara portable.
	texto := ui.TextoConCara(c.glifo, tema.TextoLG, tema.Emoji(), color.NRGBA{})
	return &celdaEmojiRenderer{c: c, fondo: fondo, texto: texto, objetos: append(fondo.Objetos(), texto)}
}

type celdaEmojiRenderer struct {
	c       *celdaEmoji
	fondo   *ui.Caja
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *celdaEmojiRenderer) MinSize() fyne.Size           { return fyne.NewSize(LadoCeldaEmoji, LadoCeldaEmoji) }
func (r *celdaEmojiRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *celdaEmojiRenderer) Destroy()                     {}

func (r *celdaEmojiRenderer) Layout(t fyne.Size) {
	r.fondo.Redimensionar(t)
	ancho := ui.MedirConCara(r.texto.Text, tema.TextoLG, r.texto.FontSource).Width
	ui.ColocarTexto(r.texto, tema.TextoLG, (t.Width-ancho)/2, (t.Height-tema.TextoLG.Linea)/2)
}

func (r *celdaEmojiRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := color.NRGBA{}
	switch {
	case r.c.i == r.c.s.activo:
		fondo = tema.Color("selected", v)
	case r.c.encima:
		fondo = tema.Color("bg-subtle", v)
	}
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
	r.Layout(r.c.Size())
}

// ---- En la Sala ----

// AbrirEmojis despliega el selector colgando de su disparador; al elegir, se cierra y
// `al` recibe el emoji. El foco va al buscador solo con puntero fino, como en la
// web: abrirlo con el dedo no debe sacar el teclado encima del catálogo.
//
// En el teléfono (`useEnHoja` de la web), el selector va en la hoja de los diálogos: junto
// al botón no cabe.
func (sa *Sala) AbrirEmojis(disparador fyne.CanvasObject, al func(string)) {
	var m *ui.Menu
	var d *ui.Dialogo
	c := fyne.CurrentApp().Driver().CanvasForObject(disparador)
	hoja := c != nil && c.Size().Width > 0 && c.Size().Width < ui.Estrecho
	s := NuevoSelectorEmoji(sa.P.T, func(e string) {
		if d != nil {
			d.Cerrar()
		} else {
			m.Cerrar()
		}
		al(e)
	})
	if _, ok := sa.S.(EmojiPropio); ok && sa.ElegirFicheros != nil {
		s.AlSubir = sa.subirEmojiOrg
		s.AlElegirImagen = func(al func(string)) {
			pedir(sa, func(context.Context) ([]string, error) { return sa.ElegirFicheros() }, func(rutas []string, err error) {
				if err == nil && len(rutas) > 0 {
					al(rutas[0])
				}
			})
		}
	}
	s.EnHoja = hoja
	s.montar()
	if hoja {
		sa.emojis = s
		d = ui.NuevoDialogo(sa.P.T.T("emoji.label", nil), s).Con(ui.DialogoSM, nil)
		if !tactilEnEmojis() {
			d.Foco = s.Caja() // con puntero fino, a buscar; con el dedo, al panel
		}
		d.Mostrar(c)
		return
	}
	m = ui.NuevoMenu(s)
	m.ContenidoAjustable = true
	m.Rotulo = sa.P.T.T("emoji.label", nil)
	sa.emojis = s
	m.Mostrar(disparador)
	if !tactilEnEmojis() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(disparador); c != nil {
			c.Focus(s.Caja())
		}
	}
}
