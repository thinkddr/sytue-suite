// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El lienzo de Calcus es el [editor.Lienzo] que el cromo aloja.
var _ editor.Lienzo = (*Lienzo)(nil)

// modelo es el `SheetModel` de la web RECORTADO a lo que este lienzo entiende: el
// índice de la hoja activa y, por hoja, su nombre, sus celdas, su formato de número,
// su formato visual y sus paneles inmovilizados.
//
// Lo que NO está aquí —`externalTables`…— no es que se ignore: si aparece, el
// documento se abre en SOLO LECTURA.
// Ver Cargar.
type modelo struct {
	Active  int              `json:"active"`
	Sheets  []hoja           `json:"sheets"`
	Protect *ProteccionLibro `json:"protect,omitempty"`
	Names   []Nombre         `json:"names,omitempty"`
}

// hoja es una `SheetData` tal y como se ESCRIBE: solo con los mapas que tienen algo,
// como la web (un documento viejo {name, cells} sale igual que entró).
type hoja struct {
	Name     string                `json:"name"`
	TabColor string                `json:"tabColor,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Cells    map[string]string     `json:"cells"`
	Formats  map[string]FormatoNum `json:"formats,omitempty"`
	Styles   map[string]Estilo     `json:"styles,omitempty"`
	Freeze   *Fijas                `json:"freeze,omitempty"`
	// El contrato es el SheetData de la web, que va en camelCase.
	CondFormats []ReglaCond          `json:"condFormats,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Validations []Validacion         `json:"validations,omitempty"`
	Autofilter  *Autofiltro          `json:"autofilter,omitempty"`
	Protect     *Proteccion          `json:"protect,omitempty"`
	Pivots      []analisis.Dinamica  `json:"pivots,omitempty"`
	Scenarios   []analisis.Escenario `json:"scenarios,omitempty"`
	RowGroups   []analisis.Grupo     `json:"rowGroups,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	ColGroups   []analisis.Grupo     `json:"colGroups,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	// Los objetos de la hoja (`objetos.go` y `tablas.go`).
	Sparklines []Minigrafico  `json:"sparklines,omitempty"`
	Tables     []TablaFmt     `json:"tables,omitempty"`
	Charts     []Grafico      `json:"charts,omitempty"`
	Slicers    []Segmentacion `json:"slicers,omitempty"`
	// Las medidas personalizadas de la web (`colWidths`, `rowHeights`: px de maqueta). Este
	// lienzo no las pinta (la rejilla es de medida fija), pero las conserva al guardar.
	ColWidths  map[string]float64 `json:"colWidths,omitempty"`  //nolint:tagliatelle // el nombre lo fija la web
	RowHeights map[string]float64 `json:"rowHeights,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	// Las celdas combinadas (`merges`: rangos A1): tampoco se pintan, pero se conservan.
	Merges []string `json:"merges,omitempty"`
}

// hojaLeida es una `SheetData` tal y como se LEE: el formato llega sin sanear, y se
// sanea igual que `sanitizeSheet` (lo que la web descartaría, aquí también).
type hojaLeida struct {
	Name     string                  `json:"name"`
	TabColor string                  `json:"tabColor"` //nolint:tagliatelle // el nombre lo fija la web
	Cells    map[string]string       `json:"cells"`
	Formats  map[string]formatoCrudo `json:"formats"`
	Styles   map[string]estiloCrudo  `json:"styles"`
	Freeze   *struct {
		Cols, Rows any
	} `json:"freeze"`
	CondFormats []reglaCruda        `json:"condFormats"` //nolint:tagliatelle // el nombre lo fija la web
	Validations []validacionCruda   `json:"validations"`
	Autofilter  *autofiltroCrudo    `json:"autofilter"`
	Protect     any                 `json:"protect"`
	Pivots      []any               `json:"pivots"`
	Scenarios   []any               `json:"scenarios"`
	RowGroups   any                 `json:"rowGroups"` //nolint:tagliatelle // el nombre lo fija la web
	ColGroups   any                 `json:"colGroups"` //nolint:tagliatelle // el nombre lo fija la web
	Sparklines  []minigraficoCrudo  `json:"sparklines"`
	Tables      []tablaCruda        `json:"tables"`
	Charts      []graficoCrudo      `json:"charts"`
	Slicers     []segmentacionCruda `json:"slicers"`
	ColWidths   map[string]float64  `json:"colWidths"`  //nolint:tagliatelle // el nombre lo fija la web
	RowHeights  map[string]float64  `json:"rowHeights"` //nolint:tagliatelle // el nombre lo fija la web
	Merges      []string            `json:"merges"`
}

// Las únicas claves que este lienzo sabe leer. Cualquier otra en el JSON significa que
// el documento tiene algo que no sabemos volver a escribir.
var (
	clavesModelo = map[string]bool{"active": true, "sheets": true, "protect": true, "names": true}
	clavesHoja   = map[string]bool{
		"name": true, "tabColor": true, "cells": true, "formats": true, "styles": true, "freeze": true, "condFormats": true,
		"validations": true, "autofilter": true, "protect": true, "rowGroups": true, "colGroups": true, "pivots": true, "scenarios": true,
		"sparklines": true, "tables": true, "charts": true, "slicers": true, "colWidths": true, "rowHeights": true, "merges": true,
	}
)

// reColorPestana es el color de pestaña que la web acepta.
var reColorPestana = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Objeto es el widget que el cromo pone en el área central.
func (c *Lienzo) Objeto() fyne.CanvasObject { return c }

// Cargar lee el contenido del registro LWW `doc:sheet` y dice si lo entiende ENTERO.
//
// # Devolver false es la función, no el fallo
//
// Esta es la promesa que en una hoja de cálculo importa más que en ningún otro sitio:
// una fórmula que se guarda mal destruye datos en silencio y nadie lo nota hasta meses
// después. Así que aquí se dice «no» a todo lo que no sepamos reescribir EXACTAMENTE,
// y el cromo enseña la franja de solo lectura. Hoy se dice «no» a:
//
//   - cualquier clave del JSON que no sea `active`/`sheets` (libro) o `name`/`cells`
//     (hoja): formatos de número, estilos, formato condicional, gráficos, tablas,
//     validaciones, paneles congelados, protección, esquema…;
//   - cualquier celda cuyo HTML no sea el envoltorio canónico de la web —alineación,
//     negrita y cursiva alrededor de texto escapado— (ver `celdaEntendida`): un
//     `<span>`, un `<br>` o una entidad que `stripHtml` no decodifica es algo que la
//     web pinta de una manera y lee de otra, y reescribirlo sería adivinar;
//   - cualquier referencia fuera de A1:Z100, que es la rejilla que la web declara
//     (`COLS`/`ROWS`). Serializar recorre esa rejilla, así que una celda de más allá
//     se perdería al guardar.
//
// Un contenido vacío es un libro nuevo, y ese sí se entiende.
//
// # Lo que no se entiende se ENSEÑA igual, cerrado
//
// Decir «no» es no dejar ESCRIBIR, no dejar la rejilla en blanco: si el JSON se lee y
// trae hojas, se pintan sus celdas —con el texto que da `rawText`— aunque el libro
// lleve formatos de número o gráficos que aquí no se pintan. Es la hoja protegida de
// la web: se ve, se recorre y se copia.
//
// # Y el lienzo se cierra a sí mismo
//
// Cuando la respuesta es «no», este método enciende además su propio
// [Lienzo.SoloLectura]. El cromo apaga los comandos de la CINTA, pero no sabe nada de
// las teclas de la rejilla: sin este cerrojo, un documento que no entendemos seguiría
// dejando escribir en una celda a base de teclear. El cinturón va aquí porque aquí
// está el que sabe que no lo entiende.
func (c *Lienzo) Cargar(contenido string) bool {
	m, entendido := leer(contenido)
	if m != nil {
		c.poner(*m)
	}
	c.SoloLectura(!entendido)
	c.avisar()
	return entendido
}

// leer interpreta el documento: el modelo que se puede ENSEÑAR (nil si ni eso) y si se
// entiende entero.
func leer(contenido string) (*leido, bool) {
	if strings.TrimSpace(contenido) == "" {
		return nil, true
	}
	var m struct {
		Active  int         `json:"active"`
		Sheets  []hojaLeida `json:"sheets"`
		Protect any         `json:"protect"`
		Names   any         `json:"names"`
	}
	if err := json.Unmarshal([]byte(contenido), &m); err != nil || len(m.Sheets) == 0 {
		return nil, false
	}
	return &leido{activa: m.Active, hojas: m.Sheets, proteccion: protegerLibroDe(m.Protect), nombres: sanearNombres(m.Names)}, entendido(contenido, m.Sheets)
}

// leido es el documento ya interpretado, antes de montarlo en el libro.
type leido struct {
	activa     int
	hojas      []hojaLeida
	proteccion *ProteccionLibro
	nombres    []Nombre
}

// entendido son las tres comprobaciones de Cargar: ninguna clave desconocida, ninguna
// celda fuera del envoltorio canónico y ninguna fuera de la rejilla.
func entendido(contenido string, leidas []hojaLeida) bool {
	var libro map[string]json.RawMessage
	var hojas []map[string]json.RawMessage
	if json.Unmarshal([]byte(contenido), &libro) != nil || !soloClaves(libro, clavesModelo) ||
		json.Unmarshal(libro["sheets"], &hojas) != nil {
		return false
	}
	for _, h := range hojas {
		if !soloClaves(h, clavesHoja) {
			return false
		}
	}
	for _, h := range leidas {
		for ref, valor := range h.Cells {
			if !celdaEntendida(valor) || !enLaRejilla(ref) {
				return false
			}
		}
		// Un formato de una celda fuera de la rejilla se perdería al guardar.
		for ref := range h.Formats {
			if !enLaRejilla(ref) {
				return false
			}
		}
		for ref := range h.Styles {
			if !enLaRejilla(ref) {
				return false
			}
		}
	}
	return true
}

// poner monta el libro con lo leído. Se cargan TODAS las hojas en el motor, no solo la
// activa: una fórmula puede referirse a otra hoja y sin ellas daría #REF!.
func (c *Lienzo) poner(m leido) {
	nombres := make([]string, len(m.hojas))
	for i, h := range m.hojas {
		nombres[i] = h.Name
		if nombres[i] == "" {
			nombres[i] = c.t("sheet.defaultName", i18n.Vars{"n": i + 1}) // `sanitizeSheet`
		}
	}
	// Se sustituye el CONTENIDO del libro y no el puntero: la rejilla, la barra y las
	// pestañas lo tienen guardado, y un puntero nuevo las dejaría mirando el viejo.
	rotulo := c.Libro.rotulo
	*c.Libro = *NuevoLibroCon(nombres, clamp(m.activa, len(nombres)-1))
	c.Libro.rotulo = rotulo
	for i, h := range m.hojas {
		c.Libro.cargarHoja(i, h)
	}
	c.Libro.f.proteccion = m.proteccion
	c.Libro.f.nombres = m.nombres
	// El motor de NuevoLibroCon resuelve las estructuradas contra el libro TEMPORAL que
	// se acaba de copiar; se rehace sobre este, que ya tiene sus tablas.
	c.Libro.rehacerMotor()
	c.Rejilla.desX, c.Rejilla.desY = 0, 0
	c.Rejilla.Seleccionar(0, 0, false)
	c.Barra.Sincronizar()
}

// cargarHoja monta en la hoja i lo leído de su SheetData: las celdas, el formato, las
// reglas y los paneles, saneado como `sanitizeSheet`.
func (l *Libro) cargarHoja(i int, h hojaLeida) {
	dest := &l.f.hojas[i]
	for ref, valor := range h.Cells {
		if k, ok := celdaDe(ref); ok {
			l.CargarEn(i, k.C, k.F, valor)
		}
	}
	for ref, crudo := range h.Formats {
		if k, ok := celdaDe(ref); ok {
			if fm, ok := crudo.sanear(); ok {
				dest.formatos[k] = fm
			}
		}
	}
	for ref, crudo := range h.Styles {
		if k, ok := celdaDe(ref); ok {
			if e, ok := crudo.sanear(); ok {
				dest.estilos[k] = e
			}
		}
	}
	for _, vc := range h.Validations {
		if v, ok := vc.sanear(); ok {
			dest.validaciones = append(dest.validaciones, v)
		}
	}
	for _, cr := range h.CondFormats {
		if r, ok := cr.sanear(); ok {
			dest.condiciones = append(dest.condiciones, r)
		}
	}
	if h.Freeze != nil {
		dest.fijas = Fijas{Cols: clamp(enteroJS(h.Freeze.Cols), Columnas-1), Rows: clamp(enteroJS(h.Freeze.Rows), Filas-1)}
	}
	if h.Autofilter != nil {
		dest.filtro = h.Autofilter.sanear()
	}
	dest.proteccion = protegerDe(h.Protect)
	cargarObjetos(dest, h)
	cargarAnalisis(dest, h)
}

// cargarObjetos es la parte de `sanitizeSheet` de los objetos: cada uno saneado, y lo
// que no vale, fuera, como en la web.
func cargarObjetos(dest *hojaLibro, h hojaLeida) {
	for _, cr := range h.Sparklines {
		if m, ok := cr.sanear(); ok {
			dest.minis = append(dest.minis, m)
		}
	}
	for _, cr := range h.Tables {
		if t := cr.sanear(); t != nil {
			dest.tablas = append(dest.tablas, *t)
		}
	}
	for _, cr := range h.Charts {
		if g, ok := cr.sanear(); ok {
			dest.graficos = append(dest.graficos, g)
		}
	}
	for _, cr := range h.Slicers {
		if sg, ok := cr.sanear(); ok {
			dest.segs = append(dest.segs, sg)
		}
	}
}

// cargarAnalisis monta el esquema, los escenarios y las tablas dinámicas de la hoja.
func cargarAnalisis(dest *hojaLibro, h hojaLeida) {
	dest.anchos, dest.altos = sanearMedidas(h.ColWidths), sanearMedidas(h.RowHeights)
	dest.fusiones = h.Merges
	if reColorPestana.MatchString(h.TabColor) { // `sanitizeSheet`: solo un #RRGGBB
		dest.pestana = h.TabColor
	}
	// `sanitizeGroups`: vacío es «sin esquema» y no se guarda.
	if g := analisis.SanearGrupos(h.RowGroups, Filas); len(g) > 0 {
		dest.grupFilas = g
	}
	if g := analisis.SanearGrupos(h.ColGroups, Columnas); len(g) > 0 {
		dest.grupCols = g
	}
	for _, crudo := range h.Scenarios {
		// `sanitizeScenario`, sin repetir nombre.
		if e, ok := analisis.SanearEscenario(crudo); ok && !slices.ContainsFunc(dest.escenarios, func(x analisis.Escenario) bool { return x.Name == e.Name }) {
			dest.escenarios = append(dest.escenarios, e)
		}
	}
	for _, crudo := range h.Pivots {
		if d, ok := sanearDinamica(crudo); ok && !slices.ContainsFunc(dest.dinamicas, func(x analisis.Dinamica) bool { return x.Name == d.Name }) {
			dest.dinamicas = append(dest.dinamicas, d)
		}
	}
}

// sanearMedidas deja las medidas con índice entero y tamaño finito y positivo (lo que la
// web guarda); nil si no queda ninguna.
func sanearMedidas(m map[string]float64) map[string]float64 {
	var out map[string]float64
	for k, v := range m {
		if n, err := strconv.Atoi(k); err != nil || n < 0 || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			continue
		}
		if out == nil {
			out = map[string]float64{}
		}
		out[k] = v
	}
	return out
}

// celdaDe lee una referencia A1. Lo que no se lee ya lo filtró Cargar; aquí solo queda
// ser tolerante.
func celdaDe(ref string) (Celda, bool) {
	r, err := formula.ParseCellWord(ref, "")
	if err != nil {
		return Celda{}, false
	}
	return Celda{C: r.Col, F: r.Row}, true
}

// enteroJS es `Math.trunc(Number(v) || 0)`: un número, o una cadena con uno, truncado;
// cualquier otra cosa, 0.
func enteroJS(v any) int {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0
		}
		return int(math.Trunc(x))
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return int(math.Trunc(n))
	case bool:
		if x {
			return 1
		}
	}
	return 0
}

// Serializar devuelve el `SheetModel` para guardar. El cromo solo lo llama si el
// último Cargar dijo que sí, así que aquí no hay nada que no sepamos escribir.
//
// Escribe solo las celdas con contenido, que es lo que hace la web (`cells` son «solo
// celdas no vacías»), y cada una con su HTML: la que no se ha tocado sale con los
// mismos bytes con los que entró.
func (c *Lienzo) Serializar() string {
	m := modelo{Active: c.Libro.Activa(), Protect: c.Libro.f.proteccion, Names: c.Libro.f.nombres}
	for _, h := range c.Libro.f.hojas {
		m.Sheets = append(m.Sheets, h.serializar())
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "" // `modelo` es JSON puro: esto no puede pasar
	}
	return string(b)
}

// Estadisticas son las tres cifras de la barra de estado. En Calcus la unidad es la
// HOJA, y las palabras son 0, igual que en la web.
func (c *Lienzo) Estadisticas() (unidad, total, palabras int) {
	return c.Libro.Activa() + 1, len(c.Libro.Hojas()), 0
}

// soloClaves dice si el objeto no trae ninguna clave fuera de las conocidas.
func soloClaves(obj map[string]json.RawMessage, conocidas map[string]bool) bool {
	for k := range obj {
		if !conocidas[k] {
			return false
		}
	}
	return true
}

// enLaRejilla dice si una referencia A1 cae dentro de la rejilla de la web.
func enLaRejilla(ref string) bool {
	r, err := formula.ParseCellWord(ref, "")
	if err != nil {
		return false
	}
	return r.Col >= 0 && r.Col < Columnas && r.Row >= 0 && r.Row < Filas
}

// serializar es la hoja como la escribe la web: las celdas siempre, y los mapas de
// formato y los paneles solo si tienen algo.
func (h hojaLibro) serializar() hoja {
	out := hoja{Name: h.nombre, TabColor: h.pestana, Cells: make(map[string]string, len(h.celdas))}
	for k, v := range h.celdas {
		out.Cells[Ref(k.C, k.F)] = v
	}
	if len(h.formatos) > 0 {
		out.Formats = make(map[string]FormatoNum, len(h.formatos))
		for k, v := range h.formatos {
			out.Formats[Ref(k.C, k.F)] = v
		}
	}
	if len(h.estilos) > 0 {
		out.Styles = make(map[string]Estilo, len(h.estilos))
		for k, v := range h.estilos {
			out.Styles[Ref(k.C, k.F)] = v
		}
	}
	if h.fijas != (Fijas{}) {
		f := h.fijas
		out.Freeze = &f
	}
	out.CondFormats, out.Validations, out.Autofilter, out.Protect = h.condiciones, h.validaciones, h.filtro, h.proteccion
	out.RowGroups, out.ColGroups, out.Pivots, out.Scenarios = h.grupFilas, h.grupCols, h.dinamicas, h.escenarios
	out.Sparklines, out.Tables, out.Charts, out.Slicers = h.minis, h.tablas, h.graficos, h.segs
	out.ColWidths, out.RowHeights, out.Merges = h.anchos, h.altos, h.fusiones
	return out
}
