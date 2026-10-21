// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los dos gestores de reglas de la cinta: el formato condicional (`CondFormatMenu` de
// InicioSheets.tsx) y la validación de datos (`ValidationMenu` de Datos.tsx). Los dos
// son un emergente con la lista de lo que hay en la hoja y, debajo, el formulario para
// añadir; cambiar el tipo de regla rehace el formulario, como en la web.

var (
	anchoPanelCond       = tema.Espacio(80) // w-80
	anchoPanelValidacion = tema.Espacio(64) // w-64
	anchoCampoRango      = tema.Espacio(24) // w-24
	altoListaReglas      = tema.Espacio(40) // max-h-40
	ladoBotonRegla       = tema.Espacio(6)  // h-6 w-6
)

// Los selectores de los gestores, en px como los escribe la web.
const (
	anchoSelectorRegla     float32 = 188
	anchoSelectorOp        float32 = 128
	anchoSelectorCriterio  float32 = 232
	anchoCampoPuesto               = 16 * 3.5 // w-16
	valorPorDefectoPuesto          = "10"
	claseReglaPorDefecto           = "cell"
	criterioPorDefecto             = "list"
	operadorPorDefecto             = "between"
	operadorCondPorDefecto         = "gt"
)

// Las opciones de los selectores: KINDS, COND_OPS, SCALE_PRESETS e ICON_SET_OPTIONS.
var (
	clasesRegla = []struct{ valor, clave string }{
		{"cell", "sheet.cond.compare"},
		{"formula", "cond.formula"},
		{"top", "sheet.cond.topN"},
		{"bottom", "sheet.cond.bottomN"},
		{"dup", "sheet.cond.dup"},
		{"unique", "sheet.cond.unique"},
		{"dataBar", "sheet.cond.dataBar"},
		{"colorScale", "sheet.colorScale"},
		{"iconSet", "sheet.iconSet"},
	}
	opsRegla = []struct{ valor, clave string }{
		{"gt", "cond.gt"},
		{"ge", "cond.gte"},
		{"lt", "cond.lt"},
		{"le", "cond.lte"},
		{"eq", "cond.eq"},
		{"ne", "cond.ne"},
		{"contains", "cond.contains"},
	}
	escalasRegla = []struct {
		valor, clave string
		escala       []string
	}{
		{"gyr", "sheet.scale.gyr", []string{"#16A34A", "#FEF08A", "#DC2626"}},
		{"wb", "sheet.scale.wb", []string{"#FFFFFF", "#2563EB"}},
		{"wg", "sheet.scale.wg", []string{"#FFFFFF", "#16A34A"}},
	}
	juegosRegla = []struct{ valor, clave string }{
		{"arrows", "sheet.icons.arrows"}, {"traffic", "sheet.cond.trafficLight"}, {"symbols", "sheet.icons.symbols"},
	}
	// rellenosRegla es FILLS (la paleta de resaltado sin «quitar») y coloresBarra,
	// BAR_COLORS.
	rellenosRegla = []string{"#FEF08A", "#FED7AA", "#BBF7D0", "#BFDBFE", "#E9D5FF", "#FBCFE8", "#E2E8F0"}
	coloresBarra  = []string{"#2563EB", "#16A34A", "#DC2626", "#D97706", "#7C3AED", "#0D9488"}
	// Los criterios y los operadores de la validación: DATA_VALIDATION_TYPES y _OPS.
	tiposCriterio = []struct{ valor, clave string }{
		{"list", "valid.kind.list"},
		{"number", "valid.kind.number"},
		{"date", "valid.kind.date"},
		{"textLength", "valid.kind.textLength"},
		{"formula", "cond.formula"},
	}
	opsCriterio = []struct{ valor, clave string }{
		{"between", "query.op.between"},
		{"notBetween", "cond.notBetween"},
		{"eq", "query.op.eq"},
		{"ne", "query.op.ne"},
		{"gt", "query.op.gt"},
		{"lt", "query.op.lt"},
		{"ge", "cond.gte"},
		{"le", "cond.lte"},
	}
)

// opciones traduce una lista de (valor, clave) a opciones de un selector.
func opciones(c *editor.Contexto, lista []struct{ valor, clave string }) []editor.Opcion {
	out := make([]editor.Opcion, len(lista))
	for i, x := range lista {
		out[i] = editor.Opcion{Valor: x.valor, Rotulo: c.Traducir(x.clave)}
	}
	return out
}

// ---- formato condicional ----

// formCond es el estado del formulario, que sobrevive a rehacer el emergente.
type formCond struct {
	rango, clase, op, valor, formula, puesto string
	porcien                                  bool
	relleno, barra, escala, iconos           string
}

func (lz *Lienzo) botonCondicional(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdFormatoCond, c.Traducir("cf.title"), func() {
		f := &formCond{
			rango: rangoTexto(lz.Rejilla), clase: claseReglaPorDefecto, op: operadorCondPorDefecto,
			puesto: valorPorDefectoPuesto, relleno: rellenosRegla[0], barra: coloresBarra[0], escala: "gyr", iconos: "arrows",
		}
		lz.abrirCondicional(c, b, f)
	})
	b.Inerte = c.SoloLectura
	return b
}

// rangoTexto es la selección en A1 (`readSheetState().selection`): «B2» o «B2:D5».
func rangoTexto(r *Rejilla) string {
	c1, f1, c2, f2 := r.Rango()
	return textoRango(Celda{C: c1, F: f1}, Celda{C: c2, F: f2})
}

// abrirCondicional pinta el gestor. Se vuelve a llamar —cerrando el anterior— cada vez
// que algo cambia la forma del formulario o la lista.
func (lz *Lienzo) abrirCondicional(c *editor.Contexto, disparador *editor.BotonCinta, f *formCond) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		lz.abrirCondicional(c, disparador, f)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cab := entre(ui.NuevoParrafo(c.Traducir("cf.rulesThisSheet"), tema.TextoXS, ui.Medio, "text", v),
		ui.NuevoParrafo(c.Traducir("cf.priority"), tema.Texto2XS, ui.Normal, "text-subtle", v))
	filas := []fyne.CanvasObject{cab}
	reglas := lz.Libro.Condiciones()
	if len(reglas) == 0 {
		filas = append(filas, conMargen(ui.NuevoParrafo(c.Traducir("cf.noRules"), tema.TextoXS, ui.Normal, "text-muted", v)))
	} else {
		lista := make([]fyne.CanvasObject, 0, len(reglas))
		for i, r := range reglas {
			lista = append(lista, lz.filaRegla(c, i, len(reglas), r, rehacer))
		}
		filas = append(filas, conMargen(topeAlto(altoListaReglas, container.New(ui.Columna{Hueco: huecoPanel}, lista...))))
	}
	filas = append(filas, lz.formularioCond(c, f, rehacer))
	em = abrirEmergente(disparador, panel{ancho: anchoPanelCond, hueco: huecoPanel, filas: filas}.objeto())
}

// filaRegla es una regla de la lista: su muestra, «rango · etiqueta» y subir, bajar y
// quitar.
func (lz *Lienzo) filaRegla(c *editor.Contexto, i, n int, r ReglaCond, rehacer func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	muestra := ui.NuevaCaja(tema.Color("border", v), tema.Color("border", v), 2)
	if col, ok := editor.Hex(previaRegla(r)); ok {
		muestra.Pintar(col, tema.Color("border", v))
	}
	accion := func(i iconos.Ed, rotulo string, inerte bool, f func()) fyne.CanvasObject {
		b := editor.NuevoBotonCinta(i, rotulo, func() { f(); rehacer() })
		b.Inerte = inerte || c.SoloLectura
		return container.New(ladoFijo{ladoBotonRegla}, b)
	}
	vars := i18n.Vars{"range": r.Rango}
	texto := r.Rango + " · " + etiquetaRegla(c, r)
	return container.New(fila{hueco: tema.Espacio(1.5), elastico: 1},
		container.New(ladoFijo{tema.Espacio(3)}, cajaDe(muestra)),
		ui.NuevoParrafo(texto, tema.TextoXS, ui.Normal, "text", v).Recortado(1),
		accion(iconos.EdChevronArriba, c.Traducir("sheet.rule.up", vars), i == 0, func() { c.Mandar(CmdCondMover, strconv.Itoa(i)+",-1") }),
		accion(iconos.EdChevronAbajo, c.Traducir("sheet.rule.down", vars), i == n-1, func() { c.Mandar(CmdCondMover, strconv.Itoa(i)+",1") }),
		accion(iconos.EdCerrar, c.Traducir("sheet.rule.remove", vars), false, func() { c.Mandar(CmdCondQuitar, strconv.Itoa(i)) }),
	)
}

// previaRegla es `rulePreview`: el relleno, el color de la barra o el del medio de la
// escala.
func previaRegla(r ReglaCond) string {
	switch {
	case r.Relleno != "":
		return r.Relleno
	case r.Color != "":
		return r.Color
	case len(r.Escala) > 0:
		return r.Escala[(len(r.Escala)-1)/2]
	}
	return ""
}

// etiquetaRegla es `condRuleLabel`: las dos componen la misma etiqueta con las mismas
// claves del catálogo, así que se lee igual en cada idioma a los dos lados.
func etiquetaRegla(c *editor.Contexto, r ReglaCond) string {
	switch r.Clase {
	case "formula":
		return c.Traducir("sheet.condLabel.formula", i18n.Vars{"formula": r.Formula})
	case "top", "bottom":
		clave := "sheet.condLabel." + r.Clase
		if r.Porcien {
			clave += "Percent"
		}
		return c.Traducir(clave, i18n.Vars{"n": r.Puesto})
	case "dup":
		return c.Traducir("sheet.cond.dup")
	case "unique":
		return c.Traducir("sheet.cond.unique")
	case "dataBar":
		return c.Traducir("sheet.cond.dataBar")
	case "colorScale":
		n := len(r.Escala)
		if n == 0 {
			n = 2 // `r.scale?.length ?? 2`
		}
		return c.Traducir("sheet.condLabel.colorScale", i18n.Vars{"n": n})
	case "iconSet":
		juego := r.Iconos
		if juego == "" {
			juego = "arrows"
		}
		return c.Traducir("sheet.condLabel.iconSet", i18n.Vars{"icons": juego})
	}
	valor := ""
	if r.Valor != nil {
		valor = *r.Valor
	}
	for _, o := range opsRegla {
		if o.valor == r.Op {
			return strings.TrimSpace(c.Traducir(o.clave) + " " + valor)
		}
	}
	return valor
}

// formularioCond es la mitad de abajo del gestor: rango y tipo, los campos de ese tipo,
// las muestras de color y «Añadir regla».
func (lz *Lienzo) formularioCond(c *editor.Contexto, f *formCond, rehacer func()) fyne.CanvasObject {
	rango := ui.NuevaEntrada("A1:B10").ConPropia(campoCinta)
	rango.Ancho = anchoCampoRango
	rango.PonerTexto(f.rango)
	rango.AlCambiar(func(s string) { f.rango = s })
	clase := editor.NuevoSelectorCinta(c.Traducir("sheet.ruleType"), anchoSelectorRegla, opciones(c, clasesRegla), f.clase,
		func(s string) { f.clase = s; rehacer() })
	filas := append([]fyne.CanvasObject{container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, rango, clase)}, lz.camposCond(c, f)...)

	anadir := botonPop(c.Traducir("cf.addRule"), false, func() {
		if regla, ok := f.regla(); ok {
			b, _ := json.Marshal(regla)
			c.Mandar(CmdCondAnadir, string(b))
			rehacer()
		}
	})
	_, puede := f.regla()
	anadir.Inerte = !puede || c.SoloLectura
	var izq fyne.CanvasObject = canvas.NewRectangle(color.NRGBA{})
	if f.usaRelleno() {
		izq = muestras(rellenosRegla, f.relleno, c.Traducir("group.fill"), func(col string) { f.relleno = col; rehacer() })
	}
	filas = append(filas, entre(izq, anadir)) // el botón con `ml-auto`
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return container.New(bordeArriba{}, canvas.NewRectangle(tema.Color("border", v)),
		container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, filas...))
}

// camposCond son los campos propios del tipo de regla elegido.
func (lz *Lienzo) camposCond(c *editor.Contexto, f *formCond) []fyne.CanvasObject {
	switch f.clase {
	case "cell":
		op := editor.NuevoSelectorCinta(c.Traducir("common.condition"), anchoSelectorOp, opciones(c, opsRegla), f.op,
			func(s string) { f.op = s })
		valor := ui.NuevaEntrada(c.Traducir("common.value")).ConPropia(campoCinta)
		valor.PonerTexto(f.valor)
		valor.AlCambiar(func(s string) { f.valor = s })
		return []fyne.CanvasObject{container.New(fila{hueco: tema.Espacio(1.5), elastico: 1}, op, valor)}
	case "formula":
		fx := ui.NuevaEntrada("=$A1>PROMEDIO($A$1:$A$10)").ConPropia(campoCinta)
		fx.PonerTexto(f.formula)
		fx.AlCambiar(func(s string) { f.formula = s })
		return []fyne.CanvasObject{fx}
	case "top", "bottom":
		n := ui.NuevaEntrada("").ConPropia(campoCinta)
		n.Ancho = anchoCampoPuesto
		n.PonerTexto(f.puesto)
		n.AlCambiar(func(s string) { f.puesto = s })
		pct := nuevaCasillaXS(c.Traducir("sheet.mask.percent"), f.porcien, tema.Espacio(1.5), func(si bool) { f.porcien = si })
		return []fyne.CanvasObject{container.New(ui.Fila{Hueco: tema.Espacio(2)}, n, pct)}
	case "dataBar":
		return []fyne.CanvasObject{muestras(coloresBarra, f.barra, c.Traducir("sheet.barColor"), func(col string) { f.barra = col })}
	case "colorScale":
		ops := make([]editor.Opcion, len(escalasRegla))
		for i, e := range escalasRegla {
			ops[i] = editor.Opcion{Valor: e.valor, Rotulo: c.Traducir(e.clave)}
		}
		return []fyne.CanvasObject{container.New(ui.Fila{}, editor.NuevoSelectorCinta(c.Traducir("sheet.colorScale"), anchoSelectorRegla, ops, f.escala, func(s string) { f.escala = s }))}
	case "iconSet":
		return []fyne.CanvasObject{container.New(ui.Fila{}, editor.NuevoSelectorCinta(c.Traducir("sheet.iconSet"), anchoSelectorRegla, opciones(c, juegosRegla), f.iconos, func(s string) { f.iconos = s }))}
	}
	return nil
}

func (f *formCond) usaRelleno() bool {
	switch f.clase {
	case "cell", "formula", "top", "bottom", "dup", "unique":
		return true
	}
	return false
}

// regla es `add`: la regla que manda el formulario, o false si le falta algo
// (`canAdd`).
func (f *formCond) regla() (map[string]any, bool) {
	rango := strings.ToUpper(strings.TrimSpace(f.rango))
	if rango == "" {
		return nil, false
	}
	r := map[string]any{"range": rango}
	switch f.clase {
	case "cell":
		if strings.TrimSpace(f.valor) == "" {
			return nil, false
		}
		r["op"], r["value"], r["fill"] = f.op, strings.TrimSpace(f.valor), f.relleno
	case "formula":
		if strings.TrimSpace(f.formula) == "" {
			return nil, false
		}
		r["kind"], r["formula"], r["fill"] = f.clase, strings.TrimSpace(f.formula), f.relleno
	case "top", "bottom":
		n, err := strconv.ParseFloat(strings.TrimSpace(f.puesto), 64)
		if err != nil || n <= 0 {
			return nil, false
		}
		r["kind"], r["rank"], r["percent"], r["fill"] = f.clase, n, f.porcien, f.relleno
	case "dup", "unique":
		r["kind"], r["fill"] = f.clase, f.relleno
	case "dataBar":
		r["kind"], r["color"] = f.clase, f.barra
	case "colorScale":
		for _, e := range escalasRegla {
			if e.valor == f.escala {
				r["kind"], r["scale"] = f.clase, e.escala
			}
		}
	case "iconSet":
		r["kind"], r["icons"] = f.clase, f.iconos
	}
	return r, true
}

// muestras es `Swatches`: una fila de colores que se eligen, el actual marcado.
func muestras(colores []string, actual, rotulo string, elegir func(string)) fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(colores))
	for i, hex := range colores {
		col, _ := editor.Hex(hex)
		objs[i] = nuevaMuestraRegla(col, rotulo+" "+hex, hex == actual, func() { elegir(hex) })
	}
	return container.New(ui.Fila{Hueco: tema.Espacio(1)}, objs...)
}

// Medidas de la muestra de `Swatches`: `h-5 w-5 rounded-[3px]` y el check `size="sm"`.
const (
	RadioMuestraRegla float32 = 3
	LadoCheckMuestra  float32 = 14
)

// LadoMuestraRegla es el `h-5 w-5`.
var LadoMuestraRegla = tema.Espacio(5)

// muestraRegla es el botón de `Swatches`: `relative h-5 w-5 rounded-[3px] border
// border-border` con su color de fondo, y el elegido con un check blanco en
// `mix-blend-difference` —que pinta, sobre cada color, su inverso—. No es
// `ui.MuestraColor`: esa es redonda y marca el elegido con un aro, y la web no.
type muestraRegla struct {
	widget.BaseWidget
	col      color.NRGBA
	rotulo   string
	elegida  bool
	alPulsar func()
	enfocado bool
}

var (
	_ fyne.Tappable   = (*muestraRegla)(nil)
	_ fyne.Focusable  = (*muestraRegla)(nil)
	_ fyne.Accessible = (*muestraRegla)(nil)
)

func nuevaMuestraRegla(c color.NRGBA, rotulo string, elegida bool, alPulsar func()) *muestraRegla {
	m := &muestraRegla{col: c, rotulo: rotulo, elegida: elegida, alPulsar: alPulsar}
	m.ExtendBaseWidget(m)
	return m
}

func (m *muestraRegla) Tapped(*fyne.PointEvent) {
	if m.alPulsar != nil {
		m.alPulsar()
	}
}
func (m *muestraRegla) FocusGained() { m.enfocado = true; m.Refresh() }
func (m *muestraRegla) FocusLost()   { m.enfocado = false; m.Refresh() }
func (m *muestraRegla) TypedRune(r rune) {
	if r == ' ' {
		m.Tapped(nil)
	}
}

func (m *muestraRegla) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(m, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		m.Tapped(nil)
	}
}
func (m *muestraRegla) AccessibilityLabel() string             { return m.rotulo }
func (m *muestraRegla) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (m *muestraRegla) MinSize() fyne.Size {
	m.ExtendBaseWidget(m)
	return fyne.NewSize(LadoMuestraRegla, LadoMuestraRegla)
}

// inverso es el blanco en `mix-blend-difference` sobre ese color: |255 − c| por canal.
func inverso(c color.NRGBA) color.NRGBA {
	return color.NRGBA{R: 255 - c.R, G: 255 - c.G, B: 255 - c.B, A: 255}
}

func (m *muestraRegla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &muestraReglaRenderer{
		m: m, caja: ui.NuevaCaja(m.col, tema.Color("border", v), RadioMuestraRegla),
		aro:   ui.NuevoAro(RadioMuestraRegla, ui.SinHueco, v),
		check: iconos.TrazoEdConTinta(iconos.EdTic, LadoCheckMuestra, inverso(m.col)),
	}
	// El check nace escondido en la suelta: sin el campo, se pintaba hasta el primer
	// Refresh. Se toca el campo y no Hide(), que repintaría a mitad de crear el renderer.
	if rs, ok := r.check.Objeto().(*canvas.Raster); ok {
		rs.Hidden = !m.elegida
	}
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.check.Objeto())...)
	return r
}

type muestraReglaRenderer struct {
	m     *muestraRegla
	caja  *ui.Caja
	aro   *ui.Aro
	check *iconos.Dibujo
	objs  []fyne.CanvasObject
}

func (r *muestraReglaRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *muestraReglaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *muestraReglaRenderer) Destroy()                     {}
func (r *muestraReglaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	r.check.Colocar((t.Width-LadoCheckMuestra)/2, (t.Height-LadoCheckMuestra)/2, LadoCheckMuestra)
}

func (r *muestraReglaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(r.m.col, tema.Color("border", v))
	r.check.Ver(r.m.elegida)
	r.aro.Pintar(v)
	r.aro.Ver(r.m.enfocado)
	if s := r.m.Size(); s.Width > 0 {
		r.Layout(s)
	}
}

// ---- validación de datos ----

// formValidacion es el estado del formulario de validación.
type formValidacion struct {
	tipo, texto, op, a, b, formula, error string
	entero, estricta                      bool
}

func (lz *Lienzo) botonValidacion(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdListaPuntos, c.Traducir("data.validation"), func() {
		lz.abrirValidacion(c, b, &formValidacion{tipo: criterioPorDefecto, op: operadorPorDefecto})
	})
	b.Activo = len(lz.Libro.Validaciones()) > 0 // `aria-pressed={validations.length > 0}`
	b.Inerte = c.SoloLectura
	return b
}

// abrirValidacion es `ValidationMenu`: las validaciones de la hoja y el formulario.
func (lz *Lienzo) abrirValidacion(c *editor.Contexto, disparador *editor.BotonCinta, f *formValidacion) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		lz.abrirValidacion(c, disparador, f)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filas := []fyne.CanvasObject{ui.NuevoParrafo(c.Traducir("data.validation"), tema.TextoXS, ui.Medio, "text", v)}
	if vs := lz.Libro.Validaciones(); len(vs) > 0 {
		lista := make([]fyne.CanvasObject, 0, len(vs))
		for i, x := range vs {
			quitar := editor.NuevoBotonCinta(iconos.EdPapelera, c.Traducir("sheet.validation.remove", i18n.Vars{"range": x.Rango}),
				func() { c.Mandar(CmdValQuitar, strconv.Itoa(i)); rehacer() })
			quitar.Inerte = c.SoloLectura
			lista = append(lista, container.New(fila{hueco: tema.Espacio(1), elastico: 0},
				ui.NuevoParrafo(x.Rango+": "+etiquetaValidacion(c, x), tema.TextoXS, ui.Normal, "text", v).Recortado(1),
				container.New(ladoFijo{ladoBotonRegla}, quitar)))
		}
		filas = append(filas, conMargen(topeAlto(tema.Espacio(28), container.New(ui.Columna{}, lista...))))
	}
	etiquetado := func(clave string, o fyne.CanvasObject) fyne.CanvasObject {
		return container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
			ui.NuevoParrafo(c.Traducir(clave), tema.Texto2XS, ui.Normal, "text-subtle", v), o)
	}
	filas = append(filas, etiquetado("common.criterion", editor.NuevoSelectorCinta(c.Traducir("common.criterion"),
		anchoSelectorCriterio, opciones(c, tiposCriterio), f.tipo, func(s string) { f.tipo = s; rehacer() })))
	filas = append(filas, lz.camposValidacion(c, f, etiquetado, rehacer)...)
	errMsg := ui.NuevaEntrada(c.Traducir("data.validation.invalid")).ConPropia(campoPanel)
	errMsg.PonerTexto(f.error)
	errMsg.AlCambiar(func(s string) { f.error = s })
	estricta := nuevaCasillaXS(c.Traducir("data.validation.reject"), f.estricta, tema.Espacio(1.5), func(si bool) { f.estricta = si })
	seleccion := rangoTexto(lz.Rejilla)
	aplicar := botonPop(c.Traducir("data.validation.applyTo", i18n.Vars{"range": seleccion}), false, func() {
		if carga, ok := f.carga(seleccion); ok {
			b, _ := json.Marshal(carga)
			c.Mandar(CmdValAnadir, string(b))
			em.Cerrar()
		}
	})
	_, puede := f.carga(seleccion)
	aplicar.Inerte = !puede || c.SoloLectura
	filas = append(filas, etiquetado("data.validation.errorMsg", errMsg), estricta, aplicar)
	em = abrirEmergente(disparador, panel{ancho: anchoPanelValidacion, hueco: tema.Espacio(1.5), filas: filas}.objeto())
}

// camposValidacion son los del criterio elegido: la lista, la comparación o la fórmula.
func (lz *Lienzo) camposValidacion(c *editor.Contexto, f *formValidacion, etiquetado func(string, fyne.CanvasObject) fyne.CanvasObject, rehacer func()) []fyne.CanvasObject {
	campo := func(ph, valor string, poner func(string)) *ui.Entrada {
		e := ui.NuevaEntrada(ph).ConPropia(campoPanel)
		e.PonerTexto(valor)
		e.AlCambiar(poner)
		return e
	}
	switch f.tipo {
	case "list":
		area := ui.NuevaAreaTexto(c.Traducir("data.validation.listPlaceholder"))
		area.PonerTexto(f.texto)
		area.AlCambiar(func(s string) { f.texto = s })
		return []fyne.CanvasObject{etiquetado("data.validation.allowedValues", area)}
	case "formula":
		return []fyne.CanvasObject{etiquetado("data.validation.formula", campo("=A1>0", f.formula, func(s string) { f.formula = s }))}
	}
	ph, rotulo := "0", "common.value"
	switch f.tipo {
	case "date":
		ph, rotulo = c.Traducir("data.validation.datePlaceholder"), "valid.kind.date"
	case "textLength":
		rotulo = "valid.kind.textLength"
	}
	out := []fyne.CanvasObject{etiquetado("common.condition", editor.NuevoSelectorCinta(c.Traducir("common.condition"),
		anchoSelectorCriterio, opciones(c, opsCriterio), f.op, func(s string) { f.op = s; rehacer() }))}
	operandos := []fyne.CanvasObject{etiquetado(rotulo, campo(ph, f.a, func(s string) { f.a = s }))}
	if f.op == "between" || f.op == "notBetween" {
		operandos = append(operandos, etiquetado("common.and", campo(ph, f.b, func(s string) { f.b = s })))
	}
	out = append(out, container.New(&ui.Rejilla{Hueco: tema.Espacio(1.5), Fijas: len(operandos)}, operandos...))
	if f.tipo == "number" {
		entero := nuevaCasillaXS(c.Traducir("data.validation.integerOnly"), f.entero, tema.Espacio(1.5), func(si bool) { f.entero = si })
		out = append(out, entero)
	}
	return out
}

// carga es el `payload` de `apply`, o false si el criterio no vale (`canApply`).
func (f *formValidacion) carga(rango string) (map[string]any, bool) {
	p := map[string]any{"range": rango, "strict": f.estricta}
	if e := strings.TrimSpace(f.error); e != "" {
		p["error"] = e
	}
	switch f.tipo {
	case "list":
		vs := valoresValidacion(f.texto)
		if len(vs) == 0 {
			return nil, false
		}
		p["values"] = vs
	case "formula":
		if strings.TrimSpace(f.formula) == "" {
			return nil, false
		}
		p["type"], p["formula"] = "formula", strings.TrimSpace(f.formula)
	default:
		dos := f.op == "between" || f.op == "notBetween"
		if _, ok := operando(f.tipo, f.a); !ok {
			return nil, false
		}
		if _, ok := operando(f.tipo, f.b); dos && !ok {
			return nil, false
		}
		p["type"], p["op"], p["a"] = f.tipo, f.op, strings.TrimSpace(f.a)
		if dos {
			p["b"] = strings.TrimSpace(f.b)
		}
		if f.tipo == "number" && f.entero {
			p["integer"] = true
		}
	}
	return p, true
}

// etiquetaValidacion es `validationLabel`, con las mismas claves que la web.
func etiquetaValidacion(c *editor.Contexto, v Validacion) string {
	switch v.Tipo {
	case "", "list":
		return strings.Join(v.Valores, ", ")
	case "formula":
		return c.Traducir("sheet.condLabel.formula", i18n.Vars{"formula": v.Formula})
	}
	op := map[string]string{"eq": "=", "ne": "≠", "gt": ">", "lt": "<", "ge": "≥", "le": "≤"}[v.Op]
	operandos := v.A
	switch v.Op {
	case "between", "notBetween":
		op = c.Traducir(map[string]string{"between": "query.op.between", "notBetween": "cond.notBetween"}[v.Op])
		operandos = v.A + " " + c.Traducir("common.and") + " " + v.B
	}
	nombre := map[string]string{"number": "valid.kind.number", "date": "valid.kind.date"}[v.Tipo]
	if nombre == "" {
		nombre = "valid.kind.textLength"
	}
	return strings.TrimSpace(c.Traducir(nombre) + " " + op + " " + operandos)
}

// ---- piezas de maquetación ----

// ladoFijo coloca su hijo en un cuadrado de ese lado.
type ladoFijo struct{ lado float32 }

func (l ladoFijo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(l.lado, l.lado) }
func (l ladoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos((t.Width-l.lado)/2, (t.Height-l.lado)/2))
		o.Resize(fyne.NewSize(l.lado, l.lado))
	}
}

// topeAlto es `max-h-* overflow-auto`: mide lo que su contenido hasta ese alto, y lo
// que se sale se desplaza y se recorta, sin la sombra de borde de Fyne.
func topeAlto(alto float32, o fyne.CanvasObject) fyne.CanvasObject {
	return desplazable(o, fyne.NewSize(o.MinSize().Width, min(o.MinSize().Height, alto)))
}

// desplazable es un `overflow-auto` de esa medida mínima.
func desplazable(o fyne.CanvasObject, medida fyne.Size) fyne.CanvasObject {
	d := container.NewVScroll(o)
	d.SetMinSize(medida)
	return container.NewThemeOverride(d, tema.SinSombras{})
}

// bordeArriba es el `border-border border-t pt-2` que separa el formulario de la lista:
// el primer hijo es la línea y el segundo, lo de debajo.
type bordeArriba struct{}

func (bordeArriba) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[1].MinSize()
	return fyne.NewSize(m.Width, m.Height+tema.Borde+tema.Espacio(2))
}

func (bordeArriba) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, tema.Borde))
	d := tema.Borde + tema.Espacio(2)
	objs[1].Move(fyne.NewPos(0, d))
	objs[1].Resize(fyne.NewSize(t.Width, t.Height-d))
}

// recuadro es `border-border rounded-sm border p-1.5`: una caja con borde y su relleno.
func recuadro(dentro fyne.CanvasObject) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioSM)
	return container.New(recuadroLayout{c}, append(c.Objetos(), dentro)...)
}

type recuadroLayout struct{ c *ui.Caja }

func (l recuadroLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[len(objs)-1].MinSize()
	d := 2 * (tema.Borde + tema.Espacio(1.5))
	return fyne.NewSize(m.Width+d, m.Height+d)
}

func (l recuadroLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	l.c.Redimensionar(t)
	d := tema.Borde + tema.Espacio(1.5)
	dentro := objs[len(objs)-1]
	dentro.Move(fyne.NewPos(d, d))
	dentro.Resize(fyne.NewSize(t.Width-2*d, t.Height-2*d))
}

// cajaDe convierte una ui.Caja en un objeto que se puede meter en un contenedor.
func cajaDe(c *ui.Caja) fyne.CanvasObject {
	return container.New(cajaLayout{c}, c.Objetos()...)
}

type cajaLayout struct{ c *ui.Caja }

func (l cajaLayout) MinSize([]fyne.CanvasObject) fyne.Size     { return fyne.NewSize(0, 0) }
func (l cajaLayout) Layout(_ []fyne.CanvasObject, t fyne.Size) { l.c.Redimensionar(t) }

// fila es `flex items-center gap-*` con UN hijo en `flex-1` (o en `justify-between`,
// con el elástico vacío entre los dos lados): los demás a su ancho mínimo, centrados en
// alto, y el elástico se lleva lo que sobra.
type fila struct {
	hueco    float32
	elastico int
}

func (f fila) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += f.hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (f fila) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	sobra := t.Width - f.MinSize(objs).Width
	var x float32
	for i, o := range objs {
		m := o.MinSize()
		w := m.Width
		if i == f.elastico {
			w += max(sobra, 0)
		}
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(fyne.NewSize(w, m.Height))
		x += w + f.hueco
	}
}

// entre es `justify-between`: lo de la izquierda y lo de la derecha, separados.
func entre(izq, der fyne.CanvasObject) fyne.CanvasObject {
	return container.New(fila{elastico: 1}, izq, canvas.NewRectangle(color.NRGBA{}), der)
}
