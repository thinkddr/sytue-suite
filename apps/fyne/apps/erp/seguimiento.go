// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"cmp"
	"fmt"
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

// # Lo que cuelga del registro (`Seguimiento.tsx`, `Libro.tsx` y `Verifactu.tsx`)
//
// Debajo de los campos: los asientos que generó el documento, sus tareas, sus
// documentos, lo que dijo la Agencia Tributaria de la factura, el historial y la
// trazabilidad. Medido en la ficha del arnés (2026-09-23), FV-2026-00041:
//
//	asientos      h3 14 + `mb-2`, asiento 164,25 (`px-4 py-3`), «Anular» `mt-1`
//	tareas        cabecera 24, filas 31,5 con `space-y-1`                     97,5
//	documentos    cabecera 24, lista `rounded-md border`, fila 38 (`px-3 py-2`) 71
//	VERI*FACTU    insignia, CSV y enlace que saltan (38,5), error, reenviar 115,5
//	historial     nota de dos filas (44) + entradas 29,75/28 con `space-y-2` 171,75
//	trazabilidad  `border-t pt-4`, cabecera 14, `mb-2`, rejilla de 29             65

// Medidas del seguimiento.
const (
	// TituloSeccionMB es el `mb-2` del `<h3>` de la sección de asientos, y
	// SeccionGap el `space-y-2` de las demás.
	TituloSeccionMB float32 = 2 * 0.25 * tema.Rem
	SeccionGap      float32 = 2 * 0.25 * tema.Rem
	// TrackingWide es el `tracking-wide` de los `<h3>`.
	TrackingWide float32 = 0.025
	// AsientoPX y AsientoPY son el `px-4 py-3` de un asiento.
	AsientoPX float32 = 4 * 0.25 * tema.Rem
	AsientoPY float32 = 3 * 0.25 * tema.Rem
	// CeldaAsientoPY es el `py-1` de su tabla.
	CeldaAsientoPY float32 = 1 * 0.25 * tema.Rem
	// AnularMT es el `mt-1` de «Anular».
	AnularMT float32 = 1 * 0.25 * tema.Rem
	// TareasGap es el `space-y-1` de la lista de tareas.
	TareasGap float32 = 1 * 0.25 * tema.Rem
	// DocPX y DocPY son el `px-3 py-2` de un documento.
	DocPX float32 = 3 * 0.25 * tema.Rem
	DocPY float32 = 2 * 0.25 * tema.Rem
	// NotaPX y NotaPY son el `px-2 py-1.5` del área de la nota.
	NotaPX float32 = 2 * 0.25 * tema.Rem
	NotaPY float32 = 1.5 * 0.25 * tema.Rem
	// AltoValorTraza es la segunda línea de la trazabilidad: la mono dentro de un
	// renglón de `text-xs` levanta la caja a 15. Medido: 29 la celda entera.
	AltoValorTraza float32 = 16
	// PlanP es el `p-3` del editor de «Planificar tarea»; su `gap-2` y el `gap-1` de
	// cada etiqueta.
	PlanP          float32 = 3 * 0.25 * tema.Rem
	PlanGap        float32 = 2 * 0.25 * tema.Rem
	PlanEtiquetaGp float32 = 1 * 0.25 * tema.Rem
	// AltoSelectTarea es el `<select>` del tipo de tarea: 28,5, medido.
	AltoSelectTarea float32 = 28.5
	// AnchoVencePlan es el `<input type="date">` del navegador a `text-sm`: 118,38.
	// Medido; su ancho lo pone el navegador y no hay clase de la que sacarlo.
	AnchoVencePlan float32 = 118.38
)

// AltoNota es el `rows={2}` de la nota: dos cajas de `text-sm`, `py-1.5` y el borde.
var AltoNota = 2*tema.TextoSM.Linea + 2*NotaPY + 2*tema.Borde

// tituloSeccion es el `<h3 class="text-xs font-semibold tracking-wide uppercase
// text-text-subtle">` de cada bloque.
func tituloSeccion(texto string, v tema.Variante) *ui.Versal {
	return ui.NuevoVersalCon(texto, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v), TrackingWide)
}

// pila apila bloques con su margen de arriba, a todo el ancho.
type pila struct{ bloques []bloqueFicha }

func (p *pila) add(o fyne.CanvasObject, alto, arriba float32) {
	p.bloques = append(p.bloques, bloqueFicha{obj: o, alto: alto, arriba: arriba})
}

func (p *pila) alto() float32 {
	var a float32
	for _, b := range p.bloques {
		a += b.arriba + b.alto
	}
	return a
}

func (p *pila) objeto() (fyne.CanvasObject, float32) {
	objs := make([]fyne.CanvasObject, len(p.bloques))
	for i, b := range p.bloques {
		objs[i] = b.obj
	}
	return container.New(p, objs...), p.alto()
}

func (p *pila) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, p.alto()) }

func (p *pila) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	var y float32
	for _, b := range p.bloques {
		y += b.arriba
		b.obj.Move(fyne.NewPos(0, y))
		b.obj.Resize(fyne.NewSize(t.Width, b.alto))
		y += b.alto
	}
}

// fila pone objetos en fila con sus medidas, centrados en vertical en `alto`; los de
// `der` pegados a la derecha (`justify-between`).
type fila struct {
	izq, der []pieza
	hueco    float32
	alto     float32
}

func (f *fila) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, f.alto) }

func (f *fila) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for _, p := range f.izq {
		colocarPieza(p, x, (t.Height-p.alto)/2)
		x += p.ancho + f.hueco
	}
	x = t.Width
	for i := len(f.der) - 1; i >= 0; i-- {
		p := f.der[i]
		x -= p.ancho
		colocarPieza(p, x, (t.Height-p.alto)/2)
		x -= f.hueco
	}
}

func colocarPieza(p pieza, x, y float32) {
	if t, ok := p.obj.(*canvas.Text); ok && p.texto {
		ui.ColocarTexto(t, escalonDe(t), x, y)
		return
	}
	p.obj.Resize(fyne.NewSize(p.ancho, p.alto))
	p.obj.Move(fyne.NewPos(x, y))
}

func objetosDe(ps ...[]pieza) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, grupo := range ps {
		for _, p := range grupo {
			out = append(out, p.obj)
		}
	}
	return out
}

func textoPieza(s string, e tema.Escalon, p ui.Peso, token string, v tema.Variante) pieza {
	return pieza{ui.Texto(s, e, p, tema.Color(token, v)), ui.Medir(s, e, p).Width, e.Linea, true}
}

// botonXS es un botón `xs` con su icono de 12 y el `mr-1` que le pone la web.
func botonXS(texto string, variante ui.Variante, ic iconos.Icono, al func()) *ui.Boton {
	b := ui.NuevoBoton(texto, al).Con(variante, ui.TallaXS)
	if ic != iconos.Ninguno {
		b.ConIcono(ic, 12)
		if texto != "" {
			b.MargenIcono = 1 * 0.25 * tema.Rem
		}
	}
	return b
}

// --- Los asientos -------------------------------------------------------------

func (f *Ficha) seccionAsientos(v tema.Variante) (fyne.CanvasObject, float32) {
	p := &pila{}
	t := tituloSeccion(f.t("nav.ledger"), v)
	p.add(t.Objeto(), tema.TextoXS.Linea, 0)
	for i, a := range f.Asientos {
		arriba := TituloSeccionMB
		if i > 0 {
			arriba = SeccionGap
		}
		o, alto := AsientoCompacto(v, a, f.loc, f.tr, f.anchoCampos())
		p.add(o, alto, arriba)
		if a.Estado == "asentado" {
			asiento := a
			b := ui.NuevoBoton(f.t("ledger.void"), func() {
				if f.AlAnular != nil {
					f.AlAnular(asiento)
				}
			}).Con(ui.Fantasma, ui.TallaXS)
			b.Inerte = f.Guardando
			fl := &fila{der: []pieza{piezaDe(b, b.MinSize())}, alto: b.MinSize().Height}
			p.add(container.New(fl, b), fl.alto, AnularMT)
		}
	}
	return p.objeto()
}

// importeAsiento es un importe del libro, o una raya si es cero: una columna de ceros
// no informa.
func importeAsiento(c int64, l i18n.Locale) string {
	if c == 0 {
		return SinValor
	}
	return i18n.FormatCurrency(l, c)
}

// AsientoCompacto es UN asiento con sus apuntes, como en el libro diario y en la ficha
// del documento que lo generó: referencia, fecha, concepto y estado, y la tabla de
// cuenta, concepto, debe y haber.
func AsientoCompacto(v tema.Variante, a api.ERPAsiento, l i18n.Locale, tr *i18n.Translator, ancho float32) (fyne.CanvasObject, float32) {
	util := ancho - 2*tema.Borde - 2*AsientoPX
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), radioTarjeta())
	objs := caja.Objetos()
	// La cabecera: `flex flex-wrap items-baseline gap-2`, y el concepto con `mr-auto`.
	fecha := a.Fecha
	if t, ok := fechaISO(a.Fecha); ok {
		fecha = i18n.FormatDate(l, t)
	}
	ref := textoCelda(a.Ref, celdaERP{mono: true, escalon: tema.TextoXS, tinta: "text"}, v)
	cab := make([]pieza, 0, 4)
	cab = append(cab,
		pieza{ref, medirTexto(a.Ref, tema.TextoXS, true, ui.Normal), tema.TextoSM.Linea, false},
		textoPieza(fecha, tema.TextoSM, ui.Normal, "text-muted", v),
		textoPieza(a.Concepto, tema.TextoSM, ui.Medio, "text", v),
	)
	tono := ui.InsigniaExito
	if a.Estado == "anulado" {
		tono = ui.InsigniaPeligro
	}
	cab = append(cab, piezaDe(ui.NuevaInsignia(a.Estado, tono), ui.MedidaInsignia(a.Estado)))
	anchos, altos := medidasDe(cab)
	altos[0] = tema.TextoSM.Linea // la mono va en la línea de `text-sm` de sus vecinas
	pos, tam := Envolver(anchos, altos, HuecoCabecera, HuecoCabecera, util)
	// El concepto lleva `mr-auto`: si el estado cabe en su misma línea, va pegado a la
	// derecha (el libro, a 854); si salta, empieza la línea siguiente (la ficha, a 349).
	if n := len(pos) - 1; tam.Height <= max(altos[0], altos[n])+0.01 {
		pos[n].X = util - anchos[n]
	}
	for _, p := range cab {
		objs = append(objs, p.obj)
	}
	// La tabla: `w-full text-sm` con el reparto automático del navegador.
	cols := []string{tr.T("ledger.account", nil), tr.T("ledger.concept", nil), tr.T("ledger.debit", nil), tr.T("ledger.credit", nil)}
	celdas := make([][]celdaERP, 0, len(a.Apuntes))
	cabCel := make([]celdaERP, 4)
	for i, c := range cols {
		cabCel[i] = celdaERP{texto: c, escalon: tema.TextoSM, tinta: "text-subtle"}
	}
	for _, ap := range a.Apuntes {
		celdas = append(celdas, []celdaERP{
			{texto: ap.Cuenta, mono: true, escalon: tema.TextoXS, tinta: "text"},
			{texto: ap.Concepto, escalon: tema.TextoSM, tinta: "text-muted"},
			{texto: importeAsiento(ap.Debe, l), escalon: tema.TextoSM, tinta: "text"},
			{texto: importeAsiento(ap.Haber, l), escalon: tema.TextoSM, tinta: "text"},
		})
	}
	maxs, mins := make([]float32, 4), make([]float32, 4)
	for i, c := range cabCel {
		maxs[i] = medirTexto(c.texto, c.escalon, false, ui.Medio)
		mins[i] = maxs[i]
	}
	for _, fila := range celdas {
		for i, c := range fila {
			w := medirTexto(c.texto, c.escalon, c.mono, ui.Normal)
			maxs[i] = max(maxs[i], w)
			if i != 1 { // el concepto se trunca: su mínimo es cero
				mins[i] = max(mins[i], w)
			}
		}
	}
	mins[1] = 0
	anchosCol := RepartoERP(util, maxs, mins, make([]float32, 4))
	tabla := &tablaAsiento{anchos: anchosCol}
	altoFila := tema.TextoSM.Linea + 2*CeldaAsientoPY
	for i, c := range cabCel {
		t := ui.Texto(c.texto, tema.TextoSM, ui.Medio, tema.Color(c.tinta, v))
		tabla.textos = append(tabla.textos, celdaTexto{
			t: t, fila: 0, col: i, derecha: i >= 2,
			ancho: medirTexto(c.texto, tema.TextoSM, false, ui.Medio), e: tema.TextoSM,
		})
		objs = append(objs, t)
	}
	for f, fila := range celdas {
		d := canvas.NewRectangle(tema.Color("border", v))
		tabla.filetes = append(tabla.filetes, d)
		objs = append(objs, d)
		for i, c := range fila {
			s := c.texto
			ancho := medirTexto(s, c.escalon, c.mono, ui.Normal)
			if i == 1 && ancho > anchosCol[1] {
				t := ui.Texto(s, c.escalon, ui.Normal, tema.Color(c.tinta, v))
				ancho = ui.Truncar(t, s, c.escalon, ui.Normal, anchosCol[1])
				tabla.textos = append(tabla.textos, celdaTexto{t: t, fila: f + 1, col: i, ancho: ancho, e: c.escalon})
				objs = append(objs, t)
				continue
			}
			t := textoCelda(s, c, v)
			tabla.textos = append(tabla.textos, celdaTexto{t: t, fila: f + 1, col: i, derecha: i >= 2, ancho: ancho, e: c.escalon})
			objs = append(objs, t)
		}
	}
	tabla.altoFila = altoFila
	altoTabla := altoFila + float32(len(celdas))*(altoFila+tema.Borde)
	l2 := &asientoLay{caja: caja, cab: cab, pos: pos, altoCab: tam.Height, tabla: tabla, altoTabla: altoTabla}
	l2.contenedor = container.New(tabla, tabla.objetos()...)
	objs2 := append(caja.Objetos(), objetosDe(cab)...)
	objs2 = append(objs2, l2.contenedor)
	alto := 2*tema.Borde + 2*AsientoPY + tam.Height + TituloSeccionMB + altoTabla
	return container.New(l2, objs2...), alto
}

type celdaTexto struct {
	t       *canvas.Text
	fila    int
	col     int
	derecha bool
	ancho   float32
	e       tema.Escalon
}

// tablaAsiento coloca la tabla de apuntes: cabecera y filas con su filete arriba.
type tablaAsiento struct {
	anchos   []float32
	textos   []celdaTexto
	filetes  []*canvas.Rectangle
	altoFila float32
}

func (t *tablaAsiento) objetos() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(t.filetes)+len(t.textos))
	for _, d := range t.filetes {
		out = append(out, d)
	}
	for _, c := range t.textos {
		out = append(out, c.t)
	}
	return out
}

func (t *tablaAsiento) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (t *tablaAsiento) Layout(_ []fyne.CanvasObject, tam fyne.Size) {
	yDe := func(f int) float32 {
		if f == 0 {
			return 0
		}
		return t.altoFila + float32(f-1)*(t.altoFila+tema.Borde) + tema.Borde
	}
	for i, d := range t.filetes {
		d.Move(fyne.NewPos(0, yDe(i+1)-tema.Borde))
		d.Resize(fyne.NewSize(tam.Width, tema.Borde))
	}
	for _, c := range t.textos {
		x := float32(0)
		for i := range c.col {
			x += t.anchos[i]
		}
		if c.derecha {
			x += t.anchos[c.col] - c.ancho
		}
		ui.ColocarTexto(c.t, c.e, x, yDe(c.fila)+CeldaAsientoPY+(tema.TextoSM.Linea-c.e.Linea)/2)
	}
}

type asientoLay struct {
	caja       *ui.Caja
	cab        []pieza
	pos        []fyne.Position
	altoCab    float32
	tabla      *tablaAsiento
	altoTabla  float32
	contenedor fyne.CanvasObject
}

func (a *asientoLay) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (a *asientoLay) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	a.caja.Redimensionar(t)
	x0, y0 := tema.Borde+AsientoPX, tema.Borde+AsientoPY
	for i, p := range a.cab {
		if t, texto := p.obj.(*canvas.Text); texto {
			// `items-baseline`: los textos van a la línea de `text-sm` de la fila.
			e := escalonDe(t)
			ui.ColocarTexto(t, e, x0+a.pos[i].X, y0+a.pos[i].Y+(tema.TextoSM.Linea-e.Linea)/2)
			continue
		}
		p.obj.Resize(fyne.NewSize(p.ancho, p.alto))
		p.obj.Move(fyne.NewPos(x0+a.pos[i].X, y0+a.pos[i].Y))
	}
	a.contenedor.Move(fyne.NewPos(x0, y0+a.altoCab+TituloSeccionMB))
	a.contenedor.Resize(fyne.NewSize(t.Width-2*x0, a.altoTabla))
}

// --- Tareas, documentos, requisitos, VERI*FACTU e historial ---------------------

// seguimiento es el bloque de debajo de los campos: `mt-6 border-t pt-4 space-y-5`.
func (f *Ficha) seguimiento(v tema.Variante) (fyne.CanvasObject, float32) {
	p := &pila{}
	filete := canvas.NewRectangle(tema.Color("border", v))
	p.add(filete, tema.Borde, 0)
	secciones := []func(tema.Variante) (fyne.CanvasObject, float32){f.tareas, f.documentos}
	if f.Modelo.Contable && !f.Modelo.Cobro && f.Requisitos != nil && len(f.Requisitos.Completa) > 0 {
		secciones = append(secciones, f.requisitos)
	}
	if f.Modelo.Clave == api.ERPFacturaVenta && f.Facturae && f.Verifactu != nil && len(f.Verifactu.Registros) > 0 {
		secciones = append(secciones, f.verifactu)
	}
	secciones = append(secciones, f.historial)
	for i, s := range secciones {
		o, alto := s(v)
		arriba := SeguimientoGap
		if i == 0 {
			arriba = SeguimientoPT
		}
		p.add(o, alto, arriba)
	}
	return p.objeto()
}

func (f *Ficha) escribe() bool { return !f.SoloLectura && !f.EnPapelera }

// cabeceraSeccion es la fila de un bloque: su título y, a la derecha, su botón.
func cabeceraSeccion0(v tema.Variante, titulo string, boton *ui.Boton) (fyne.CanvasObject, float32) {
	t := tituloSeccion(titulo, v)
	izq := []pieza{{t.Objeto(), t.Ancho(), tema.TextoXS.Linea, false}}
	alto := tema.TextoXS.Linea
	var der []pieza
	if boton != nil {
		der = append(der, piezaDe(boton, boton.MinSize()))
		alto = max(alto, boton.MinSize().Height)
	}
	fl := &fila{izq: izq, der: der, hueco: HuecoCabecera, alto: alto}
	return container.New(fl, objetosDe(izq, der)...), alto
}

// rueda14 es el `<Spinner size={14} />` de lo que todavía no ha llegado.
func rueda14(v tema.Variante) (fyne.CanvasObject, float32) {
	r := ui.NuevaRueda(14, "text-muted", v)
	fl := &fila{izq: []pieza{piezaDe(r, fyne.NewSize(14, 14))}, alto: 14}
	return container.New(fl, r), 14
}

func textoXS(s, token string, v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := ui.NuevoParrafo(s, tema.TextoXS, ui.Normal, token, v)
	return p, ui.AltoParrafo(p, ancho)
}

// tonoVence es el de la fecha de una tarea: vencida, hoy o más adelante.
func tonoVence(vence, hoy string) ui.Tono {
	switch {
	case vence < hoy:
		return ui.InsigniaPeligro
	case vence == hoy:
		return ui.InsigniaAviso
	}
	return ui.InsigniaNeutra
}

// HoyISO es la fecha de hoy en AAAA-MM-DD en la hora de quien mira (`hoyISO`).
func HoyISO(ahora time.Time) string { return ahora.Local().Format(time.DateOnly) }

// diaMes es la fecha de una tarea: «20 sept».
func diaMes(l i18n.Locale, iso string) string {
	if t, ok := fechaISO(iso); ok {
		return i18n.FormatDayMonth(l, t)
	}
	return iso
}

// RotuloTipo es el nombre de un tipo de tarea (`rotuloTipo`).
func RotuloTipo(tipo string, tr *i18n.Translator) string {
	claves := map[string]string{
		"subir_documento": "tasks.kind.upload", "pedir_documento": "tasks.kind.request",
		"llamada": "tasks.kind.call", "correo": "tasks.kind.email",
		"reunion": "tasks.kind.meeting", "tarea": "tasks.kind.todo",
	}
	if c, hay := claves[tipo]; hay {
		return tr.T(c, nil)
	}
	return tipo
}

func (f *Ficha) tareas(v tema.Variante) (fyne.CanvasObject, float32) {
	p := &pila{}
	var plan *ui.Boton
	if f.escribe() {
		plan = botonXS(f.t("tasks.plan"), ui.Fantasma, iconos.Mas, f.AlPlanear)
		if f.Planificando {
			plan.Variante = ui.Fantasma // el `hover` se queda pintado mientras está abierto
		}
	}
	o, alto := cabeceraSeccion0(v, f.t("tasks.title"), plan)
	p.add(o, alto, 0)
	if f.Planificando && f.escribe() {
		if f.plan == nil {
			f.plan = nuevoPlan(f.tr, f.Equipo)
		}
		o, alto := f.plan.objeto(v, f.anchoCampos(), func() {
			if f.AlPlanificar != nil {
				f.AlPlanificar(f.plan.tipo(), strings.TrimSpace(f.plan.resumen.Texto()), f.plan.vence(), f.plan.responsable())
			}
		})
		p.add(o, alto, SeccionGap)
	}
	switch {
	case f.Tareas == nil:
		o, alto := rueda14(v)
		p.add(o, alto, SeccionGap)
	case len(f.Tareas) == 0:
		o, alto := textoXS(f.t("tasks.none"), "text-subtle", v, f.anchoCampos())
		p.add(o, alto, SeccionGap)
	default:
		hoy := HoyISO(time.Now())
		for i, a := range f.Tareas {
			o, alto := f.filaTarea(v, a, hoy)
			arriba := TareasGap
			if i == 0 {
				arriba = SeccionGap
			}
			p.add(o, alto, arriba)
		}
	}
	return p.objeto()
}

// filaTarea es una tarea del registro: resumen y tipo · responsable a la izquierda;
// la fecha y «Hecha» a la derecha.
func (f *Ficha) filaTarea(v tema.Variante, a api.ERPActividad, hoy string) (fyne.CanvasObject, float32) {
	fecha := diaMes(f.loc, a.Vence)
	var der []pieza
	der = append(der, piezaDe(ui.NuevaInsignia(fecha, tonoVence(a.Vence, hoy)), ui.MedidaInsignia(fecha)))
	if f.escribe() {
		act := a
		h := botonXS(f.t("tasks.done"), ui.Secundario, iconos.Tic, func() {
			if f.AlHecha != nil {
				f.AlHecha(act)
			}
		})
		der = append(der, piezaDe(h, h.MinSize()))
	}
	var anchoDer float32
	for i, p := range der {
		anchoDer += p.ancho
		if i > 0 {
			anchoDer += HuecoCabecera
		}
	}
	cabe := max(f.anchoCampos()-anchoDer-HuecoCabecera, 0)
	resumen := ui.Texto(a.Resumen, tema.TextoSM, ui.Normal, tema.Color("text", v))
	ui.Truncar(resumen, a.Resumen, tema.TextoSM, ui.Normal, cabe)
	sub := RotuloTipo(a.Tipo, f.tr) + " · " + a.ResponsableNombre
	detalle := ui.Texto(sub, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	alto := 2 * tema.TextoSM.Linea
	l := &filaDosLineas{a: resumen, b: detalle, der: der, alto: alto}
	objs := append([]fyne.CanvasObject{resumen, detalle}, objetosDe(der)...)
	return container.New(l, objs...), alto
}

// filaDosLineas: dos renglones de `text-sm` a la izquierda (el segundo con letra `xs`
// dentro) y un grupo centrado a la derecha.
type filaDosLineas struct {
	a, b *canvas.Text
	der  []pieza
	alto float32
}

func (f *filaDosLineas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, f.alto) }

func (f *filaDosLineas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(f.a, tema.TextoSM, 0, 0)
	ui.ColocarTexto(f.b, tema.TextoXS, 0, tema.TextoSM.Linea+(tema.TextoSM.Linea-tema.TextoXS.Linea)/2)
	x := t.Width
	for i := len(f.der) - 1; i >= 0; i-- {
		p := f.der[i]
		x -= p.ancho
		colocarPieza(p, x, (t.Height-p.alto)/2)
		x -= HuecoCabecera
	}
}

func (f *Ficha) documentos(v tema.Variante) (fyne.CanvasObject, float32) {
	d := &Documentos{
		tr: f.tr, loc: f.loc, Titulo: f.t("docs.title"), Adjuntos: f.Adjuntos, Escribe: f.escribe(),
		Subiendo: f.SubiendoDoc, Ancho: f.anchoCampos(), AlSubir: f.AlSubir,
		AlAbrir: f.AlAbrirAdjunto, AlBorrar: f.AlBorrarAdjunto,
	}
	return d.Objeto(v)
}

// Documentos es `DocumentosRegistro` de `Seguimiento.tsx`: el título en versalitas con
// «Subir» a la derecha, lo que tiene que traer el documento si se sabe, y la lista. Lo
// usan la ficha de un registro y cada presentación de Obligaciones.
type Documentos struct {
	tr  *i18n.Translator
	loc i18n.Locale
	// Titulo es el rótulo del bloque: «Documentos» o el nombre del documento pedido.
	Titulo string
	// Requisitos es lo que tiene que traer para valer; vacío, no se pinta.
	Requisitos []string
	// Adjuntos son los subidos; nil, cargando.
	Adjuntos []api.ERPAdjunto
	// Escribe pinta «Subir» y las papeleras.
	Escribe  bool
	Subiendo bool
	Ancho    float32

	AlSubir  func()
	AlAbrir  func(api.ERPAdjunto)
	AlBorrar func(api.ERPAdjunto)
}

// Objeto arma el bloque (`space-y-2`) y dice cuánto mide.
func (d *Documentos) Objeto(v tema.Variante) (fyne.CanvasObject, float32) {
	p := &pila{}
	var subir *ui.Boton
	if d.Escribe {
		subir = botonXS(d.tr.T("docs.upload", nil), ui.Secundario, iconos.Subir, d.AlSubir)
		subir.Cargando = d.Subiendo
		// ponytail: sin selector de ficheros del sistema (el móvil, un Linux sin zenity
		// ni kdialog) «Subir» está y no abre nada.
		subir.Inerte = d.AlSubir == nil
	}
	o, alto := cabeceraSeccion0(v, d.Titulo, subir)
	p.add(o, alto, 0)
	if len(d.Requisitos) > 0 {
		o, alto := listaMarcada(d.Requisitos, false, tema.TextoXS, v, d.Ancho)
		p.add(o, alto, SeccionGap)
	}
	switch {
	case d.Adjuntos == nil:
		o, alto := rueda14(v)
		p.add(o, alto, SeccionGap)
	case len(d.Adjuntos) == 0:
		o, alto := textoXS(d.tr.T("docs.empty", nil), "text-subtle", v, d.Ancho)
		p.add(o, alto, SeccionGap)
	default:
		o, alto := d.lista(v)
		p.add(o, alto, SeccionGap)
	}
	return p.objeto()
}

// lista es `ul.rounded-md.border.divide-y` con una fila por documento: el nombre
// subrayado que lo abre, su tamaño y fecha, y la papelera.
func (d *Documentos) lista(v tema.Variante) (fyne.CanvasObject, float32) {
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	col := &columnaFilas{}
	var hijos []fyne.CanvasObject
	altoFila := ui.LadoIconoSM + 2*DocPY
	for i, a := range d.Adjuntos {
		adj := a
		info := i18n.FormatBytes(d.loc, float64(a.Tamano)) + " · " + i18n.FormatDayMonth(d.loc, a.CreadoEn.Local())
		der := []pieza{textoPieza(info, tema.TextoXS, ui.Normal, "text-subtle", v)}
		if d.Escribe {
			b := ui.NuevoBoton("", func() {
				if d.AlBorrar != nil {
					d.AlBorrar(adj)
				}
			}).Con(ui.Fantasma, ui.TallaXS)
			b.ConIcono(iconos.Papelera, 12)
			b.Rotulo = d.tr.T("action.delete", nil) + " " + a.Nombre
			der = append(der, piezaDe(b, b.MinSize()))
		}
		var anchoDer float32
		for j, p := range der {
			anchoDer += p.ancho
			if j > 0 {
				anchoDer += HuecoCabecera
			}
		}
		cabe := max(d.Ancho-2*tema.Borde-2*DocPX-anchoDer-HuecoCabecera, 0)
		enlace := ui.NuevoEnlace(a.Nombre, tema.TextoSM, ui.Normal, "text", func() {
			if d.AlAbrir != nil {
				d.AlAbrir(adj)
			}
		})
		ancho := min(ui.Medir(a.Nombre, tema.TextoSM, ui.Normal).Width, cabe)
		fl := &fila{
			izq: []pieza{{enlace, ancho, tema.TextoSM.Linea, false}}, der: der, hueco: HuecoCabecera,
			alto: altoFila,
		}
		interior := container.New(fl, objetosDe(fl.izq, fl.der)...)
		c := container.New(&conMargen{x: DocPX}, interior)
		col.filas = append(col.filas, bloque{c, altoFila, 0})
		hijos = append(hijos, c)
		if i > 0 {
			r := canvas.NewRectangle(tema.Color("border", v))
			col.filetes = append(col.filetes, r)
			hijos = append(hijos, r)
		}
	}
	lista := container.New(col, hijos...)
	alto := col.alto() + 2*tema.Borde
	l := &enCaja{caja: caja, hijo: lista}
	return container.New(l, append(caja.Objetos(), lista)...), alto
}

// conMargen coloca su hijo con un margen horizontal a cada lado.
type conMargen struct{ x float32 }

func (c *conMargen) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (c *conMargen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(c.x, 0))
		o.Resize(fyne.NewSize(max(t.Width-2*c.x, 0), t.Height))
	}
}

// enCaja pone el hijo dentro del borde de su caja.
type enCaja struct {
	caja *ui.Caja
	hijo fyne.CanvasObject
}

func (e *enCaja) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (e *enCaja) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	e.caja.Redimensionar(t)
	e.hijo.Move(fyne.NewPos(tema.Borde, tema.Borde))
	e.hijo.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), max(t.Height-2*tema.Borde, 0)))
}

// requisitos es `RequisitosDeFactura` sobre el panel de la ficha (`surface`): plegado,
// y se abre y se cierra al pulsar su rótulo, como el `<details>` de la web.
func (f *Ficha) requisitos(v tema.Variante) (fyne.CanvasObject, float32) {
	return RequisitosDeFactura(f.tr, f.Requisitos, f.VerRequisitos, f.AlRequisitos, tema.Color("surface", v), v, f.anchoCampos())
}

// estadosVF son el rótulo y el tono de cada estado de un registro de VERI*FACTU.
var estadosVF = map[string]struct {
	clave string
	tono  ui.Tono
}{
	api.ERPVFPendiente:  {"vf.state.pending", ui.InsigniaInfo},
	api.ERPVFCorrecto:   {"vf.state.accepted", ui.InsigniaExito},
	api.ERPVFConErrores: {"vf.state.acceptedWithErrors", ui.InsigniaAviso},
	api.ERPVFIncorrecto: {"vf.state.rejected", ui.InsigniaPeligro},
}

// verifactu es lo que contestó la Agencia a la factura y, si se puede corregir,
// «Volver a enviar».
func (f *Ficha) verifactu(v tema.Variante) (fyne.CanvasObject, float32) {
	p := &pila{}
	t := tituloSeccion("VERI*FACTU", v)
	p.add(t.Objeto(), tema.TextoXS.Linea, 0)
	ultimo, _ := f.Verifactu.Ultimo()
	e := estadosVF[ultimo.Estado]
	rot := f.t(e.clave)
	if ultimo.Tipo == "anulacion" {
		rot = f.t("vf.cancellation") + " · " + rot
	}
	ps := []pieza{piezaDe(ui.NuevaInsignia(rot, e.tono), ui.MedidaInsignia(rot))}
	if ultimo.CSV != "" {
		s := "CSV " + ultimo.CSV
		ps = append(ps, pieza{
			textoCelda(s, celdaERP{mono: true, escalon: tema.TextoXS, tinta: "text-subtle"}, v),
			medirTexto(s, tema.TextoXS, true, ui.Normal), tema.TextoXS.Linea, true,
		})
	}
	if f.Verifactu.QR != "" {
		qr := f.Verifactu.QR
		en := ui.NuevoEnlace(f.t("vf.check"), tema.TextoXS, ui.Normal, "text-muted", func() { abrirURL(qr) })
		ps = append(ps, piezaDe(en, fyne.NewSize(ui.Medir(f.t("vf.check"), tema.TextoXS, ui.Normal).Width, tema.TextoXS.Linea)))
	}
	anchos, altos := medidasDe(ps)
	pos, tam := Envolver(anchos, altos, HuecoCabecera, HuecoCabecera, f.anchoCampos())
	p.add(container.New(&piezasEn{ps: ps, pos: pos, alto: tam.Height}, objetosDe(ps)...), tam.Height, SeccionGap)
	if ultimo.Descripcion != "" {
		s := ultimo.Descripcion
		if ultimo.Codigo != nil {
			s = fmt.Sprintf("%d · %s", *ultimo.Codigo, s)
		}
		o, alto := textoXS(s, "text-muted", v, f.anchoCampos())
		p.add(o, alto, SeccionGap)
	}
	if !f.SoloLectura && !f.EnPapelera && ultimo.Corregible() {
		b := ui.NuevoBoton(f.t("vf.resend"), f.AlReenviarVF).Con(ui.Secundario, ui.TallaXS)
		b.Cargando = f.Reenviando
		cabe := f.anchoCampos() - b.MinSize().Width - HuecoCabecera
		pista := ui.NuevoParrafo(f.t("vf.fixHint"), tema.TextoXS, ui.Normal, "text-subtle", v)
		altoP := ui.AltoParrafo(pista, cabe)
		alto := max(altoP, b.MinSize().Height)
		fl := &fila{izq: []pieza{{pista, cabe, altoP, false}}, der: []pieza{piezaDe(b, b.MinSize())}, hueco: HuecoCabecera, alto: alto}
		p.add(container.New(fl, pista, b), alto, SeccionGap)
	}
	return p.objeto()
}

// piezasEn coloca piezas en las posiciones ya calculadas.
type piezasEn struct {
	ps   []pieza
	pos  []fyne.Position
	alto float32
}

func (p *piezasEn) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, p.alto) }
func (p *piezasEn) Layout(_ []fyne.CanvasObject, _ fyne.Size) {
	colocarPiezas(p.ps, p.pos, 0, 0)
}

// sucesos son los rótulos de lo que no es ni nota ni cambio.
var sucesos = map[string]string{
	"creado": "history.created", "borrado": "history.deleted", "restaurado": "history.restored",
}

func (f *Ficha) historial(v tema.Variante) (fyne.CanvasObject, float32) {
	p := &pila{}
	t := tituloSeccion(f.t("history.title"), v)
	p.add(t.Objeto(), tema.TextoXS.Linea, 0)
	if f.escribe() {
		b := ui.NuevoBoton(f.t("history.addNote"), func() {
			if texto := strings.TrimSpace(f.nota.Texto()); texto != "" && f.AlAnotar != nil {
				f.AlAnotar(texto)
			}
		}).Con(ui.Secundario, ui.TallaSM)
		b.Cargando = f.Anotando
		b.Inerte = strings.TrimSpace(f.nota.Texto()) == ""
		cabe := f.anchoCampos() - b.MinSize().Width - HuecoCabecera
		f.nota.Ancho = cabe
		fl := &fila{izq: []pieza{{f.nota, cabe, AltoNota, false}}, alto: AltoNota}
		fl.der = []pieza{piezaDe(b, b.MinSize())}
		o := container.New(&abajo{fl}, f.nota, b)
		p.add(o, AltoNota, SeccionGap)
		f.nota.AlCambiar(func(s string) {
			vacio := strings.TrimSpace(s) == ""
			if b.Inerte != vacio {
				b.Inerte = vacio
				b.Refresh()
			}
		})
	}
	switch {
	case f.Historial == nil:
		o, alto := rueda14(v)
		p.add(o, alto, SeccionGap)
	case len(f.Historial) == 0:
		o, alto := textoXS(f.t("history.empty"), "text-subtle", v, f.anchoCampos())
		p.add(o, alto, SeccionGap)
	default:
		for _, e := range f.Historial {
			o, alto := f.entradaHistorial(v, e)
			p.add(o, alto, SeccionGap)
		}
	}
	return p.objeto()
}

// abajo es la fila con `items-end`: todo apoyado en la base.
type abajo struct{ f *fila }

func (a *abajo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, a.f.alto) }
func (a *abajo) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	x := float32(0)
	for _, p := range a.f.izq {
		colocarPieza(p, x, t.Height-p.alto)
		x += p.ancho + a.f.hueco
	}
	x = t.Width
	for i := len(a.f.der) - 1; i >= 0; i-- {
		p := a.f.der[i]
		x -= p.ancho
		colocarPieza(p, x, t.Height-p.alto)
	}
}

// entradaHistorial es una línea del historial: quién y cuándo, y debajo la nota, los
// cambios campo a campo o lo que pasó.
func (f *Ficha) entradaHistorial(v tema.Variante, e api.ERPEntradaHistorial) (fyne.CanvasObject, float32) {
	quien := e.Nombre
	switch {
	case quien != "":
	case e.Sub != "":
		quien = f.t("history.someone")
	default:
		quien = "Sytue"
	}
	cuando := i18n.FormatDateTime(f.loc, e.CreadoEn.Local())
	ancho := f.anchoCampos()
	p := &pila{}
	cab := []pieza{
		textoPieza(quien, tema.TextoXS, ui.Medio, "text", v),
		textoPieza(" · "+cuando, tema.TextoXS, ui.Normal, "text-subtle", v),
	}
	anchos, altos := medidasDe(cab)
	pos := []fyne.Position{{}, {X: anchos[0]}}
	p.add(container.New(&piezasEn{ps: cab, pos: pos, alto: altos[0]}, objetosDe(cab)...), tema.TextoXS.Linea, 0)
	switch e.Tipo {
	case "nota":
		par := ui.NuevoParrafo(e.Texto, tema.TextoSM, ui.Normal, "text", v)
		p.add(par, ui.AltoParrafo(par, ancho), 0)
	case "cambio":
		for clave, c := range e.Cambios {
			s := f.rotuloHistorial(clave) + ": " + f.valorHistorial(clave, c.Antes) + " → " + f.valorHistorial(clave, c.Despues)
			o, alto := textoXS(s, "text-muted", v, ancho)
			p.add(o, alto, 0)
		}
	default:
		o, alto := textoXS(f.t(sucesos[e.Tipo]), "text-muted", v, ancho)
		p.add(o, alto, 0)
	}
	return p.objeto()
}

func (f *Ficha) rotuloHistorial(clave string) string {
	if clave == "estado" {
		return f.t("history.status")
	}
	for _, c := range f.Modelo.Campos {
		if c.Clave == clave {
			return c.Etiqueta
		}
	}
	return clave
}

func (f *Ficha) valorHistorial(clave string, v any) string {
	if clave == "estado" {
		if s, ok := v.(string); ok {
			return RotuloEstado(f.Modelo, s)
		}
		return SinValor
	}
	for _, c := range f.Modelo.Campos {
		if c.Clave == clave {
			return Celda(c, v, f.loc, f.Rotulos)
		}
	}
	if v == nil {
		return SinValor
	}
	return texto(v)
}

// --- La trazabilidad ------------------------------------------------------------

func (f *Ficha) trazabilidad(v tema.Variante) (fyne.CanvasObject, float32) {
	r := f.Registro
	p := &pila{}
	p.add(canvas.NewRectangle(tema.Color("border", v)), tema.Borde, 0)
	t := ui.NuevoVersalCon(f.t("form.auditTitle"), tema.TextoXS, ui.Medio, tema.Color("text-muted", v), TrackingEtapa)
	copiar := ui.NuevoEnlace(f.t("form.copyId"), tema.TextoXS, ui.Normal, "primary", f.AlCopiarID)
	fl := &fila{
		izq:  []pieza{{t.Objeto(), t.Ancho(), tema.TextoXS.Linea, false}},
		der:  []pieza{piezaDe(copiar, fyne.NewSize(ui.Medir(f.t("form.copyId"), tema.TextoXS, ui.Normal).Width, tema.TextoXS.Linea))},
		alto: tema.TextoXS.Linea,
	}
	p.add(container.New(fl, objetosDe(fl.izq, fl.der)...), tema.TextoXS.Linea, SeguimientoPT)
	celdas := make([]pieza, 0, 2)
	for _, c := range []struct {
		rotulo string
		cuando time.Time
	}{{f.t("form.createdAt"), r.CreadoEn}, {f.t("form.editedAt"), r.EditadoEn}} {
		rot := ui.Texto(c.rotulo+":", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		s := i18n.FormatDateTime(f.loc, c.cuando.Local())
		val := textoCelda(s, celdaERP{mono: true, escalon: tema.TextoXS, tinta: "text"}, v)
		o := container.New(&dosLineas{a: rot, b: val}, rot, val)
		celdas = append(celdas, pieza{o, 0, tema.TextoXS.Linea + AltoValorTraza, false})
	}
	alto := tema.TextoXS.Linea + AltoValorTraza
	p.add(container.New(&rejilla2{ps: celdas, hueco: HuecoCabeceraFila}, objetosDe(celdas)...), alto, TituloSeccionMB)
	return p.objeto()
}

type dosLineas struct{ a, b *canvas.Text }

func (d *dosLineas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (d *dosLineas) Layout(_ []fyne.CanvasObject, _ fyne.Size) {
	ui.ColocarTexto(d.a, tema.TextoXS, 0, 0)
	ui.ColocarTexto(d.b, tema.TextoXS, 0, tema.TextoXS.Linea+(AltoValorTraza-tema.TextoXS.Linea)/2)
}

// rejilla2 es `grid grid-cols-2 gap-3`.
type rejilla2 struct {
	ps    []pieza
	hueco float32
}

func (r *rejilla2) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (r *rejilla2) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ancho := (t.Width - r.hueco) / 2
	for i, p := range r.ps {
		p.obj.Move(fyne.NewPos(float32(i%2)*(ancho+r.hueco), 0))
		p.obj.Resize(fyne.NewSize(ancho, p.alto))
	}
}

// --- Planificar una tarea -------------------------------------------------------

// planTarea es el editor de «Planificar tarea»: tipo, qué hay que hacer y cuándo vence.
// Sus campos viven lo que la ficha, como los demás.
type planTarea struct {
	tr       *i18n.Translator
	selector *ui.Selector
	resumen  *ui.Entrada
	fecha    *ui.Entrada
	// quien es el «Responsable»: la primera opción es «Yo» y las demás, `equipo`.
	quien  *ui.Selector
	equipo []api.ERPPersona
}

func nuevoPlan(tr *i18n.Translator, equipo []api.ERPPersona) *planTarea {
	ops := make([]string, 0, len(api.ERPTiposActividad))
	elegida := 0
	for i, t := range api.ERPTiposActividad {
		ops = append(ops, RotuloTipo(t, tr))
		if t == "tarea" {
			elegida = i
		}
	}
	p := &planTarea{tr: tr}
	p.selector = ui.NuevoSelector(ops, nil)
	// El `<select>` del tipo: la misma clase que el campo y 28,5 de alto (medido).
	p.selector.Propio = selectorPropio(CampoTarea, AltoSelectTarea)
	p.selector.Elegida = elegida
	p.resumen = ui.NuevaEntrada("").ConPropia(CampoTarea)
	p.fecha = ui.CampoFecha(ui.CampoMD).ConPropia(CampoTarea)
	p.fecha.PonerTexto(ui.EscribirFecha(time.Now()))
	if len(equipo) > 0 {
		nombres := make([]string, 1, 1+len(equipo))
		nombres[0] = tr.T("tasks.ownerMe", nil)
		for _, e := range equipo {
			if e.Yo { // «Yo (su nombre)» el primero, y no otra vez entre los demás
				nombres[0] = tr.T("tasks.ownerMeNamed", map[string]any{"name": cmp.Or(e.Nombre, e.Correo)})
				continue
			}
			nombres = append(nombres, cmp.Or(e.Nombre, e.Correo))
			p.equipo = append(p.equipo, e)
		}
		p.quien = ui.NuevoSelector(nombres, nil)
		p.quien.Propio = selectorPropio(CampoTarea, AltoSelectTarea)
	}
	return p
}

// responsable es la persona elegida; la de Sub vacío es «Yo».
func (p *planTarea) responsable() api.ERPPersona {
	if p.quien == nil || p.quien.Elegida <= 0 || p.quien.Elegida > len(p.equipo) {
		return api.ERPPersona{}
	}
	return p.equipo[p.quien.Elegida-1]
}

func (p *planTarea) tipo() string { return api.ERPTiposActividad[p.selector.Elegida] }

func (p *planTarea) vence() string {
	if t, _, ok := ui.LeerFecha(p.fecha.Texto(), time.Local); ok {
		return t.Format(time.DateOnly)
	}
	return HoyISO(time.Now())
}

// objeto es la caja del editor: `rounded-md border p-3`, y dentro, en fila con los
// bordes de abajo alineados (`sm:flex-row sm:items-end`), las tres etiquetas y Guardar.
//
// El resumen es `flex-1 min-w-0` y en el panel de 349 NO le queda sitio: en la web mide
// 0 de ancho, su rótulo se parte palabra a palabra y la caja crece a 110,75 (medido).
// Aquí se copia tal cual; es la web, no un error de esta.
func (p *planTarea) objeto(v tema.Variante, ancho float32, guardar func()) (fyne.CanvasObject, float32) {
	util := ancho - 2*tema.Borde - 2*PlanP
	anchoTipo := p.selector.MinSize().Width // la opción más ancha con la flecha: 125, medido
	g := ui.NuevoBoton(p.tr.T("action.save", nil), guardar).Con(ui.Primario, ui.TallaSM)
	g.Inerte = strings.TrimSpace(p.resumen.Texto()) == ""
	p.resumen.AlCambiar(func(s string) {
		vacio := strings.TrimSpace(s) == ""
		if g.Inerte != vacio {
			g.Inerte = vacio
			g.Refresh()
		}
	})
	anchoQuien := float32(0)
	if p.quien != nil {
		anchoQuien = p.quien.MinSize().Width + PlanGap // el `<select>` de «Responsable», tras «Vence»
	}
	anchoResumen := max(util-anchoTipo-AnchoVencePlan-anchoQuien-g.MinSize().Width-3*PlanGap, 0)
	etiqueta := func(rot string, ancho float32, ctrl fyne.CanvasObject, altoCtrl float32) (pieza, float32) {
		par := ui.NuevoParrafo(rot, tema.TextoXS, ui.Normal, "text-muted", v)
		altoP := max(ui.AltoParrafo(par, ancho), tema.TextoXS.Linea)
		alto := altoP + PlanEtiquetaGp + altoCtrl
		// El `<input>` no baja de sus 16 (relleno y borde) aunque su columna mida 0.
		anchoCtrl := max(ancho, 2*(2*0.25*tema.Rem)+2*tema.Borde)
		c := container.New(&etiquetaPlan{p: par, c: ctrl, altoP: altoP, altoC: altoCtrl, anchoC: anchoCtrl}, par, ctrl)
		return pieza{c, ancho, alto, false}, alto
	}
	tipo, a1 := etiqueta(p.tr.T("tasks.type", nil), anchoTipo, p.selector, AltoSelectTarea)
	resumen, a2 := etiqueta(p.tr.T("tasks.summary", nil), anchoResumen, p.resumen, CampoTarea.Alto())
	p.fecha.Ancho = AnchoVencePlan
	vence, a3 := etiqueta(p.tr.T("tasks.due", nil), AnchoVencePlan, p.fecha, CampoTarea.Alto())
	fila := max(a1, a2, a3, g.MinSize().Height)
	piezas := []pieza{tipo, resumen, vence}
	if p.quien != nil {
		quien, a4 := etiqueta(p.tr.T("tasks.owner", nil), anchoQuien-PlanGap, p.quien, AltoSelectTarea)
		piezas, fila = append(piezas, quien), max(fila, a4)
	}
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	l := &filaPlan{caja: caja, ps: append(piezas, piezaDe(g, g.MinSize())), alto: fila}
	objs := append(caja.Objetos(), objetosDe(l.ps)...)
	return container.New(l, objs...), fila + 2*PlanP + 2*tema.Borde
}

type etiquetaPlan struct {
	p                    *ui.Parrafo
	c                    fyne.CanvasObject
	altoP, altoC, anchoC float32
}

func (e *etiquetaPlan) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (e *etiquetaPlan) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	e.p.Move(fyne.NewPos(0, 0))
	e.p.Resize(fyne.NewSize(t.Width, e.altoP))
	e.c.Move(fyne.NewPos(0, e.altoP+PlanEtiquetaGp))
	e.c.Resize(fyne.NewSize(e.anchoC, e.altoC))
}

type filaPlan struct {
	caja *ui.Caja
	ps   []pieza
	alto float32
}

func (f *filaPlan) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (f *filaPlan) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.caja.Redimensionar(t)
	x, base := tema.Borde+PlanP, tema.Borde+PlanP+f.alto
	for _, p := range f.ps {
		p.obj.Resize(fyne.NewSize(p.ancho, p.alto))
		p.obj.Move(fyne.NewPos(x, base-p.alto))
		x += p.ancho + PlanGap
	}
}
