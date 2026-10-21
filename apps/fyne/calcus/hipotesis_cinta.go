// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"math"
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

// El grupo «Análisis de hipótesis» de Datos: `GoalSeekMenu`, `SolverMenu`,
// `DataTableMenu` y `ScenarioMenu` de Datos.tsx. Cada uno es un emergente que se rehace
// al teclear —lo que depende de lo escrito, como en la web, se calcula en cada tecla—.

var (
	anchoPanelObjetivo = tema.Espacio(64) // w-64
	anchoPanelAnalisis = tema.Espacio(72) // w-72
	altoListaAnalisis  = tema.Espacio(24) // max-h-24
)

// topeVariablesSolver es `SOLVER_MAX_VARS`.
const topeVariablesSolver = 12

// motivosObjetivo y motivosSolver son PROBE_REASONS y SOLVER_REASONS.
var (
	motivosObjetivo = map[string]string{
		"bad-target": "data.goalSeek.err.badTarget", "bad-var": "data.goalSeek.err.badVar",
		"target-not-formula": "data.goalSeek.err.targetNotFormula", "var-is-formula": "data.goalSeek.err.varIsFormula",
	}
	motivosSolver = map[string]string{
		"bad-target": "data.solver.err.badTarget", "target-not-formula": "data.solver.err.targetNotFormula",
		"no-vars": "data.solver.err.noVars", "bad-var": "data.solver.err.badVar",
		"var-is-formula": "data.solver.err.varIsFormula", "bad-constraint": "data.solver.err.badConstraint",
	}
)

// grupoHipotesis son los cuatro botones del grupo.
func (lz *Lienzo) grupoHipotesis(c *editor.Contexto) []fyne.CanvasObject {
	nuevo := func(i iconos.Ed, clave string, abrir func(*editor.BotonCinta)) *editor.BotonCinta {
		var b *editor.BotonCinta
		b = editor.NuevoBotonCinta(i, c.Traducir(clave), func() { abrir(b) })
		b.Inerte = c.SoloLectura
		return b
	}
	return []fyne.CanvasObject{
		nuevo(iconos.EdObjetivo, "data.goalSeek", func(b *editor.BotonCinta) { lz.abrirObjetivo(c, b) }),
		nuevo(iconos.EdSolver, "data.solver", func(b *editor.BotonCinta) { lz.abrirSolver(c, b) }),
		nuevo(iconos.EdTablaDatos, "data.dataTable", func(b *editor.BotonCinta) { lz.abrirTablaDatos(c, b) }),
		nuevo(iconos.EdEscenarios, "data.scenarios", func(b *editor.BotonCinta) { lz.abrirEscenarios(c, b) }),
	}
}

// entradaPanel es el `inputCls` con su texto, lo que hace al cambiar y, si hay, Intro.
func entradaPanel(ph, texto string, alCambiar func(string), alIntro func()) *ui.Entrada {
	e := ui.NuevaEntrada(ph).ConPropia(campoPanel)
	e.PonerTexto(texto)
	e.AlCambiar(alCambiar)
	if alIntro != nil {
		e.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
				return false
			}
			alIntro()
			return true
		}
	}
	return e
}

// ancla es `anchorCell`: la primera celda de la selección.
func ancla(r *Rejilla) string { return Ref(r.sel.C, r.sel.F) }

// rotuloPanel es el título del emergente: `text-text pb-1 text-xs font-medium`.
func rotuloPanel(texto string, v tema.Variante) fyne.CanvasObject {
	return conPie(ui.NuevoParrafo(texto, tema.TextoXS, ui.Medio, "text", v), tema.Espacio(1))
}

// errorPanel es `text-danger-text text-2xs pt-2`.
func errorPanel(texto string, v tema.Variante) fyne.CanvasObject {
	return ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, "danger-text", v)
}

// emergenteVivo es un emergente que se rehace entero al cambiar su estado. Los campos
// de texto se crean UNA vez, fuera de `armar`, y se vuelven a poner: así rehacer al
// teclear no les quita el foco.
type emergenteVivo struct {
	em     *ui.Emergente
	cuerpo *fyne.Container
	armar  func() []fyne.CanvasObject
}

func (e *emergenteVivo) rehacer() {
	e.cuerpo.Objects = e.armar()
	e.cuerpo.Refresh()
	if e.em != nil {
		e.em.Recolocar()
	}
}

// abrirVivo abre un emergente de ese ancho cuyo contenido da `armar`.
func abrirVivo(disparador *editor.BotonCinta, ancho float32, armar func(e *emergenteVivo) []fyne.CanvasObject) *emergenteVivo {
	e := &emergenteVivo{cuerpo: container.New(ui.Columna{})}
	e.armar = func() []fyne.CanvasObject { return armar(e) }
	e.cuerpo.Objects = e.armar()
	e.em = abrirEmergente(disparador, panel{ancho: ancho, filas: []fyne.CanvasObject{e.cuerpo}}.objeto())
	return e
}

// ---- Buscar objetivo ----

// abrirObjetivo es `GoalSeekMenu`: la celda, el valor y la celda que cambia; «Aceptar»
// busca en un motor desechable y enseña el resultado para aplicarlo o no.
func (lz *Lienzo) abrirObjetivo(c *editor.Contexto, disparador *editor.BotonCinta) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	definir, valor, cambiando, fallo := ancla(lz.Rejilla), "0", "", ""
	var res *analisis.ResultadoObjetivo
	var vivo *emergenteVivo
	resolver := func() {
		fallo, res = "", nil
		n, ok := numeroEsValor(valor)
		if !ok {
			fallo = c.Traducir("data.goalSeek.err.badValue")
			vivo.rehacer()
			return
		}
		f, actual, motivo := lz.Libro.SondaObjetivo(strings.TrimSpace(definir), strings.TrimSpace(cambiando))
		if motivo != "" {
			fallo = c.Traducir(motivosObjetivo[motivo])
			vivo.rehacer()
			return
		}
		r := analisis.BuscarObjetivo(f, n, analisis.OpcionesObjetivo{X0: actual})
		res = &r
		vivo.rehacer()
	}
	puede := func() bool {
		_, okv := numeroEsValor(valor)
		_, ok1 := celdaUnica(definir)
		_, ok2 := celdaUnica(cambiando)
		return ok1 && ok2 && okv
	}
	intro := func() {
		if puede() {
			resolver()
		}
	}
	cambia := func(p *string) func(string) { return func(s string) { *p = s; vivo.rehacer() } }
	eDef := entradaPanel("B5", definir, cambia(&definir), nil)
	eVal := entradaPanel("", valor, cambia(&valor), intro)
	eCam := entradaPanel("B3", cambiando, cambia(&cambiando), intro)
	vivo = abrirVivo(disparador, anchoPanelObjetivo, func(e *emergenteVivo) []fyne.CanvasObject {
		filas := []fyne.CanvasObject{
			rotuloPanel(c.Traducir("data.goalSeek"), v),
			conPie(conEtiqueta(c.Traducir("data.goalSeek.setCell"), eDef), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.withValue"), eVal), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.goalSeek.byCell"), eCam), tema.Espacio(2)),
		}
		ok := botonPop(c.Traducir("action.ok"), false, resolver)
		ok.Inerte = !puede()
		filas = append(filas, anchoEntero(ok))
		if fallo != "" {
			filas = append(filas, conArriba(errorPanel(fallo, v), tema.Espacio(2)))
		}
		if res != nil {
			celda := strings.ToUpper(strings.TrimSpace(cambiando))
			titulo := c.Traducir("data.goalSeek.approx")
			if res.Resuelto {
				titulo = c.Traducir("data.goalSeek.found", i18n.Vars{"cell": celda})
			}
			bloque := []fyne.CanvasObject{
				ui.NuevoParrafo(titulo, tema.Texto2XS, ui.Normal, "text", v),
				ui.NuevoParrafo(celda+" = "+formula.Display(res.Valor), tema.TextoXS, ui.Medio, "text", v),
			}
			if !res.Resuelto {
				bloque = append(bloque, meta(c.Traducir("data.goalSeek.residual", i18n.Vars{
					"error": formula.Display(res.Error), "iterations": res.Iteraciones,
				}), v))
			}
			valorHallado := res.Valor
			bloque = append(bloque, conArriba(rejillaBotones(2,
				botonPop(c.Traducir("action.apply"), false, func() {
					b, _ := json.Marshal(map[string]any{"cell": strings.TrimSpace(cambiando), "value": valorHallado})
					c.Mandar(CmdObjetivo, string(b))
					e.em.Cerrar()
				}),
				botonPop(c.Traducir("action.cancel"), false, func() { e.em.Cerrar() }),
			), tema.Espacio(2)))
			filas = append(filas, resultadoPanel(container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, bloque...), v))
		}
		return filas
	})
}

// conArriba añade el `pt-*` de un bloque.
func conArriba(o fyne.CanvasObject, pt float32) fyne.CanvasObject {
	return container.New(margenArriba{pt}, o)
}

type margenArriba struct{ m float32 }

func (m margenArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	s := objs[0].MinSize()
	return fyne.NewSize(s.Width, s.Height+m.m)
}

func (m margenArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, m.m))
	objs[0].Resize(fyne.NewSize(t.Width, t.Height-m.m))
}

// resultadoPanel es `border-border mt-2 border-t pt-2`: el bloque del resultado.
func resultadoPanel(o fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	return conArriba(container.New(bordeArriba{}, cajaDe(ui.NuevaCaja(tema.Color("border", v), tema.Color("border", v), 0)), o), tema.Espacio(2))
}

// ---- Solver ----

// filaSolver es `SolverRow`.
type filaSolver struct{ izq, rel, der string }

// trozosRefs es `splitRefs`: las referencias de un campo, separadas por comas, puntos y
// comas o espacios.
func trozosRefs(texto string) []string {
	return strings.FieldsFunc(texto, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' })
}

// variablesSolver es `expandVarRefs`: las celdas (sueltas o de rangos), sin repetir y
// hasta el tope.
func variablesSolver(texto string) []string {
	var out []string
	visto := map[string]bool{}
	for _, tok := range trozosRefs(texto) {
		r, ok := formula.ParseRangeRef(strings.ToUpper(tok))
		if !ok {
			continue
		}
		for f := r.R1; f <= r.R2; f++ {
			for col := r.C1; col <= r.C2; col++ {
				ref := string(rune('A'+min(max(col, 0), 25))) + strconv.Itoa(f+1)
				if !visto[ref] {
					visto[ref] = true
					out = append(out, ref)
					if len(out) >= topeVariablesSolver {
						return out
					}
				}
			}
		}
	}
	return out
}

// abrirSolver es `SolverMenu`.
func (lz *Lienzo) abrirSolver(c *editor.Contexto, disparador *editor.BotonCinta) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	objetivo, tipo, meta0, vars, fallo := ancla(lz.Rejilla), "max", "0", "", ""
	var filas []filaSolver
	var res *analisis.ResultadoSolver
	var celdas []string
	var vivo *emergenteVivo
	puede := func() bool {
		_, ok := celdaUnica(objetivo)
		_, okv := numeroEsValor(meta0)
		return ok && len(variablesSolver(vars)) > 0 && (tipo != "value" || okv)
	}
	resolver := func() {
		fallo, res = "", nil
		if !puede() {
			return
		}
		var activas []RestriccionTexto
		var rels []string
		for _, f := range filas {
			if strings.TrimSpace(f.izq) != "" && strings.TrimSpace(f.der) != "" {
				activas = append(activas, RestriccionTexto{Izq: strings.TrimSpace(f.izq), Rel: f.rel, Der: strings.TrimSpace(f.der)})
				rels = append(rels, f.rel)
			}
		}
		celdas = variablesSolver(vars)
		evaluar, actual, motivo := lz.Libro.SondaSolver(strings.TrimSpace(objetivo), celdas, activas)
		if motivo != "" {
			fallo = c.Traducir(motivosSolver[motivo])
			vivo.rehacer()
			return
		}
		n, _ := numeroEsValor(meta0)
		r := analisis.Resolver(evaluar, actual, analisis.MetaSolver{Tipo: tipo, Valor: n}, rels)
		res = &r
		vivo.rehacer()
	}
	cambia := func(p *string) func(string) { return func(s string) { *p = s; vivo.rehacer() } }
	eObj := entradaPanel("B5", objetivo, cambia(&objetivo), nil)
	eMeta := entradaPanel("", meta0, cambia(&meta0), nil)
	eVars := entradaPanel(c.Traducir("data.solver.varsPlaceholder"), vars, cambia(&vars), nil)
	var campos [][2]*ui.Entrada // los de cada restricción, a la par de `filas`
	vivo = abrirVivo(disparador, anchoPanelAnalisis, func(e *emergenteVivo) []fyne.CanvasObject {
		out := []fyne.CanvasObject{
			rotuloPanel(c.Traducir("data.solver"), v),
			conPie(conEtiqueta(c.Traducir("data.targetCell"), eObj), tema.Espacio(1.5)),
		}
		metas := make([]fyne.CanvasObject, 0, 3)
		for _, k := range []struct{ valor, clave string }{{"max", "solver.goal.max"}, {"min", "solver.goal.min"}, {"value", "solver.goal.value"}} {
			metas = append(metas, botonPop(c.Traducir(k.clave), tipo == k.valor, func() { tipo = k.valor; e.rehacer() }))
		}
		fila := container.New(&ui.Rejilla{Fijas: len(metas), Hueco: tema.Espacio(1)}, metas...)
		var objetivoFila fyne.CanvasObject = fila
		if tipo == "value" {
			objetivoFila = container.NewBorder(nil, nil, nil, container.New(ladoFijoAncho{tema.Espacio(16)}, eMeta), fila)
		}
		out = append(out,
			conPie(container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, meta(c.Traducir("data.solver.goal"), v), objetivoFila), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.solver.vars"), eVars), tema.Espacio(1.5)))
		// `text-text-muted text-2xs inline-flex h-5 … px-1 hover:bg-bg-subtle hover:text-text`
		// con el «+» de 14: no es POP_BTN ni `Button`, es el botón de texto.
		anadir := ui.NuevoBoton(c.Traducir("action.add"), func() {
			i := len(filas)
			filas = append(filas, filaSolver{rel: "lte"})
			campos = append(campos, [2]*ui.Entrada{
				entradaPanel("C3", "", func(s string) { filas[i].izq = s }, nil),
				entradaPanel(c.Traducir("data.constraint.limitPlaceholder"), "", func(s string) { filas[i].der = s }, nil),
			})
			e.rehacer()
		}).Con(ui.Fantasma, ui.TallaTexto).ConIcono(iconos.Mas, 14)
		anadir.Atenuado = true
		out = append(out, container.NewBorder(nil, nil, meta(c.Traducir("data.constraints"), v), anadir))
		if len(filas) == 0 {
			out = append(out, conPie(meta(c.Traducir("data.solver.noConstraints"), v), tema.Espacio(1.5)))
		} else {
			lista := make([]fyne.CanvasObject, 0, len(filas))
			for i := range filas {
				lista = append(lista, lz.filaRestriccion(c, &filas, &campos, i, e))
			}
			out = append(out, conPie(container.New(ui.Columna{Hueco: tema.Espacio(1)}, lista...), tema.Espacio(1.5)))
		}
		resolverB := botonPop(c.Traducir("data.solver.solve"), false, resolver)
		resolverB.Inerte = !puede()
		out = append(out, anchoEntero(resolverB))
		if fallo != "" {
			out = append(out, conArriba(errorPanel(fallo, v), tema.Espacio(2)))
		}
		if res != nil {
			out = append(out, resultadoPanel(lz.resultadoSolver(c, *res, celdas, objetivo, e, v), v))
		}
		return out
	})
}

// filaRestriccion es una fila de restricción: la celda, la relación, el límite y quitar.
// Los campos de cada fila se guardan aparte (`campos`) para no perder el foco al rehacer;
// quitar una fila rehace los de las de debajo, que cambian de índice.
func (lz *Lienzo) filaRestriccion(c *editor.Contexto, filas *[]filaSolver, campos *[][2]*ui.Entrada, i int, e *emergenteVivo) fyne.CanvasObject {
	f := (*filas)[i]
	rel := editor.NuevoSelectorCinta(c.Traducir("data.relation"), 40,
		[]editor.Opcion{{Valor: "lte", Rotulo: "≤"}, {Valor: "gte", Rotulo: "≥"}, {Valor: "eq", Rotulo: "="}},
		f.rel, func(s string) { (*filas)[i].rel = s })
	quitar := editor.NuevoBotonCinta(iconos.EdPapelera, c.Traducir("data.constraint.remove"), func() {
		*filas = append((*filas)[:i:i], (*filas)[i+1:]...)
		*campos = (*campos)[:i]
		for j := i; j < len(*filas); j++ {
			*campos = append(*campos, [2]*ui.Entrada{
				entradaPanel("C3", (*filas)[j].izq, func(s string) { (*filas)[j].izq = s }, nil),
				entradaPanel(c.Traducir("data.constraint.limitPlaceholder"), (*filas)[j].der, func(s string) { (*filas)[j].der = s }, nil),
			})
		}
		e.rehacer()
	})
	izq, der := (*campos)[i][0], (*campos)[i][1]
	return container.New(&ui.Rejilla{Fijas: 2, Hueco: tema.Espacio(1)},
		container.NewBorder(nil, nil, nil, rel, izq), container.NewBorder(nil, nil, nil, container.New(ladoFijo{ladoBotonRegla}, quitar), der))
}

// resultadoSolver es el bloque del resultado: factible o no, el objetivo, las
// variables y aplicar o cancelar.
func (lz *Lienzo) resultadoSolver(c *editor.Contexto, r analisis.ResultadoSolver, celdas []string, objetivo string,
	e *emergenteVivo, v tema.Variante,
) fyne.CanvasObject {
	titulo := c.Traducir("data.solver.infeasible")
	if r.Factible {
		titulo = c.Traducir("data.solver.feasible")
	}
	obj := "—"
	if !math.IsNaN(r.Objetivo) {
		obj = formula.Display(r.Objetivo)
	}
	bloque := []fyne.CanvasObject{
		ui.NuevoParrafo(titulo, tema.Texto2XS, ui.Normal, "text", v),
		ui.NuevoParrafo(strings.ToUpper(strings.TrimSpace(objetivo))+" = "+obj, tema.TextoXS, ui.Medio, "text", v),
	}
	lista := make([]fyne.CanvasObject, 0, len(celdas))
	for i, ref := range celdas {
		x := 0.0
		if i < len(r.Valores) {
			x = r.Valores[i]
		}
		lista = append(lista, container.NewBorder(nil, nil, meta(ref, v), ui.NuevoParrafo(formula.Display(x), tema.Texto2XS, ui.Normal, "text", v)))
	}
	bloque = append(bloque, topeAlto(altoListaAnalisis, container.New(ui.Columna{}, lista...)))
	if !r.Factible && r.Violacion > 0 && !math.IsInf(r.Violacion, 0) {
		bloque = append(bloque, meta(c.Traducir("data.solver.violation", i18n.Vars{
			"max": formula.Display(r.Violacion), "evaluations": r.Iteraciones,
		}), v))
	}
	bloque = append(bloque, conArriba(rejillaBotones(2,
		botonPop(c.Traducir("action.apply"), false, func() {
			vals := map[string]float64{}
			for i, ref := range celdas {
				if i < len(r.Valores) {
					vals[ref] = r.Valores[i]
				}
			}
			b, _ := json.Marshal(map[string]any{"values": vals})
			c.Mandar(CmdSolver, string(b))
			e.em.Cerrar()
		}),
		botonPop(c.Traducir("action.cancel"), false, func() { e.em.Cerrar() }),
	), tema.Espacio(2)))
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, bloque...)
}

// ladoFijoAncho da a su hijo un ancho fijo (`w-16`) y su alto.
type ladoFijoAncho struct{ ancho float32 }

func (l ladoFijoAncho) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.ancho, objs[0].MinSize().Height)
}

func (l ladoFijoAncho) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// ---- Tabla de datos ----

// abrirTablaDatos es `DataTableMenu`.
func (lz *Lienzo) abrirTablaDatos(c *editor.Contexto, disparador *editor.BotonCinta) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rango, fila, col := rangoTexto(lz.Rejilla), "", ""
	unaOVacia := func(s string) bool { _, ok := celdaUnica(s); return strings.TrimSpace(s) == "" || ok }
	puede := func() bool {
		r, ok := formula.ParseRangeRef(rango)
		_, okf := celdaUnica(fila)
		_, okc := celdaUnica(col)
		return ok && r.C2 > r.C1 && r.R2 > r.R1 && unaOVacia(fila) && unaOVacia(col) && (okf || okc)
	}
	var vivo *emergenteVivo
	aplicar := func() {
		if !puede() {
			return
		}
		req := map[string]any{"range": strings.ToUpper(strings.TrimSpace(rango))}
		if strings.TrimSpace(fila) != "" {
			req["rowInput"] = strings.ToUpper(strings.TrimSpace(fila))
		}
		if strings.TrimSpace(col) != "" {
			req["colInput"] = strings.ToUpper(strings.TrimSpace(col))
		}
		b, _ := json.Marshal(req)
		c.Mandar(CmdTablaDatos, string(b))
		vivo.em.Cerrar()
	}
	cambiar := func(p *string) func(string) { return func(s string) { *p = s; vivo.rehacer() } }
	eRango := entradaPanel("A1:D5", rango, cambiar(&rango), aplicar)
	eFila := entradaPanel(c.Traducir("data.dataTable.optionalCell", i18n.Vars{"example": "B1"}), fila, cambiar(&fila), aplicar)
	eCol := entradaPanel(c.Traducir("data.dataTable.optionalCell", i18n.Vars{"example": "B2"}), col, cambiar(&col), aplicar)
	vivo = abrirVivo(disparador, anchoPanelAnalisis, func(*emergenteVivo) []fyne.CanvasObject {
		out := []fyne.CanvasObject{
			rotuloPanel(c.Traducir("data.dataTable"), v),
			conPie(meta(c.Traducir("data.dataTable.explain"), v), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.dataTable.range"), eRango), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.dataTable.rowInput"), eFila), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.dataTable.colInput"), eCol), tema.Espacio(2)),
		}
		if !unaOVacia(fila) || !unaOVacia(col) {
			out = append(out, conPie(errorPanel(c.Traducir("data.dataTable.singleCell"), v), tema.Espacio(1.5)))
		}
		b := botonPop(c.Traducir("data.dataTable.fill"), false, aplicar)
		b.Inerte = !puede()
		return append(out, anchoEntero(b))
	})
}

// ---- Escenarios ----

// abrirEscenarios es `ScenarioMenu`: la lista, uno nuevo y el informe resumen.
func (lz *Lienzo) abrirEscenarios(c *editor.Contexto, disparador *editor.BotonCinta) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	sel := rangoTexto(lz.Rejilla)
	nombre, rango, resultados, destino := "", sel, "", ""
	if r, ok := formula.ParseRangeRef(sel); ok {
		destino = Ref(min(r.C2+2, 25), r.R1)
	}
	var vivo *emergenteVivo
	var crear func()
	cambiar := func(p *string) func(string) { return func(s string) { *p = s; vivo.rehacer() } }
	eNombre := entradaPanel(c.Traducir("data.scenario.optimistic"), nombre, cambiar(&nombre), func() { crear() })
	eRango := entradaPanel("B3:B5", rango, cambiar(&rango), func() { crear() })
	eRes := entradaPanel("B6, B7", resultados, func(s string) { resultados = s }, nil)
	eDest := entradaPanel("F1", destino, cambiar(&destino), nil)
	vivo = abrirVivo(disparador, anchoPanelAnalisis, func(e *emergenteVivo) []fyne.CanvasObject {
		esc := lz.Libro.Escenarios()
		r, okr := formula.ParseRangeRef(rango)
		demasiadas := okr && (r.C2-r.C1+1)*(r.R2-r.R1+1) > analisis.TopeEscenario
		repetido := false
		for _, x := range esc {
			repetido = repetido || x.Name == strings.TrimSpace(nombre)
		}
		puedeCrear := strings.TrimSpace(nombre) != "" && okr && !demasiadas && !repetido
		_, okd := formula.ParseRangeRef(destino)
		puedeInforme := len(esc) > 0 && okd
		crear = func() {
			if !puedeCrear {
				return
			}
			b, _ := json.Marshal(map[string]any{"name": strings.TrimSpace(nombre), "range": strings.ToUpper(strings.TrimSpace(rango))})
			c.Mandar(CmdEscAnadir, string(b))
			eNombre.PonerTexto("") // rehace el panel por su AlCambiar
		}
		out := []fyne.CanvasObject{rotuloPanel(c.Traducir("data.scenarios"), v)}
		if len(esc) == 0 {
			out = append(out, conPie(meta(c.Traducir("data.scenario.empty"), v), tema.Espacio(2)))
		} else {
			lista := make([]fyne.CanvasObject, 0, len(esc))
			for _, x := range esc {
				lista = append(lista, lz.filaEscenario(c, x, e, v))
			}
			out = append(out, conPie(topeAlto(tema.Espacio(40), container.New(ui.Columna{}, lista...)), tema.Espacio(2)))
		}
		out = append(out,
			conPie(overline(c.Traducir("data.scenario.new")), tema.Espacio(1)),
			conPie(conEtiqueta(c.Traducir("common.name"), eNombre), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.scenario.changing"), eRango), tema.Espacio(1)),
		)
		if demasiadas {
			out = append(out, conPie(errorPanel(c.Traducir("data.scenario.tooMany", i18n.Vars{"max": analisis.TopeEscenario}), v), tema.Espacio(1)))
		}
		if repetido && strings.TrimSpace(nombre) != "" {
			out = append(out, conPie(errorPanel(c.Traducir("data.scenario.duplicate"), v), tema.Espacio(1)))
		}
		captura := botonPop(c.Traducir("data.scenario.capture"), false, crear)
		captura.Inerte = !puedeCrear
		informe := botonPop(c.Traducir("data.scenario.createReport"), false, func() {
			if !puedeInforme {
				return
			}
			refs := trozosRefs(strings.ToUpper(resultados))
			b, _ := json.Marshal(map[string]any{"target": strings.ToUpper(strings.TrimSpace(destino)), "results": refs})
			c.Mandar(CmdEscInforme, string(b))
			e.em.Cerrar()
		})
		informe.Inerte = !puedeInforme
		out = append(out,
			conArriba(anchoEntero(captura), tema.Espacio(1)),
			conArriba(conPie(overline(c.Traducir("data.summaryReport")), tema.Espacio(1)), tema.Espacio(2)),
			conPie(conEtiqueta(c.Traducir("data.scenario.resultCells"), eRes), tema.Espacio(1.5)),
			conPie(conEtiqueta(c.Traducir("data.targetTopLeft"), eDest), tema.Espacio(1.5)),
			anchoEntero(informe),
		)
		return out
	})
}

// filaEscenario es una fila de la lista: el nombre con sus celdas, aplicar, volver a
// capturar y quitar.
func (lz *Lienzo) filaEscenario(c *editor.Contexto, x analisis.Escenario, e *emergenteVivo, v tema.Variante) fyne.CanvasObject {
	nombre := x.Name
	aplicar := ui.NuevoBoton(c.Traducir("action.apply"), func() { c.Mandar(CmdEscAplicar, nombre); e.rehacer() }).Con(ui.Secundario, ui.TallaXS)
	recapturar := editor.NuevoBotonCinta(iconos.EdHistorial, c.Traducir("data.scenario.update", i18n.Vars{"name": nombre}),
		func() { c.Mandar(CmdEscActualizar, nombre); e.rehacer() })
	quitar := editor.NuevoBotonCinta(iconos.EdPapelera, c.Traducir("data.scenario.deleteNamed", i18n.Vars{"name": nombre}),
		func() { c.Mandar(CmdEscQuitar, nombre); e.rehacer() })
	for _, b := range []*editor.BotonCinta{recapturar, quitar} {
		b.Inerte = c.SoloLectura
	}
	aplicar.Inerte = c.SoloLectura
	texto := ui.NuevoParrafoConTinta(x.Name+" · "+strconv.Itoa(len(x.Values)), tema.TextoXS, ui.Normal, tema.Color("text", v)).Recortado(1)
	return container.New(fila{hueco: tema.Espacio(1), elastico: 0}, texto, aplicar,
		container.New(ladoFijo{ladoBotonRegla}, recapturar), container.New(ladoFijo{ladoBotonRegla}, quitar))
}
