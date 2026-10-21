// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los paneles de la pestaña «Insertar» (`ribbon/tabs/Insertar.tsx`) que no son de un
// producto: la rejilla de «Tabla», el formulario de «Enlace» y la de «Símbolo». Viven en
// el cromo porque Docus y Diapos los enseñan igual.

// RellenoEmergenteCinta es el `p-2` del panel de un RibbonPopover.
var RellenoEmergenteCinta = tema.Espacio(2)

// NuevoBotonEmergente es un RibbonPopover: el botón de 28 de la cinta que despliega
// debajo el panel que construye `cuerpo`, con `cerrar` para cerrarlo desde dentro.
func NuevoBotonEmergente(i iconos.Ed, rotulo string, cuerpo func(cerrar func()) fyne.CanvasObject) *BotonCinta {
	b := NuevoBotonCinta(i, rotulo, nil)
	var em *ui.Emergente
	b.AlPulsar = func() {
		if em != nil && em.Abierto() {
			em.Cerrar()
			return
		}
		em = ui.MostrarEmergente(b, cuerpo(func() {
			if em != nil {
				em.Cerrar()
			}
		}), RellenoEmergenteCinta)
	}
	return b
}

// ---- Tabla ----

// Medidas de TablaPanel: la rejilla de 8 × 8 de `h-4 w-4` con `gap-0.5`.
var (
	LadoCasillaTabla  = tema.Espacio(4)
	HuecoCasillaTabla = tema.Espacio(0.5)
)

// ColumnasTabla es el `GRID` de TablaPanel: 8 filas y 8 columnas.
const ColumnasTabla = 8

// SelectorTabla es la rejilla de TablaPanel: al pasar por encima se marcan las casillas
// hasta la del puntero, y al pulsar se inserta esa tabla. Con el teclado, las flechas
// mueven la marca e Intro inserta.
type SelectorTabla struct {
	widget.BaseWidget
	Filas, Cols int // lo marcado; 0 es nada
	AlCambiar   func()
	AlElegir    func(filas, cols int)
	enfocado    bool
}

var (
	_ desktop.Hoverable = (*SelectorTabla)(nil)
	_ fyne.Tappable     = (*SelectorTabla)(nil)
	_ fyne.Focusable    = (*SelectorTabla)(nil)
)

// NuevoSelectorTabla crea la rejilla.
func NuevoSelectorTabla(alElegir func(filas, cols int)) *SelectorTabla {
	s := &SelectorTabla{AlElegir: alElegir}
	s.ExtendBaseWidget(s)
	return s
}

// MinSize es la rejilla entera.
func (s *SelectorTabla) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	lado := ColumnasTabla*LadoCasillaTabla + (ColumnasTabla-1)*HuecoCasillaTabla
	return fyne.NewSize(lado, lado)
}

func (s *SelectorTabla) casilla(p fyne.Position) (int, int) {
	paso := LadoCasillaTabla + HuecoCasillaTabla
	return min(int(p.Y/paso)+1, ColumnasTabla), min(int(p.X/paso)+1, ColumnasTabla)
}

func (s *SelectorTabla) marcar(f, c int) {
	if f == s.Filas && c == s.Cols {
		return
	}
	s.Filas, s.Cols = f, c
	s.Refresh()
	if s.AlCambiar != nil {
		s.AlCambiar()
	}
}

// MouseIn marca hasta la casilla del puntero.
func (s *SelectorTabla) MouseIn(e *desktop.MouseEvent) { s.marcar(s.casilla(e.Position)) }

// MouseMoved sigue al puntero.
func (s *SelectorTabla) MouseMoved(e *desktop.MouseEvent) { s.marcar(s.casilla(e.Position)) }

// MouseOut es el `onMouseLeave` de la rejilla: se desmarca todo.
func (s *SelectorTabla) MouseOut() { s.marcar(0, 0) }

// Tapped inserta la tabla de la casilla pulsada.
func (s *SelectorTabla) Tapped(e *fyne.PointEvent) {
	f, c := s.casilla(e.Position)
	if s.AlElegir != nil {
		s.AlElegir(f, c)
	}
}

// FocusGained marca la primera casilla, como el `onFocus` del primer botón.
func (s *SelectorTabla) FocusGained() {
	s.enfocado = true
	if s.Filas == 0 {
		s.marcar(1, 1)
	}
	s.Refresh()
}

// FocusLost quita el aro.
func (s *SelectorTabla) FocusLost() { s.enfocado = false; s.Refresh() }

// TypedRune no hace nada: la rejilla solo atiende a flechas e Intro.
func (s *SelectorTabla) TypedRune(rune) {}

// TypedKey mueve la marca con las flechas e inserta con Intro o Espacio.
func (s *SelectorTabla) TypedKey(k *fyne.KeyEvent) {
	if ui.SubirEscape(s, k) { // Escape y «atrás» son del diálogo de encima
		return
	}
	f, c := max(s.Filas, 1), max(s.Cols, 1)
	switch k.Name {
	case fyne.KeyUp:
		f = max(f-1, 1)
	case fyne.KeyDown:
		f = min(f+1, ColumnasTabla)
	case fyne.KeyLeft:
		c = max(c-1, 1)
	case fyne.KeyRight:
		c = min(c+1, ColumnasTabla)
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeySpace:
		if s.AlElegir != nil {
			s.AlElegir(f, c)
		}
		return
	default:
		return
	}
	s.marcar(f, c)
}

// CreateRenderer pinta las 64 casillas: `rounded-[2px] border`, marcadas en
// `border-primary bg-selected` y las demás en `border-border bg-surface`.
func (s *SelectorTabla) CreateRenderer() fyne.WidgetRenderer {
	r := &selectorTablaRenderer{s: s}
	for range ColumnasTabla * ColumnasTabla {
		c := canvas.NewRectangle(color.NRGBA{})
		c.CornerRadius, c.StrokeWidth = 2, tema.Borde
		r.casillas = append(r.casillas, c)
	}
	r.Refresh()
	return r
}

type selectorTablaRenderer struct {
	s        *SelectorTabla
	casillas []*canvas.Rectangle
}

func (r *selectorTablaRenderer) MinSize() fyne.Size { return r.s.MinSize() }
func (r *selectorTablaRenderer) Destroy()           {}
func (r *selectorTablaRenderer) Objects() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(r.casillas))
	for i, c := range r.casillas {
		out[i] = c
	}
	return out
}

func (r *selectorTablaRenderer) Layout(fyne.Size) {
	paso := LadoCasillaTabla + HuecoCasillaTabla
	for i, c := range r.casillas {
		c.Move(fyne.NewPos(float32(i%ColumnasTabla)*paso, float32(i/ColumnasTabla)*paso))
		c.Resize(fyne.NewSize(LadoCasillaTabla, LadoCasillaTabla))
	}
}

func (r *selectorTablaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for i, c := range r.casillas {
		f, col := i/ColumnasTabla+1, i%ColumnasTabla+1
		if f <= r.s.Filas && col <= r.s.Cols {
			c.FillColor, c.StrokeColor = tema.Color("selected", v), tema.Color("primary", v)
		} else {
			c.FillColor, c.StrokeColor = tema.Color("surface", v), tema.Color("border", v)
		}
		c.Refresh()
	}
	r.Layout(r.s.Size())
}

// NuevoPanelTabla es TablaPanel: la rejilla, debajo «F × C» (o «Elige el tamaño») y la
// casilla de «Fila de encabezado».
func NuevoPanelTabla(ctx *Contexto, alInsertar func(filas, cols int, cabecera bool)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cabecera := false
	sel := NuevoSelectorTabla(func(f, c int) { alInsertar(f, c, cabecera) })
	rotulo := ui.Texto(ctx.Traducir("insert.table.pickSize"), tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	centro := container.NewCenter(rotulo)
	sel.AlCambiar = func() {
		rotulo.Text = ctx.Traducir("insert.table.pickSize")
		if sel.Filas > 0 {
			rotulo.Text = strconv.Itoa(sel.Filas) + " × " + strconv.Itoa(sel.Cols)
		}
		rotulo.Refresh()
		centro.Refresh()
	}
	casilla := ui.NuevaCasilla(ctx.Traducir("insert.table.headerRow"), func(si bool) { cabecera = si })
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, container.NewCenter(sel), centro, casilla)
}

// ---- Enlace ----

// esquemaSeguro es la frontera de confianza de EnlacePanel: `https:`, `http:`, `mailto:`
// y las anclas pasan; lo demás —`javascript:` incluido— se toma por un dominio.
var esquemaSeguro = regexp.MustCompile(`(?i)^(https?:|mailto:|#)`)

// NormalizarEnlace es lo que hace la web con lo tecleado: sin esquema reconocido, se le
// pone `https://`.
func NormalizarEnlace(crudo string) string {
	crudo = strings.TrimSpace(crudo)
	if crudo == "" || esquemaSeguro.MatchString(crudo) {
		return crudo
	}
	return "https://" + crudo
}

// NuevoPanelEnlace es EnlacePanel: la URL, el texto visible (precargado con la
// selección) y «Insertar enlace». Con la URL vacía no hace nada.
func NuevoPanelEnlace(ctx *Contexto, seleccion string, alInsertar func(href, texto string)) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	url := ui.NuevaEntrada(ctx.Traducir("insert.link.urlExample")).Con(ui.CampoSM)
	texto := ui.NuevaEntrada(ctx.Traducir("insert.visibleText")).Con(ui.CampoSM)
	url.Ancho, texto.Ancho = tema.Espacio(56), tema.Espacio(56) // `w-56`
	texto.PonerTexto(seleccion)
	enviar := func() {
		href := NormalizarEnlace(url.Texto())
		if href == "" {
			return
		}
		visible := strings.TrimSpace(texto.Texto())
		if visible == "" {
			visible = href
		}
		alInsertar(href, visible)
	}
	etiqueta := func(s string) fyne.CanvasObject {
		return ui.NuevoParrafo(s, tema.TextoXS, ui.Medio, "text", v)
	}
	boton := ui.NuevoBoton(ctx.Traducir("insert.link.insert"), enviar).Con(ui.Primario, ui.TallaXS)
	panel := container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
		etiqueta("URL"), url, etiqueta(ctx.Traducir("insert.link.text")), texto, container.NewHBox(boton))
	// Sin foco en la URL, teclear seguiría escribiendo en el documento: se le da en
	// cuanto el panel está en su ventana.
	fyne.Do(func() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(url); c != nil {
			c.Focus(url)
		}
	})
	return panel
}

// ---- Símbolo ----

// Simbolos son los 24 de SimboloPanel, en su orden, con la clave de su nombre.
var Simbolos = []struct{ Car, Clave string }{
	{"—", "symbol.emDash"},
	{"–", "symbol.enDash"},
	{"…", "symbol.ellipsis"},
	{"«", "symbol.guillemetOpen"},
	{"»", "symbol.guillemetClose"},
	{"¿", "symbol.questionOpen"},
	{"¡", "symbol.exclamationOpen"},
	{"§", "symbol.section"},
	{"¶", "symbol.pilcrow"},
	{"©", "symbol.copyright"},
	{"®", "symbol.registered"},
	{"™", "symbol.trademark"},
	{"€", "symbol.euro"},
	{"£", "symbol.pound"},
	{"¥", "symbol.yen"},
	{"°", "symbol.degree"},
	{"±", "symbol.plusMinus"},
	{"×", "symbol.times"},
	{"÷", "symbol.divide"},
	{"≠", "symbol.notEqual"},
	{"≤", "symbol.lessEqual"},
	{"≥", "symbol.greaterEqual"},
	{"→", "symbol.rightArrow"},
	{"Ω", "symbol.omega"},
}

// NuevoPanelSimbolos es SimboloPanel: 6 columnas de botones de 28 con el carácter en
// `text-sm`, y cada uno cae en el cursor.
func NuevoPanelSimbolos(ctx *Contexto, alElegir func(string)) fyne.CanvasObject {
	const porFila = 6 // `grid-cols-6`
	filas := make([]fyne.CanvasObject, 0, len(Simbolos)/porFila)
	fila := make([]fyne.CanvasObject, 0, porFila)
	for _, s := range Simbolos {
		b := ui.NuevoBoton(s.Car, func() { alElegir(s.Car) }).Con(ui.Fantasma, ui.TallaXS)
		b.Ancho = LadoBotonCinta
		b.Rotulo = ctx.Traducir("insert.shape", i18n.Vars{"shape": ctx.Traducir(s.Clave)})
		if fila = append(fila, b); len(fila) == porFila {
			filas = append(filas, container.New(ui.Fila{Hueco: tema.Espacio(0.5)}, fila...))
			fila = make([]fyne.CanvasObject, 0, porFila)
		}
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, filas...)
}
