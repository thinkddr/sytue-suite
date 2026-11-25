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
	// PasoBarra y AltoBarra son `BAR_H` y `BAR_INNER_H`: la barra mide 24, como las
	// píldoras, que es el objetivo táctil de WCAG 2.5.8 (medía 18 y era lo único de la
	// celda por debajo), y cada fila suma los 2 px de separación que hay entre píldoras.
	PasoBarra float32 = 26
	AltoBarra float32 = 24
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
	// NumeroSemana, si no es nil, pone el número de semana ISO en la esquina del lunes
	// de cada semana (`numeroSemana` de los ajustes) con su nombre accesible
	// («Semana 45», `grid.weekNumber`).
	NumeroSemana func(n int) string
	// AlMas despliega todos los eventos del día junto a su «+N más» (`ancla`).
	AlMas func(dia time.Time, ancla fyne.CanvasObject)
	// AlMover guarda una ocurrencia arrastrada a otro día (`arrastremes.go`). nil: las
	// píldoras y las barras solo se abren.
	AlMover func(o Ocurrencia, inicio, fin time.Time)

	// arr es el arrastre en curso y renderer, donde están las celdas que resalta.
	arr      *arrastreMes
	renderer *mesRenderer
	// disp es el último reparto (`disponer`) y dispDe, de qué: MinSize lo pide en cada
	// colocación, y repartir otra vez los eventos de las seis semanas era casi la mitad
	// de lo que asignaba un repintado del mes.
	disp   disposicionMes
	dispDe claveDisposicion
}

// claveDisposicion es de lo que sale el reparto: el mes, la zona y los eventos, que se
// reconocen por su lista (el primero y cuántos). Quien cambia los eventos pone una lista
// nueva, como el arrastre (`soltarEnDia`) o la App al fundirlos; no los toca en su sitio.
type claveDisposicion struct {
	cursor time.Time
	zona   *time.Location
	primer *Ocurrencia
	n      int
}

// claveMes es todo lo que decide las piezas de la rejilla: si un repintado da la misma
// que el anterior, las celdas de antes valen tal cual (ver mesRenderer.Refresh).
type claveMes struct {
	claveDisposicion
	locale                i18n.Locale
	v                     tema.Variante
	hoy                   time.Time
	semana, mas           bool
	sinTitulo, desdeAntes string
}

func (m *Mes) claveDisposicion() claveDisposicion {
	k := claveDisposicion{cursor: m.Cursor, zona: m.zona(), n: len(m.Eventos)}
	if len(m.Eventos) > 0 {
		k.primer = &m.Eventos[0]
	}
	return k
}

// clave es la de la rejilla de ahora. `hoy` va porque la celda de hoy se pinta distinta;
// los rótulos que se componen con el catálogo cambian con el idioma.
func (m *Mes) clave(v tema.Variante) claveMes {
	return claveMes{
		claveDisposicion: m.claveDisposicion(), locale: m.Locale, v: v, hoy: inicioDelDia(time.Now(), m.zona()),
		semana: m.NumeroSemana != nil, mas: m.MasRotulo != nil, sinTitulo: m.SinTitulo, desdeAntes: m.DesdeAntes,
	}
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
	return LunesDe(time.Date(cursor.Year(), cursor.Month(), 1, 0, 0, 0, 0, loc))
}

// LunesDe es el lunes de la semana de `t`, a la misma hora (`startOfWeek` de dates.ts):
// la semana empieza en lunes, como decide la app. Es LA regla de «primer día de la
// semana» del paquete; la usan la rejilla del mes, el rango de la semana, la semana
// plegada del teléfono y sus teclas Inicio/Fin.
func LunesDe(t time.Time) time.Time {
	// Lunes = 0. `time.Weekday` pone el domingo en 0, así que se rota.
	return t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
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

// # Lo que cabe en una semana (CAL-08)
//
// Cada semana mide lo que le toca del alto (el `flex-1 min-h-0` de la web), no lo que
// pide su celda más llena: hasta el 2026-11-24 `MinSize` daba a las seis el alto de la
// más llena, a 1280×800 el mes no cabía —la sexta semana quedaba debajo, desplazando
// la vista— y el «+N más» no saltaba nunca. Ahora cada semana enseña las filas de barras
// y las píldoras que caben ENTERAS en su alto, con los topes de la web (`MAX_BAR_ROWS` y
// `MAX_LANES`), y lo demás va al «+N más» de su día. Las cuentas son las de
// `fichasQueCaben` de CalendarGrid.tsx, que mide el alto de la semana desde el
// 2026-11-25; aquí además las filas de barras ceden antes que la última píldora, para
// que nada se salga de su semana.

// pasoFicha es lo que ocupa una píldora con su `gap-0.5` (`FICHA_H + FICHA_GAP`).
var pasoFicha = AltoPildora + espacio0p5

// AltoMinimoSemana es lo menos que mide una semana con algo dentro: el número del día,
// una píldora y el «+N más», con el relleno y el filete de abajo. Con menos alto el mes
// no se aplasta: desplaza la vista (un teléfono en horizontal).
var AltoMinimoSemana = CarrilBarras + espacio0p5 + pasoFicha + tema.TextoXS.Linea + RellenoCelda + tema.Borde

// altoBarras es lo que ocupan `filas` filas de barras hasta el pie de la última, sin la
// separación de debajo: esa la pone el `mt-0.5` de las píldoras (`barsAreaH`).
func altoBarras(filas int) float32 {
	if filas == 0 {
		return 0
	}
	return float32(filas)*PasoBarra - (PasoBarra - AltoBarra)
}

// libreBajo es el alto que queda en una celda de una semana de `alto` bajo el número del
// día y `filas` filas de barras, sin el relleno y el filete de abajo (`CELDA_PIE`). El
// sesenta y cuatroavo absorbe el redondeo de la coma flotante.
func libreBajo(alto float32, filas int) float32 {
	return alto - RellenoCelda - tema.Borde - CarrilBarras - altoBarras(filas) + 1.0/64
}

// fichas es `fichasQueCaben`: cuántas píldoras caben enteras bajo `filas` filas de
// barras, todas sin «+N más» debajo y con él.
func fichas(alto float32, filas int) (todas, conMas int) {
	libre := libreBajo(alto, filas)
	return max(int(libre/pasoFicha), 0), max(int((libre-espacio0p5-tema.TextoXS.Linea)/pasoFicha), 0)
}

// cabenBarras dice si `filas` filas de barras caben en la semana, con el «+N más» debajo
// si `mas`.
func cabenBarras(alto float32, filas int, mas bool) bool {
	libre := libreBajo(alto, filas)
	if mas {
		libre -= espacio0p5 + tema.TextoXS.Linea
	}
	return libre >= 0
}

// cabenDosLineas dice si la píldora en dos líneas cabe en una celda sin barras: la
// web la pone con la ventana a 640 de alto o más, «donde caben sus 38 px».
func cabenDosLineas(alto float32) bool {
	return libreBajo(alto, 0)-espacio0p5 >= AltoPildoraDosLineas
}

// firmaAlto es todo lo que decide el alto de una semana: si cambia, la rejilla se rehace.
type firmaAlto struct {
	fichas [MaxFilasBarra + 1][2]int
	barras [MaxFilasBarra + 1][2]bool
	dos    bool
}

func firmaDe(alto float32) firmaAlto {
	var f firmaAlto
	for filas := range MaxFilasBarra + 1 {
		f.fichas[filas][0], f.fichas[filas][1] = fichas(alto, filas)
		f.barras[filas] = [2]bool{cabenBarras(alto, filas, false), cabenBarras(alto, filas, true)}
	}
	f.dos = cabenDosLineas(alto)
	return f
}

// repartoSemana es lo que se ve de una semana: las barras de sus filas visibles y,
// por día, cuántas píldoras se pintan, cuántas cosas van al «+N más» y si la única
// píldora va en dos líneas.
type repartoSemana struct {
	filas    int
	visibles []Tramo[Ocurrencia]
	pildoras []int
	ocultas  []int
	dos      []bool
}

// repartirSemana decide qué cabe en una semana de `alto` con esos tramos de barra y las
// píldoras de cada día. Si todo cabe sin «+N más», se ve todo (con los topes de la web);
// si no, las filas de barras dejan sitio al menos a una píldora y su «+N más», y de cada
// día se ven las píldoras que quepan enteras con el «+N más» debajo.
func repartirSemana(alto float32, tramos []Tramo[Ocurrencia], chips [][]Ocurrencia) repartoSemana {
	filas := filasDeBarras(alto, tramos, chips)
	r := repartoSemana{filas: filas, pildoras: make([]int, len(chips)), dos: make([]bool, len(chips))}
	r.visibles, r.ocultas = Limitar(tramos, filas, len(chips))
	tope := max(1, MaxCarriles-filas)
	todas, conMas := fichas(alto, filas)
	for c, cs := range chips {
		n := len(cs)
		if r.ocultas[c] == 0 && n <= tope && n <= todas {
			r.pildoras[c] = n
		} else {
			r.pildoras[c] = min(n, tope, conMas)
		}
		r.ocultas[c] += n - r.pildoras[c]
		// La única píldora de un día sin barras en su semana —ni escondidas: entonces
		// lleva «+N más» debajo— sigue en una segunda línea en vez de cortarse en
		// «08:00 Reunión s…» (`dosLineas` de la web).
		r.dos[c] = filas == 0 && n == 1 && r.pildoras[c] == 1 && r.ocultas[c] == 0 && cabenDosLineas(alto)
	}
	return r
}

// filasDeBarras es cuántas filas de barras se ven en la semana. Sin píldoras, las que
// quepan (con el «+N más» debajo si se esconde alguna); con ellas, si con todas algún
// día necesita «+N más», las que dejen sitio al menos a una píldora y su «+N más».
func filasDeBarras(alto float32, tramos []Tramo[Ocurrencia], chips [][]Ocurrencia) int {
	filas := min(Filas(tramos), MaxFilasBarra)
	conPildoras := false
	for _, cs := range chips {
		conPildoras = conPildoras || len(cs) > 0
	}
	switch {
	case !conPildoras:
		for mas := Filas(tramos) > filas; filas > 0 && !cabenBarras(alto, filas, mas); mas = true {
			filas--
		}
	case !cabeTodo(alto, tramos, chips, filas):
		for _, conMas := fichas(alto, filas); filas > 0 && conMas < 1; _, conMas = fichas(alto, filas) {
			filas--
		}
	}
	return filas
}

// cabeTodo dice si, con `filas` filas de barras, ningún día necesita «+N más».
func cabeTodo(alto float32, tramos []Tramo[Ocurrencia], chips [][]Ocurrencia, filas int) bool {
	_, ocultas := Limitar(tramos, filas, len(chips))
	todas, _ := fichas(alto, filas)
	for c, cs := range chips {
		if ocultas[c] > 0 || len(cs) > max(1, MaxCarriles-filas) || len(cs) > todas {
			return false
		}
	}
	return true
}

// MinSize es el de seis semanas con lo mínimo de cada una (AltoMinimoSemana si lleva
// algo, el carril de barras vacío si no): lo que no quepa en el alto que le den va al
// «+N más», y el scroll de la vista se encarga de lo que no llegue a ese mínimo.
func (m *Mes) MinSize() fyne.Size {
	alto := float32(CarrilBarras + PasoBarra)
	d := m.disponer()
	for w, tramos := range d.semanas {
		lleno := len(tramos) > 0
		for _, cs := range d.chips[w*7 : w*7+7] {
			lleno = lleno || len(cs) > 0
		}
		if lleno {
			alto = max(alto, AltoMinimoSemana)
		}
	}
	return fyne.NewSize(0, AltoDiasSemana+tema.Borde+6*alto)
}

// CreateRenderer construye la fila de nombres, las 42 celdas y las barras.
func (m *Mes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &mesRenderer{m: m, fondo: canvas.NewRectangle(tema.Color("surface", v)), alto: AltoSemana(m.Size().Height)}
	m.renderer = r
	r.rehacer(v)
	return r
}

type mesRenderer struct {
	m *Mes
	// alto es el de semana con el que se armó la rejilla: lo que se ve de cada una
	// depende de él (repartirSemana), y Layout la rehace si con el nuevo cabe otra cosa.
	alto float32
	// hecho es la clave del último rehacer: con la misma, Refresh no rehace nada.
	hecho   claveMes
	fondo   *canvas.Rectangle
	dias    []*ui.Versal      // los siete nombres de día
	filete  *canvas.Rectangle // el `border-b` de la fila de nombres
	celdas  []*celdaMes
	barras  [][]*barraEvento  // por semana, las barras visibles
	semanas []*pastillaSemana // el número de cada semana, si se pide
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
// abarca más de una jornada; si no, chip en su día, ordenados por su inicio. Se reparte
// una vez por mes y lista de eventos (`dispDe`).
func (m *Mes) disponer() disposicionMes {
	k := m.claveDisposicion()
	if m.disp.dias != nil && m.dispDe == k {
		return m.disp
	}
	m.disp, m.dispDe = m.repartir(), k
	return m.disp
}

func (m *Mes) repartir() disposicionMes {
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
	r.hecho = m.clave(v)
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
	var barras []fyne.CanvasObject
	for w, tramos := range d.semanas {
		// Filas de barras visibles de la semana y píldoras de cada día, las que caben
		// en su alto (tope MAX_BAR_ROWS y MAX_LANES); lo demás cuenta como «+N más».
		rep := repartirSemana(r.alto, tramos, d.chips[w*7:w*7+7])
		for c := range 7 {
			i := w*7 + c
			ver := d.chips[i][:rep.pildoras[c]]
			r.celdas[i] = nuevaCelda(m, d.dias[i], ver, rep.ocultas[c], altoBarras(rep.filas), v)
			r.celdas[i].dosLineas = rep.dos[c]
			r.objetos = append(r.objetos, r.celdas[i])
		}
		// Las barras van DESPUÉS de las celdas de su semana: cruzan las celdas por
		// encima y reciben el clic; el resto del área pasa a la celda de debajo.
		for _, t := range rep.visibles {
			b, obj := nuevaBarraMes(m, t, m.rotuloEvento(t.Item), m.nombreBarra(t))
			r.barras[w] = append(r.barras[w], b)
			barras = append(barras, obj)
		}
	}
	r.objetos = append(r.objetos, barras...)
	r.semanas = r.semanas[:0]
	if m.NumeroSemana != nil {
		for w := range len(d.semanas) {
			_, n := d.dias[w*7].ISOWeek()
			ps := nuevaPastillaSemana(n, m.NumeroSemana(n))
			r.semanas = append(r.semanas, ps)
			r.objetos = append(r.objetos, ps)
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

// Layout coloca la fila de nombres, las seis semanas y sus barras. Si con el alto
// nuevo caben otros carriles, antes rehace la rejilla (42 celdas, baratas).
func (r *mesRenderer) Layout(t fyne.Size) {
	if alto := AltoSemana(t.Height); alto != r.alto {
		antes := firmaDe(r.alto)
		r.alto = alto
		if firmaDe(alto) != antes {
			r.rehacer(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		}
	}
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
	// `absolute top-[6px] right-1.5` en la celda del lunes, como la web.
	for w, ps := range r.semanas {
		y := arriba + float32(w)*alto + RellenoCelda
		ps.Move(fyne.NewPos(Cuadricula.X(t.Width, 0)+ancho-RellenoCelda-LadoPastillaSemana, y))
		ps.Resize(fyne.NewSize(LadoPastillaSemana, LadoPastillaSemana))
	}
}

// Refresh rehace la rejilla: cambiar de mes o recibir eventos nuevos cambia las 42
// celdas, no solo su color. Si nada de lo que la decide ha cambiado (`claveMes`), se
// queda como está: la pantalla se conserva entre repintados (Pantalla.vistaMes), y un
// repintado por la campana o una invitación no toca el mes.
func (r *mesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.m.clave(v) == r.hecho {
		return
	}
	r.fondo.FillColor = tema.Color("surface", v)
	r.rehacer(v)
	r.Layout(r.m.Size())
	canvas.Refresh(r.m)
}
