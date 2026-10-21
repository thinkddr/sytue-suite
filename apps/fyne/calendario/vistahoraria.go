// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"math"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las vistas de SEMANA y DÍA: `TimeGrid.tsx`. El eje de 0 a 24 a la izquierda; cada
// evento colocado por su hora y dimensionado por su duración, los que se solapan lado a
// lado; la franja de «todo el día» arriba, la línea de la hora actual y el arrastre para
// CREAR (pintando el rango), MOVER (también de día, en la semana) y REDIMENSIONAR (desde
// el borde de abajo), imantado a 15 minutos o a 30.
//
// Los números salen de sus clases con el rem de 14:
//
//	hora                 48 px (`HOUR_PX`), 0,8 px por minuto · 1152 el día entero
//	eje                  `w-14` = 49, rótulo `sytue-meta-dense` a `right-1.5`
//	cabecera de día      `px-3 py-2`, versalita y número alineados por la BASE
//	franja todo el día   fila de 22, barra de 18, `4` de relleno arriba y abajo
//	bloque               `rounded-sm border`, franja de color de 3, `py-0.5 pr-1 pl-2`
//
// La cabecera de la web es `sticky` dentro del mismo desplazamiento que el cuerpo; aquí
// es una franja fija encima de un `container.Scroll` que solo mueve el cuerpo, que es
// lo mismo a la vista.
//
// En la web la barra de desplazamiento cubre también la cabecera y le quita ancho a las
// columnas: Blink la pinta clásica, de 15 px, en un ordenador (en el teléfono va
// superpuesta y no ocupa). La de Fyne va superpuesta y fina, así que se le reserva ese
// canalón a la derecha de la cabecera y del cuerpo: las columnas miden lo mismo que en
// la web y la barra de Fyne cae dentro del canalón.
//
// ponytail: el canalón va en `surface` y no con el carril gris claro de Blink, que no
// tiene token; la barra es la de Fyne.
const (
	// CanalonBarra es el ancho de la barra clásica de Blink en escritorio.
	CanalonBarra float32 = 15

	// HoraPx es `HOUR_PX`: el alto de una hora, que fija la escala de toda la rejilla.
	HoraPx float32 = 48
	// PxPorMinuto es `PX_PER_MIN`.
	PxPorMinuto = HoraPx / 60
	// AltoDiaHorario es `TOTAL_PX`: el día entero.
	AltoDiaHorario = MinutosDia * PxPorMinuto
	// RellenoFranja es `STRIP_PAD`.
	RellenoFranja float32 = 4
	// AnchoFranjaColor es el `w-[3px]` del color del evento en un bloque.
	AnchoFranjaColor float32 = 3
	// HuecoBloque es lo que se le resta al ancho de cada bloque: `calc(… − 3px)`.
	HuecoBloque float32 = 3
	// AltoAsa es el `h-2` del asa de redimensionar.
	AltoAsa = 2 * 0.25 * tema.Rem
	// AltoConRango es a partir de cuánto alto un bloque enseña su franja horaria.
	AltoConRango float32 = 34
	// AltoConAsa es a partir de cuánto alto un bloque se deja redimensionar.
	AltoConAsa float32 = 24
	// PasoIman30 es el otro paso del conmutador de imantado.
	PasoIman30 = 30
)

// AnchoEje es el `w-14` de la columna de las horas.
var AnchoEje = tema.Espacio(14)

// escalonTight es un escalón con `leading-tight` (1,25): el de los rótulos de un bloque.
func escalonTight(e tema.Escalon) tema.Escalon { return tema.Escalon{Tam: e.Tam, Linea: e.Tam * 1.25} }

// lineaBase es dónde cae la línea base DENTRO de la caja de línea de un escalón, la
// misma que pinta `ui.ColocarTexto`. Sirve para `items-baseline`. Siempre en
// semi-negrita, que es la única cara que se alinea así en la cabecera.
func lineaBase(e tema.Escalon) float32 {
	return ui.LineaBase(e, ui.SemiNegrita)
}

// alMinuto es `dateAtMinute`: el día a las 00:00 más `min` minutos. `time.Date` desborda
// igual que `setMinutes` de JS.
func alMinuto(dia time.Time, minuto int, loc *time.Location) time.Time {
	d := dia.In(loc)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, minuto, 0, 0, loc)
}

// minutoActual es `currentMinutes`.
func minutoActual(loc *time.Location) int {
	n := time.Now().In(loc)
	return n.Hour()*60 + n.Minute()
}

// diasHorarios son los días de la vista: los siete de la semana o el del cursor.
func (p *Pantalla) diasHorarios() []time.Time {
	loc := p.zona()
	if p.Vista == VistaDia {
		return []time.Time{inicioDelDia(p.Cursor, loc)}
	}
	desde, _ := Rango(VistaSemana, p.Cursor, loc)
	d := make([]time.Time, 7)
	for i := range d {
		d[i] = desde.AddDate(0, 0, i)
	}
	return d
}

// paso es el imantado en curso: 15 por defecto.
func (p *Pantalla) paso() int {
	if p.pasoIman == 0 {
		return PasoIman
	}
	return p.pasoIman
}

// disposicionHoraria es el `layout` y el `allDayBars` de TimeGrid: por día, los bloques
// con hora repartidos en columnas; y las barras de día completo de la franja.
type disposicionHoraria struct {
	dias    []time.Time
	bloques [][]Bloque[Ocurrencia]
	franja  []Tramo[Ocurrencia]
	// todoElDia es el rótulo «Todo el día» ya partido en las líneas que caben en el
	// `w-14` del eje. Vacío si no hay franja.
	todoElDia []string
}

func (p *Pantalla) disponerHorario(dias []time.Time) disposicionHoraria {
	loc := p.zona()
	d := disposicionHoraria{dias: dias, bloques: make([][]Bloque[Ocurrencia], len(dias))}
	var barras []Barra[Ocurrencia]
	for _, o := range p.Eventos {
		ini, fin := ocurrenciaInstantes(o, loc)
		if o.Event.AllDay {
			if desde, hasta, ok := Tramar(dias, ini, fin, loc); ok {
				barras = append(barras, Barra[Ocurrencia]{Item: o, Desde: desde, Hasta: hasta})
			}
			continue
		}
		for i, dia := range dias {
			if iv, ok := MinutosDelDia(ini, fin, dia, loc); ok {
				d.bloques[i] = append(d.bloques[i], Bloque[Ocurrencia]{Item: o, Intervalo: iv})
			}
		}
	}
	for i := range d.bloques {
		d.bloques[i] = Columnas(d.bloques[i])
	}
	if s := Semanas(Apilar(barras), 1, len(dias)); len(s) > 0 {
		d.franja = s[0]
	}
	if len(d.franja) > 0 {
		d.todoElDia = ui.LineasDe(ui.NuevoParrafoConTinta(p.t("event.allDay", nil), tema.Texto2XS, ui.Normal, color.NRGBA{}),
			AnchoEje-2*espacio1p5)
	}
	return d
}

// vistaHoraria monta la cabecera de días, la franja y el cuerpo desplazable.
func (p *Pantalla) vistaHoraria() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	d := p.disponerHorario(p.diasHorarios())
	cab := nuevaCabeceraHoraria(p, d)
	cuerpo := nuevoCuerpoHorario(p, d)
	canalon := CanalonBarra
	if ui.EsTactil() {
		canalon = 0
	}
	desplazar, conScroll := desplazable(container.New(ui.Margen{Der: canalon}, cuerpo))
	// Al abrir, encuadra la jornada: con hoy a la vista, la hora actual; si no, las
	// 08:00, una hora por encima. Solo al MONTAR la vista: navegar de semana conserva
	// el desplazamiento, que por eso vive en la pantalla y no aquí.
	if !p.scrollPuesto {
		foco := 8 * 60
		for _, dia := range d.dias {
			if esHoy(dia) {
				foco = minutoActual(p.zona())
				break
			}
		}
		p.scrollHorario, p.scrollPuesto = max(0, float32(foco-60)*PxPorMinuto), true
	}
	desplazar.Offset = fyne.NewPos(0, p.scrollHorario)
	desplazar.OnScrolled = func(pos fyne.Position) { p.scrollHorario = pos.Y }
	return container.NewStack(canvas.NewRectangle(tema.Color("surface", v)),
		container.New(&arriba{Alto: MedidaCabeceraHoraria(d)}, container.New(ui.Margen{Der: canalon}, cab), conScroll))
}

// ---- La cabecera: días y franja de «todo el día» ----

// AltoFilaDias es la fila de los días: `py-2` y, dentro, versalita y número alineados
// por la base, más el `border-b`.
func AltoFilaDias() float32 {
	bajoXS := tema.TextoXS.Linea - lineaBase(tema.TextoXS)
	bajoSM := tema.TextoSM.Linea - lineaBase(tema.TextoSM)
	contenido := max(lineaBase(tema.TextoXS), lineaBase(tema.TextoSM)) +
		max(bajoXS, bajoSM)
	return 2*espacio2 + contenido + tema.Borde
}

// AltoFranja es la fila de «todo el día»: lo más alto entre el rótulo (que parte en
// líneas dentro de su `w-14`, con `leading-tight`) y las filas de barras con su
// relleno, más el `border-b`. Cero si no hay.
func AltoFranja(d disposicionHoraria) float32 {
	if len(d.franja) == 0 {
		return 0
	}
	barras := float32(Filas(d.franja))*PasoBarra + 2*RellenoFranja
	texto := float32(len(d.todoElDia))*escalonTight(tema.Texto2XS).Linea + 2*espacio1p5
	return max(barras, texto) + tema.Borde
}

// MedidaCabeceraHoraria es lo que ocupa la cabecera entera.
func MedidaCabeceraHoraria(d disposicionHoraria) float32 {
	return AltoFilaDias() + AltoFranja(d)
}

type cabeceraHoraria struct {
	widget.BaseWidget
	p *Pantalla
	d disposicionHoraria
}

func nuevaCabeceraHoraria(p *Pantalla, d disposicionHoraria) *cabeceraHoraria {
	c := &cabeceraHoraria{p: p, d: d}
	c.ExtendBaseWidget(c)
	return c
}

func (c *cabeceraHoraria) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p, l := c.p, c.p.locale()
	r := &cabeceraHorariaRenderer{c: c, fondo: canvas.NewRectangle(tema.Color("surface", v))}
	r.objetos = []fyne.CanvasObject{r.fondo}
	nombres := i18n.WeekdaysShort(l)
	for _, dia := range c.d.dias {
		hoy := esHoy(dia)
		tinta := "text"
		if hoy {
			tinta = "primary-text"
		}
		col := &colDia{
			fondo:  canvas.NewRectangle(color.NRGBA{}),
			semana: ui.NuevoVersal(nombres[(int(dia.Weekday())+6)%7], tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
			numero: ui.Texto(strconv.Itoa(dia.Day()), tema.TextoSM, ui.SemiNegrita, tema.Color(tinta, v)),
			raya:   canvas.NewRectangle(tema.Color("border", v)),
		}
		if hoy {
			col.fondo.FillColor = tema.Color("selected", v)
		}
		r.cols = append(r.cols, col)
		r.objetos = append(r.objetos, col.fondo, col.raya)
		r.objetos = append(r.objetos, col.semana.Objetos()...)
		r.objetos = append(r.objetos, col.numero)
	}
	r.iman = nuevoBotonIman(p)
	r.filete = canvas.NewRectangle(tema.Color("border", v))
	r.objetos = append(r.objetos, r.iman, r.filete)
	if len(c.d.franja) > 0 {
		for _, linea := range c.d.todoElDia {
			tx := ui.Texto(linea, tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v))
			r.rotulo = append(r.rotulo, tx)
			r.objetos = append(r.objetos, tx)
		}
		for _, t := range c.d.franja {
			b := nuevaBarraEvento(t, resumenDe(t.Item, p.t("event.untitled", nil)),
				resumenDe(t.Item, p.t("event.untitled", nil)), p.AlAbrirEvento)
			r.barras = append(r.barras, b)
			r.objetos = append(r.objetos, b)
		}
		r.fileteFranja = canvas.NewRectangle(tema.Color("border", v))
		r.objetos = append(r.objetos, r.fileteFranja)
	}
	return r
}

type colDia struct {
	fondo  *canvas.Rectangle
	semana *ui.Versal
	numero *canvas.Text
	raya   *canvas.Rectangle // el `border-l`, salvo en la primera
}

type cabeceraHorariaRenderer struct {
	c                    *cabeceraHoraria
	fondo                *canvas.Rectangle
	cols                 []*colDia
	iman                 *botonIman
	filete, fileteFranja *canvas.Rectangle
	rotulo               []*canvas.Text
	barras               []*barraEvento
	objetos              []fyne.CanvasObject
}

func (r *cabeceraHorariaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaCabeceraHoraria(r.c.d))
}
func (r *cabeceraHorariaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cabeceraHorariaRenderer) Destroy()                     {}
func (r *cabeceraHorariaRenderer) Refresh()                     { canvas.Refresh(r.c) }

func (r *cabeceraHorariaRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	fila := AltoFilaDias()
	rejilla := ui.Rejilla{Fijas: len(r.cols)}
	util := max(t.Width-AnchoEje, 0)
	ancho := rejilla.AnchoColumna(util)
	bXS, bSM := lineaBase(tema.TextoXS), lineaBase(tema.TextoSM)
	base := max(bXS, bSM)
	for i, col := range r.cols {
		x := AnchoEje + rejilla.X(util, i)
		col.fondo.Move(fyne.NewPos(x, 0))
		col.fondo.Resize(fyne.NewSize(ancho, fila-tema.Borde))
		col.raya.Hidden = i == 0
		col.raya.Move(fyne.NewPos(x, 0))
		col.raya.Resize(fyne.NewSize(tema.Borde, fila-tema.Borde))
		tx := x + espacio3
		if i > 0 {
			tx += tema.Borde
		}
		col.semana.Colocar(tx, espacio2+base-bXS)
		tx += col.semana.Ancho() + espacio1p5
		ui.ColocarTexto(col.numero, tema.TextoSM, tx, espacio2+base-bSM)
	}
	m := r.iman.MinSize()
	r.iman.Move(fyne.NewPos((AnchoEje-m.Width)/2, (fila-tema.Borde-m.Height)/2))
	r.iman.Resize(m)
	r.filete.Move(fyne.NewPos(0, fila-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	if r.fileteFranja == nil {
		return
	}
	alto := AltoFranja(r.c.d)
	e := escalonTight(tema.Texto2XS)
	for i, tx := range r.rotulo {
		w := ui.Medir(tx.Text, tema.Texto2XS, ui.Normal).Width
		ui.ColocarTexto(tx, e, AnchoEje-espacio1p5-w, fila+espacio1p5+float32(i)*e.Linea)
	}
	for _, b := range r.barras {
		colocarBarra(b, b.t, AnchoEje, fila+RellenoFranja+float32(b.t.Fila)*PasoBarra, util, len(r.cols))
	}
	r.fileteFranja.Move(fyne.NewPos(0, fila+alto-tema.Borde))
	r.fileteFranja.Resize(fyne.NewSize(t.Width, tema.Borde))
}

// botonIman es el conmutador de imantado de la esquina: «15 min» ↔ «30 min».
//
//	sytue-meta-dense rounded px-1.5 py-0.5 font-medium tabular-nums
//	hover:text-text hover:bg-bg-subtle
type botonIman struct {
	widget.BaseWidget
	p *Pantalla

	encima, enfocado bool
}

var (
	_ fyne.Tappable     = (*botonIman)(nil)
	_ fyne.Focusable    = (*botonIman)(nil)
	_ desktop.Hoverable = (*botonIman)(nil)
)

func nuevoBotonIman(p *Pantalla) *botonIman {
	b := &botonIman{p: p}
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonIman) rotulo() string {
	return b.p.t("grid.snapValue", i18n.Vars{"count": b.p.paso()})
}

// Tapped alterna 15 ↔ 30. Solo cambia cómo se imanta el arrastre siguiente.
func (b *botonIman) Tapped(*fyne.PointEvent) {
	if b.p.paso() == PasoIman {
		b.p.pasoIman = PasoIman30
	} else {
		b.p.pasoIman = PasoIman
	}
	b.Refresh()
}

func (b *botonIman) FocusGained()                   { b.enfocado = true; b.Refresh() }
func (b *botonIman) FocusLost()                     { b.enfocado = false; b.Refresh() }
func (b *botonIman) TypedRune(rune)                 {}
func (b *botonIman) TypedKey(e *fyne.KeyEvent)      { pulsarConTecla(b, e, func() { b.Tapped(nil) }) }
func (b *botonIman) MouseIn(*desktop.MouseEvent)    { b.encima = true; b.Refresh() }
func (b *botonIman) MouseMoved(*desktop.MouseEvent) {}
func (b *botonIman) MouseOut()                      { b.encima = false; b.Refresh() }

// AccessibilityLabel es `grid.snapAria`: dice el paso y que se cambia pulsando.
func (b *botonIman) AccessibilityLabel() string {
	return b.p.t("grid.snapAria", i18n.Vars{"count": b.p.paso()})
}
func (b *botonIman) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// MinSize: el rótulo con su `px-1.5 py-0.5`.
func (b *botonIman) MinSize() fyne.Size {
	return fyne.NewSize(ui.Medir(b.rotulo(), tema.Texto2XS, ui.Medio).Width+2*espacio1p5,
		tema.Texto2XS.Linea+2*espacio0p5)
}

func (b *botonIman) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonImanRenderer{
		b:     b,
		fondo: canvas.NewRectangle(color.NRGBA{}),
		texto: ui.Texto(b.rotulo(), tema.Texto2XS, ui.Medio, tema.Color("text-subtle", v)),
		aro:   ui.NuevoAro(tema.Espacio(1), 0, v),
	}
	r.fondo.CornerRadius = tema.Espacio(1) // `rounded`: 0,25rem
	r.objetos = r.aro.Antes(r.fondo, r.texto)
	r.Refresh()
	return r
}

type botonImanRenderer struct {
	b       *botonIman
	fondo   *canvas.Rectangle
	texto   *canvas.Text
	aro     *ui.Aro
	objetos []fyne.CanvasObject
}

func (r *botonImanRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonImanRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonImanRenderer) Destroy()                     {}

func (r *botonImanRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	ui.ColocarTexto(r.texto, tema.Texto2XS, espacio1p5, espacio0p5)
	r.aro.Colocar(t)
}

func (r *botonImanRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.texto.Text = r.b.rotulo()
	r.fondo.FillColor = color.NRGBA{}
	r.texto.Color = tema.Color("text-subtle", v)
	if r.b.encima {
		r.fondo.FillColor = tema.Color("bg-subtle", v)
		r.texto.Color = tema.Color("text", v)
	}
	r.fondo.Refresh()
	r.texto.Refresh()
	r.aro.Ver(r.b.enfocado)
}

// ---- El cuerpo: eje, columnas, bloques y arrastre ----

// Los tres gestos de la rejilla.
const (
	gestoCrear = iota + 1
	gestoMover
	gestoEstirar
)

// arrastre es el `Drag` de TimeGrid: lo que se está pintando mientras se arrastra.
type arrastre struct {
	gesto int
	dia   int
	// Crear: el minuto donde se empezó y el de ahora.
	ancla, actual int
	// Mover y estirar: el evento, su columna de solape y su intervalo en curso.
	o         Ocurrencia
	desde     int
	duracion  int // mover
	hasta     int // estirar
	agarre    float64
	col, cols int
	movido    bool
	// bloque es el que se agarró: ancla la vista rápida si fue un clic sin mover.
	bloque *bloqueHorario
}

// Fantasma es el bloque que se queda donde se soltó mientras se guarda (`pending`): sin
// él, el bloque volvería a su sitio viejo hasta que llega la recarga.
type Fantasma struct {
	Ocurrencia
	Dia                     time.Time
	Desde, Hasta, Col, Cols int
}

type cuerpoHorario struct {
	widget.BaseWidget
	p *Pantalla
	d disposicionHoraria

	arr *arrastre
	// ahora es el minuto de la línea roja; lo mueve el reloj de la App.
	ahora int
	// renderer es donde viven los bloques ya colocados, para saber cuál se pulsó.
	renderer *cuerpoRenderer
	// arrastrable es el envoltorio de escritorio, si lo hay.
	arrastrable *cuerpoArrastrable
}

// cuerpoArrastrable es el cuerpo en ESCRITORIO: además de pulsarse, se arrastra. En un
// teléfono deslizar tiene que DESPLAZAR la rejilla —es lo que hace el navegador, que se
// queda el gesto y manda `pointercancel`—, así que allí el cuerpo no es arrastrable.
type cuerpoArrastrable struct{ cuerpoHorario }

var (
	_ fyne.Tappable  = (*cuerpoHorario)(nil)
	_ fyne.Draggable = (*cuerpoArrastrable)(nil)
)

func nuevoCuerpoHorario(p *Pantalla, d disposicionHoraria) fyne.CanvasObject {
	if ui.EsTactil() {
		c := &cuerpoHorario{p: p, d: d, ahora: minutoActual(p.zona())}
		c.ExtendBaseWidget(c)
		p.cuerpoHorario = c
		return c
	}
	c := &cuerpoArrastrable{cuerpoHorario{p: p, d: d, ahora: minutoActual(p.zona())}}
	c.arrastrable = c
	c.ExtendBaseWidget(c)
	p.cuerpoHorario = &c.cuerpoHorario
	return c
}

// MinSize: el día entero de alto.
func (c *cuerpoHorario) MinSize() fyne.Size { return fyne.NewSize(0, AltoDiaHorario) }

// geometria son el ancho de la región de columnas y el de cada columna.
func (c *cuerpoHorario) geometria() (rejilla ui.Rejilla, util, ancho float32) {
	rejilla = ui.Rejilla{Fijas: len(c.d.dias)}
	util = max(c.Size().Width-AnchoEje, 0)
	return rejilla, util, rejilla.AnchoColumna(util)
}

// localizar es `locate`: la columna de día y el minuto crudo de un punto del cuerpo.
func (c *cuerpoHorario) localizar(p fyne.Position) (dia int, minuto float64) {
	_, util, _ := c.geometria()
	n := len(c.d.dias)
	dia = int(math.Floor(float64((p.X - AnchoEje) / (util / float32(n)))))
	return max(0, min(n-1, dia)), float64(p.Y / PxPorMinuto)
}

// Tapped es el clic sin arrastre en una columna vacía: un evento de una hora que
// empieza en el cuarto (o la media) más cercano.
func (c *cuerpoHorario) Tapped(e *fyne.PointEvent) {
	if e.Position.X < AnchoEje || c.p.AlCrearRango == nil {
		return
	}
	dia, m := c.localizar(e.Position)
	lo := Imantar(m, c.p.paso())
	loc := c.p.zona()
	c.p.AlCrearRango(alMinuto(c.d.dias[dia], lo, loc), alMinuto(c.d.dias[dia], min(MinutosDia, lo+60), loc))
}

// bloqueEn es el bloque visible bajo un punto del cuerpo, si lo hay.
func (c *cuerpoHorario) bloqueEn(p fyne.Position) (*bloqueHorario, bool) {
	r := c.renderer
	if r == nil {
		return nil, false
	}
	for i := len(r.bloques) - 1; i >= 0; i-- {
		b := r.bloques[i]
		if b.Hidden {
			continue
		}
		bp, bs := b.Position(), b.Size()
		if p.X >= bp.X && p.X < bp.X+bs.Width && p.Y >= bp.Y && p.Y < bp.Y+bs.Height {
			return b, true
		}
	}
	return nil, false
}

// Dragged empieza o sigue un gesto. El primero decide cuál por dónde se PULSÓ —el punto
// de ahora menos lo arrastrado—: un bloque editable se mueve, su asa lo estira, y todo
// lo demás (también un bloque de solo lectura, como en la web) crea.
func (c *cuerpoArrastrable) Dragged(e *fyne.DragEvent) {
	p := c.p
	if c.arr == nil {
		ini := e.Position.Subtract(e.Dragged)
		if ini.X < AnchoEje {
			return
		}
		dia, m := c.localizar(ini)
		c.arr = &arrastre{gesto: gestoCrear, dia: dia}
		if b, ok := c.bloqueEn(ini); ok && !b.bl.Item.SoloLectura {
			a := &arrastre{dia: b.dia, o: b.bl.Item, desde: b.bl.Desde, col: b.bl.Col, cols: b.bl.Cols, bloque: b}
			if b.Size().Height >= AltoConAsa && ini.Y >= b.Position().Y+b.Size().Height-AltoAsa {
				a.gesto, a.hasta = gestoEstirar, b.bl.Hasta
			} else {
				a.gesto, a.duracion, a.agarre = gestoMover, b.bl.Hasta-b.bl.Desde, m-float64(b.bl.Desde)
			}
			c.arr = a
		} else {
			c.arr.ancla = Imantar(m, p.paso())
			c.arr.actual = c.arr.ancla
		}
	}
	dia, m := c.localizar(e.Position)
	a := c.arr
	switch a.gesto {
	case gestoCrear:
		a.actual = Imantar(m, p.paso())
	case gestoMover:
		a.dia, a.movido = dia, true
		a.desde = max(0, min(MinutosDia-MinutosMinimos, Imantar(m-a.agarre, p.paso())))
	case gestoEstirar:
		a.hasta = min(MinutosDia, max(a.desde+MinutosMinimos, Imantar(m, p.paso())))
	}
	c.Refresh()
}

// DragEnd es `onRegionUp`: crea con el rango pintado, o guarda lo movido.
func (c *cuerpoArrastrable) DragEnd() {
	a := c.arr
	c.arr = nil
	if a == nil {
		return
	}
	p, loc := c.p, c.p.zona()
	dia := c.d.dias[a.dia]
	switch a.gesto {
	case gestoCrear:
		lo, hi := min(a.ancla, a.actual), max(a.ancla, a.actual)
		if hi-lo < p.paso() {
			hi = min(MinutosDia, lo+60) // clic sin arrastre: una hora
		}
		c.Refresh()
		if p.AlCrearRango != nil {
			p.AlCrearRango(alMinuto(dia, lo, loc), alMinuto(dia, hi, loc))
		}
		return
	case gestoMover:
		if !a.movido {
			c.Refresh()
			if p.AlAbrirEvento != nil {
				p.AlAbrirEvento(a.o, a.bloque)
			}
			return
		}
		a.hasta = a.desde + a.duracion
	}
	p.pendiente = &Fantasma{Ocurrencia: a.o, Dia: dia, Desde: a.desde, Hasta: a.hasta, Col: a.col, Cols: a.cols}
	c.Refresh()
	if p.AlMoverEvento != nil {
		p.AlMoverEvento(a.o, alMinuto(dia, a.desde, loc), alMinuto(dia, a.hasta, loc))
	}
}

func (c *cuerpoHorario) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cuerpoRenderer{c: c}
	for h := 1; h < 24; h++ {
		etq := ui.Texto(dosCifras(h)+":00", tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v))
		r.horas = append(r.horas, etq)
	}
	for range 24 {
		r.lineas = append(r.lineas, canvas.NewRectangle(tema.Color("border", v)))
	}
	for range len(c.d.dias) - 1 {
		r.rayas = append(r.rayas, canvas.NewRectangle(tema.Color("border", v)))
	}
	sinTitulo := c.p.t("event.untitled", nil)
	for i, bl := range c.d.bloques {
		for _, b := range bl {
			r.bloques = append(r.bloques, nuevoBloqueHorario(c.p, b, i, sinTitulo))
		}
	}
	r.fantasma = nuevoBloqueFantasma()
	r.borrador = nuevoBorrador()
	r.punto = canvas.NewRectangle(tema.Color("danger", v))
	r.punto.CornerRadius = 4
	r.linea = canvas.NewRectangle(tema.Color("danger", v))
	r.rehacer()
	return r
}

func dosCifras(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

type cuerpoRenderer struct {
	c        *cuerpoHorario
	horas    []*canvas.Text
	lineas   []*canvas.Rectangle // una por hora, a lo ancho de la región
	rayas    []*canvas.Rectangle // el `border-l` de cada columna menos la primera
	bloques  []*bloqueHorario
	fantasma *bloqueFantasma
	borrador *borrador
	punto    *canvas.Rectangle
	linea    *canvas.Rectangle
	objetos  []fyne.CanvasObject
}

func (r *cuerpoRenderer) rehacer() {
	r.c.renderer = r
	r.objetos = r.objetos[:0]
	for _, l := range r.lineas {
		r.objetos = append(r.objetos, l)
	}
	for _, l := range r.rayas {
		r.objetos = append(r.objetos, l)
	}
	for _, h := range r.horas {
		r.objetos = append(r.objetos, h)
	}
	for _, b := range r.bloques {
		r.objetos = append(r.objetos, b)
	}
	r.objetos = append(r.objetos, r.fantasma, r.borrador, r.linea, r.punto)
}

func (r *cuerpoRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *cuerpoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cuerpoRenderer) Destroy()                     {}

// Layout coloca eje, líneas, bloques y lo que se esté arrastrando.
func (r *cuerpoRenderer) Layout(t fyne.Size) {
	c := r.c
	rejilla, util, ancho := c.geometria()
	x0 := func(dia int) float32 { return AnchoEje + rejilla.X(util, dia) }
	for i, h := range r.horas {
		y := float32(i+1) * HoraPx
		w := ui.Medir(h.Text, tema.Texto2XS, ui.Normal).Width
		// `absolute right-1.5 -translate-y-1/2`: centrado en la línea de su hora.
		ui.ColocarTexto(h, tema.Texto2XS, AnchoEje-espacio1p5-w, y-tema.Texto2XS.Linea/2)
	}
	for i, l := range r.lineas {
		l.Move(fyne.NewPos(AnchoEje, float32(i)*HoraPx))
		l.Resize(fyne.NewSize(util, tema.Borde))
	}
	for i, l := range r.rayas {
		l.Move(fyne.NewPos(x0(i+1), 0))
		l.Resize(fyne.NewSize(tema.Borde, t.Height))
	}

	// El evento que se arrastra o se guarda se esconde en su sitio viejo: se pinta el
	// fantasma en el nuevo. Como en la web, se compara por evento, no por ocurrencia.
	activo := ""
	if c.arr != nil && c.arr.gesto != gestoCrear {
		activo = c.arr.o.Event.ID
	} else if c.p.pendiente != nil {
		activo = c.p.pendiente.Event.ID
	}
	for _, b := range r.bloques {
		b.Hidden = activo != "" && b.bl.Item.Event.ID == activo
		colocarBloque(b, x0(b.dia), ancho, b.bl.Desde, b.bl.Hasta, b.bl.Col, b.bl.Cols)
	}

	r.fantasma.Hide()
	r.borrador.Hide()
	loc := c.p.zona()
	switch a := c.arr; {
	case a != nil && a.gesto == gestoCrear:
		lo, hi := min(a.ancla, a.actual), max(a.ancla, a.actual)
		fin := hi
		if hi == lo {
			fin = lo + 60
		}
		r.borrador.rotulo = rangoHoras(c.p, c.d.dias[a.dia], lo, fin, loc)
		r.borrador.Show()
		r.borrador.Move(fyne.NewPos(x0(a.dia)+espacio0p5, float32(lo)*PxPorMinuto))
		r.borrador.Resize(fyne.NewSize(max(ancho-2*espacio0p5, 0), float32(max(MinutosMinimos, hi-lo))*PxPorMinuto))
		r.borrador.Refresh()
	case a != nil:
		hasta := a.hasta
		if a.gesto == gestoMover {
			hasta = a.desde + a.duracion
		}
		r.fantasma.poner(resumenDe(a.o, c.p.t("event.untitled", nil)), rangoHoras(c.p, c.d.dias[a.dia], a.desde, hasta, loc), false)
		r.fantasma.Show()
		colocarBloque(r.fantasma, x0(a.dia), ancho, a.desde, hasta, a.col, a.cols)
	case c.p.pendiente != nil:
		f := c.p.pendiente
		for i, dia := range c.d.dias {
			if mismoDia(dia, f.Dia) {
				r.fantasma.poner(resumenDe(f.Ocurrencia, c.p.t("event.untitled", nil)), rangoHoras(c.p, dia, f.Desde, f.Hasta, loc), true)
				r.fantasma.Show()
				colocarBloque(r.fantasma, x0(i), ancho, f.Desde, f.Hasta, f.Col, f.Cols)
			}
		}
	}

	// La línea de la hora actual, solo en la columna de hoy.
	r.linea.Hide()
	r.punto.Hide()
	for i, dia := range c.d.dias {
		if !esHoy(dia) {
			continue
		}
		y := float32(c.ahora) * PxPorMinuto
		r.linea.Move(fyne.NewPos(x0(i), y))
		r.linea.Resize(fyne.NewSize(ancho, 2))
		r.punto.Move(fyne.NewPos(x0(i)-3, y-4))
		r.punto.Resize(fyne.NewSize(8, 8))
		r.linea.Show()
		r.punto.Show()
	}
}

// colocarBloque es `blockStyle`: arriba por su inicio, alto por su duración (con el
// mínimo de 20 minutos), y a lo ancho su parte de la columna menos 3 px.
func colocarBloque(o fyne.CanvasObject, x, anchoCol float32, desde, hasta, col, cols int) {
	if cols == 0 {
		cols = 1
	}
	o.Move(fyne.NewPos(x+float32(col)/float32(cols)*anchoCol, float32(desde)*PxPorMinuto))
	o.Resize(fyne.NewSize(max(anchoCol/float32(cols)-HuecoBloque, 0), float32(max(MinutosMinimos, hasta-desde))*PxPorMinuto))
}

// rangoHoras es `rangeLabel`: «10:00 – 11:30» con la plantilla del catálogo.
func rangoHoras(p *Pantalla, dia time.Time, desde, hasta int, loc *time.Location) string {
	l := p.locale()
	return p.t("when.timeRange", i18n.Vars{
		"start": i18n.FormatTimeDigits(l, alMinuto(dia, desde, loc)),
		"end":   i18n.FormatTimeDigits(l, alMinuto(dia, hasta, loc)),
	})
}

func (r *cuerpoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, l := range append(r.lineas, r.rayas...) {
		l.FillColor = tema.Color("border", v)
	}
	for _, h := range r.horas {
		h.Color = tema.Color("text-subtle", v)
	}
	r.punto.FillColor = tema.Color("danger", v)
	r.linea.FillColor = tema.Color("danger", v)
	r.Layout(r.c.Size())
	canvas.Refresh(r.c)
}

// Ahora mueve la línea roja al minuto actual: la llama el reloj de la App cada minuto.
func (c *cuerpoHorario) Ahora() {
	c.ahora = minutoActual(c.p.zona())
	c.Refresh()
}
