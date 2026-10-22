// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Paridad web: la `Rama` de NoteTree.tsx, una nota del árbol del panel de en medio.
//
//	fila      `flex items-center gap-1 rounded-md pr-1`, sangría `nivel × 14px`
//	pliegue   `<IconButton size="sm">` (24) con el chevron de 14, o un hueco `w-7` (24,5)
//	botón     `flex-1 items-center gap-1.5 py-1.5 text-base`: glifo de 14, título y meta
//	          (`text-xs`), chincheta de 12 y un punto de 7 por estado
//	«…»       `<IconButton size="sm">` con el `ellipsis` de 14, visible al pasar
//
// 5,25 + 19,25 + 14 + 5,25 = 43,75, que es lo MEDIDO en producción. Renombrando, el botón
// deja su sitio al campo (`py-1`, 28,25) y la fila mide eso.
const (
	HuecoFilaArbol  = 1 * 0.25 * tema.Rem
	PRFilaArbol     = 1 * 0.25 * tema.Rem
	HuecoBotonArbol = 1.5 * 0.25 * tema.Rem
	PYBotonArbol    = 1.5 * 0.25 * tema.Rem
	// HuecoSinPliegue es el `w-7` de la fila sin hijos; con hijos va el botón de 24.
	HuecoSinPliegue = 7 * 0.25 * tema.Rem
	GlifoFilaArbol  = 14
	GlifoPinArbol   = 12
	PuntoArbol      = 2 * 0.25 * tema.Rem
	// DobleToque es cuánto puede pasar entre los dos clics de un doble clic.
	DobleToque = 400 * time.Millisecond
)

// AltoFilaArbol es la fila en reposo.
func AltoFilaArbol() float32 {
	return 2*PYBotonArbol + tema.TextoBase.Linea + tema.TextoXS.Linea
}

// FilaNotaArbol es una nota del árbol: pliegue, botón de abrir (con doble clic para
// renombrar) y «…».
//
// ES UN WIDGET por la regla 1 del patrón: se pulsa, se enfoca y se anuncia con el título
// de la nota.
type FilaNotaArbol struct {
	widget.BaseWidget

	Titulo, Meta string
	// Glifo es la ruta del icono (vacía, sin icono) y TintaGlifo su token.
	Glifo, TintaGlifo string
	Nivel             int
	Activa, Hecha     bool
	Fijada            bool
	// Recordatorio es el indicador del mío, entre la chincheta y los puntos; nil, no hay.
	Recordatorio fyne.CanvasObject
	// Puntos son los colores de los estados de etiqueta puestos.
	Puntos []color.NRGBA
	// Pliegue es el botón de plegar, si la nota tiene hijas. Mas es el «…», si hay
	// acciones. Campo es el de renombrar, mientras se renombra.
	Pliegue *botonMas
	Mas     *botonMas
	Campo   *CampoEnLinea

	AlPulsar func()

	encima, enfocado bool
}

var (
	_ fyne.Widget       = (*FilaNotaArbol)(nil)
	_ fyne.Tappable     = (*FilaNotaArbol)(nil)
	_ fyne.Focusable    = (*FilaNotaArbol)(nil)
	_ desktop.Hoverable = (*FilaNotaArbol)(nil)
)

// NuevaFilaNotaArbol crea la fila.
func NuevaFilaNotaArbol(titulo string) *FilaNotaArbol {
	f := &FilaNotaArbol{Titulo: titulo}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el título, que es lo que dice el botón de la web.
func (f *FilaNotaArbol) AccessibilityLabel() string { return f.Titulo }

// AccessibilityRole: se pulsa para abrir la nota.
func (f *FilaNotaArbol) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

// Tapped abre la nota.
func (f *FilaNotaArbol) Tapped(*fyne.PointEvent) { llamar0(f.AlPulsar) }

// MouseIn y MouseOut son el `hover:bg-bg-subtle` y el `group-hover/fila` del «…».
func (f *FilaNotaArbol) MouseIn(*desktop.MouseEvent)    { f.encima = true; f.Refresh() }
func (f *FilaNotaArbol) MouseMoved(*desktop.MouseEvent) {}
func (f *FilaNotaArbol) MouseOut()                      { f.encima = false; f.Refresh() }

// FocusGained y FocusLost encienden el aro del botón.
func (f *FilaNotaArbol) FocusGained() { f.enfocado = true; f.Refresh() }
func (f *FilaNotaArbol) FocusLost()   { f.enfocado = false; f.Refresh() }

// TypedRune y TypedKey: Espacio e Intro abren, como un `<button>`.
func (f *FilaNotaArbol) TypedRune(r rune) {
	if r == ' ' {
		llamar0(f.AlPulsar)
	}
}

func (f *FilaNotaArbol) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(f, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		llamar0(f.AlPulsar)
	}
}

// verMas dice si el «…» se ve: la fila encima o enfocada, el puntero o el foco en él o
// en el pliegue, y siempre en táctil.
func (f *FilaNotaArbol) verMas() bool {
	if f.Mas == nil {
		return false
	}
	dentro := f.encima || f.enfocado || f.Mas.dentro || f.Mas.enfoque
	if f.Pliegue != nil {
		dentro = dentro || f.Pliegue.dentro || f.Pliegue.enfoque
	}
	return dentro || fyne.CurrentApp().Driver().Device().IsMobile()
}

// CreateRenderer arma la fila. Termina en Refresh (regla 7).
func (f *FilaNotaArbol) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaArbolRenderer{f: f, fondo: canvas.NewRectangle(color.NRGBA{}), aro: ui.NuevoAro(tema.RadioMD, ui.SinHueco, v)}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = r.aro.Antes(r.fondo)
	if f.Pliegue != nil {
		r.objetos = append(r.objetos, f.Pliegue)
	}
	if f.Campo != nil {
		r.objetos = append(r.objetos, f.Campo)
	} else {
		if f.Glifo != "" {
			r.glifo = Glifo(f.Glifo, GlifoFilaArbol, f.TintaGlifo, v)
			r.objetos = append(r.objetos, r.glifo)
		}
		tinta := "text"
		if f.Hecha {
			tinta = "text-muted" // `text-text-muted line-through`
		}
		r.titulo = ui.Texto(f.Titulo, tema.TextoBase, ui.Normal, tema.Color(tinta, v))
		r.meta = ui.Texto(f.Meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		r.objetos = append(r.objetos, r.titulo, r.meta)
		if f.Hecha {
			r.tachado = canvas.NewRectangle(tema.Color(tinta, v))
			r.objetos = append(r.objetos, r.tachado)
		}
		if f.Fijada {
			r.pin = Glifo(FijarD, GlifoPinArbol, "primary-text", v)
			r.objetos = append(r.objetos, r.pin)
		}
		if f.Recordatorio != nil {
			r.objetos = append(r.objetos, f.Recordatorio)
		}
		for _, c := range f.Puntos {
			p := punto(PuntoArbol, c)
			r.puntos = append(r.puntos, p)
			r.objetos = append(r.objetos, p)
		}
	}
	if f.Mas != nil {
		r.objetos = append(r.objetos, f.Mas)
	}
	r.Refresh()
	return r
}

type filaArbolRenderer struct {
	f                 *FilaNotaArbol
	fondo             *canvas.Rectangle
	aro               *ui.Aro
	glifo, pin        fyne.CanvasObject
	titulo, meta      *canvas.Text
	tachado           *canvas.Rectangle
	puntos            []fyne.CanvasObject
	objetos           []fyne.CanvasObject
	botonX, botonAnch float32
}

func (r *filaArbolRenderer) MinSize() fyne.Size {
	alto := AltoFilaArbol()
	if r.f.Campo != nil {
		alto = AltoCampoEnLinea(PYCampoArbol)
	}
	return fyne.NewSize(r.sangria()+HuecoSinPliegue+HuecoFilaArbol+PRFilaArbol, alto)
}

func (r *filaArbolRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaArbolRenderer) Destroy()                     {}

func (r *filaArbolRenderer) sangria() float32 { return float32(r.f.Nivel) * Sangria }

func (r *filaArbolRenderer) Layout(t fyne.Size) {
	f := r.f
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	x := r.sangria()
	if f.Pliegue != nil {
		lado := ui.LadoIconoSM
		f.Pliegue.Move(fyne.NewPos(x, (t.Height-lado)/2))
		f.Pliegue.Resize(fyne.NewSize(lado, lado))
		x += lado
	} else {
		x += HuecoSinPliegue
	}
	x += HuecoFilaArbol
	der := t.Width - PRFilaArbol
	if f.Mas != nil {
		lado := ui.LadoIconoSM
		f.Mas.Move(fyne.NewPos(der-lado, (t.Height-lado)/2))
		f.Mas.Resize(fyne.NewSize(lado, lado))
		der -= lado + HuecoFilaArbol
	}
	r.botonX, r.botonAnch = x, max(der-x, 0)
	r.aro.ColocarEn(fyne.NewPos(x, 0), fyne.NewSize(r.botonAnch, t.Height))
	if f.Campo != nil {
		f.Campo.Move(fyne.NewPos(x, (t.Height-AltoCampoEnLinea(PYCampoArbol))/2))
		f.Campo.Resize(fyne.NewSize(r.botonAnch, AltoCampoEnLinea(PYCampoArbol)))
		return
	}
	r.colocarBoton(x, der, t.Height)
}

// colocarBoton reparte el botón: glifo, textos (flex-1), chincheta y puntos, con
// `gap-1.5` entre todos y centrados en vertical.
func (r *filaArbolRenderer) colocarBoton(x, der, alto float32) {
	centro := func(o fyne.CanvasObject, lado, en float32) {
		o.Move(fyne.NewPos(en, (alto-lado)/2))
		o.Resize(fyne.NewSize(lado, lado))
	}
	if r.glifo != nil {
		centro(r.glifo, GlifoFilaArbol, x)
		x += GlifoFilaArbol + HuecoBotonArbol
	}
	// Lo de la derecha, de fuera hacia dentro.
	for i := len(r.puntos) - 1; i >= 0; i-- {
		der -= PuntoArbol
		centro(r.puntos[i], PuntoArbol, der)
		der -= HuecoBotonArbol
	}
	if o := r.f.Recordatorio; o != nil {
		m := o.MinSize()
		der -= m.Width
		o.Move(fyne.NewPos(der, (alto-m.Height)/2))
		o.Resize(m)
		der -= HuecoBotonArbol
	}
	if r.pin != nil {
		der -= GlifoPinArbol
		centro(r.pin, GlifoPinArbol, der)
		der -= HuecoBotonArbol
	}
	cabe := max(der-x+HuecoBotonArbol, 0)
	arriba := (alto - tema.TextoBase.Linea - tema.TextoXS.Linea) / 2
	visto := ui.Truncar(r.titulo, r.f.Titulo, tema.TextoBase, ui.Normal, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoBase, x, arriba)
	if r.tachado != nil {
		colocarTachado(r.tachado, tema.TextoBase, x, arriba, visto)
	}
	ui.Truncar(r.meta, r.f.Meta, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.meta, tema.TextoXS, x, arriba+tema.TextoBase.Linea)
}

// Refresh: `bg-primary/10` la activa y `bg-bg-subtle` al pasar, sobre la superficie del
// panel.
func (r *filaArbolRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f := r.f
	switch {
	case f.Activa:
		r.fondo.FillColor = tokens.SobreSRGB(velo(tema.Color("primary", v), 0.1), tema.Color("surface", v))
	case f.encima:
		r.fondo.FillColor = tema.Color("bg-subtle", v)
	default:
		r.fondo.FillColor = color.NRGBA{}
	}
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(f.enfocado)
	if f.Mas != nil {
		if f.verMas() {
			f.Mas.Show()
		} else {
			f.Mas.Hide()
		}
	}
}

// Cursor es la mano sobre el botón.
func (f *FilaNotaArbol) Cursor() desktop.Cursor { return desktop.PointerCursor }

// botonPliegue es el chevron de plegar: `aria-expanded`, y avisa a su fila al pasar para
// que el fondo de la fila no parpadee.
func botonPliegue(rotulo string, plegada bool, alCambiar, alPulsar func()) *botonMas {
	b := nuevoBotonMas(rotulo, alCambiar)
	b.Icono, b.TamIcono = iconos.ChevronAbajo, GlifoFilaArbol
	if plegada {
		b.Icono = iconos.ChevronDerecha
	}
	b.AlPulsar = alPulsar
	return b
}
