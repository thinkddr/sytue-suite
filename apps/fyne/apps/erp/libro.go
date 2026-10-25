// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
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
	// Fallo es la banda roja de arriba (`m-6`), que en la web SUSTITUYE a la pantalla.
	Fallo string
	// Ventana y Ocupado deciden el ancho, como en la lista.
	Ventana float32
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
	if o := b.fallo(v); o != nil {
		return o
	}
	if b.Asientos == nil {
		return enPagina(centrado(ui.NuevaRueda(22, "text-muted", v), AltoEsqueleto), AltoEsqueleto, AnchoContenidoLista, 0)
	}
	ancho := anchoUtilDe(b.Ventana, AnchoContenidoLista)
	p := &pila{}
	titulo := ui.Texto(b.tr.T("nav.ledger", nil), tema.Texto2XL, ui.SemiNegrita, tema.Color("text", v))
	p.add(textoSolo(titulo), tema.Texto2XL.Linea, 0)
	if len(b.Asientos) == 0 {
		o, alto := vacioLibro(b.tr, v)
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
	return enPagina(o, alto, AnchoContenidoLista, MargenLista)
}

// vacioLibro es «Todavía no hay nada asentado», centrado con su `py-16`.
func vacioLibro(tr *i18n.Translator, v tema.Variante) (fyne.CanvasObject, float32) {
	t := ui.Texto(tr.T("ledger.empty", nil), tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
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

// fallo es la banda roja que la web pone EN LUGAR de la pantalla: `m-6 rounded-md px-4
// py-3 text-sm`.
func (b *Libro) fallo(v tema.Variante) fyne.CanvasObject {
	if b.Fallo == "" {
		return nil
	}
	bnd := &banda{
		caja:     ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD),
		texto:    ui.Texto(b.Fallo, tema.TextoSM, ui.Normal, tema.Color("danger-text", v)),
		completo: b.Fallo,
	}
	o := container.New(bnd, append(bnd.caja.Objetos(), bnd.texto)...)
	return container.New(&conMargenes{m: MargenLista, alto: AltoBandaAviso}, o)
}

// conMargenes pone su hijo con el mismo margen por los cuatro lados.
type conMargenes struct{ m, alto float32 }

func (c *conMargenes) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, c.alto+2*c.m)
}

func (c *conMargenes) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(c.m, c.m))
		o.Resize(fyne.NewSize(max(t.Width-2*c.m, 0), c.alto))
	}
}

// enPagina centra un bloque de alto conocido en el `max-w-*` de `main`, con `py`.
func enPagina(o fyne.CanvasObject, alto, maximo, py float32) fyne.CanvasObject {
	return container.New(&pagina0{o: o, alto: alto, maximo: maximo, py: py}, o)
}

type pagina0 struct {
	o            fyne.CanvasObject
	alto, maximo float32
	py           float32
}

func (p *pagina0) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, p.alto+2*p.py) }
func (p *pagina0) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	caja := min(t.Width, p.maximo)
	x := (t.Width-caja)/2 + MargenLista
	p.o.Move(fyne.NewPos(x, p.py))
	p.o.Resize(fyne.NewSize(max(caja-2*MargenLista, 0), p.alto))
}

// --- El sumas y saldos ------------------------------------------------------------

// ObjetoBalance es el sumas y saldos: cuenta, debe, haber y saldo, y abajo los totales
// con «Cuadra» o «No cuadra», que es lo primero que mira quien abre esta pantalla.
func (b *Libro) ObjetoBalance(v tema.Variante) fyne.CanvasObject {
	if o := b.fallo(v); o != nil {
		return o
	}
	if b.Balance == nil {
		return enPagina(centrado(ui.NuevaRueda(22, "text-muted", v), AltoEsqueleto), AltoEsqueleto, AnchoBalance, 0)
	}
	ancho := anchoUtilDe(b.Ventana, AnchoBalance)
	p := &pila{}
	titulo := ui.Texto(b.tr.T("nav.balance", nil), tema.Texto2XL, ui.SemiNegrita, tema.Color("text", v))
	p.add(textoSolo(titulo), tema.Texto2XL.Linea, 0)
	if len(b.Balance.Cuentas) == 0 {
		o, alto := vacioLibro(b.tr, v)
		p.add(o, alto, TituloLibroMB)
	} else {
		o, alto := b.tablaBalance(v, ancho)
		p.add(o, alto, TituloLibroMB)
	}
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoBalance, MargenLista)
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
