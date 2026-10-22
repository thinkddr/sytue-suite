// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package notes

// Contrapartida de apps/frontend/apps/notes/src/components/DiasVista.tsx y de lo que
// NotebookDashboard.tsx añade al diario: los puntos de la barra de semana, soltar una
// nota sobre un día, las vistas Semana y Días, «Este día» y la agenda de Calendar. Va
// dentro de la pantalla del cuaderno; no es una pantalla aparte del inventario.

import (
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las vistas del diario, como `VISTAS_DIARIO` de la web.
const (
	VistaDia   = "day"
	VistaSem   = "week"
	VistaDias  = "days"
	tramoDias  = 14 // `TRAMO_DIAS`: días por petición en la vista Días
	maxResumen = 140
)

var vistasDiario = []string{VistaDia, VistaSem, VistaDias}

// estadoDiario es lo que el diario trae del servicio además de las notas del día. Cada
// carga lleva su clave: se pide cuando cambia, y la respuesta que llega tarde (de otra
// clave) se tira.
type estadoDiario struct {
	vista string
	// version sube con lo que las otras vistas no ven pasar (mover, crear un evento).
	version  int
	tramos   int
	marcas   map[string]api.DailyDay
	marcasDe string
	dias     map[string]api.DailyDay
	diasDe   string
	diasMal  bool
	este     []api.DailyDay
	esteDe   string
	agenda   []api.EventoAgenda
	agendaDe string
	// semana y semanaDia son la barra pintada y su día central: donde se busca el día
	// sobre el que se suelta una nota.
	semana    *fyne.Container
	semanaDia time.Time
}

func (p *Pantalla) vistaDiario() string {
	if p.dia.vista == "" {
		return VistaDia
	}
	return p.dia.vista
}

// semanaDe son los siete días de la semana de `d`, de lunes a domingo (`semanaDe`).
func semanaDe(d time.Time) []time.Time {
	lunes := d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	out := make([]time.Time, 7)
	for i := range out {
		out[i] = lunes.AddDate(0, 0, i)
	}
	return out
}

var (
	cierreBloque = regexp.MustCompile(`(?i)<(br|/p|/li|/h\d|/div)\b[^>]*>`)
	etiqueta     = regexp.MustCompile(`<[^>]*>`)
	espacios     = regexp.MustCompile(`\s+`)
)

// resumenNota es `resumenNota` de la web: el título o el texto del cuerpo, sin HTML.
func resumenNota(titulo, contenido string, tope int) string {
	s := strings.TrimSpace(titulo)
	if s == "" {
		s = html.UnescapeString(etiqueta.ReplaceAllString(cierreBloque.ReplaceAllString(contenido, " "), ""))
		s = strings.TrimSpace(espacios.ReplaceAllString(strings.ReplaceAll(s, " ", " "), " "))
	}
	if r := []rune(s); len(r) > tope {
		return string(r[:tope-1]) + "…"
	}
	return s
}

func porFecha(dias []api.DailyDay) map[string]api.DailyDay {
	m := make(map[string]api.DailyDay, len(dias))
	for _, d := range dias {
		m[d.Date] = d
	}
	return m
}

// asegurarDiario pide lo que falte para pintar el día `dia` en la vista elegida.
func (p *Pantalla) asegurarDiario(dia time.Time) {
	if p.svc == nil || !p.hayRed() {
		return
	}
	fecha := dia.Format(time.DateOnly)
	v := "|" + strconv.Itoa(p.dia.version+len(p.notas))
	p.pedirDiario(cargaMarcas, &p.dia.marcasDe, fecha+v, dia.AddDate(0, 0, -3), dia.AddDate(0, 0, 3))
	if p.vistaDiario() != VistaDia {
		fechas := p.fechasVista(dia)
		desde, hasta := fechas[len(fechas)-1], fechas[0]
		if p.vistaDiario() == VistaSem {
			desde, hasta = fechas[0], fechas[6]
		}
		p.pedirDiario(cargaDias, &p.dia.diasDe, p.vistaDiario()+desde.Format(time.DateOnly)+hasta.Format(time.DateOnly)+v, desde, hasta)
		return
	}
	p.pedirDiario(cargaEsteDia, &p.dia.esteDe, fecha, dia, dia)
	if p.Suite != nil && p.Suite.Calendar != nil {
		p.pedirDiario(cargaAgenda, &p.dia.agendaDe, fecha+v, dia, dia)
	}
}

// pedirDiario lanza la carga si su clave ha cambiado desde la última vez.
func (p *Pantalla) pedirDiario(c carga, clave *string, nueva string, desde, hasta time.Time) {
	if *clave != nueva {
		*clave = nueva
		go p.cargarDiario(c, clave, nueva, desde, hasta)
	}
}

// carga es qué trae cargarDiario.
type carga int

const (
	cargaMarcas carga = iota
	cargaDias
	cargaEsteDia
	cargaAgenda
)

// cargarDiario trae una carga fuera del hilo de la interfaz, con su plazo, y la pone si
// su clave sigue siendo la que se pidió: la que llega tarde se tira. Corre en su propia
// goroutine y no hereda contexto de quien repinta: lleva el suyo, con el plazo de la
// pantalla, como cargarImagen.
//
//nolint:contextcheck // se lanza con `go` desde el repintado, que no tiene contexto
func (p *Pantalla) cargarDiario(c carga, clave *string, nueva string, desde, hasta time.Time) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	var poner func()
	var err error
	switch c {
	case cargaMarcas:
		var r []api.DailyDay
		r, err = p.svc.DailyRange(ctx, desde.Format(time.DateOnly), hasta.Format(time.DateOnly), false)
		poner = func() { p.dia.marcas = porFecha(r) }
	case cargaDias:
		var todos []api.DailyDay
		// Un tramo por petición: el servicio da 31 días con notas como mucho.
		for fin := hasta; err == nil && !fin.Before(desde); fin = fin.AddDate(0, 0, -tramoDias) {
			ini := fin.AddDate(0, 0, -(tramoDias - 1))
			if ini.Before(desde) {
				ini = desde
			}
			var r []api.DailyDay
			r, err = p.svc.DailyRange(ctx, ini.Format(time.DateOnly), fin.Format(time.DateOnly), true)
			todos = append(todos, r...)
		}
		poner = func() { p.dia.dias, p.dia.diasMal = porFecha(todos), false }
	case cargaEsteDia:
		var r []api.DailyDay
		r, err = p.svc.ThisDay(ctx, desde.Format(time.DateOnly))
		poner = func() { p.dia.este = r }
	case cargaAgenda:
		var r []api.EventoAgenda
		r, err = p.Suite.Agenda(ctx, desde)
		poner = func() { p.dia.agenda = r }
	}
	enHilo(func() {
		if *clave != nueva {
			return
		}
		if err == nil {
			poner()
		} else if c == cargaDias {
			p.dia.diasMal = true
		}
		p.Repintar()
	})
}

// fechasVista son los días de la vista Semana (lunes a domingo) o Días (de `dia` hacia
// atrás, un tramo por cada «Días anteriores»).
func (p *Pantalla) fechasVista(dia time.Time) []time.Time {
	if p.vistaDiario() == VistaSem {
		return semanaDe(dia)
	}
	n := tramoDias * max(p.dia.tramos, 1)
	out := make([]time.Time, n)
	for i := range out {
		out[i] = dia.AddDate(0, 0, -i)
	}
	return out
}

// marcaDia es el punto de un día en la barra: con lo cargado y, el abierto, con lo que
// ya está en pantalla.
func (p *Pantalla) marcaDia(dia, abierto time.Time) (int, string) {
	total, pendientes := 0, 0
	// El día abierto lo deciden sus notas cargadas: el recuento del servidor no se entera
	// de lo que acaba de pasar. Mientras carga aún son las del día anterior, y el diario
	// de otra persona no es el mío de ese día.
	cargado := p.lienzo.Fase == Exito || p.lienzo.Fase == Vacia
	if dia.Equal(abierto) && cargado && p.esMio(p.activo.Crudo.OwnerSub) {
		total = len(p.notas)
		for _, n := range p.notas {
			if n.Tipo == "todo" && !n.Marcada && !n.Cancelada {
				pendientes++
			}
		}
	} else if m, ok := p.dia.marcas[dia.Format(time.DateOnly)]; ok {
		total, pendientes = m.Total, m.Pending
	}
	if total == 0 {
		return 0, ""
	}
	marca := 1
	if pendientes > 0 {
		marca = 2
	}
	return marca, p.tr.T("daily.marks", i18n.Vars{"total": total, "pending": pendientes})
}

// selectorVista es el segmentado Día / Semana / Días (`.notes-seg`).
func (p *Pantalla) selectorVista(v tema.Variante) fyne.CanvasObject {
	bs := make([]fyne.CanvasObject, 0, len(vistasDiario))
	for _, vista := range vistasDiario {
		bs = append(bs, segmento(p.tr.T("daily.view."+vista, nil), "", p.vistaDiario() == vista, v, func() {
			p.cerrarEditor()
			p.dia.vista, p.dia.tramos = vista, 1
			p.Repintar()
		}))
	}
	return container.New(filaCentro{hueco: tema.Espacio(0.5)}, bs...)
}

// soltarEnDia es el final del asa de una nota del diario: si el puntero está sobre un
// día de la barra que no es el abierto, la nota se mueve allí. Si no, vuelve a su sitio.
func (p *Pantalla) soltarEnDia(n Nota, en fyne.Position) {
	defer p.Repintar()
	if p.dia.semana == nil || !p.puedeEscribir() || p.activo.Cifrado {
		return
	}
	drv := fyne.CurrentApp().Driver()
	for i, o := range p.dia.semana.Objects {
		a, t := drv.AbsolutePositionForObject(o), o.Size()
		if en.X < a.X || en.Y < a.Y || en.X > a.X+t.Width || en.Y > a.Y+t.Height {
			continue
		}
		if i != 3 {
			fecha := p.dia.semanaDia.AddDate(0, 0, i-3).Format(time.DateOnly)
			go func() { _ = p.moverADia(n.ID, fecha) }()
		}
		return
	}
}

// moverADia abre (o crea) el diario de `fecha` sin ir a él y mueve la nota allí, con
// deshacer (`moverADia` de la web). Fuera del hilo de la interfaz.
func (p *Pantalla) moverADia(id, fecha string) error {
	d, err := time.ParseInLocation(time.DateOnly, fecha, time.Local)
	if err != nil {
		return err
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	nb, err := p.svc.DailyNotebook(ctx, fecha, i18n.FormatDateLong(p.loc, d), false)
	if err != nil {
		enHilo(func() { p.fallo("move.failed", err) })
		return err
	}
	destino := DeCuaderno(nb)
	fyne.DoAndWait(func() {
		if !slices.ContainsFunc(p.cuadernos, func(c Cuaderno) bool { return c.ID == destino.ID }) {
			p.cuadernos = append(p.cuadernos, destino)
		}
	})
	err = p.moverACuaderno(id, destino, true)
	enHilo(func() { p.dia.version++; p.Repintar() })
	return err
}

// crearEvento es «Añadir al calendario»: un evento de día completo el día del diario (u
// hoy) en el calendario por defecto, enlazado a la nota.
func (p *Pantalla) crearEvento(n Nota) {
	if p.Suite == nil || p.Suite.Calendar == nil || p.activo.Cifrado || !p.puedeEscribir() {
		return
	}
	fecha := p.activo.Crudo.DailyDate
	if fecha == "" {
		fecha = time.Now().Format(time.DateOnly)
	}
	resumen := resumenNota(n.Titulo, n.Contenido, 200)
	if resumen == "" {
		resumen = p.tr.T("note.untitled", nil)
	}
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		l, err := p.Suite.CrearEventoDelDia(ctx, fecha, resumen)
		if err == nil {
			err = p.editar(n.ID, Parche{Enlaces: append(slices.Clone(n.Enlaces), l)}, false)
		}
		enHilo(func() {
			if err != nil {
				p.fallo("event.failed", err)
				return
			}
			p.dia.version++
			p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.tr.T("event.created", nil), ""))
			p.Repintar()
		})
	}()
}

// ---- Lo que se pinta ----

func (p *Pantalla) texto(s string, e tema.Escalon, peso ui.Peso, tinta string, v tema.Variante) *ui.Parrafo {
	return ui.NuevoParrafo(s, e, peso, tinta, v)
}

// cabeceraEnDia es la cabecera de un día que lo abre (`notes-days-head`): texto que se
// pulsa, no un botón. Hoy va en `primary-text`.
func (p *Pantalla) cabeceraEnDia(rotulo string, d time.Time, e tema.Escalon, v tema.Variante) fyne.CanvasObject {
	tinta := "text"
	if d.Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
		tinta = "primary-text"
	}
	o := NuevaOpcion(rotulo, lineaTok(conMayuscula(rotulo), e, ui.SemiNegrita, tinta, v), func() {
		if !p.hayRed() {
			return
		}
		p.dia.vista = VistaDia
		llamar1(p.lateral.AlDia, d.Format(time.DateOnly))
	})
	o.PX, o.Radio, o.Superficie = tema.Espacio(1), tema.RadioSM, "bg"
	return container.New(ui.FilaAuto{Derecha: 1}, o)
}

// lineaNota es una nota en una línea: la casilla de una tarea o un punto, y su resumen.
func (p *Pantalla) lineaNota(n api.Note, v tema.Variante) fyne.CanvasObject {
	var marca fyne.CanvasObject
	lado := tema.Espacio(3.5)
	tinta := "text"
	if n.Kind == "todo" {
		caja := canvas.NewRectangle(tema.Color("surface", v))
		caja.StrokeColor, caja.StrokeWidth, caja.CornerRadius = tema.Color("border-strong", v), tema.Borde, tema.Espacio(0.5)
		if n.Checked {
			caja.FillColor, caja.StrokeColor = tema.Color("primary", v), tema.Color("primary", v)
			tinta = "text-muted"
		}
		marca = caja
	} else {
		lado = tema.Espacio(1)
		marca = canvas.NewCircle(tema.Color("text-subtle", v))
	}
	cuerpo := p.texto(resumenNota(n.Title, n.Content, maxResumen), tema.TextoSM, ui.Normal, tinta, v)
	return container.New(filaMarca{lado: lado, hueco: tema.Espacio(2), ancho: tema.Espacio(3.5), pad: tema.Espacio(1)}, marca, cuerpo)
}

// filaMarca es `flex items-start gap-2 px-1 py-1`: la marca, centrada en la primera
// línea, y el texto que se reparte en lo que queda. Da su alto al ancho real (AltoCon),
// que es lo que una fila de Fyne no sabe.
type filaMarca struct{ lado, hueco, ancho, pad float32 }

func (f filaMarca) resto(ancho float32) float32 { return max(ancho-2*f.pad-f.ancho-f.hueco, 1) }

func (f filaMarca) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return 2*f.pad + max(tema.TextoSM.Linea, ui.AltoA(objs[1], f.resto(ancho)))
}

func (f filaMarca) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(2*f.pad+f.ancho+f.hueco, 2*f.pad+tema.TextoSM.Linea)
}

func (f filaMarca) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(f.pad+(f.ancho-f.lado)/2, f.pad+(tema.TextoSM.Linea-f.lado)/2))
	objs[0].Resize(fyne.NewSize(f.lado, f.lado))
	w := f.resto(t.Width)
	objs[1].Move(fyne.NewPos(f.pad+f.ancho+f.hueco, f.pad))
	objs[1].Resize(fyne.NewSize(w, ui.AltoA(objs[1], w)))
}

// listaDia son las notas de un día, una línea cada una. Una nota que es solo un separador
// no tiene línea que enseñar aquí. `vacio` dice «nada todavía» en un día sin notas; en la
// semana se deja en blanco.
func (p *Pantalla) listaDia(d api.DailyDay, vacio bool, v tema.Variante) []ui.Bloque {
	out := make([]ui.Bloque, 0, len(d.Notes))
	for _, n := range d.Notes {
		if resumenNota(n.Title, n.Content, maxResumen) == "" {
			continue
		}
		out = append(out, ui.Bloque{Obj: p.lineaNota(n, v)})
	}
	if len(out) == 0 && vacio {
		return []ui.Bloque{{Obj: container.New(ui.Margen{Izq: tema.Espacio(1), Arr: tema.Espacio(1), Abj: tema.Espacio(1)},
			p.texto(p.tr.T("daily.empty", nil), tema.TextoSM, ui.Normal, "text-subtle", v))}}
	}
	return out
}

// vistaDias es Semana (siete columnas, una en el teléfono) o Días (el diario seguido).
func (p *Pantalla) vistaDias(dia time.Time, v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	if p.dia.diasMal {
		return container.New(ui.Margen{Izq: tema.Espacio(6), Arr: tema.Espacio(6)}, p.texto(t("error.loadFailed", nil), tema.TextoSM, ui.Normal, "text-muted", v))
	}
	if p.dia.dias == nil {
		return centrar(ui.NuevaRueda(20, "text-muted", v), v)
	}
	margenX := tema.Espacio(4)
	if p.anchoVentana() >= ui.CorteSM {
		margenX = tema.Espacio(6)
	}
	margen := ui.Margen{Izq: margenX, Der: margenX, Abj: tema.Espacio(6)}
	fechas := p.fechasVista(dia)
	if p.vistaDiario() == VistaSem {
		cols := make([]fyne.CanvasObject, 0, 7)
		apiladas := make([]ui.Bloque, 0, 7)
		movil := p.anchoVentana() < ui.CorteMD
		// Siete columnas separadas por una raya fina, sin tarjetas (`.notes-week-col`): el
		// ancho del cuerpo de cada una, para que los párrafos sepan cuántas líneas ocupan.
		cuerpo := p.anchoContenido() - 2*margenX
		if !movil {
			cuerpo /= 7
		}
		cuerpo = max(cuerpo-2*tema.Espacio(2), 1)
		for i, d := range fechas {
			rotulo := i18n.WeekdaysShort(p.loc)[(int(d.Weekday())+6)%7] + " " + strconv.Itoa(d.Day())
			bloques := append([]ui.Bloque{{Obj: p.cabeceraEnDia(rotulo, d, tema.TextoSM, v), Margen: tema.Espacio(1.5)}}, p.listaDia(p.dia.dias[d.Format(time.DateOnly)], false, v)...)
			col := container.New(&columnaSemana{raya: i < 6 && !movil, abajo: movil && i < 6, v: v},
				canvas.NewRectangle(tema.Color("border-subtle", v)),
				container.New(ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(3), Abj: tema.Espacio(3)},
					ui.AlAnchoFluido(cuerpo, ui.PilaBloques(bloques...))))
			cols = append(cols, col)
			apiladas = append(apiladas, ui.Bloque{Obj: col})
		}
		margenSem := ui.Margen{Izq: margenX, Der: margenX, Arr: tema.Espacio(2), Abj: tema.Espacio(8)}
		// `grid-cols-1 md:grid-cols-7`: por debajo de md, una columna con el alto de cada
		// día; la rejilla igualaría todas al más alto.
		if movil {
			return container.New(margenSem, ui.PilaBloques(apiladas...))
		}
		return container.New(margenSem, container.New(&rejillaVentana{cortes: ui.Cortes{1, 0, 7, 0, 0, 0}}, cols...))
	}
	var bloques []ui.Bloque
	for _, d := range fechas {
		dd, ok := p.dia.dias[d.Format(time.DateOnly)]
		if !ok && !d.Equal(dia) {
			continue
		}
		entrada := append([]ui.Bloque{{Obj: p.cabeceraEnDia(i18n.FormatWeekdayDayMonth(p.loc, d), d, tema.TextoLG, v), Margen: tema.Espacio(1)}}, p.listaDia(dd, true, v)...)
		bloques = append(bloques, ui.Bloque{Obj: ui.PilaBloques(entrada...), Margen: tema.Espacio(6)})
	}
	mas := botonDiario(t("daily.more", nil), ui.TallaSM, false, func() { p.dia.tramos = max(p.dia.tramos, 1) + 1; p.Repintar() })
	bloques = append(bloques, ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1}, mas)})
	// La misma columna de lectura que el día (`.notes-col`), con `mt-6`.
	margen.Arr = tema.Espacio(6)
	margen.Abj = tema.Espacio(10)
	return container.New(&acotado{ancho: AnchoLectura + 2*margenX}, container.New(margen, ui.PilaBloques(bloques...)))
}

// columnaSemana es una columna de la semana: su contenido y una raya fina a la derecha
// (o debajo, en el teléfono), que es el `border-r`/`border-b` de `.notes-week-col`.
type columnaSemana struct {
	raya, abajo bool
	v           tema.Variante
}

func (c *columnaSemana) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, max(m.Height, map[bool]float32{true: 0, false: 16 * tema.Rem}[c.abajo]))
}

func (c *columnaSemana) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(fyne.NewSize(t.Width, objs[1].MinSize().Height))
	switch {
	case c.raya:
		objs[0].Move(fyne.NewPos(t.Width-tema.Borde, 0))
		objs[0].Resize(fyne.NewSize(tema.Borde, t.Height))
	case c.abajo:
		objs[0].Move(fyne.NewPos(0, t.Height-tema.Borde))
		objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	default:
		objs[0].Resize(fyne.NewSize(0, 0))
	}
}

// esteDia es «Este día»: el mismo día de años anteriores, solo si hay algo.
func (p *Pantalla) esteDia(v tema.Variante) fyne.CanvasObject {
	if len(p.dia.este) == 0 {
		return nil
	}
	bloques := []ui.Bloque{{Obj: p.texto(strings.ToUpper(p.tr.T("onThisDay.title", nil)), tema.TextoXS, ui.Medio, "text-subtle", v), Margen: tema.Espacio(2)}}
	for _, d := range p.dia.este {
		f, err := time.ParseInLocation(time.DateOnly, d.Date, time.Local)
		if err != nil {
			continue
		}
		entrada := append([]ui.Bloque{{Obj: p.cabeceraEnDia(i18n.FormatDateLong(p.loc, f), f, tema.TextoBase, v), Margen: tema.Espacio(1)}}, p.listaDia(d, true, v)...)
		bloques = append(bloques, ui.Bloque{Obj: ui.PilaBloques(entrada...), Margen: tema.Espacio(2)})
	}
	margenX := tema.Espacio(4)
	if p.anchoVentana() >= ui.CorteSM {
		margenX = tema.Espacio(6)
	}
	// `.notes-this-day`: una raya fina arriba, `mt-2 pt-5 pb-10`, en la columna de lectura.
	raya := canvas.NewRectangle(tema.Color("border-subtle", v))
	raya.SetMinSize(fyne.NewSize(1, tema.Borde))
	cuerpo := ui.PilaBloques(ui.Bloque{Obj: raya, Margen: tema.Espacio(5)}, ui.Bloque{Obj: ui.PilaBloques(bloques...)})
	return container.New(&acotado{ancho: AnchoLectura + 2*margenX},
		container.New(ui.Margen{Izq: margenX, Der: margenX, Arr: tema.Espacio(2), Abj: tema.Espacio(10)}, cuerpo))
}

// agendaDia es la agenda del día desde Sytue Calendar; sin eventos, nada.
func (p *Pantalla) agendaDia(v tema.Variante, interior float32) fyne.CanvasObject {
	if len(p.dia.agenda) == 0 {
		return nil
	}
	// Las mismas filas que las notas (`.notes-agenda`): el glifo del calendario en la
	// columna fija, el nombre y la hora a la derecha. Sin caja; una raya fina al final.
	bloques := make([]ui.Bloque, 0, len(p.dia.agenda)+1)
	for _, e := range p.dia.agenda {
		hora := p.tr.T("agenda.allDay", nil)
		if !e.DiaCompleto {
			hora = i18n.FormatTime(p.loc, e.Inicio.Local())
		}
		ruta := e.Ruta
		nombre := ui.NuevoEnlace(e.Nombre, tema.TextoBase, ui.Normal, "text-muted", func() {
			if p.AbrirCalendar != nil {
				p.AbrirCalendar(ruta)
			}
		})
		glifo := container.NewCenter(Icono(iconos.Calendario, 14, "text-subtle", v))
		fila := container.New(ui.FilaAuto{Hueco: tema.Espacio(2), Derecha: 2, Elastico: 2, AlturaFila: tema.Espacio(8)},
			glifo, nombre, lineaTok(hora, tema.TextoXS, ui.Normal, "text-subtle", v))
		bloques = append(bloques, ui.Bloque{Obj: fila})
	}
	raya := canvas.NewRectangle(tema.Color("border-subtle", v))
	raya.SetMinSize(fyne.NewSize(1, tema.Borde))
	bloques[len(bloques)-1].Margen = tema.Espacio(2)
	bloques = append(bloques, ui.Bloque{Obj: raya})
	return container.New(ui.Margen{Arr: tema.Espacio(1)}, ui.AlAnchoFluido(interior, ui.PilaBloques(bloques...)))
}
