// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El diálogo de la TABLA DINÁMICA, como `PivotModal.tsx`: el origen y el destino, y las
// cuatro zonas —Filtros, Columnas, Filas y Valores— con sus campos y las opciones de
// cada uno. No calcula nada: manda `pivot` con la definición, como la web.

// zonasDinamica son las cuatro zonas, en el orden de la web.
var zonasDinamica = []struct{ id, clave string }{
	{"filters", "pivot.zone.filters"}, {"cols", "layout.columns"}, {"rows", "common.rows"}, {"values", "common.values"},
}

// Las opciones de los selectores del diálogo, con su clave.
var (
	gruposDinamica  = []string{"none", "years", "quarters", "months", "days", "range"}
	mostrarDinamica = map[string]string{
		"normal": "pivot.show.normal", "pctGrand": "pivot.show.pctGrand", "pctCol": "pivot.show.pctCol",
		"pctRow": "pivot.show.pctRow", "pctParent": "pivot.show.pctParent",
	}
	fnDinamica = map[string]string{
		"sum": "query.agg.sum", "count": "pivot.agg.count", "avg": "query.agg.avg", "max": "query.agg.max",
		"min": "query.agg.min", "product": "pivot.agg.product", "countNums": "pivot.agg.countNums",
		"stdev": "pivot.agg.stdev", "stdevp": "pivot.agg.stdevp", "var": "pivot.agg.var", "varp": "pivot.agg.varp",
	}
	// formatosDinamica son NUMFMT_PRESETS: el formato de un campo de valores.
	formatosDinamica = []struct{ id, clave string }{
		{"general", "numfmt.general"},
		{"number", "numfmt.number"},
		{"thousands", "sheet.mask.thousands"},
		{"currency", "numfmt.currency"},
		{"percent", "sheet.mask.percent"},
		{"date", "numfmt.date"},
	}
)

// Las medidas del diálogo, de PivotModal.tsx.
var (
	huecoDialogoDinamica = tema.Espacio(3)   // gap-3
	huecoZona            = tema.Espacio(1.5) // gap-1.5
	rellenoZona          = tema.Espacio(2)   // p-2
	ladoBotonCampo       = tema.Espacio(5)   // h-5 w-5
)

// botonDinamica es «Tabla dinámica» de la cinta: edita la del ancla o crea una.
func (lz *Lienzo) botonDinamica(c *editor.Contexto) *editor.BotonCinta {
	b := editor.NuevoBotonCinta(iconos.EdTabla, c.Traducir("data.pivot"), func() { lz.abrirDinamica(c) })
	b.Inerte = c.SoloLectura
	return b
}

// botonesActualizar son «Actualizar» y «Actualizar todo» del grupo de datos externos.
// Aquí solo actualizan tablas dinámicas: el ancla dentro de una, esa; todas, todas.
//
// ponytail: los datos externos todavía no llegan a este lienzo; cuando lleguen, estos
// dos botones los actualizan también, como en la web.
func (lz *Lienzo) botonesActualizar(c *editor.Contexto) [2]*editor.BotonCinta {
	hay := len(lz.Libro.Dinamicas()) > 0
	uno := editor.NuevoBotonCinta(iconos.EdActualizar, c.Traducir("data.refreshCurrent"), func() {
		s := lz.Rejilla.sel
		if d := lz.Libro.DinamicaEn(s.C, s.F); d != nil {
			c.Mandar(CmdDinamicaAct, d.Name)
		}
	})
	todos := editor.NuevoBotonCinta(iconos.EdActualizarTodo, c.Traducir("data.refreshAll"), func() { c.Mandar(CmdDinamicaAct, "") })
	uno.Inerte, todos.Inerte = !hay || c.SoloLectura, !hay || c.SoloLectura
	return [2]*editor.BotonCinta{uno, todos}
}

// borradorDinamica es `newPivotDraft`: una definición nueva con el destino dos columnas
// a la derecha del origen.
func borradorDinamica(origen string) analisis.Dinamica {
	d := analisis.Dinamica{Source: origen}
	d.Rows, d.Cols, d.Values = []analisis.CampoDinamica{}, []analisis.CampoDinamica{}, []analisis.ValorDinamica{}
	col, fila := 7, 0
	if r, ok := formula.ParseRangeRef(origen); ok {
		col, fila = min(r.C2+2, 25), r.R1
	}
	d.Anchor = Ref(col, fila)
	return d
}

// abrirDinamica abre el diálogo.
func (lz *Lienzo) abrirDinamica(c *editor.Contexto) {
	s := lz.Rejilla.sel
	f := &formDinamica{d: borradorDinamica(rangoTexto(lz.Rejilla))}
	if d := lz.Libro.DinamicaEn(s.C, s.F); d != nil {
		f.d, f.editando = copiaDinamica(*d), true
		f.d.Range = ""
	}
	cv := fyne.CurrentApp().Driver().CanvasForObject(lz)
	if cv == nil {
		return
	}
	var dlg *ui.Dialogo
	zonas := container.New(ui.Columna{})
	// Actualizar y eliminar a la izquierda en escritorio; en el teléfono, una pareja encima
	// de Cancelar | Crear (`ui.PieAcciones`, el pie de Modal.tsx).
	izquierda := 0
	if f.editando {
		izquierda = 2
	}
	pie := container.New(ui.PieAcciones{Hueco: tema.Espacio(2), Izquierda: izquierda})
	origen := ui.NuevaEntrada("").ConPropia(campoPanel)
	origen.PonerTexto(f.d.Source)
	destino := ui.NuevaEntrada("").ConPropia(campoPanel)
	destino.PonerTexto(f.d.Anchor)
	var rehacer func()
	rehacer = func() {
		zonas.Objects = lz.zonasDinamica(c, f, cv.Size().Width, rehacer)
		zonas.Refresh()
		pie.Objects = lz.pieDinamica(c, f, func() { dlg.Cerrar() })
		pie.Refresh()
		if dlg != nil {
			dlg.Recolocar()
		}
	}
	origen.AlCambiar(func(t string) { f.d.Source = t; rehacer() })
	destino.AlCambiar(func(t string) { f.d.Anchor = t; rehacer() })
	// `grid-cols-1 sm:grid-cols-2`: por debajo de `sm`, una columna (a 165 los rótulos
	// no cabían y se montaban).
	columnas := 2
	if cv.Size().Width > 0 && cv.Size().Width < ui.CorteSM {
		columnas = 1
	}
	arriba := container.New(&ui.Rejilla{Fijas: columnas, Hueco: huecoDialogoDinamica},
		ui.GrupoCampo(c.Traducir("data.pivot.source"), false, origen, "", ""),
		ui.GrupoCampo(c.Traducir("data.targetTopLeft"), false, destino, "", ""))
	titulo := c.Traducir("data.pivot")
	if f.editando {
		titulo = c.Traducir("pivot.edit")
	}
	dlg = ui.NuevoDialogo(titulo, container.New(ui.Columna{Hueco: huecoDialogoDinamica}, arriba, zonas)).Con(ui.DialogoLG, pie)
	rehacer()
	dlg.Mostrar(cv)
}

// formDinamica es el borrador del diálogo: la definición, qué campo tiene sus opciones
// abiertas («rows:0») y si se edita una que ya existe.
type formDinamica struct {
	d        analisis.Dinamica
	abierto  string
	editando bool
}

// copiaDinamica es una copia sin compartir listas: el borrador se edita sin tocar el
// libro hasta que se aplica.
func copiaDinamica(d analisis.Dinamica) analisis.Dinamica {
	b, _ := json.Marshal(d)
	var x analisis.Dinamica
	_ = json.Unmarshal(b, &x)
	if x.Rows == nil {
		x.Rows = []analisis.CampoDinamica{}
	}
	if x.Cols == nil {
		x.Cols = []analisis.CampoDinamica{}
	}
	if x.Values == nil {
		x.Values = []analisis.ValorDinamica{}
	}
	return x
}

// valido es el `valid` de la web: rangos que se leen, un origen con encabezados y
// algún campo puesto.
func (f *formDinamica) valido(cabeceras []string) bool {
	_, ok1 := formula.ParseRangeRef(f.d.Source)
	_, ok2 := formula.ParseRangeRef(f.d.Anchor)
	return ok1 && ok2 && len(cabeceras) > 0 && len(f.d.Rows)+len(f.d.Cols)+len(f.d.Values) > 0
}

// cabecerasDinamica son los nombres de los campos del origen (la primera fila) y lo leído.
func (lz *Lienzo) cabecerasDinamica(c *editor.Contexto, f *formDinamica) ([]string, [][]formula.Value) {
	rejilla := lz.Libro.valoresRango(f.d.Source, f.d.SourceSheet)
	if len(rejilla) == 0 {
		return nil, nil
	}
	out := make([]string, len(rejilla[0]))
	for i, h := range rejilla[0] {
		out[i] = formula.Display(h)
		if strings.TrimSpace(out[i]) == "" {
			out[i] = c.Traducir("common.columnN", i18n.Vars{"n": i + 1})
		}
	}
	return out, rejilla
}

// pieDinamica son las acciones: actualizar y quitar (al editar) a la izquierda, y
// cancelar y crear o aplicar a la derecha.
func (lz *Lienzo) pieDinamica(c *editor.Contexto, f *formDinamica, cerrar func()) []fyne.CanvasObject {
	cab, _ := lz.cabecerasDinamica(c, f)
	var izq []fyne.CanvasObject
	if f.editando {
		nombre := f.d.Name
		act := ui.NuevoBoton(c.Traducir("pivot.refresh"), func() { c.Mandar(CmdDinamicaAct, nombre); cerrar() }).Con(ui.Secundario, ui.TallaSM)
		act.Icono = iconos.Rotar
		izq = append(izq, act,
			ui.NuevoBoton(c.Traducir("pivot.delete"), func() { c.Mandar(CmdDinamicaQuitarla, nombre); cerrar() }).Con(ui.Peligro, ui.TallaSM))
	}
	rotulo := c.Traducir("data.pivot.create")
	if f.editando {
		rotulo = c.Traducir("action.apply")
	}
	crear := ui.NuevoBoton(rotulo, func() {
		if !f.valido(cab) {
			return
		}
		d := copiaDinamica(f.d)
		d.Source, d.Anchor = strings.ToUpper(strings.TrimSpace(d.Source)), strings.ToUpper(strings.TrimSpace(d.Anchor))
		b, _ := json.Marshal(d)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if d.Name == "" {
			delete(m, "name")
		}
		b, _ = json.Marshal(m)
		c.Mandar(CmdDinamica, string(b))
		cerrar()
	}).Con(ui.Primario, ui.TallaSM)
	crear.Inerte = !f.valido(cab)
	return append(izq, ui.NuevoBoton(c.Traducir("action.cancel"), cerrar).Con(ui.Secundario, ui.TallaSM), crear)
}

// zonasDinamica es lo que depende del origen: las cuatro zonas (o el aviso) y las tres
// casillas de los totales.
func (lz *Lienzo) zonasDinamica(c *editor.Contexto, f *formDinamica, ventana float32, rehacer func()) []fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cab, rejilla := lz.cabecerasDinamica(c, f)
	var out []fyne.CanvasObject
	if len(cab) == 0 {
		return []fyne.CanvasObject{ui.NuevoParrafo(c.Traducir("data.pivot.pickFields"), tema.TextoSM, ui.Normal, "text-subtle", v)}
	}
	var cajas []fyne.CanvasObject
	for _, z := range zonasDinamica {
		cajas = append(cajas, lz.zonaDinamica(c, f, z.id, z.clave, cab, rejilla, rehacer, v))
	}
	rej := ui.RejillaTarjetas(ventana, huecoDialogoDinamica, ui.Cortes{1, 2})
	out = append(out, container.New(&rej, cajas...))
	if len(f.d.Rows)+len(f.d.Cols)+len(f.d.Values) == 0 {
		out = append(out, ui.NuevoParrafo(c.Traducir("pivot.empty"), tema.TextoSM, ui.Normal, "text-subtle", v))
	}
	casilla := func(clave string, puesto *bool, poner func(*bool)) fyne.CanvasObject {
		ca := ui.NuevaCasilla(c.Traducir(clave), func(si bool) {
			if si {
				poner(nil)
			} else {
				no := false
				poner(&no)
			}
			rehacer()
		})
		ca.Marcada = puesto == nil || *puesto
		return ca
	}
	// `flex flex-wrap gap-x-4 gap-y-1.5`: en un teléfono no caben en una fila.
	envoltura := ui.RejillaTarjetas(ventana, tema.Espacio(1.5), ui.Cortes{1, 3})
	out = append(out, container.New(&envoltura,
		casilla("pivot.grandRows", f.d.GrandRows, func(b *bool) { f.d.GrandRows = b }),
		casilla("pivot.grandCols", f.d.GrandCols, func(b *bool) { f.d.GrandCols = b }),
		casilla("pivot.subtotals", f.d.Subtotals, func(b *bool) { f.d.Subtotals = b }),
	))
	return []fyne.CanvasObject{container.New(ui.Columna{Hueco: huecoDialogoDinamica}, out...)}
}

// camposDe son los campos de una zona como referencias de campo (los valores, por su
// columna), y ponerCampos los vuelve a dejar.
func (f *formDinamica) camposDe(z string) []analisis.CampoDinamica {
	switch z {
	case "filters":
		return f.d.Filters
	case "cols":
		return f.d.Cols
	case "rows":
		return f.d.Rows
	}
	return nil
}

func (f *formDinamica) ponerCampos(z string, l []analisis.CampoDinamica) {
	switch z {
	case "filters":
		f.d.Filters = l
	case "cols":
		f.d.Cols = l
	case "rows":
		f.d.Rows = l
	}
}

// zonaDinamica es una de las cuatro zonas: su título, sus campos y «Añadir campo».
func (lz *Lienzo) zonaDinamica(c *editor.Contexto, f *formDinamica, z, clave string, cab []string,
	rejilla [][]formula.Value, rehacer func(), v tema.Variante,
) fyne.CanvasObject {
	filas := []fyne.CanvasObject{ui.NuevoParrafo(c.Traducir(clave), tema.TextoXS, ui.Medio, "text", v)}
	nombreCampo := func(campo int) string {
		if campo < len(cab) {
			return cab[campo]
		}
		return c.Traducir("common.columnN", i18n.Vars{"n": campo + 1})
	}
	n := len(f.d.Values)
	if z != "values" {
		n = len(f.camposDe(z))
	}
	for i := range n {
		id := z + ":" + strconv.Itoa(i)
		var nombre string
		var mover func(dir int)
		var quitar func()
		var opciones fyne.CanvasObject
		if z == "values" {
			vr := f.d.Values[i]
			nombre = vr.Name
			if nombre == "" {
				nombre = c.Traducir("pivot.valueOf", i18n.Vars{"fn": c.Traducir(fnDinamica[vr.Fn]), "field": nombreCampo(vr.Field)})
			}
			mover = func(dir int) { f.d.Values = moverEn(f.d.Values, i, dir) }
			quitar = func() { f.d.Values = slices.Delete(slices.Clone(f.d.Values), i, i+1) }
			if f.abierto == id {
				opciones = lz.opcionesValor(c, f, i, rehacer)
			}
		} else {
			campos := f.camposDe(z)
			nombre = nombreCampo(campos[i].Field)
			mover = func(dir int) { f.ponerCampos(z, moverEn(f.camposDe(z), i, dir)) }
			quitar = func() { f.ponerCampos(z, slices.Delete(slices.Clone(f.camposDe(z)), i, i+1)) }
			if f.abierto == id {
				opciones = lz.opcionesCampo(c, f, z, i, rejilla, rehacer)
			}
		}
		filas = append(filas, filaCampo(c, nombre, i, n, func() {
			if f.abierto == id {
				f.abierto = ""
			} else {
				f.abierto = id
			}
			rehacer()
		}, func(dir int) { mover(dir); f.abierto = ""; rehacer() }, func() { quitar(); f.abierto = ""; rehacer() }, opciones, v))
	}
	anadir := ui.NuevoSelector(append([]string{c.Traducir("pivot.addField")}, cab...), func(k int) {
		if k <= 0 {
			return
		}
		if z == "values" {
			f.d.Values = append(slices.Clone(f.d.Values), analisis.ValorDinamica{Field: k - 1, Fn: "sum"})
		} else {
			f.ponerCampos(z, append(slices.Clone(f.camposDe(z)), analisis.CampoDinamica{Field: k - 1}))
		}
		rehacer()
	}).Con(ui.CampoSM)
	filas = append(filas, anadir)
	return cajaZona(container.New(ui.Columna{Hueco: huecoZona}, filas...), v)
}

// moverEn mueve el elemento i una posición arriba (−1) o abajo (+1).
func moverEn[T any](l []T, i, dir int) []T {
	out := slices.Clone(l)
	j := i + dir
	if j < 0 || j >= len(out) {
		return out
	}
	out[i], out[j] = out[j], out[i]
	return out
}

// filaCampo es un campo de una zona: `bg-bg-subtle rounded-sm px-1.5 py-1` con su nombre
// (que abre sus opciones), sus flechas y el aspa; y debajo, si están abiertas, las
// opciones.
func filaCampo(c *editor.Contexto, nombre string, i, n int, abrir func(), mover func(int), quitar func(),
	opciones fyne.CanvasObject, v tema.Variante,
) fyne.CanvasObject {
	boton := func(ic iconos.Icono, rotulo string, inerte bool, fn func()) fyne.CanvasObject {
		b := ui.NuevoBotonIcono(ic, rotulo, fn)
		b.Lado, b.TamIcono, b.FondoEncima = ladoBotonCampo, iconos.TallaXS, "selected"
		b.Inerte = inerte
		return b
	}
	titulo := ui.NuevoEnlace(nombre, tema.TextoXS, ui.Normal, "text", abrir)
	titulo.Boton, titulo.Nombre = true, c.Traducir("pivot.field.options", i18n.Vars{"field": nombre})
	cabeza := container.New(fila{hueco: tema.Espacio(1), elastico: 0},
		container.New(anchoCero{}, titulo), // `truncate`: el nombre se corta, no empuja
		boton(iconos.ChevronArriba, c.Traducir("pivot.field.up"), i == 0, func() { mover(-1) }),
		boton(iconos.ChevronAbajo, c.Traducir("pivot.field.down"), i == n-1, func() { mover(1) }),
		boton(iconos.Cerrar, c.Traducir("pivot.field.remove", i18n.Vars{"field": nombre}), false, quitar),
	)
	dentro := []fyne.CanvasObject{cabeza}
	if opciones != nil {
		dentro = append(dentro, opciones)
	}
	fondo := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("bg-subtle", v), tema.RadioSM)
	return container.New(cajaCon{c: fondo, x: tema.Espacio(1.5), y: tema.Espacio(1)},
		append(fondo.Objetos(), container.New(ui.Columna{Hueco: huecoZona}, dentro...))...)
}

// cajaZona es la `section` de una zona: `border-border rounded-md border p-2`.
func cajaZona(dentro fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	c := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	return container.New(cajaCon{c: c, x: rellenoZona + tema.Borde, y: rellenoZona + tema.Borde}, append(c.Objetos(), dentro)...)
}

// cajaCon pone una ui.Caja detrás del último hijo, con ese relleno.
type cajaCon struct {
	c    *ui.Caja
	x, y float32
}

func (l cajaCon) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*l.x, m.Height+2*l.y)
}

func (l cajaCon) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	l.c.Redimensionar(t)
	d := objs[len(objs)-1]
	d.Move(fyne.NewPos(l.x, l.y))
	d.Resize(fyne.NewSize(t.Width-2*l.x, t.Height-2*l.y))
}

// etiquetado es `Lbl`: el rótulo `sytue-meta-dense` encima de su control.
func etiquetado(rotulo string, o fyne.CanvasObject) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, ui.NuevoParrafo(rotulo, tema.Texto2XS, ui.Normal, "text-subtle", v), o)
}

// selectorDe es un `<Select size="sm">` de opciones (valor, clave) con la elegida.
func selectorDe(c *editor.Contexto, valores, claves []string, actual string, elegir func(string)) *ui.Selector {
	rotulos := make([]string, len(claves))
	for i, k := range claves {
		rotulos[i] = c.Traducir(k)
	}
	s := ui.NuevoSelector(rotulos, func(i int) { elegir(valores[i]) }).Con(ui.CampoSM)
	s.Elegida = max(0, slices.Index(valores, actual))
	return s
}

// opcionesCampo son las de un campo de filas, columnas o filtros: agrupar, el orden
// (salvo en filtros), el intervalo si se agrupan números, y los elementos que se ven.
func (lz *Lienzo) opcionesCampo(c *editor.Contexto, f *formDinamica, z string, i int, rejilla [][]formula.Value, rehacer func()) fyne.CanvasObject {
	campo := f.camposDe(z)[i]
	poner := func(nuevo analisis.CampoDinamica) {
		l := slices.Clone(f.camposDe(z))
		l[i] = nuevo
		f.ponerCampos(z, l)
	}
	grupo := "none"
	switch {
	case campo.Bin != nil:
		grupo = "range"
	case campo.Date != "":
		grupo = campo.Date
	}
	claves := make([]string, len(gruposDinamica))
	for k, g := range gruposDinamica {
		claves[k] = "pivot.group." + g
	}
	arriba := []fyne.CanvasObject{etiquetado(c.Traducir("pivot.group"), selectorDe(c, gruposDinamica, claves, grupo, func(g string) {
		campo.Date, campo.Bin = "", nil
		switch {
		case g == "range":
			campo.Bin = &analisis.Intervalo{Start: 0, End: 100, Step: 10}
		case g != "none":
			campo.Date = g
		}
		poner(campo)
		rehacer()
	}))}
	if z != "filters" {
		orden := "asc"
		if campo.Order == "desc" {
			orden = "desc"
		}
		arriba = append(arriba, etiquetado(c.Traducir("pivot.order"), selectorDe(c, []string{"asc", "desc"},
			[]string{"pivot.order.asc", "pivot.order.desc"}, orden, func(o string) {
				campo.Order = map[bool]string{true: "desc"}[o == "desc"]
				poner(campo)
				rehacer()
			})))
	}
	filas := []fyne.CanvasObject{container.New(&ui.Rejilla{Fijas: 2, Hueco: huecoZona}, arriba...)}
	if campo.Bin != nil {
		trio := make([]fyne.CanvasObject, 0, 3)
		for k, clave := range []string{"pivot.bin.start", "pivot.bin.end", "pivot.bin.step"} {
			e := ui.NuevaEntrada("").ConPropia(campoPanel)
			e.PonerTexto(formula.FormatNumber([]float64{campo.Bin.Start, campo.Bin.End, campo.Bin.Step}[k]))
			e.AlCambiar(func(t string) {
				x, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
				if err != nil || (k == 2 && x <= 0) {
					return
				}
				b := *f.camposDe(z)[i].Bin
				switch k {
				case 0:
					b.Start = x
				case 1:
					b.End = x
				default:
					b.Step = x
				}
				actual := f.camposDe(z)[i]
				actual.Bin = &b
				poner(actual)
			})
			trio = append(trio, etiquetado(c.Traducir(clave), e))
		}
		filas = append(filas, container.New(&ui.Rejilla{Fijas: 3, Hueco: huecoZona}, trio...))
	}
	claves2, rotulos := analisis.ElementosDinamica(rejilla, campo, rotulosDinamica(func(k string, x i18n.Vars) string { return c.Traducir(k, x) }))
	casillas := make([]fyne.CanvasObject, 0, len(claves2))
	for k, clave := range claves2 {
		ca := ui.NuevaCasilla(rotulos[k], func(si bool) {
			actual := f.camposDe(z)[i]
			oculto := slices.DeleteFunc(slices.Clone(actual.Hidden), func(x string) bool { return x == clave })
			if !si {
				oculto = append(oculto, clave)
			}
			if len(oculto) == 0 {
				oculto = nil
			}
			actual.Hidden = oculto
			poner(actual)
		})
		ca.Marcada = !slices.Contains(campo.Hidden, clave)
		casillas = append(casillas, ca)
	}
	lista := topeAlto(tema.Espacio(28)-2*(tema.Borde+tema.Espacio(1.5)), container.New(ui.Columna{}, casillas...))
	filas = append(filas, etiquetado(c.Traducir("pivot.items"), recuadro(lista)))
	return container.New(ui.Columna{Hueco: huecoZona}, filas...)
}

// opcionesValor son las de un campo de valores: cómo se resume, cómo se muestra, su
// formato y su nombre.
func (lz *Lienzo) opcionesValor(c *editor.Contexto, f *formDinamica, i int, rehacer func()) fyne.CanvasObject {
	vr := f.d.Values[i]
	poner := func(x analisis.ValorDinamica) {
		l := slices.Clone(f.d.Values)
		l[i] = x
		f.d.Values = l
	}
	claves := make([]string, len(analisis.FuncionesDinamica))
	for k, fn := range analisis.FuncionesDinamica {
		claves[k] = fnDinamica[fn]
	}
	mostrar := make([]string, len(analisis.MostrarDinamica))
	for k, s := range analisis.MostrarDinamica {
		mostrar[k] = mostrarDinamica[s]
	}
	show := vr.Show
	if show == "" {
		show = "normal"
	}
	fmtID := "general"
	if fm := formatoValor(vr); fm != nil {
		fmtID = fm.ID
	}
	ids, cls := make([]string, len(formatosDinamica)), make([]string, len(formatosDinamica))
	for k, x := range formatosDinamica {
		ids[k], cls[k] = x.id, x.clave
	}
	nombre := ui.NuevaEntrada("").ConPropia(campoPanel)
	nombre.PonerTexto(vr.Name)
	nombre.AlCambiar(func(t string) {
		x := f.d.Values[i]
		x.Name = map[bool]string{false: t}[strings.TrimSpace(t) == ""]
		poner(x)
	})
	return container.New(&ui.Rejilla{Fijas: 2, Hueco: huecoZona},
		etiquetado(c.Traducir("pivot.fn"), selectorDe(c, analisis.FuncionesDinamica, claves, vr.Fn, func(fn string) {
			x := f.d.Values[i]
			x.Fn = fn
			poner(x)
			rehacer()
		})),
		etiquetado(c.Traducir("pivot.show"), selectorDe(c, analisis.MostrarDinamica, mostrar, show, func(s string) {
			x := f.d.Values[i]
			x.Show = map[bool]string{false: s}[s == "normal"]
			poner(x)
			rehacer()
		})),
		etiquetado(c.Traducir("pivot.format"), selectorDe(c, ids, cls, fmtID, func(id string) {
			x := f.d.Values[i]
			x.Fmt = nil
			if id != "general" {
				x.Fmt = FormatoNum{ID: id}
			}
			poner(x)
			rehacer()
		})),
		etiquetado(c.Traducir("common.name"), nombre),
	)
}

// anchoCero deja que su hijo encoja hasta nada: es el `min-w-0` que permite a un
// `truncate` cortar en vez de desbordar.
type anchoCero struct{}

func (anchoCero) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, objs[0].MinSize().Height)
}

func (anchoCero) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}
