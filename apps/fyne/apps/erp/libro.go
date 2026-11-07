// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El libro diario y el sumas y saldos (`Libro.tsx`)
//
// No salen del catálogo: un asiento no es un registro con campos declarados. Aquí solo
// se LEEN; corregir contabilidad se hace con otro asiento, desde la ficha.
//
// Medido en el arnés (1280×800, 2026-09-23):
//
//	libro    `max-w-5xl px-6 py-6`, título `text-2xl` (31,5) con `mb-5`, asientos de 141,5
//	         con `space-y-3`; la insignia de estado a la derecha (`mr-auto` del concepto)
//	balance  `max-w-4xl` (742 de contenido), cabecera 28,5, filas 30,75, pie 32

// Medidas de las dos pantallas.
const (
	// AnchoBalance es `max-w-4xl`: 56 rem.
	AnchoBalance float32 = 56 * tema.Rem
	// TituloLibroMB es el `mb-5` del título y AsientosGap el `space-y-3`.
	TituloLibroMB float32 = 5 * 0.25 * tema.Rem
	AsientosGap   float32 = 3 * 0.25 * tema.Rem
	// VacioLibroPY es el `py-16` de «Todavía no hay nada asentado».
	VacioLibroPY float32 = 16 * 0.25 * tema.Rem
	// CeldaBalancePX y CeldaBalancePY son el `px-3 py-2` del balance.
	CeldaBalancePX float32 = 3 * 0.25 * tema.Rem
	CeldaBalancePY float32 = 2 * 0.25 * tema.Rem
)

// Libro es el estado de las dos pantallas de contabilidad.
type Libro struct {
	tr  *i18n.Translator
	loc i18n.Locale
	// Asientos es el libro; nil, cargando.
	Asientos []api.ERPAsiento
	// Balance es el sumas y saldos; nil, cargando.
	Balance *api.ERPBalance
	// Fallo es la banda roja que va debajo de la cabecera, en lugar del contenido.
	Fallo string
	// Ventana y Ocupado deciden el ancho, como en la lista.
	Ventana float32
	// Periodo es el corte elegido en el selector junto al título (periodo.go); vacío,
	// «todo». AlPeriodo lo cambia y vuelve a pedir; nil, sin selector.
	Periodo   string
	AlPeriodo func(string)
	// selPeriodo es el MISMO selector de una pintada a otra, como `Lista.buscador`:
	// elegir repinta, y uno nuevo en cada Repintar dejaba el foco en el que ya no se ve
	// —con las flechas había que volver con Tab a cada paso—. En la web el `<select>`
	// conserva el foco.
	selPeriodo *ui.Selector
}

// periodo es el elegido, con «todo» por defecto.
func (b *Libro) periodo() string {
	if b.Periodo == "" {
		return "todo"
	}
	return b.Periodo
}

// cabecera es el `CabeceraPeriodo` de la web: el título y, a la derecha, el `<select>`
// del período con las medidas del de la barra de lote. Es más bajo que el título, así
// que la fila mide lo que el título.
func (b *Libro) cabecera(clave string, v tema.Variante) fyne.CanvasObject {
	titulo := ui.Texto(b.tr.T(clave, nil), tema.Texto2XL, ui.SemiNegrita, tema.Color("text", v))
	if b.AlPeriodo == nil {
		return textoSolo(titulo)
	}
	opciones := make([]string, len(Periodos))
	for i, p := range Periodos {
		opciones[i] = b.tr.T(rotuloPeriodo[p], nil)
	}
	if b.selPeriodo == nil {
		b.selPeriodo = ui.NuevoSelector(nil, nil)
	}
	sel := b.selPeriodo
	sel.Opciones, sel.AlElegir = opciones, func(i int) { b.AlPeriodo(Periodos[i]) }
	sel.Elegida = max(slices.Index(Periodos, b.periodo()), 0)
	sel.Propio = selectorPropio(CampoLote, AltoSelectLote)
	sel.Ancho = AnchoSelectLote(opciones)
	sel.Rotulo = b.tr.T("ledger.period", nil)
	sel.Refresh()
	return container.New(&cabeceraPeriodo{t: titulo, sel: sel}, titulo, sel)
}

// cabeceraPeriodo pone el título a la izquierda y el selector a la derecha, centrado.
type cabeceraPeriodo struct {
	t   *canvas.Text
	sel *ui.Selector
}

func (c *cabeceraPeriodo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, tema.Texto2XL.Linea)
}

func (c *cabeceraPeriodo) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(c.t, tema.Texto2XL, 0, (t.Height-tema.Texto2XL.Linea)/2)
	m := c.sel.MinSize()
	c.sel.Resize(m)
	c.sel.Move(fyne.NewPos(t.Width-m.Width, (t.Height-m.Height)/2))
}

// claveVacio es `textoVacio`: con un corte puesto, «Nada asentado en este período».
func (b *Libro) claveVacio() string {
	if b.periodo() == "todo" {
		return "ledger.empty"
	}
	return "ledger.emptyPeriod"
}

// anchoUtilDe es el `max-w-*` centrado en `main` menos su `px-6`.
func anchoUtilDe(ventana, maximo float32) float32 {
	main := ventana - ocupaCarcasa(ventana)
	if ventana == 0 {
		main = maximo
	}
	return max(min(main, maximo)-2*MargenLista, 0)
}

// ObjetoLibro es el libro diario.
func (b *Libro) ObjetoLibro(v tema.Variante) fyne.CanvasObject {
	p := &pila{}
	p.add(b.cabecera("nav.ledger", v), tema.Texto2XL.Linea, 0)
	if b.pendiente(p, b.Asientos == nil, v) {
		o, alto := p.objeto()
		return enPagina(o, alto, AnchoContenidoLista)
	}
	ancho := anchoUtilDe(b.Ventana, AnchoContenidoLista)
	if len(b.Asientos) == 0 {
		o, alto := vacioLibro(b.tr, b.claveVacio(), v)
		p.add(o, alto, TituloLibroMB)
	}
	for i, a := range b.Asientos {
		o, alto := AsientoCompacto(v, a, b.loc, b.tr, ancho)
		arriba := AsientosGap
		if i == 0 {
			arriba = TituloLibroMB
		}
		p.add(o, alto, arriba)
	}
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoContenidoLista)
}

// vacioLibro es «Todavía no hay nada asentado» (o la clave que toque), centrado con su
// `py-16`.
func vacioLibro(tr *i18n.Translator, clave string, v tema.Variante) (fyne.CanvasObject, float32) {
	t := ui.Texto(tr.T(clave, nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	alto := tema.TextoSM.Linea + 2*VacioLibroPY
	return container.New(&alCentro{t: t, alto: alto}, t), alto
}

type alCentro struct {
	t    *canvas.Text
	alto float32
}

func (c *alCentro) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }
func (c *alCentro) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	e := escalonDe(c.t)
	ancho := ui.Medir(c.t.Text, e, ui.Normal).Width
	ui.ColocarTexto(c.t, e, (t.Width-ancho)/2, (t.Height-e.Linea)/2)
}

// textoSolo es un `canvas.Text` colocado en su caja de línea.
func textoSolo(t *canvas.Text) fyne.CanvasObject {
	return container.New(&textoEn1{t: t, alto: escalonDe(t).Linea}, t)
}

// AltoCargandoLibro es el `py-16` con la rueda de 22 de `Cargando` en Libro.tsx.
var AltoCargandoLibro = 22 + 2*VacioLibroPY

// pendiente pone, DEBAJO de la cabecera, la banda roja del fallo (`rounded-md px-4 py-3
// text-sm`) o la rueda mientras carga, y dice si puso alguna. La cabecera con el
// selector va SIEMPRE: pintar solo la rueda o solo el fallo quitaba el selector a cada
// cambio de período, y con un período que fallaba no había forma de volver a «Todo».
func (b *Libro) pendiente(p *pila, cargando bool, v tema.Variante) bool {
	switch {
	case b.Fallo != "":
		bnd := &banda{
			caja:     ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD),
			texto:    ui.Texto(b.Fallo, tema.TextoSM, ui.Normal, tema.Color("danger-text", v)),
			completo: b.Fallo,
		}
		p.add(container.New(bnd, append(bnd.caja.Objetos(), bnd.texto)...), AltoBandaAviso, TituloLibroMB)
	case cargando:
		p.add(centrado(ui.NuevaRueda(22, "text-muted", v), AltoCargandoLibro), AltoCargandoLibro, TituloLibroMB)
	default:
		return false
	}
	return true
}

// enPagina centra un bloque de alto conocido en el `max-w-*` de `main`, con su `px-6
// py-6` (MargenLista).
func enPagina(o fyne.CanvasObject, alto, maximo float32) fyne.CanvasObject {
	return container.New(&pagina0{o: o, alto: alto, maximo: maximo}, o)
}

type pagina0 struct {
	o            fyne.CanvasObject
	alto, maximo float32
}

func (p *pagina0) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, p.alto+2*MargenLista)
}

func (p *pagina0) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	caja := min(t.Width, p.maximo)
	x := (t.Width-caja)/2 + MargenLista
	p.o.Move(fyne.NewPos(x, MargenLista))
	p.o.Resize(fyne.NewSize(max(caja-2*MargenLista, 0), p.alto))
}

// --- El sumas y saldos ------------------------------------------------------------

// ObjetoBalance es el sumas y saldos: cuenta, debe, haber y saldo, y abajo los totales
// con «Cuadra» o «No cuadra», que es lo primero que mira quien abre esta pantalla.
func (b *Libro) ObjetoBalance(v tema.Variante) fyne.CanvasObject {
	p := &pila{}
	p.add(b.cabecera("nav.balance", v), tema.Texto2XL.Linea, 0)
	if b.pendiente(p, b.Balance == nil, v) {
		o, alto := p.objeto()
		return enPagina(o, alto, AnchoBalance)
	}
	ancho := anchoUtilDe(b.Ventana, AnchoBalance)
	if len(b.Balance.Cuentas) == 0 {
		o, alto := vacioLibro(b.tr, b.claveVacio(), v)
		p.add(o, alto, TituloLibroMB)
	} else {
		o, alto := b.tablaBalance(v, ancho)
		p.add(o, alto, TituloLibroMB)
	}
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoBalance)
}

// Altos de las filas del balance, medidos: la cabecera en versalitas (28,5 con medio
// filete), cada fila con su `border-b` (30,75) y el pie con la insignia (32).
var (
	// Border-collapse reparte medio filete al encabezado y al pie.
	AltoCabeceraBalance = tema.TextoXS.Linea + 2*CeldaBalancePY + 0.5
	AltoFilaBalance     = tema.TextoSM.Linea + 2*CeldaBalancePY + tema.Borde
	AltoPieBalance      = tema.Espacio(5) + 2*CeldaBalancePY + 0.5
)

func (b *Libro) tablaBalance(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	l, bal := b.loc, b.Balance
	cab := []string{
		b.tr.T("ledger.account", nil), b.tr.T("ledger.debit", nil), b.tr.T("ledger.credit", nil),
		b.tr.T("balance.balance", nil),
	}
	filas := make([][]celdaERP, 0, len(bal.Cuentas))
	for _, c := range bal.Cuentas {
		filas = append(filas, []celdaERP{
			{texto: c.Cuenta, mono: true, escalon: tema.TextoXS, tinta: "text"},
			{texto: importeAsiento(c.Debe, l), escalon: tema.TextoSM, tinta: "text"},
			{texto: importeAsiento(c.Haber, l), escalon: tema.TextoSM, tinta: "text"},
			{texto: i18n.FormatCurrency(l, c.Saldo), escalon: tema.TextoSM, tinta: "text"},
		})
	}
	cuadra := bal.TotalDebe == bal.TotalHaber
	sello, tono := b.tr.T("balance.squares", nil), ui.InsigniaExito
	if !cuadra {
		sello, tono = b.tr.T("balance.doesNotSquare", nil), ui.InsigniaPeligro
	}
	pie := []string{b.tr.T("balance.total", nil), i18n.FormatCurrency(l, bal.TotalDebe), i18n.FormatCurrency(l, bal.TotalHaber)}
	// El reparto automático del navegador con el `px-3` de cada celda.
	maxs := make([]float32, 4)
	for i, c := range cab {
		maxs[i] = ui.MedirVersalCon(c, tema.TextoXS, ui.Negrita, ui.TrackingOverline) + 2*CeldaBalancePX
	}
	for _, f := range filas {
		for i, c := range f {
			p := ui.Normal
			if i == 3 {
				p = ui.Medio
			}
			maxs[i] = max(maxs[i], medirTexto(c.texto, c.escalon, c.mono, p)+2*CeldaBalancePX)
		}
	}
	for i, s := range pie {
		maxs[i] = max(maxs[i], ui.Medir(s, tema.TextoSM, ui.Medio).Width+2*CeldaBalancePX)
	}
	maxs[3] = max(maxs[3], ui.MedidaInsignia(sello).Width+2*CeldaBalancePX)
	anchos := RepartoERP(ancho, maxs, maxs, make([]float32, 4))

	t := &tablaBalanceLay{anchos: anchos}
	var objs []fyne.CanvasObject
	for i, c := range cab {
		x := ui.NuevoVersal(c, tema.TextoXS, ui.Negrita, tema.Color("text", v))
		t.cab = append(t.cab, x)
		t.anchoCab = append(t.anchoCab, x.Ancho())
		objs = append(objs, x.Objetos()...)
		_ = i
	}
	for _, f := range filas {
		var fila []celdaTexto
		for i, c := range f {
			p := ui.Normal
			if i == 3 {
				p = ui.Medio
			}
			var tx *canvas.Text
			if c.mono {
				tx = textoCelda(c.texto, c, v)
			} else {
				tx = ui.Texto(c.texto, c.escalon, p, tema.Color(c.tinta, v))
			}
			fila = append(fila, celdaTexto{t: tx, col: i, derecha: i > 0, ancho: medirTexto(c.texto, c.escalon, c.mono, p), e: c.escalon})
			objs = append(objs, tx)
		}
		t.filas = append(t.filas, fila)
	}
	for i, s := range pie {
		tx := ui.Texto(s, tema.TextoSM, ui.Medio, tema.Color("text", v))
		t.pie = append(t.pie, celdaTexto{t: tx, col: i, derecha: i > 0, ancho: ui.Medir(s, tema.TextoSM, ui.Medio).Width, e: tema.TextoSM})
		objs = append(objs, tx)
	}
	t.sello = ui.NuevaInsignia(sello, tono)
	objs = append(objs, t.sello)
	for range len(filas) + 1 {
		d := canvas.NewRectangle(tema.Color("border", v))
		t.filetes = append(t.filetes, d)
		objs = append(objs, d)
	}
	alto := AltoCabeceraBalance + float32(len(filas))*AltoFilaBalance + AltoPieBalance
	return container.New(t, objs...), alto
}

type tablaBalanceLay struct {
	anchos   []float32
	cab      []*ui.Versal
	anchoCab []float32
	filas    [][]celdaTexto
	pie      []celdaTexto
	sello    *ui.Insignia
	filetes  []*canvas.Rectangle
}

func (t *tablaBalanceLay) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (t *tablaBalanceLay) x(col int, derecha bool, ancho float32) float32 {
	x := float32(0)
	for i := range col {
		x += t.anchos[i]
	}
	if derecha {
		return x + t.anchos[col] - CeldaBalancePX - ancho
	}
	return x + CeldaBalancePX
}

func (t *tablaBalanceLay) Layout(_ []fyne.CanvasObject, tam fyne.Size) {
	for i, c := range t.cab {
		c.Colocar(t.x(i, i > 0, t.anchoCab[i]), CeldaBalancePY)
	}
	// Los filetes van al pie de la cabecera y de cada fila (`border-b`).
	y := AltoCabeceraBalance
	t.filetes[0].Move(fyne.NewPos(0, y-tema.Borde))
	t.filetes[0].Resize(fyne.NewSize(tam.Width, tema.Borde))
	for f, fila := range t.filas {
		for _, c := range fila {
			ui.ColocarTexto(c.t, c.e, t.x(c.col, c.derecha, c.ancho), y+CeldaBalancePY+(tema.TextoSM.Linea-c.e.Linea)/2)
		}
		y += AltoFilaBalance
		t.filetes[f+1].Move(fyne.NewPos(0, y-tema.Borde))
		t.filetes[f+1].Resize(fyne.NewSize(tam.Width, tema.Borde))
	}
	for _, c := range t.pie {
		ui.ColocarTexto(c.t, c.e, t.x(c.col, c.derecha, c.ancho), y+(AltoPieBalance-c.e.Linea)/2)
	}
	m := ui.MedidaInsignia(t.sello.Texto)
	t.sello.Resize(m)
	t.sello.Move(fyne.NewPos(t.x(3, true, m.Width), y+(AltoPieBalance-m.Height)/2))
}
