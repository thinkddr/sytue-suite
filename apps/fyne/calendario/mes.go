// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La rejilla del mes de CalendarGrid.tsx. Todos los números salen de ahí o del volcado
// de producción (`calendar/mes.ancho.json`, 1280×800, rem 14); los marcados «medido»
// son del volcado y los demás, de las clases.
//
//	fila de días    28 de alto (`px-2.5 py-2` sobre `sytue-overline`) + filete    medido
//	semana          111,41 · seis, repartiendo lo que queda                       medido
//	celda           145,703 / 145,719 de ancho, `border-r border-b`, `p-[6px]`    medido
//	número del día  círculo de 24, `text-sm tabular-nums`                         hoja
//	carril de barra 30 desde arriba (6 de relleno + 24 del círculo)               hoja
//	barra           22 de paso, 18 de alto pintado                                hoja
//
// # El ancho de columna NO es 1020/7
//
// El navegador trunca cada columna a 1/64 de píxel y ACUMULA desde ahí: 1020/7 son
// 145,714285 y lo que mide producción es 145,703 = floor(145,714285 × 64)/64. Eso es
// exactamente lo que hace `ui.Rejilla.AnchoColumna`, así que la cuadrícula se pide ahí
// y no se vuelve a calcular aquí. Ver el comentario de `apps/fyne/ui/rejilla.go`.
//
// Lo que queda por debajo, y está MEDIDO: Blink reparte los 5/64 que sobran entre
// algunas columnas (el volcado alterna 145,703 y 145,719 y suma 1019,984), mientras
// que `Rejilla` deja las siete iguales y la última acaba 5/64 corta del margen. Son
// 0,078 px de CSS en el borde derecho de la rejilla entera, y cerrarlo pide meter el
// reparto del sobrante en `Rejilla`, que es pieza compartida.
//
// ponytail: se deja así. El desvío es de 1/13000 del ancho y la pieza es de otros;
// cuando una pantalla lo necesite de verdad, el sitio es `Rejilla.AnchoColumna`, no
// una cuadrícula paralela aquí.
// AltoDiasSemana es la fila de nombres de día: `px-2.5 py-2` sobre `sytue-overline`. Es
// `var` y no `const` porque `tema.Espacio` es una función: la escala de la suite se
// declara UNA vez y no se vuelve a escribir a mano en cada pantalla.
var AltoDiasSemana = 2*espacio2 + tema.TextoXS.Linea

const (
	// RellenoCelda es el `p-[6px]` de cada celda. Va en PÍXELES en la web, no en rem,
	// porque el carril de barras se coloca con un `top` absoluto y las dos capas
	// tienen que coincidir. Es el mismo comentario que hay en CalendarGrid.tsx.
	RellenoCelda float32 = 6
	// LadoNumeroDia es el `h-[24px] w-[24px]` del círculo del día, también en píxeles.
	LadoNumeroDia float32 = 24
	// CarrilBarras es el `DATE_ROW_H` de la web: dónde empieza el carril de barras.
	CarrilBarras = RellenoCelda + LadoNumeroDia
	// PasoBarra y AltoBarra son `BAR_H` y `BAR_INNER_H`.
	PasoBarra float32 = 22
	AltoBarra float32 = 18
	// MaxFilasBarra es `MAX_BAR_ROWS` y MaxCarriles es `MAX_LANES`.
	MaxFilasBarra = 3
	MaxCarriles   = 4
	// LadoPunto es el `h-2 w-2` del punto de color de una píldora.
	LadoPunto float32 = 8
)

var (
	espacio2   = tema.Espacio(2)   // py-2
	espacio2p5 = tema.Espacio(2.5) // px-2.5
	espacio1p5 = tema.Espacio(1.5) // px-1.5 de la píldora
	espacio0p5 = tema.Espacio(0.5) // gap-0.5 entre píldoras, mt-0.5
)

// AcentoPorDefecto es el `DEFAULT_ACCENT` de CalendarGrid.tsx: el hex RESUELTO de
// `primary-600`, no un `var()`. Se escribe resuelto por lo mismo que allí: el contraste
// del rótulo se mide sobre el relleno de verdad.
var AcentoPorDefecto = color.NRGBA{R: 0x1a, G: 0x6f, B: 0xbb, A: 0xff}

// Ocurrencia es una instancia de evento ya situada, con el color de su calendario. Es
// el `GridEvent` de la web: el evento del servicio más de qué calendario viene.
type Ocurrencia struct {
	api.Instance
	// Color es el del evento si lo trae y, si no, el de su calendario.
	Color color.NRGBA
	// CalendarioID es de dónde salió, para editar.
	CalendarioID string
	// SoloLectura es una suscripción ICS o un calendario compartido para ver.
	SoloLectura bool
}

// Mes es la vista de mes: la fila de nombres de día y las seis semanas.
//
// Es un widget y no un contenedor porque cada celda recibe el clic («crear evento
// aquí») y cada píldora el suyo («abrir el evento»), y porque la rejilla se anuncia
// como una sola región, que es lo que hace la web.
type Mes struct {
	widget.BaseWidget

	// Locale decide los nombres de los días.
	Locale i18n.Locale
	// Cursor es el mes que se enseña; el día concreto solo decide cuál.
	Cursor time.Time
	// Zona es la zona de visualización. nil es la del sistema.
	Zona *time.Location
	// Eventos son las ocurrencias ya expandidas del rango visible.
	Eventos []Ocurrencia
	// MasRotulo compone el «+N más» con la clave `grid.more` del catálogo.
	MasRotulo func(n int) string
	// RotuloCrear es el `aria-label` de una celda: «Crear evento el 21 de septiembre»
	// (`nav.createEventOn`). nil: la fecha sola.
	RotuloCrear func(dia time.Time) string
	// DesdeAntes es `cal.fromEarlier`, lo que se añade al nombre del tramo de una barra
	// que viene de la semana anterior.
	DesdeAntes string
	// SinTitulo es `event.untitled`, para un evento sin resumen.
	SinTitulo string
	// AlCrear abre el modal de creación en ese día.
	AlCrear func(dia time.Time)
	// AlAbrir abre la vista rápida del evento junto a lo pulsado.
	AlAbrir func(o Ocurrencia, ancla fyne.CanvasObject)
}

var _ fyne.Widget = (*Mes)(nil)

// NuevoMes crea la rejilla.
func NuevoMes(l i18n.Locale, cursor time.Time) *Mes {
	m := &Mes{Locale: l, Cursor: cursor}
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityLabel: la rejilla se anuncia con el mes que enseña.
func (m *Mes) AccessibilityLabel() string { return i18n.FormatMonthYear(m.Locale, m.Cursor) }

// AccessibilityRole: es una región con celdas dentro, no un control.
func (m *Mes) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// zona es la de visualización, con la del sistema por defecto.
func (m *Mes) zona() *time.Location {
	if m.Zona == nil {
		return time.Local
	}
	return m.Zona
}

// Cuadricula es el reparto de columnas de la rejilla: siete fijas, sin hueco. Es
// pública para que el guardián mida contra ELLA y no contra una cuenta repetida.
var Cuadricula = ui.Rejilla{Fijas: 7}

// PrimerDia es el lunes de la semana en la que cae el día 1 del mes del cursor, que es
// donde arranca la rejilla (`monthGrid` de dates.ts).
func PrimerDia(cursor time.Time, loc *time.Location) time.Time {
	primero := time.Date(cursor.Year(), cursor.Month(), 1, 0, 0, 0, 0, loc)
	// Lunes = 0. `time.Weekday` pone el domingo en 0, así que se rota.
	desde := (int(primero.Weekday()) + 6) % 7
	return primero.AddDate(0, 0, -desde)
}

// Dias son los 42 días de la rejilla.
func Dias(cursor time.Time, loc *time.Location) []time.Time {
	inicio := PrimerDia(cursor, loc)
	dias := make([]time.Time, 42)
	for i := range dias {
		dias[i] = inicio.AddDate(0, 0, i)
	}
	return dias
}

// AltoSemana es lo que mide cada una de las seis semanas con ese alto total de
// rejilla: `flex-1` reparte lo que queda bajo la fila de nombres de día y su filete.
func AltoSemana(alto float32) float32 {
	return max(alto-AltoDiasSemana-tema.Borde, 0) / 6
}

// MinSize: la rejilla se estira a lo que le den. Su mínimo es la fila de días más seis
// semanas capaces de enseñar el número del día y un carril de barras.
func (m *Mes) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoDiasSemana+tema.Borde+6*(CarrilBarras+PasoBarra))
}

// CreateRenderer construye la fila de nombres, las 42 celdas y las barras.
func (m *Mes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &mesRenderer{m: m, fondo: canvas.NewRectangle(tema.Color("surface", v))}
	r.rehacer(v)
	return r
}

type mesRenderer struct {
	m       *Mes
	fondo   *canvas.Rectangle
	dias    []*ui.Versal      // los siete nombres de día
	filete  *canvas.Rectangle // el `border-b` de la fila de nombres
	celdas  []*celdaMes
	barras  [][]*barraEvento // por semana, las barras visibles
	objetos []fyne.CanvasObject
}

// disposicionMes es `buildMonthLayout`: las BARRAS (día completo o varios días),
// apiladas y partidas por semana, y los CHIPS (un solo día con hora) de cada celda.
type disposicionMes struct {
	dias    []time.Time
	semanas [][]Tramo[Ocurrencia]
	chips   [][]Ocurrencia
}

// disponer reparte las ocurrencias entre barras y chips. Barra si es de día completo o
// abarca más de una jornada; si no, chip en su día, ordenados por su inicio.
func (m *Mes) disponer() disposicionMes {
	loc := m.zona()
	d := disposicionMes{dias: Dias(m.Cursor, loc)}
	d.chips = make([][]Ocurrencia, len(d.dias))
	var barras []Barra[Ocurrencia]
	for _, o := range m.Eventos {
		ini, fin := ocurrenciaInstantes(o, loc)
		desde, hasta, ok := Tramar(d.dias, ini, fin, loc)
		if !ok {
			continue
		}
		if o.Event.AllDay || hasta > desde {
			barras = append(barras, Barra[Ocurrencia]{Item: o, Desde: desde, Hasta: hasta})
			continue
		}
		d.chips[desde] = append(d.chips[desde], o)
	}
	for _, c := range d.chips {
		sort.SliceStable(c, func(i, j int) bool { return c[i].Start.Before(c[j].Start) })
	}
	d.semanas = Semanas(Apilar(barras), (len(d.dias)+6)/7, 7)
	return d
}

// rehacer arma el árbol entero. Se llama al construir y cada vez que cambian el mes o
// los eventos: 42 celdas son baratas de rehacer y mucho más fáciles de leer que un
// reciclado, que es lo que hace falta con miles de filas y aquí no.
func (r *mesRenderer) rehacer(v tema.Variante) {
	m := r.m
	r.objetos = []fyne.CanvasObject{r.fondo}
	r.dias = r.dias[:0]
	for _, nombre := range i18n.WeekdaysShort(m.Locale) {
		// `sytue-overline`: versalitas de `text-xs` semi-negrita con su tracking de
		// 0,06 em. Es `text-xs` y no `2xs` (styles.css): los dos escalones tienen la
		// misma caja de línea, 14, así que el alto medido de la fila (28) no los
		// distingue, y aquí estuvo el `2xs` hasta el 2026-09-22.
		d := ui.NuevoVersal(nombre, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
		r.dias = append(r.dias, d)
		r.objetos = append(r.objetos, d.Objetos()...)
	}
	r.filete = canvas.NewRectangle(tema.Color("border", v))
	r.objetos = append(r.objetos, r.filete)

	d := m.disponer()
	r.celdas = make([]*celdaMes, len(d.dias))
	r.barras = make([][]*barraEvento, len(d.semanas))
	for w, tramos := range d.semanas {
		// Filas de barras visibles de la semana (tope MAX_BAR_ROWS); lo demás cuenta
		// como «+N más» por día, y los chips comparten el presupuesto de carriles.
		visibles, ocultas := Limitar(tramos, MaxFilasBarra, 7)
		filas := min(Filas(tramos), MaxFilasBarra)
		tope := max(1, MaxCarriles-filas)
		for c := range 7 {
			i := w*7 + c
			chips := d.chips[i]
			ver := chips[:min(len(chips), tope)]
			r.celdas[i] = nuevaCelda(m, d.dias[i], ver, ocultas[c]+len(chips)-len(ver), float32(filas)*PasoBarra, v)
			r.objetos = append(r.objetos, r.celdas[i])
		}
		// Las barras van DESPUÉS de las celdas de su semana: cruzan las celdas por
		// encima y reciben el clic; el resto del área pasa a la celda de debajo.
		for _, t := range visibles {
			b := nuevaBarraEvento(t, m.rotuloEvento(t.Item), m.nombreBarra(t), m.AlAbrir)
			r.barras[w] = append(r.barras[w], b)
		}
	}
	for _, bs := range r.barras {
		for _, b := range bs {
			r.objetos = append(r.objetos, b)
		}
	}
}

// rotuloEvento es el título, o «(sin título)» si no lo tiene.
func (m *Mes) rotuloEvento(o Ocurrencia) string {
	if o.Event.Summary != "" {
		return o.Event.Summary
	}
	return m.SinTitulo
}

// nombreBarra es el `aria-label` de una barra: el título y, en el tramo que viene de
// una semana anterior, «(viene de antes)».
func (m *Mes) nombreBarra(t Tramo[Ocurrencia]) string {
	n := m.rotuloEvento(t.Item)
	if t.SigueIzq {
		n += m.DesdeAntes
	}
	return n
}

func (r *mesRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *mesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *mesRenderer) Destroy()                     {}

// Layout coloca la fila de nombres, las seis semanas y sus barras.
func (r *mesRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	ancho := Cuadricula.AnchoColumna(t.Width)
	for c, d := range r.dias {
		// `px-2.5 py-2`, y el rótulo se recorta si no cabe (`truncate`).
		d.Colocar(Cuadricula.X(t.Width, c)+espacio2p5, espacio2)
	}
	r.filete.Move(fyne.NewPos(0, AltoDiasSemana))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	alto := AltoSemana(t.Height)
	arriba := AltoDiasSemana + tema.Borde
	for i, c := range r.celdas {
		fila, col := i/7, i%7
		// El borde de arriba de cada semana se acumula desde el alto YA repartido,
		// igual que hace `flex-1`: las seis suman lo que haya, sin dejar un hueco al
		// final por redondear cada una.
		y := arriba + float32(fila)*alto
		c.Move(fyne.NewPos(Cuadricula.X(t.Width, col), y))
		c.Resize(fyne.NewSize(ancho, alto))
	}
	// El carril de barras de cada semana arranca a `DATE_ROW_H` de su borde de
	// arriba, con una fila cada `BAR_H`.
	for w, bs := range r.barras {
		y := arriba + float32(w)*alto + CarrilBarras
		for _, b := range bs {
			colocarBarra(b, b.t, 0, y+float32(b.t.Fila)*PasoBarra, t.Width, 7)
		}
	}
}

// Refresh rehace la rejilla: cambiar de mes o recibir eventos nuevos cambia las 42
// celdas, no solo su color.
func (r *mesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.rehacer(v)
	r.Layout(r.m.Size())
	canvas.Refresh(r.m)
}
