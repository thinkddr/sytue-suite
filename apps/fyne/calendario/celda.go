// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// celdaMes es un día de la rejilla del mes: el `<button>` de CalendarGrid.tsx que crea
// un evento al pulsarlo, con su número, sus píldoras y su «+N más».
//
// Es un widget y no un contenedor porque recibe el clic, se enfoca y se anuncia con su
// fecha («Crear evento el 21 de septiembre»), que es lo que hace la web.
//
// Puntero, pulsación, teclado y rol de botón son los de `ui.Pulsable`.
type celdaMes struct {
	ui.Pulsable

	m      *Mes
	dia    time.Time
	delMes bool
	hoy    bool
	// visibles son las píldoras que caben; ocultas, las que cuenta el «+N más».
	visibles []Ocurrencia
	ocultas  int
	// reserva es el alto del carril de barras de su semana (`barsAreaH`).
	reserva float32
}

var _ fyne.Widget = (*celdaMes)(nil)

// nuevaCelda arma la celda con sus chips ya recortados al presupuesto de carriles
// (`MAX_LANES` menos las filas de barras de su semana), cuántos quedan fuera —barras
// ocultas y chips que no caben— y el alto que reserva a las barras.
func nuevaCelda(m *Mes, dia time.Time, visibles []Ocurrencia, ocultas int, reserva float32, _ tema.Variante) *celdaMes {
	c := &celdaMes{
		m:        m,
		dia:      dia,
		delMes:   dia.Month() == m.Cursor.Month() && dia.Year() == m.Cursor.Year(),
		hoy:      esHoy(dia),
		visibles: visibles,
		ocultas:  ocultas,
		reserva:  reserva,
	}
	// Pulsarla crea un evento en ese día, que es lo que hace el `<button>` de la celda.
	c.AlPulsar = func() {
		if m.AlCrear != nil {
			m.AlCrear(dia)
		}
	}
	c.ExtendBaseWidget(c)
	return c
}

// esHoy compara por día natural en la zona del propio instante, como `isToday`.
func esHoy(d time.Time) bool {
	h := time.Now().In(d.Location())
	return h.Year() == d.Year() && h.Month() == d.Month() && h.Day() == d.Day()
}

// AccessibilityLabel es el `aria-label` de la web: «Crear evento el <día de mes>».
func (c *celdaMes) AccessibilityLabel() string {
	if c.m.RotuloCrear != nil {
		return c.m.RotuloCrear(c.dia)
	}
	return i18n.FormatDayMonthLong(c.m.Locale, c.dia)
}

// CreateRenderer compone fondo, filetes, número y píldoras.
func (c *celdaMes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El número del día va en `font-semibold` SOLO cuando es hoy, y el peso se fija al
	// construir el texto porque Medir y Texto tienen que usar la MISMA cara (regla 2
	// del paquete `ui`): medir con la regular y pintar con la semi-negrita es cómo se
	// descentra un número dentro de su círculo.
	peso := ui.Normal
	if c.hoy {
		peso = ui.SemiNegrita
	}
	r := &celdaRenderer{
		c:       c,
		peso:    peso,
		fondo:   canvas.NewRectangle(color.NRGBA{}),
		der:     canvas.NewRectangle(tema.Color("border", v)),
		abajo:   canvas.NewRectangle(tema.Color("border", v)),
		circulo: canvas.NewRectangle(color.NRGBA{}),
		numero:  ui.Texto(strconv.Itoa(c.dia.Day()), tema.TextoSM, peso, tema.Color("text-muted", v)),
	}
	r.circulo.CornerRadius = LadoNumeroDia / 2
	r.objetos = []fyne.CanvasObject{r.fondo, r.der, r.abajo, r.circulo, r.numero}
	for _, o := range c.visibles {
		p := nuevaPildora(c.m, o, v)
		r.pildoras = append(r.pildoras, p)
		r.objetos = append(r.objetos, p)
	}
	if c.ocultas > 0 && c.m.MasRotulo != nil {
		r.masTexto = c.m.MasRotulo(c.ocultas)
		r.mas = ui.Texto(r.masTexto, tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.mas)
	}
	r.pintar(v)
	return r
}

type celdaRenderer struct {
	c                 *celdaMes
	peso              ui.Peso
	fondo, der, abajo *canvas.Rectangle
	circulo           *canvas.Rectangle
	numero            *canvas.Text
	pildoras          []*pildora
	masTexto          string
	mas               *canvas.Text
	objetos           []fyne.CanvasObject
}

func (r *celdaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, CarrilBarras+tema.Borde)
}
func (r *celdaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *celdaRenderer) Destroy()                     {}

// pintar fija los colores según el estado, que es donde está toda la lógica de la
// clase `cx(...)` de la web.
func (r *celdaRenderer) pintar(v tema.Variante) {
	c := r.c
	switch {
	// `focus-visible:bg-bg-subtle`: en la celda el foco es un cambio de fondo y no un
	// aro, porque la rejilla no tiene hueco donde dibujarlo.
	case c.Encima() || c.Enfocado():
		// `hover:bg-bg-subtle focus-visible:bg-bg-subtle`, que gana al tinte de
		// «fuera del mes» igual que en la hoja (va después en la cascada).
		r.fondo.FillColor = tema.Color("bg-subtle", v)
	case !c.delMes:
		// `bg-bg-subtle/40`: el mismo token al 40 %, aplanado contra la superficie
		// porque Fyne no tiene opacidad de grupo (hueco conocido del catálogo).
		r.fondo.FillColor = mezclar(tema.Color("surface", v), tema.Color("bg-subtle", v), 0.4)
	default:
		r.fondo.FillColor = tema.Color("surface", v)
	}
	r.der.FillColor = tema.Color("border", v)
	r.abajo.FillColor = tema.Color("border", v)
	switch {
	case c.hoy:
		r.circulo.FillColor = tema.Color("primary", v)
		r.numero.Color = tema.Color("on-primary", v)
	case c.delMes:
		r.circulo.FillColor = color.NRGBA{}
		r.numero.Color = tema.Color("text-muted", v)
	default:
		r.circulo.FillColor = color.NRGBA{}
		r.numero.Color = tema.Color("text-subtle", v)
	}
}

// mezclar compone `a` con `b` al `p` de opacidad. Es el aplanado que sustituye a la
// opacidad de grupo que Fyne no tiene: `tokens.Aplanar`, que redondea como el
// navegador (esta copia truncaba).
func mezclar(a, b color.NRGBA, p float32) color.NRGBA { return tokens.Aplanar(b, float64(p), a) }

// Layout coloca número, píldoras y «+N más» dentro del `p-[6px]`.
func (r *celdaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.der.Move(fyne.NewPos(t.Width-tema.Borde, 0))
	r.der.Resize(fyne.NewSize(tema.Borde, t.Height))
	r.abajo.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.abajo.Resize(fyne.NewSize(t.Width, tema.Borde))

	// El círculo del día: 24×24 pegado a la esquina del relleno.
	r.circulo.Move(fyne.NewPos(RellenoCelda, RellenoCelda))
	r.circulo.Resize(fyne.NewSize(LadoNumeroDia, LadoNumeroDia))
	n := ui.Medir(r.numero.Text, tema.TextoSM, r.peso)
	ui.ColocarTexto(r.numero, tema.TextoSM,
		RellenoCelda+(LadoNumeroDia-n.Width)/2,
		RellenoCelda+(LadoNumeroDia-tema.TextoSM.Linea)/2)

	// Las píldoras arrancan bajo el carril de barras, con `mt-0.5` y `gap-0.5`.
	ancho := max(t.Width-2*RellenoCelda, 0)
	y := CarrilBarras + r.c.reserva + espacio0p5
	for _, p := range r.pildoras {
		h := p.MinSize().Height
		p.Move(fyne.NewPos(RellenoCelda, y))
		p.Resize(fyne.NewSize(ancho, h))
		y += h + espacio0p5
	}
	if r.mas != nil {
		// `px-1.5 text-xs font-medium`, recortado con puntos suspensivos si no cabe.
		ui.Truncar(r.mas, r.masTexto, tema.TextoXS, ui.Medio, ancho-2*espacio1p5)
		ui.ColocarTexto(r.mas, tema.TextoXS, RellenoCelda+espacio1p5, y)
	}
}

// Refresh repinta los colores de estado sin rehacer el árbol.
func (r *celdaRenderer) Refresh() {
	r.pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	for _, o := range r.objetos {
		o.Refresh()
	}
}
