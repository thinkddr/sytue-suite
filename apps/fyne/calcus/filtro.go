// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"iter"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// Autofiltro es `SheetAutoFilter`: la fila de cabecera con los desplegables, las
// columnas que cubre y, por columna (su índice en texto), los valores EXCLUIDOS, que son
// los que ocultan filas. Se trata como inmutable: cambiarlo es poner otro.
type Autofiltro struct {
	Row  int                 `json:"row"`
	C1   int                 `json:"c1"`
	C2   int                 `json:"c2"`
	Excl map[string][]string `json:"excl,omitempty"`
	// Cond es la condición por columna (mayor que, contiene, N superiores, color…), con
	// las mismas claves que Excl (filtro_cond.go).
	Cond map[string]CondFiltro `json:"cond,omitempty"`
}

// autofiltroCrudo es el autofiltro tal y como llega, sin sanear.
type autofiltroCrudo struct {
	Row  any            `json:"row"`
	C1   any            `json:"c1"`
	C2   any            `json:"c2"`
	Excl map[string]any `json:"excl"`
	Cond map[string]any `json:"cond"`
}

// sanear es la rama `autofilter` de `sanitizeSheet`: columnas y fila dentro de la
// rejilla, y de las exclusiones solo las de una columna cubierta y con textos.
func (a autofiltroCrudo) sanear() *Autofiltro {
	c1 := clamp(enteroJS(a.C1), Columnas-1)
	c2 := min(max(enteroJS(a.C2), c1), Columnas-1)
	out := &Autofiltro{Row: clamp(enteroJS(a.Row), Filas-1), C1: c1, C2: c2}
	for clave, crudo := range a.Excl {
		n, err := strconv.ParseFloat(strings.TrimSpace(clave), 64)
		lista, esLista := crudo.([]any)
		if err != nil || n != math.Trunc(n) || int(n) < c1 || int(n) > c2 || !esLista {
			continue
		}
		var textos []string
		for _, v := range lista {
			if s, ok := v.(string); ok {
				textos = append(textos, s)
			}
		}
		if len(textos) > 0 {
			if out.Excl == nil {
				out.Excl = map[string][]string{}
			}
			out.Excl[strconv.Itoa(int(n))] = textos
		}
	}
	out.Cond = sanearCondsFiltro(a.Cond, c1, c2)
	return out
}

// Autofiltro es el de la hoja activa, o nil.
func (l *Libro) Autofiltro() *Autofiltro { return l.h().filtro }

// AlternarAutofiltro es el comando `autofilter`: lo quita si lo hay y, si no, lo pone
// sobre el rango; con una sola celda, sobre el rectángulo usado de la hoja. Una hoja
// vacía no se filtra.
func (l *Libro) AlternarAutofiltro(c1, f1, c2, f2 int) bool {
	h := l.h()
	if h.filtro != nil {
		l.anotar()
		l.h().filtro = nil
		return true
	}
	if c1 == c2 && f1 == f2 {
		c1, f1, c2, f2 = Columnas, Filas, -1, -1
		for k := range h.celdas {
			c1, f1, c2, f2 = min(c1, k.C), min(f1, k.F), max(c2, k.C), max(f2, k.F)
		}
		if c2 < 0 {
			return false
		}
	}
	l.anotar()
	l.h().filtro = &Autofiltro{Row: f1, C1: c1, C2: c2}
	return true
}

// Filtrar es `filterset`: esconde (o vuelve a enseñar) las filas con ese valor en esa
// columna. Una columna sin exclusiones deja de estar en el mapa.
func (l *Libro) Filtrar(col int, valor string, oculto bool) bool {
	af := l.h().filtro
	if af == nil || col < af.C1 || col > af.C2 {
		return false
	}
	lista := slices.DeleteFunc(slices.Clone(af.Excl[strconv.Itoa(col)]), func(v string) bool { return v == valor })
	if oculto {
		lista = append(lista, valor)
	}
	nuevo := af.conExcluidos(col, lista)
	l.anotar()
	l.h().filtro = nuevo
	return true
}

// OrdenarFiltro es `filtersort`, el «Ordenar» del desplegable: ordena el cuerpo del
// autofiltro (`cuerpoDelFiltro`) por la columna col. Se ordenan también las filas
// ocultas: el filtro va por valor, así que siguen ocultas donde caigan.
func (l *Libro) OrdenarFiltro(col int, desc bool) bool {
	af := l.h().filtro
	if af == nil || col < af.C1 || col > af.C2 {
		return false
	}
	f1, f2 := cuerpoDelFiltro(l.tablasActivas(), af, maps.Keys(l.h().celdas))
	if f2 <= f1 {
		return false // una fila o ninguna: `Ordenar` tomaría la hoja entera
	}
	return l.Ordenar(desc, Celda{C: col}, af.C1, f1, af.C2, f2)
}

// cuerpoDelFiltro es `autofilterBody`: si el autofiltro es el de una tabla (su
// cabecera), el cuerpo de la tabla sin la fila de totales; si no, de la fila siguiente
// a la de los botones hasta la última con algo en sus columnas. f2 ≤ f1: nada.
func cuerpoDelFiltro(tablas []tablaCompilada, af *Autofiltro, celdas iter.Seq[Celda]) (f1, f2 int) {
	for _, ct := range tablas {
		if ct.cab && ct.rect.R1 == af.Row && ct.rect.C1 == af.C1 && ct.rect.C2 == af.C2 {
			return ct.cuerpo1, ct.cuerpo2
		}
	}
	f2 = af.Row
	for k := range celdas {
		if k.C >= af.C1 && k.C <= af.C2 {
			f2 = max(f2, k.F)
		}
	}
	return af.Row + 1, f2
}

// textoFiltro es `cellText`: el valor calculado en General, sin el formato de número.
// Es lo que se agrupa en el desplegable y lo que se compara al ocultar.
func (l *Libro) textoFiltro(c, f int) string { return formula.Display(l.Valor(c, f)) }

// FilasOcultas son las filas que no se ven en la hoja activa: las que esconde el
// autofiltro (`filteredRows`) y las del detalle de un grupo contraído del esquema
// (`hiddenUnits(rowGroups)`), que la web une en el mismo conjunto. nil si no se oculta
// ninguna, que es lo corriente y deja la rejilla con su geometría de siempre.
func (l *Libro) FilasOcultas() []bool {
	var ocultas []bool
	marcar := func(f int) {
		if ocultas == nil {
			ocultas = make([]bool, Filas)
		}
		ocultas[f] = true
	}
	for _, f := range l.filtradasDe(l.f.activa) {
		marcar(f)
	}
	for f := range analisis.Ocultas(l.h().grupFilas) {
		marcar(f)
	}
	return ocultas
}

// filtradasDe es `filteredRows` de la hoja i: una fila del cuerpo se oculta si su valor
// en ALGUNA columna filtrada está excluido. Lee el motor directamente —y no Valor— porque
// también lo usa sincronizarOcultas, que corre dentro de Valor. La fila de totales de
// una tabla no se oculta nunca, aunque su valor esté excluido.
func (l *Libro) filtradasDe(i int) []int {
	h := &l.f.hojas[i]
	af := h.filtro
	if af == nil {
		return nil
	}
	totales := map[int]bool{}
	for _, ct := range compilarTablas(h.tablas) {
		if ct.tot {
			totales[ct.rect.R2] = true
		}
	}
	ocultas := map[int]bool{}
	ocultar := func(f int) {
		if !totales[f] {
			ocultas[f] = true
		}
	}
	// Los valores excluidos se miran hasta el final de la rejilla, que es lo que este
	// lienzo hacía y lo que fija `TestLaTablaCreceYSeGestionaDesdeLosComandos`: excluir
	// «(Vacías)» se lleva también las filas en blanco de debajo de los datos.
	//
	// Las condiciones, hasta la última fila usada, como la web (`filteredRows`): debajo
	// todo está vacío, y «mayor que 15» escondería todas las filas en blanco hasta el final.
	//
	// ponytail: son dos topes distintos. La web usa el de la última fila usada para los
	// dos; igualar el de los valores excluidos cambia lo que hoy se ve al excluir
	// «(Vacías)» y es decisión de producto, no de este arreglo.
	_, ultima := l.motor.UsedRange(h.nombre)
	l.ocultarExcluidas(h, af, Filas, ocultar)
	l.ocultarPorCondicion(h, af, min(ultima+1, Filas), ocultar)
	return slices.Sorted(maps.Keys(ocultas))
}

// ocultarExcluidas marca las filas del cuerpo cuyo valor en una columna está excluido.
func (l *Libro) ocultarExcluidas(h *hojaLibro, af *Autofiltro, fin int, ocultar func(int)) {
	for clave, lista := range af.Excl {
		c, err := strconv.Atoi(clave)
		if err != nil || len(lista) == 0 || c < af.C1 || c > af.C2 {
			continue
		}
		for f := af.Row + 1; f < fin; f++ {
			if slices.Contains(lista, formula.Display(l.motor.ValueAt(h.nombre, c, f))) {
				ocultar(f)
			}
		}
	}
}

// ocultarPorCondicion marca las filas del cuerpo que no pasan la condición de una columna
// (filtro_cond.go).
func (l *Libro) ocultarPorCondicion(h *hojaLibro, af *Autofiltro, fin int, ocultar func(int)) {
	for clave, cond := range af.Cond {
		c, err := strconv.Atoi(clave)
		if err != nil || c < af.C1 || c > af.C2 || fin <= af.Row+1 {
			continue
		}
		cuerpo := make([]celdaFiltro, 0, fin-af.Row-1)
		for f := af.Row + 1; f < fin; f++ {
			cuerpo = append(cuerpo, celdaFiltro{v: l.motor.ValueAt(h.nombre, c, f), estilo: h.estilos[Celda{C: c, F: f}]})
		}
		for i, pasa := range evaluarFiltro(cond, cuerpo) {
			if !pasa {
				ocultar(af.Row + 1 + i)
			}
		}
	}
}

// EntradaFiltro es `SlicerEntry`: un valor distinto de la columna y si está excluido.
type EntradaFiltro struct {
	Valor string
	// Rotulo es lo que se enseña si no es el valor: los elementos de una tabla dinámica,
	// que se guardan por su clave. Vacío, el valor (o «(Vacías)» si es "").
	Rotulo string
	Oculto bool
}

// ordenEs es el `localeCompare(b, 'es', { numeric: true })` de la web.
var ordenEs = collate.New(language.Spanish, collate.Numeric)

// ValoresFiltro es `filterEntries` + `slicerEntries`: los valores distintos del cuerpo
// de la columna, en orden es-ES numérico y con «(Vacías)» —el texto vacío— al final.
//
// ponytail: `collate` no es seguro entre rutinas; basta porque solo lo usa el hilo de
// la interfaz. Si se llama desde otra, un colador por llamada.
func (l *Libro) ValoresFiltro(col int) []EntradaFiltro {
	af := l.h().filtro
	if af == nil {
		return nil
	}
	excl := af.Excl[strconv.Itoa(col)]
	var out []EntradaFiltro
	vistos := map[string]bool{}
	for f := af.Row + 1; f < Filas; f++ {
		t := l.textoFiltro(col, f)
		if !vistos[t] {
			vistos[t] = true
			out = append(out, EntradaFiltro{Valor: t, Oculto: slices.Contains(excl, t)})
		}
	}
	slices.SortStableFunc(out, func(a, b EntradaFiltro) int {
		switch {
		case a.Valor == b.Valor:
			return 0
		case a.Valor == "":
			return 1
		case b.Valor == "":
			return -1
		}
		return ordenEs.CompareString(a.Valor, b.Valor)
	})
	return out
}

// AnchoBotonFiltro es el `w-4` del «▾» del autofiltro; de alto, `h-[24px]`, la celda.
var AnchoBotonFiltro = tema.Espacio(4)

// botonFiltro es dónde va el «▾» de la columna c: pegado a su borde derecho, en la fila
// de cabecera del autofiltro (`left: colX[c + 1] - 16`).
func (r *Rejilla) botonFiltro(c int) (fyne.Position, fyne.Size, bool) {
	af := r.L.Autofiltro()
	if af == nil || c < af.C1 || c > af.C2 {
		return fyne.Position{}, fyne.Size{}, false
	}
	vw := r.vista()
	return fyne.NewPos(vw.x(c+1)-AnchoBotonFiltro*vw.z, vw.y(af.Row)), fyne.NewSize(AnchoBotonFiltro*vw.z, vw.alto()), true
}

// filtroEn dice de qué columna es el «▾» del autofiltro que hay bajo un punto.
func (r *Rejilla) filtroEn(p fyne.Position) (int, bool) {
	af := r.L.Autofiltro()
	if af == nil {
		return 0, false
	}
	for c := af.C1; c <= af.C2; c++ {
		pos, tam, _ := r.botonFiltro(c)
		// Lo que cae bajo las cabeceras está tapado por ellas.
		cx, cy := r.canal()
		if dentro(p, pos, tam) && p.Y >= (AltoCelda+cy)*r.z() && p.X >= (AnchoCabecera+cx)*r.z() {
			return c, true
		}
	}
	return 0, false
}

// abrirFiltro es el desplegable del «▾»: los valores distintos de la columna con sus
// casillas —marcada, se ve; desmarcarla oculta las filas con ese valor—.
//
//	border-border bg-surface absolute top-full right-0 mt-0.5 max-h-56 w-max min-w-32
//	overflow-auto rounded-lg border py-1 shadow-[var(--shadow-md)]
func (c *Lienzo) abrirFiltro(col int) {
	pos, tam, ok := c.Rejilla.botonFiltro(col)
	if !ok {
		return
	}
	entradas := c.Libro.ValoresFiltro(col)
	filas := make([]fyne.CanvasObject, 0, len(entradas))
	for _, en := range entradas {
		rotulo := en.Valor
		if rotulo == "" {
			rotulo = c.t("slicer.blanks", nil)
		}
		filas = append(filas, filaFiltro(rotulo, !en.Oculto, func(visible bool) {
			b, _ := json.Marshal(map[string]any{"col": col, "value": en.Valor, "hidden": !visible})
			c.Mandar(CmdFiltrar, string(b))
		}))
	}
	// Antes de los valores, ordenar por esta columna: `<div role="menu" class="px-1">`
	// con dos `MenuItem size="sm"` y un `MenuSeparator`, que llega de borde a borde.
	var emergente *ui.Emergente
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ordenar := func(icono iconos.Ed, clave, dir string) fyne.CanvasObject {
		e := ui.NuevoElementoMenu(c.t(clave, nil), iconos.Ninguno, func() {
			emergente.Cerrar()
			b, _ := json.Marshal(map[string]any{"col": col, "dir": dir})
			c.Mandar(CmdOrdenarFiltro, string(b))
		})
		// `<EditorIcon size="md">` dentro del `<span class="text-text-muted">` del ítem.
		e.Denso, e.Glifo = true, iconos.TrazoEd(icono, iconos.TallaSM, "text-muted", v)
		return e
	}
	menu := container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1)}, container.New(ui.Columna{},
		ordenar(iconos.EdOrdenAsc, "data.sortAsc", "asc"), ordenar(iconos.EdOrdenDesc, "data.sortDesc", "desc")))
	lista := container.New(ui.Columna{}, append([]fyne.CanvasObject{menu, ui.NuevoSeparadorMenu()}, filas...)...)
	medida := lista.MinSize()
	medida.Width = max(medida.Width, tema.Espacio(32))
	medida.Height = min(medida.Height+2*tema.Espacio(1), tema.Espacio(56))
	desp := desplazable(container.New(ui.Margen{Arr: tema.Espacio(1), Abj: tema.Espacio(1)}, lista), medida)

	// El panel se cuelga de un ancla del ancho del panel que arranca en el borde
	// izquierdo de la columna, como en Excel y Calc: pegado al «▾» por la derecha, en la
	// columna A se salía de la rejilla.
	ancho := ui.MedidaEmergente(desp, 0).Width
	// El emergente cuelga a `mt-1` de su disparador; este va a `mt-0.5`, así que el
	// ancla se queda corta en la diferencia.
	c.anclaFiltro.Move(fyne.NewPos(c.Rejilla.vista().x(col), AltoBarra+pos.Y))
	c.anclaFiltro.Resize(fyne.NewSize(ancho, tam.Height-(ui.SeparacionEmergente-SeparacionValores)))
	emergente = ui.MostrarEmergente(c.anclaFiltro, desp, 0)
}

// casillaXS es la casilla de los paneles de la cinta, que en la web es un
// `<input type="checkbox">` suelto dentro de su etiqueta:
//
//	<label class="text-text flex cursor-pointer items-center gap-1.5 text-xs">
//
// La casilla de la suite (`Checkbox`) lleva la etiqueta en `text-base`; aquí va en
// `text-xs`, y se pulsa en cualquier parte de la fila, como un `<label>`.
type casillaXS struct {
	widget.BaseWidget
	casilla *ui.Eleccion
	fila    fyne.CanvasObject
}

func nuevaCasillaXS(rotulo string, marcada bool, hueco float32, alCambiar func(bool)) *casillaXS {
	f := &casillaXS{casilla: ui.NuevaCasilla(rotulo, alCambiar)}
	f.casilla.Marcada, f.casilla.SinEtiqueta = marcada, true
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fila = container.New(ui.Fila{Hueco: hueco}, f.casilla, ui.NuevoParrafo(rotulo, tema.TextoXS, ui.Normal, "text", v))
	f.ExtendBaseWidget(f)
	return f
}

func (f *casillaXS) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(f.fila) }

// Tapped es el clic en el texto: conmuta la casilla.
func (f *casillaXS) Tapped(e *fyne.PointEvent) { f.casilla.Tapped(e) }

// Marcada dice si está marcada.
func (f *casillaXS) Marcada() bool { return f.casilla.Marcada }

// Inerte la apaga (`disabled`).
func (f *casillaXS) Inerte() { f.casilla.Disable() }

// filaFiltro es una fila del desplegable del autofiltro, con su `px-2 py-0.5 gap-2`.
func filaFiltro(rotulo string, marcada bool, alCambiar func(bool)) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(0.5), Abj: tema.Espacio(0.5)},
		nuevaCasillaXS(rotulo, marcada, tema.Espacio(2), alCambiar))
}
