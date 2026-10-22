// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"math"
	"regexp"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Los informes de Sytue AI (`PartesInforme`, `Parte` y `VistaTablero` de SytueIA.tsx)
//
// Un informe son partes —una cifra, unas barras o una tabla— en una rejilla de dos
// columnas desde `md`, cada una en su `<Card class="p-4">`; las tablas ocupan la fila
// entera. Los datos son los de AHORA: un tablero guardado se vuelve a ejecutar al
// abrirlo. Medido en el arnés (ErpApp con el servicio de mentira, 1280×800, rem 14,
// 2026-09-23), el tablero «Ventas de 2026»:
//
//	página    `max-w-6xl` (1008) con `px-6 py-6` y `space-y-5`: 266 → 1274, contenido 966
//	cabecera  31,5: el `h1` `text-2xl` con el icono de 20 y `gap-2`, y los dos botones `md`
//	rejilla   `gap-4`, columnas de 476; fila de cifra y barras de 176,25, tabla de 186,25
//	tarjeta   `p-4` con el borde: título `text-sm font-medium` y `mb-3`
//	barras    120 de alto; con cuatro puntos, 110,01 de ancho y 111,5 de paso en 446
//	tabla     `Table` de la suite: columnas 335,08 · 151,78 · 158,11 · 161,28 · 127,75

// Medidas de los informes, de HOJA salvo donde dice lo contrario.
const (
	// AnchoTablero es `max-w-6xl`: 72 rem.
	AnchoTablero float32 = 72 * tema.Rem
	// HuecoTablero es el `space-y-5` entre la cabecera y la rejilla.
	HuecoTablero float32 = 5 * 0.25 * tema.Rem
	// HuecoPartes es el `gap-4` de la rejilla.
	HuecoPartes float32 = 4 * 0.25 * tema.Rem
	// TituloParteMB es el `mb-3` del título de una parte.
	TituloParteMB float32 = 3 * 0.25 * tema.Rem
	// RecortadaMT es el `mt-2` de «Calculado solo con las primeras filas».
	RecortadaMT float32 = 2 * 0.25 * tema.Rem
	// AltoBarras es el `height = 120` de BarChart, en px de CSS (el SVG no escala en alto).
	AltoBarras float32 = 120
	// VistaBarras es el `VIEW_W` del lienzo de BarChart y HuecoBarras su `GAP`, en
	// unidades del `viewBox`: el SVG se estira a lo ancho (`preserveAspectRatio="none"`).
	VistaBarras float32 = 600
	HuecoBarras float32 = 2
	// LadoIconoTablero es el `size={20}` del icono del título.
	LadoIconoTablero float32 = 20
)

// mesAgrupado es un mes agrupado («2026-09»), que se lee como mes y no como código.
var mesAgrupado = regexp.MustCompile(`^\d{4}-\d{2}$`)

// ValorInforme es `valor` de SytueIA.tsx: una celda de un informe lista para pintar. Los
// importes de un informe llegan en UNIDADES, no en céntimos como los de un registro.
func ValorInforme(col *api.ERPColumnaInforme, v any, l i18n.Locale) string {
	if v == nil {
		return ""
	}
	tipo := ""
	if col != nil {
		tipo = col.Tipo
	}
	switch tipo {
	case api.ERPColDinero:
		if n, ok := numero(v); ok {
			// `Math.round(Number(v) * 100)`: la mitad sube.
			return i18n.FormatCurrency(l, int64(math.Floor(n*100+0.5)))
		}
	case api.ERPColNumero:
		if n, ok := numero(v); ok {
			// `maximumFractionDigits: 2`: se redondea a dos y se escribe sin ceros de más.
			return i18n.FormatNumber(l, math.Round(n*100)/100)
		}
	case api.ERPColFecha:
		if t, ok := fechaISO(texto(v)); ok {
			return i18n.FormatDate(l, t)
		}
		return texto(v)
	case api.ERPColBooleano:
		if si, ok := v.(bool); ok && si {
			return "✓"
		}
		return SinValor
	}
	if s, ok := v.(string); ok && mesAgrupado.MatchString(s) {
		if t, err := time.Parse("2006-01", s); err == nil {
			return i18n.FormatMonthShortYear(l, t)
		}
	}
	if b, ok := v.(bool); ok {
		return strconv.FormatBool(b)
	}
	return texto(v)
}

// numero es `Number(v)` para lo que llega en JSON.
func numero(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

// columna i de una tabla de informe, o nil si no la hay.
func columnaDe(t api.ERPTablaInforme, i int) *api.ERPColumnaInforme {
	if i < len(t.Columnas) {
		return &t.Columnas[i]
	}
	return nil
}

// celdaDeFila es la celda i de una fila, o nil.
func celdaDeFila(f []any, i int) any {
	if i < len(f) {
		return f[i]
	}
	return nil
}

// --- Las barras (`BarChart` de @sytue/ui) --------------------------------------------

// Barras es BarChart: una serie en barras, sin ejes ni leyenda, que se estira a lo
// ancho. La barra de un cero no desaparece: se queda de 1 y en `accent-200`, porque «no
// hubo» es información y «no existe» no.
//
// Es un widget y no un contenedor porque se ANUNCIA: la web lo marca `role="img"` con la
// serie y su total, y pone debajo una tabla solo para el lector de pantalla con los
// mismos datos. Aquí el nombre accesible lleva las dos cosas.
type Barras struct {
	widget.BaseWidget
	// Puntos son la etiqueta, ya formateada, y el valor de cada barra.
	Puntos []PuntoBarras
	// Serie es qué mide: va en el nombre accesible.
	Serie string
}

// PuntoBarras es una barra.
type PuntoBarras struct {
	Rotulo string
	Valor  float64
}

var (
	_ fyne.Widget     = (*Barras)(nil)
	_ fyne.Accessible = (*Barras)(nil)
)

// NuevasBarras crea el gráfico.
func NuevasBarras(serie string, puntos []PuntoBarras) *Barras {
	b := &Barras{Serie: serie, Puntos: puntos}
	b.ExtendBaseWidget(b)
	return b
}

// AccessibilityLabel es el `aria-label` del SVG —la serie y su total— seguido de cada
// barra, que es lo que la web lee de su tabla oculta.
func (b *Barras) AccessibilityLabel() string {
	total := 0.0
	for _, p := range b.Puntos {
		total += p.Valor
	}
	s := b.Serie + ": " + strconv.FormatFloat(total, 'f', -1, 64)
	for _, p := range b.Puntos {
		s += "; " + p.Rotulo + " " + strconv.FormatFloat(p.Valor, 'f', -1, 64)
	}
	return s
}

// AccessibilityRole es el de una imagen con texto alternativo.
func (b *Barras) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MinSize es el alto de la serie; el ancho lo pone quien la coloca.
func (b *Barras) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(0, AltoBarras)
}

// geometriaBarras es dónde cae cada barra en un lienzo de `ancho` y lo que mide, con
// la cuenta de BarChart: el paso es 600/n unidades, la barra el paso menos 2, y el alto
// `round(valor/máximo·120)` con un suelo de 1. El máximo nunca es 0: dividir por él
// borraría todas las barras justo cuando el dato es «todo a cero».
func geometriaBarras(puntos []PuntoBarras, ancho float32) ([]fyne.Position, []fyne.Size) {
	maximo := 1.0
	for _, p := range puntos {
		maximo = math.Max(maximo, p.Valor)
	}
	escala := ancho / VistaBarras
	paso := VistaBarras / float32(len(puntos))
	barra := max(1, paso-HuecoBarras)
	pos, tam := make([]fyne.Position, len(puntos)), make([]fyne.Size, len(puntos))
	for i, p := range puntos {
		alto := float32(1)
		if p.Valor != 0 {
			alto = float32(max(1, math.Round(p.Valor/maximo*float64(AltoBarras))))
		}
		pos[i] = fyne.NewPos(float32(i)*paso*escala, AltoBarras-alto)
		tam[i] = fyne.NewSize(barra*escala, alto)
	}
	return pos, tam
}

// CreateRenderer pinta una barra por punto.
func (b *Barras) CreateRenderer() fyne.WidgetRenderer {
	r := &barrasRenderer{b: b}
	for range b.Puntos {
		r.barras = append(r.barras, canvas.NewRectangle(color.NRGBA{}))
	}
	r.Refresh()
	return r
}

type barrasRenderer struct {
	b      *Barras
	barras []*canvas.Rectangle
}

func (r *barrasRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *barrasRenderer) Objects() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(r.barras))
	for i, x := range r.barras {
		out[i] = x
	}
	return out
}
func (r *barrasRenderer) Destroy() {}

func (r *barrasRenderer) Layout(t fyne.Size) {
	pos, tam := geometriaBarras(r.b.Puntos, t.Width)
	for i, x := range r.barras {
		x.Move(pos[i])
		x.Resize(tam[i])
		// `rx={1}` en unidades del lienzo, que se estira a lo ancho.
		x.CornerRadius = min(1, t.Width/VistaBarras)
	}
}

func (r *barrasRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for i, x := range r.barras {
		x.FillColor = tema.Color("accent", v)
		if r.b.Puntos[i].Valor == 0 {
			x.FillColor = tema.Color("accent-200", v)
		}
		x.Refresh()
	}
}

// --- Las partes -------------------------------------------------------------------

// Informe es lo que hace falta para pintar las partes de un informe.
type Informe struct {
	tr  *i18n.Translator
	loc i18n.Locale
	// Informe es el que se pinta; Datos, una tabla por parte, o nil mientras se piden.
	Informe api.ERPInforme
	Datos   []api.ERPTablaInforme
	// DosColumnas es el `md:grid-cols-2`: la VENTANA a partir de 768.
	DosColumnas bool
	// Afuera devuelve el desplazador de la página, que se lleva lo vertical de una tabla
	// que se desplaza de lado. nil, ninguno.
	Afuera func() Desplazador
}

func (x *Informe) t(clave string) string { return x.tr.T(clave, nil) }

// cuerpoParte es `Parte`: la cifra, las barras o la tabla, con el aviso de que se
// calculó con las primeras filas si hace falta. Devuelve lo que mide.
func (x *Informe) cuerpoParte(v tema.Variante, p api.ERPParteInforme, i int, ancho float32) (fyne.CanvasObject, float32) {
	if x.Datos == nil || i >= len(x.Datos) {
		// `<Spinner size={16} />`.
		r := ui.NuevaRueda(16, "text-muted", v)
		l := &lienzo{}
		l.poner(r, 0, 0, 16, 16)
		return l.objeto(16)
	}
	t := x.Datos[i]
	if t.Error != "" {
		clave := "ai.unavailable"
		if t.Error == "sin_permiso" {
			clave = "ai.noPermission"
		}
		a := ui.NuevaAyudaEnLinea(x.t(clave)).ConTono(ui.AyudaAviso)
		h := ui.AltoAyudaEnLinea(a, ancho)
		l := &lienzo{}
		l.poner(a, 0, 0, ancho, h)
		return l.objeto(h)
	}
	l := &lienzo{}
	var y float32
	switch p.Vista {
	case api.ERPVistaCifra:
		var celda any
		if len(t.Filas) > 0 {
			celda = celdaDeFila(t.Filas[0], 0)
		}
		c := ui.Texto(ValorInforme(columnaDe(t, 0), celda, x.loc), tema.Texto3XL, ui.SemiNegrita, tema.Color("text", v))
		ui.Truncar(c, c.Text, tema.Texto3XL, ui.SemiNegrita, ancho)
		l.texto(c, 0, 0)
		y = tema.Texto3XL.Linea
	case api.ERPVistaBarras:
		if len(t.Filas) == 0 {
			// `emptyLabel`: `text-text-subtle py-6 text-center text-sm`.
			e := ui.Texto(x.t("ai.empty"), tema.TextoSM, ui.Normal, tema.Color("text-subtle", v))
			l.texto(e, (ancho-ui.Medir(e.Text, tema.TextoSM, ui.Normal).Width)/2, VacioBarrasPY)
			y = tema.TextoSM.Linea + 2*VacioBarrasPY
			break
		}
		puntos := make([]PuntoBarras, len(t.Filas))
		for j, f := range t.Filas {
			n, _ := numero(celdaDeFila(f, 1))
			puntos[j] = PuntoBarras{Rotulo: ValorInforme(columnaDe(t, 0), celdaDeFila(f, 0), x.loc), Valor: n}
		}
		l.poner(NuevasBarras(p.Titulo, puntos), 0, 0, ancho, AltoBarras)
		y = AltoBarras
	default:
		o, h := x.tabla(v, t, ancho)
		l.poner(o, 0, 0, ancho, h)
		y = h
	}
	if t.Recortada {
		y += RecortadaMT
		n := ui.NuevoParrafo(x.t("ai.truncated"), tema.TextoXS, ui.Normal, "text-subtle", v)
		h := ui.AltoParrafo(n, ancho)
		l.poner(n, 0, y, ancho, h)
		y += h
	}
	return l.objeto(y)
}

// VacioBarrasPY es el `py-6` de la serie vacía.
const VacioBarrasPY float32 = 6 * 0.25 * tema.Rem

// tarjetaParte es `<Card class="p-4">` con el título de la parte y su cuerpo.
func (x *Informe) tarjetaParte(v tema.Variante, p api.ERPParteInforme, i int, ancho float32) (fyne.CanvasObject, float32) {
	dentro := max(ancho-2*tema.Borde-2*tema.Espacio(4), 0)
	l := &lienzo{}
	tit := ui.Texto(p.Titulo, tema.TextoSM, ui.Medio, tema.Color("text-muted", v))
	ui.Truncar(tit, p.Titulo, tema.TextoSM, ui.Medio, dentro)
	l.texto(tit, 0, 0)
	y := tema.TextoSM.Linea + TituloParteMB
	c, h := x.cuerpoParte(v, p, i, dentro)
	l.poner(c, 0, y, dentro, h)
	cuerpo, alto := l.objeto(y + h)
	return ui.NuevaTarjeta("", cuerpo), alto + 2*tema.Borde + 2*tema.Espacio(4)
}

// Objeto es `PartesInforme`: la rejilla de tarjetas a `ancho`. Una tabla ocupa la fila
// entera (`md:col-span-2`) y, como en CSS, no se rellena el hueco que deja atrás: la
// colocación de la rejilla es la de por defecto, que no vuelve hacia arriba.
func (x *Informe) Objeto(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	cols := 1
	if x.DosColumnas {
		cols = 2
	}
	w1 := (ancho - float32(cols-1)*HuecoPartes) / float32(cols)
	type celda struct {
		o          fyne.CanvasObject
		fila, col  int
		ancho, alt float32
	}
	celdas := make([]celda, 0, len(x.Informe.Partes))
	fila, col := 0, 0
	for i, p := range x.Informe.Partes {
		abarca := 1
		if p.Vista == api.ERPVistaTabla && cols == 2 {
			abarca = 2
		}
		if col+abarca > cols {
			fila, col = fila+1, 0
		}
		w := w1*float32(abarca) + float32(abarca-1)*HuecoPartes
		o, h := x.tarjetaParte(v, p, i, w)
		celdas = append(celdas, celda{o, fila, col, w, h})
		col += abarca
		if col >= cols {
			fila, col = fila+1, 0
		}
	}
	// Cada fila mide lo que su parte más alta (`align-items: stretch`), y todas llevan
	// alguna: una tabla que no cabe en la fila salta a la siguiente con su hueco atrás.
	filas := 0
	for _, c := range celdas {
		filas = max(filas, c.fila+1)
	}
	altos := make([]float32, filas)
	for _, c := range celdas {
		altos[c.fila] = max(altos[c.fila], c.alt)
	}
	tops := make([]float32, filas)
	for f := 1; f < filas; f++ {
		tops[f] = tops[f-1] + altos[f-1] + HuecoPartes
	}
	l := &lienzo{}
	for _, c := range celdas {
		l.poner(c.o, float32(c.col)*(w1+HuecoPartes), tops[c.fila], c.ancho, altos[c.fila])
	}
	alto := float32(0)
	if filas > 0 {
		alto = tops[filas-1] + altos[filas-1]
	}
	return l.objeto(alto)
}

// --- El tablero guardado (`VistaTablero`) -----------------------------------------------

// Tablero es la pantalla de un tablero guardado: su título con el icono, «Descargar
// Excel» y «Borrar tablero», y sus partes con los datos de ahora.
type Tablero struct {
	Informe
	Tablero api.ERPTablero
	// Ventana es el ancho de la ventana: decide el reparto, como en la lista.
	Ventana float32
	// Descargando es la rueda de «Descargar Excel».
	Descargando       bool
	AlExcel, AlBorrar func()
}

// NuevoTablero crea la pantalla de un tablero, todavía sin datos.
func NuevoTablero(tr *i18n.Translator, t api.ERPTablero) *Tablero {
	x := &Tablero{Tablero: t}
	x.tr, x.loc, x.Informe.Informe = tr, tr.Locale(), t.Informe
	return x
}

// Objeto es la pantalla entera, en su `max-w-6xl` centrado.
func (x *Tablero) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(x.Ventana, AnchoTablero)
	x.DosColumnas = x.Ventana == 0 || x.Ventana >= ui.Estrecho
	p := &pila{}
	cab, altoCab := x.cabecera(v, ancho)
	p.add(cab, altoCab, 0)
	partes, altoPartes := x.Informe.Objeto(v, ancho)
	p.add(partes, altoPartes, HuecoTablero)
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoTablero, MargenLista)
}

// cabecera es el `<header class="flex flex-wrap items-center justify-between gap-3">`.
func (x *Tablero) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	icono := iconos.Trazo(iconos.Grafico, LadoIconoTablero, "text", v).Objeto()
	titulo := ui.Texto(x.Tablero.Titulo, tema.Texto2XL, ui.SemiNegrita, tema.Color("text", v))
	excel := ui.NuevoBoton(x.t("ai.download"), x.AlExcel).Con(ui.Secundario, ui.TallaMD)
	excel.Cargando = x.Descargando
	borrar := ui.NuevoBoton(x.t("dashboard.delete"), x.AlBorrar).Con(ui.Fantasma, ui.TallaMD)
	anchoTitulo := LadoIconoTablero + HuecoDos + ui.Medir(titulo.Text, tema.Texto2XL, ui.SemiNegrita).Width
	der := []pieza{piezaDe(excel, excel.MinSize()), piezaDe(borrar, borrar.MinSize())}
	a, h := medidasDe(der)
	posD, tamD := Envolver(a, h, HuecoDos, HuecoDos, ancho)
	l := &lienzo{}
	// `flex-wrap … gap-3`: los botones a la derecha si caben; si no, debajo.
	altoTit := tema.Texto2XL.Linea
	yTit, xD := float32(0), ancho-tamD.Width
	alto := max(altoTit, tamD.Height)
	var yD float32
	if anchoTitulo+HuecoCabecera3+tamD.Width > ancho {
		xD, yD = 0, altoTit+HuecoCabecera3
		alto = yD + tamD.Height
	} else {
		yTit, yD = (alto-altoTit)/2, (alto-tamD.Height)/2
	}
	ui.Truncar(titulo, x.Tablero.Titulo, tema.Texto2XL, ui.SemiNegrita, max(ancho-LadoIconoTablero-HuecoDos, 0))
	l.poner(icono, 0, yTit+(tema.Texto2XL.Linea-LadoIconoTablero)/2, LadoIconoTablero, LadoIconoTablero)
	l.texto(titulo, LadoIconoTablero+HuecoDos, yTit)
	l.piezas(der, posD, xD, yD)
	return l.objeto(alto)
}

// HuecoCabecera3 es el `gap-3` de la cabecera del tablero.
const HuecoCabecera3 float32 = 3 * 0.25 * tema.Rem
