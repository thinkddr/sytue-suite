// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El calendario de `<input type="date">`
//
// En la web, el calendario de un campo de fecha lo pinta el NAVEGADOR, y es distinto en
// cada uno: no hay hoja que copiar. Aquí es un `Popover` con el mes, sus flechas, las
// iniciales de la semana (de lunes a domingo, como el calendario de la suite) y seis
// semanas, pintado con los tokens de la suite. Se cuelga del icono del `CampoFecha`: lo
// tecleado sigue valiendo, que es como se mete un cumpleaños de 1985.
//
// Las flechas se anuncian con el mes al que llevan («agosto de 2026»): se entienden en
// los 26 idiomas sin claves nuevas.

// Medidas del calendario: celdas de `h-8 w-8` y la cabecera a su altura.
const (
	LadoDiaFecha    float32 = 8 * 0.25 * tema.Rem
	AltoInicialDia  float32 = 6 * 0.25 * tema.Rem
	SeparaCabeceraF float32 = 2 * 0.25 * tema.Rem
)

// ConCalendario hace que el icono del campo de fecha abra el calendario. Elegir un día
// lo escribe en el campo como `LeerFecha` lo lee.
func ConCalendario(e *Entrada, l i18n.Locale, zona *time.Location) *Entrada {
	e.AlPulsarIcono = func() { MostrarCalendario(e, e, l, zona) }
	return e
}

// MostrarCalendario abre el calendario colgado de `disparador` para el campo `e`,
// empezando en la fecha que ya tenga escrita, o en hoy.
func MostrarCalendario(disparador fyne.CanvasObject, e *Entrada, l i18n.Locale, zona *time.Location) *Emergente {
	if zona == nil {
		zona = time.Local
	}
	elegida, _, ok := LeerFecha(e.Texto(), zona)
	if !ok {
		elegida = time.Time{}
	}
	m := NuevoMesFecha(l, elegida, zona)
	var pop *Emergente
	m.AlElegir = func(d time.Time) {
		e.PonerTexto(EscribirFecha(d))
		if pop != nil {
			pop.Cerrar()
		}
	}
	pop = MostrarPopover(disparador, m.Objeto())
	return pop
}

// MesFecha es el calendario de un mes. Es estado: `Objeto` lo pinta y cada flecha lo
// repinta dentro del mismo contenedor.
type MesFecha struct {
	Locale  i18n.Locale
	Zona    *time.Location
	Elegida time.Time
	// Mes es el primer día del mes que se enseña.
	Mes      time.Time
	AlElegir func(time.Time)
	// Hoy se deja abierto para el guardián.
	Hoy time.Time

	caja *fyne.Container
}

// NuevoMesFecha enseña el mes de `elegida`, o el de hoy si es cero.
func NuevoMesFecha(l i18n.Locale, elegida time.Time, zona *time.Location) *MesFecha {
	hoy := time.Now().In(zona)
	base := elegida
	if base.IsZero() {
		base = hoy
	}
	return &MesFecha{
		Locale: l, Zona: zona, Elegida: elegida, Hoy: hoy,
		Mes: time.Date(base.Year(), base.Month(), 1, 0, 0, 0, 0, zona), caja: container.NewStack(),
	}
}

// Objeto es el contenedor que se cuelga en el emergente.
func (m *MesFecha) Objeto() fyne.CanvasObject {
	m.pintar()
	return m.caja
}

// Desplazar cambia de mes.
func (m *MesFecha) Desplazar(meses int) {
	m.Mes = m.Mes.AddDate(0, meses, 0)
	m.pintar()
}

// Dias son las 42 celdas: de lunes a domingo, empezando en la semana del día 1.
func (m *MesFecha) Dias() []time.Time {
	desde := (int(m.Mes.Weekday()) + 6) % 7
	inicio := m.Mes.AddDate(0, 0, -desde)
	out := make([]time.Time, 42)
	for i := range out {
		out[i] = inicio.AddDate(0, 0, i)
	}
	return out
}

func mismaFecha(a, b time.Time) bool {
	return !a.IsZero() && !b.IsZero() && a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func (m *MesFecha) pintar() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := 7 * LadoDiaFecha
	titulo := Texto(i18n.FormatMonthYear(m.Locale, m.Mes), tema.TextoSM, SemiNegrita, tema.Color("text", v))
	flecha := func(icono iconos.Icono, meses int) *Boton {
		b := NuevoBoton("", func() { m.Desplazar(meses) }).Con(Fantasma, TallaSM)
		b.SoloIcono, b.Icono, b.TamIcono = true, icono, iconos.TallaSM
		b.Rotulo = i18n.FormatMonthYear(m.Locale, m.Mes.AddDate(0, meses, 0))
		return b
	}
	antes, despues := flecha(iconos.ChevronIzquierda, -1), flecha(iconos.ChevronDerecha, 1)
	lado := antes.MinSize()
	altoCab := max(lado.Height, tema.TextoSM.Linea)
	ColocarTexto(titulo, tema.TextoSM, 0, (altoCab-tema.TextoSM.Linea)/2)
	despues.Move(fyne.NewPos(ancho-lado.Width, (altoCab-lado.Height)/2))
	despues.Resize(lado)
	antes.Move(fyne.NewPos(ancho-2*lado.Width, (altoCab-lado.Height)/2))
	antes.Resize(lado)
	objs := []fyne.CanvasObject{titulo, antes, despues}

	y := altoCab + SeparaCabeceraF
	for i, ini := range i18n.WeekdaysNarrow(m.Locale) {
		s := NuevoVersal(ini, tema.Texto2XS, SemiNegrita, tema.Color("text-muted", v))
		s.Colocar(float32(i)*LadoDiaFecha+(LadoDiaFecha-s.Ancho())/2, y+(AltoInicialDia-tema.Texto2XS.Linea)/2)
		objs = append(objs, s.Objetos()...)
	}
	y += AltoInicialDia
	for i, d := range m.Dias() {
		dia := d
		c := nuevoDiaFecha(m, dia)
		c.Move(fyne.NewPos(float32(i%7)*LadoDiaFecha, y+float32(i/7)*LadoDiaFecha))
		c.Resize(fyne.NewSize(LadoDiaFecha, LadoDiaFecha))
		objs = append(objs, c)
	}
	alto := y + 6*LadoDiaFecha
	m.caja.Objects = []fyne.CanvasObject{container.New(tamFijo{fyne.NewSize(ancho, alto)}, objs...)}
	m.caja.Refresh()
}

// diaFecha es un día: `grid place-items-center rounded-md text-sm tabular-nums`. Hoy va
// en seminegrita y `primary-text`; el elegido, relleno de `primary`; los de otro mes, en
// `text-subtle`.
type diaFecha struct {
	semana bool
	// marca es el punto bajo el número en la barra de semana del diario: 0 sin notas,
	// 1 con notas, 2 con algo pendiente (`notes-week-dot` de la web).
	marca int
	Pulsable
	m   *MesFecha
	dia time.Time
}

func nuevoDiaFecha(m *MesFecha, d time.Time) *diaFecha {
	c := &diaFecha{m: m, dia: d}
	c.Nombre = i18n.FormatDate(m.Locale, d)
	c.AlPulsar = func() {
		if m.AlElegir != nil {
			m.AlElegir(d)
		}
	}
	c.ExtendBaseWidget(c)
	return c
}

// La celda de la semana (`.notes-week-day` de Notes): `py-1.5 gap-1`, el nombre en
// `text-xs`, el número en un círculo de `h-8 w-8` y el punto de `h-1 w-1`.
var (
	CirculoSemana = tema.Espacio(8)
	PuntoSemana   = tema.Espacio(1)
)

func (c *diaFecha) MinSize() fyne.Size {
	if c.semana {
		return fyne.NewSize(LadoDiaFecha, 2*tema.Espacio(1.5)+tema.TextoXS.Linea+2*tema.Espacio(1)+CirculoSemana+PuntoSemana)
	}
	return fyne.NewSize(LadoDiaFecha, LadoDiaFecha)
}

// NuevaSemanaFecha reutiliza las celdas accesibles del calendario para navegar
// siete días civiles, con el día elegido en el centro (NotebookDashboard.tsx).
// `marca` dice qué punto lleva cada día (ver diaFecha.marca) y lo que se añade a su
// nombre accesible, porque el punto solo no lo oye nadie (WCAG 1.3.1); nil, ninguno.
// Las celdas son los Objects del contenedor, en orden.
func NuevaSemanaFecha(l i18n.Locale, elegida, hoy time.Time, inerte bool, alElegir func(time.Time), marca func(time.Time) (int, string)) *fyne.Container {
	var dias []fyne.CanvasObject
	for i := -3; i <= 3; i++ {
		d := elegida.AddDate(0, 0, i)
		m := &MesFecha{Locale: l, Elegida: elegida, Hoy: hoy, Mes: d, AlElegir: alElegir}
		c := nuevoDiaFecha(m, d)
		c.semana, c.Inerte = true, inerte
		c.Nombre = i18n.FormatFullDate(l, d)
		if marca != nil {
			var extra string
			c.marca, extra = marca(d)
			if extra != "" {
				c.Nombre += " · " + extra
			}
		}
		dias = append(dias, c)
	}
	return container.New(Rejilla{Fijas: 7}, dias...)
}

func (c *diaFecha) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &diaFechaRenderer{c: c, fondo: canvas.NewRectangle(color.NRGBA{}), aro: NuevoAro(tema.RadioMD, SinHueco, v).Abierto()}
	r.fondo.CornerRadius = tema.RadioMD
	peso := Normal
	if mismaFecha(c.dia, c.m.Hoy) {
		peso = SemiNegrita
	} else if c.semana {
		peso = Medio
	}
	r.peso = peso
	r.escalon = tema.TextoSM
	if c.semana {
		r.escalon = tema.TextoBase
		r.fondo.CornerRadius = tema.RadioLG
	}
	r.texto = Texto(strconv.Itoa(c.dia.Day()), r.escalon, peso, tema.Color("text", v))
	fondos := []fyne.CanvasObject{r.fondo}
	if c.semana {
		r.subtitulo = Texto(i18n.WeekdaysShort(c.m.Locale)[(int(c.dia.Weekday())+6)%7], tema.TextoXS, Normal, tema.Color("text-subtle", v))
		r.circulo = canvas.NewRectangle(color.NRGBA{})
		r.circulo.CornerRadius = CirculoSemana / 2
		r.punto = canvas.NewRectangle(color.NRGBA{})
		r.punto.CornerRadius = PuntoSemana / 2
		fondos = append(fondos, r.circulo)
		r.objetos = append(r.objetos, r.subtitulo, r.punto)
	}
	r.objetos = append(append(append(fondos, r.texto), r.objetos...), r.aro.Objetos()...)
	r.Refresh()
	return r
}

type diaFechaRenderer struct {
	subtitulo *canvas.Text
	// circulo y punto solo los tiene la semana.
	circulo, punto *canvas.Rectangle
	escalon        tema.Escalon
	c              *diaFecha
	fondo          *canvas.Rectangle
	texto          *canvas.Text
	peso           Peso
	aro            *Aro
	objetos        []fyne.CanvasObject
}

func (r *diaFechaRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *diaFechaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *diaFechaRenderer) Destroy()                     {}

func (r *diaFechaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	if r.circulo != nil {
		y := tema.Espacio(1.5)
		ancho := Medir(r.subtitulo.Text, tema.TextoXS, Normal).Width
		ColocarTexto(r.subtitulo, tema.TextoXS, (t.Width-ancho)/2, y)
		y += tema.TextoXS.Linea + tema.Espacio(1)
		r.circulo.Move(fyne.NewPos((t.Width-CirculoSemana)/2, y))
		r.circulo.Resize(fyne.NewSize(CirculoSemana, CirculoSemana))
		a := Medir(r.texto.Text, r.escalon, r.peso).Width
		ColocarTexto(r.texto, r.escalon, (t.Width-a)/2, y+(CirculoSemana-r.escalon.Linea)/2)
		y += CirculoSemana + tema.Espacio(1)
		r.punto.Move(fyne.NewPos((t.Width-PuntoSemana)/2, y))
		r.punto.Resize(fyne.NewSize(PuntoSemana, PuntoSemana))
		r.aro.Colocar(t)
		return
	}
	a := Medir(r.texto.Text, tema.TextoSM, r.peso).Width
	y := (t.Height - tema.TextoSM.Linea) / 2
	if r.subtitulo != nil {
		ancho := Medir(r.subtitulo.Text, tema.TextoXS, Normal).Width
		ColocarTexto(r.subtitulo, tema.TextoXS, (t.Width-ancho)/2, tema.Espacio(1))
		y = tema.Espacio(1) + tema.TextoXS.Linea + tema.Espacio(0.5)
	}
	ColocarTexto(r.texto, tema.TextoSM, (t.Width-a)/2, y)
	r.aro.Colocar(t)
}

func (r *diaFechaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := r.c
	delMes := c.dia.Month() == c.m.Mes.Month()
	r.fondo.FillColor = color.NRGBA{}
	switch {
	case mismaFecha(c.dia, c.m.Elegida):
		r.fondo.FillColor, r.texto.Color = tema.Color("primary", v), tema.Color("on-primary", v)
	case c.Encima():
		r.fondo.FillColor, r.texto.Color = tema.Color("bg-subtle", v), tema.Color("text", v)
	case mismaFecha(c.dia, c.m.Hoy):
		r.texto.Color = tema.Color("primary-text", v)
	case delMes:
		r.texto.Color = tema.Color("text", v)
	default:
		r.texto.Color = tema.Color("text-subtle", v)
	}
	if r.circulo != nil {
		// La semana: velo de celda al pasar, círculo `primary` en el elegido, hoy en
		// `primary-text` y el resto en `text-muted`.
		r.fondo.FillColor, r.circulo.FillColor = color.NRGBA{}, color.NRGBA{}
		if c.Encima() {
			r.fondo.FillColor = tema.Color("bg-subtle", v)
		}
		tinta := tema.Color("text-muted", v)
		switch {
		case mismaFecha(c.dia, c.m.Elegida):
			r.circulo.FillColor, tinta = tema.Color("primary", v), tema.Color("on-primary", v)
		case mismaFecha(c.dia, c.m.Hoy):
			tinta = tema.Color("primary-text", v)
		}
		if c.Inerte {
			tinta.A /= 2 // `disabled:opacity-50`
		}
		r.texto.Color = tinta
		r.punto.FillColor = color.NRGBA{}
		switch c.marca {
		case 1:
			r.punto.FillColor = tema.Color("text-subtle", v)
		case 2:
			r.punto.FillColor = tema.Color("primary", v)
		}
		r.subtitulo.Color = tema.Color("text-subtle", v)
		r.subtitulo.Refresh()
		r.circulo.Refresh()
		r.punto.Refresh()
	}
	r.aro.Pintar(v)
	r.aro.Ver(c.Enfocado())
	r.fondo.Refresh()
	r.texto.Refresh()
}
