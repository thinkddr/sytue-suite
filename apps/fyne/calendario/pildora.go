// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// AltoPildora es lo que mide el `EventChip` de CalendarGrid.tsx: `py-0.5` a cada lado
// sobre la caja de línea de `text-xs`, con su borde dentro (`border-box`).
//
//	1,75 + 14 + 1,75 = 17,5   y el `border` de 1 va DENTRO, así que sigue siendo 17,5
//
// La versión `expanded` (`py-1`), que la vista de día usa, mide 21.
var (
	AltoPildora          = 2*espacio0p5 + tema.TextoXS.Linea
	AltoPildoraExpandida = 2*tema.Espacio(1) + tema.TextoXS.Linea
)

// pildora es el `EventChip`: una píldora sobre `bg-subtle` con borde, el punto del
// color del calendario, la HORA de inicio (`text-muted font-medium tabular-nums`) y el
// resumen recortado.
//
// La hora sale solo desde `sm:` (ventana de 640 o más, `hidden sm:inline`): en el
// teléfono la celda mide unos 50 px y la hora se comería el título entero.
//
// El color del evento va en el PUNTO y no en el relleno. No es estética: el rótulo va
// sobre `bg-subtle` con `text-text`, así que su contraste está garantizado pase lo que
// pase con el color que traiga el calendario. Es lo que dice el comentario de la web.
//
// Puntero, pulsación, teclado y rol de botón son los de `ui.Pulsable`.
type pildora struct {
	ui.Pulsable

	m *Mes
	o Ocurrencia
}

var _ fyne.Widget = (*pildora)(nil)

// nuevaPildora arma la píldora. Pulsarla abre la vista rápida, y el clic NO llega a la
// celda de debajo: Fyne entrega al objeto más profundo que sepa recibirlo, así que aquí
// sale gratis el `e.stopPropagation()` de la web.
func nuevaPildora(m *Mes, o Ocurrencia, _ tema.Variante) *pildora {
	p := &pildora{m: m, o: o}
	p.AlPulsar = func() {
		if m.AlAbrir != nil {
			m.AlAbrir(o, p) // la píldora es el ancla de la vista rápida
		}
	}
	p.ExtendBaseWidget(p)
	return p
}

// resumen es el título del evento, con el «(sin título)» del catálogo si viene vacío.
func (p *pildora) resumen() string {
	if p.o.Event.Summary != "" {
		return p.o.Event.Summary
	}
	return p.m.SinTitulo
}

// hora es la de inicio en la zona de visualización: «08:00», con la convención del
// idioma (`timeLabel`).
func (p *pildora) hora() string {
	return i18n.FormatTimeDigits(p.m.Locale, p.o.Start.In(p.m.zona()))
}

// estrecha dice si la ventana está por debajo de `sm:` (640), que es el teléfono en
// vertical. Mide la VENTANA, como la media query, no el objeto (ver ui.CorteSM). Sin
// lienzo todavía, la del escritorio.
//
// Lo usan la píldora (sin hora en estrecho) y la barra de día completo, que en la
// franja de la semana y del día sí se pinta por debajo de 640 (vistahoraria.go). El mes
// por debajo de 640 ya no es esta rejilla: es el mes del teléfono (vistamescompacta.go).
func estrecha(o fyne.CanvasObject) bool {
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
		return c.Size().Width < ui.CorteSM
	}
	return false
}

// conHora: la hora de inicio sale solo desde `sm:` (`hidden sm:inline`).
func (p *pildora) conHora() bool { return !estrecha(p) }

// AccessibilityLabel es el `title` de la web: el resumen del evento.
func (p *pildora) AccessibilityLabel() string { return p.resumen() }

func (p *pildora) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pildoraRenderer{
		p:     p,
		caja:  ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioSM),
		punto: canvas.NewRectangle(p.o.Color),
		hora:  ui.Texto(p.hora(), tema.TextoXS, ui.Medio, tema.Color("text-muted", v)),
		texto: ui.Texto(p.resumen(), tema.TextoXS, ui.Normal, tema.Color("text", v)),
		aro:   ui.NuevoAro(tema.RadioSM, 0, v),
	}
	r.punto.CornerRadius = LadoPunto / 2
	// El aro DELANTE: es relleno y la caja opaca le tapa el centro. Pintado encima,
	// tapaba la píldora entera (regla de `ui.Aro.Antes`).
	r.objetos = r.aro.Antes(append(r.caja.Objetos(), r.punto, r.hora, r.texto)...)
	r.aro.Ver(false)
	r.Refresh() // regla 7 de `ui`
	return r
}

type pildoraRenderer struct {
	p       *pildora
	caja    *ui.Caja
	punto   *canvas.Rectangle
	hora    *canvas.Text
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

// MinSize es la medida declarada: el alto de la píldora y, de ancho, lo que pida su
// contenido. En la celda se estira a todo el ancho útil (`min-w-0 flex`).
func (r *pildoraRenderer) MinSize() fyne.Size {
	ancho := 2*espacio1p5 + LadoPunto + tema.Espacio(1.5) +
		ui.Medir(r.p.resumen(), tema.TextoXS, ui.Normal).Width
	if r.p.conHora() {
		ancho += ui.Medir(r.p.hora(), tema.TextoXS, ui.Medio).Width + tema.Espacio(1.5)
	}
	return fyne.NewSize(ancho, AltoPildora)
}

func (r *pildoraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pildoraRenderer) Destroy()                     {}

// Layout coloca punto, hora y rótulo dentro del `px-1.5 gap-1.5`. La hora es
// `shrink-0`: lo que no cabe se recorta del título.
func (r *pildoraRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	x := espacio1p5 + tema.Borde
	r.punto.Move(fyne.NewPos(x, (t.Height-LadoPunto)/2))
	r.punto.Resize(fyne.NewSize(LadoPunto, LadoPunto))
	x += LadoPunto + tema.Espacio(1.5)
	if r.p.conHora() {
		r.hora.Show()
		ui.ColocarTexto(r.hora, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
		x += ui.Medir(r.p.hora(), tema.TextoXS, ui.Medio).Width + tema.Espacio(1.5)
	} else {
		r.hora.Hide()
	}
	cabe := t.Width - x - espacio1p5 - tema.Borde
	ui.Truncar(r.texto, r.p.resumen(), tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(r.texto, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
	r.aro.Colocar(t)
}

// Refresh pinta el hover de la web: `hover:border-border-strong hover:bg-surface`.
func (r *pildoraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde := tema.Color("bg-subtle", v), tema.Color("border", v)
	if r.p.Encima() {
		relleno, borde = tema.Color("surface", v), tema.Color("border-strong", v)
	}
	r.caja.Pintar(relleno, borde)
	r.punto.FillColor = r.p.o.Color
	r.punto.Refresh()
	r.hora.Color = tema.Color("text-muted", v)
	r.hora.Refresh()
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
	r.aro.Ver(r.p.Enfocado())
}

// colorDe convierte el `#rrggbb` que trae un calendario o un evento en un color. Lo
// que no se pueda leer cae en el acento por defecto, igual que la web.
func colorDe(s string) color.NRGBA {
	if len(s) != 7 || s[0] != '#' {
		return AcentoPorDefecto
	}
	var c color.NRGBA
	for i, dest := range []*uint8{&c.R, &c.G, &c.B} {
		alto, ok1 := hex(s[1+2*i])
		bajo, ok2 := hex(s[2+2*i])
		if !ok1 || !ok2 {
			return AcentoPorDefecto
		}
		*dest = alto<<4 | bajo
	}
	c.A = 255
	return c
}

func hex(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}
