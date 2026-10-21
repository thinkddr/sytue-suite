// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # El formato condicional
//
// Es `condFormats` de SheetData y `buildCondDecor` de `sheets/format.ts`: una lista de
// reglas en orden de PRIORIDAD que deciden, celda a celda, un relleno, una barra de
// datos o un icono. Cada canal lo pone la primera regla que lo produce; el relleno
// condicional gana al del estilo.

// ReglaCond es `CondRule`: los campos de todas las clases de regla, cada una usa los
// suyos. `kind` vacío es la regla de celda de siempre ({range, op, value, fill}).
type ReglaCond struct {
	Rango   string   `json:"range"`
	Clase   string   `json:"kind,omitempty"`
	Op      string   `json:"op,omitempty"`
	Valor   *string  `json:"value,omitempty"`
	Relleno string   `json:"fill,omitempty"`
	Formula string   `json:"formula,omitempty"`
	Puesto  int      `json:"rank,omitempty"`
	Porcien bool     `json:"percent,omitempty"`
	Color   string   `json:"color,omitempty"`
	Escala  []string `json:"scale,omitempty"`
	Iconos  string   `json:"icons,omitempty"`
	// Tinta es `textColor`: el color de letra de una regla que pinta, con relleno o sin él.
	Tinta string `json:"textColor,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
}

// pinta dice si una regla de las que pintan la celda (no barras, escalas ni iconos) tiene
// algo que pintar: el relleno o la letra.
func (r ReglaCond) pinta() bool { return r.Relleno != "" || r.Tinta != "" }

var (
	clasesCond = []string{"cell", "formula", "top", "bottom", "dup", "unique", "dataBar", "colorScale", "iconSet"}
	opsCond    = []string{"gt", "ge", "lt", "le", "eq", "ne", "contains"}
	juegosIcon = []string{"arrows", "traffic", "symbols"}
)

// reglaCruda es una CondRule como llega del documento.
type reglaCruda struct {
	Rango   any `json:"range"`
	Clase   any `json:"kind"`
	Op      any `json:"op"`
	Valor   any `json:"value"`
	Relleno any `json:"fill"`
	Formula any `json:"formula"`
	Puesto  any `json:"rank"`
	Porcien any `json:"percent"`
	Color   any `json:"color"`
	Escala  any `json:"scale"`
	Iconos  any `json:"icons"`
	Tinta   any `json:"textColor"` //nolint:tagliatelle // el nombre lo fija la web
}

// sanear es `sanitizeCondRule`: false si la regla no vale. Cada clase se queda solo con
// sus campos, así el JSON que se vuelve a escribir es el mínimo, como el de la web.
func (c reglaCruda) sanear() (ReglaCond, bool) {
	rango, ok := c.Rango.(string)
	if !ok {
		return ReglaCond{}, false
	}
	if _, ok := formula.ParseRangeRef(rango); !ok {
		return ReglaCond{}, false
	}
	clase, _ := c.Clase.(string)
	if !contiene(clasesCond, clase) {
		clase = "cell"
	}
	return c.sanearClase(clase, ReglaCond{Rango: rango})
}

// sanearClase valida los campos de cada clase de regla y se queda solo con los suyos.
// Las de relleno van por sanearRelleno; las demás, aquí.
func (c reglaCruda) sanearClase(clase string, r ReglaCond) (ReglaCond, bool) {
	switch clase {
	case "cell", "formula", "top", "bottom", "dup", "unique":
		r, ok := c.sanearRelleno(clase, r)
		if !ok {
			return ReglaCond{}, false
		}
		// Lo que pinta: el relleno, la letra o los dos (`withFill`).
		if s, ok := c.Relleno.(string); ok && reHex.MatchString(s) {
			r.Relleno = s
		}
		if s, ok := c.Tinta.(string); ok && reHex.MatchString(s) {
			r.Tinta = s
		}
		return r, r.pinta()
	case "dataBar":
		s, ok := c.Color.(string)
		if !ok || !reHex.MatchString(s) {
			return ReglaCond{}, false
		}
		r.Clase, r.Color = clase, s
		return r, true
	case "colorScale":
		lista, _ := c.Escala.([]any)
		for _, x := range lista {
			if s, ok := x.(string); ok && reHex.MatchString(s) {
				r.Escala = append(r.Escala, s)
			}
		}
		if len(r.Escala) != 2 && len(r.Escala) != 3 {
			return ReglaCond{}, false
		}
		r.Clase = clase
		return r, true
	}
	// iconSet
	r.Clase, r.Iconos = clase, "arrows"
	if s, ok := c.Iconos.(string); ok && contiene(juegosIcon, s) {
		r.Iconos = s
	}
	return r, true
}

// sanearRelleno son los campos propios de las clases que rellenan (el relleno lo pone
// quien llama). La clase «cell» no se escribe: es la de por defecto.
func (c reglaCruda) sanearRelleno(clase string, r ReglaCond) (ReglaCond, bool) {
	switch clase {
	case "cell":
		v, okV := c.Valor.(string)
		op, okOp := c.Op.(string)
		if !okV || !okOp || !contiene(opsCond, op) {
			return ReglaCond{}, false
		}
		r.Op, r.Valor = op, &v
	case "formula":
		f, ok := c.Formula.(string)
		if !ok || strings.TrimSpace(f) == "" {
			return ReglaCond{}, false
		}
		r.Clase, r.Formula = clase, strings.TrimSpace(f)
	case "top", "bottom":
		n := numeroJS(c.Puesto)
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) <= 0 {
			return ReglaCond{}, false
		}
		r.Clase, r.Porcien, r.Puesto = clase, c.Porcien == true, int(math.Trunc(n))
		tope := 1000
		if r.Porcien {
			tope = 100
		}
		r.Puesto = min(tope, r.Puesto)
	default: // dup, unique: sin campos propios
		r.Clase = clase
	}
	return r, true
}

// numeroJS es `Number(v)`: un número, o una cadena que lo es; si no, NaN.
func numeroJS(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		if n, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return n
		}
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return math.NaN()
}

var reNoNumerico = regexp.MustCompile(`[^0-9.,-]`)

// numeroES es `parseEsNumber`: el número que escribió una persona en es-ES. Coma
// decimal si la hay; si no, punto decimal («1.234» se lee 1,234).
func numeroTecleado(texto string) (float64, bool) {
	limpio := reNoNumerico.ReplaceAllString(texto, "")
	if limpio == "" || limpio == "-" {
		return 0, false
	}
	if strings.LastIndex(limpio, ",") > strings.LastIndex(limpio, ".") {
		limpio = strings.ReplaceAll(strings.ReplaceAll(limpio, ".", ""), ",", ".")
	} else {
		limpio = strings.ReplaceAll(limpio, ",", "")
	}
	n, err := strconv.ParseFloat(limpio, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, false
	}
	return n, true
}

// cumple es `condMatches`: la regla de celda sobre su VALOR. Los errores y las celdas
// vacías nunca cumplen.
func (r ReglaCond) cumple(v formula.Value) bool {
	if r.Op == "" || r.Valor == nil || formula.IsError(v) || v == "" {
		return false
	}
	texto := formula.Display(v)
	if r.Op == "contains" {
		return strings.Contains(strings.ToLower(texto), strings.ToLower(strings.TrimSpace(*r.Valor)))
	}
	if n, esNum := v.(float64); esNum {
		if rv, ok := numeroTecleado(*r.Valor); ok {
			switch r.Op {
			case "gt":
				return n > rv
			case "ge":
				return n >= rv
			case "lt":
				return n < rv
			case "le":
				return n <= rv
			case "eq":
				return n == rv
			case "ne":
				return n != rv
			}
			return false
		}
	}
	if r.Op == "eq" || r.Op == "ne" {
		igual := strings.EqualFold(strings.TrimSpace(texto), strings.TrimSpace(*r.Valor))
		return igual == (r.Op == "eq")
	}
	return false
}

// barraDatos es `CellDataBar`.
type barraDatos struct {
	frac, eje float64
	neg       bool
	color     string
}

// decoracion es `CellDecor`: lo que el formato condicional le pone a una celda.
type decoracion struct {
	relleno string
	tinta   string // el color de letra
	barra   *barraDatos
	icono   string // «juego:nivel»
}

// decorar es `buildCondDecor`: la decoración de cada celda de la hoja activa según sus
// reglas, en orden de prioridad. `evalFormula` evalúa una regla de fórmula desplazada a
// una celda (la pone el libro, con una celda de apoyo del motor).
func decorar(reglas []ReglaCond, valor func(c, f int) formula.Value, evalFormula func(f string, dc, df int) bool) map[Celda]*decoracion {
	d := decorador{mapa: map[Celda]*decoracion{}, valor: valor, evalFormula: evalFormula}
	for _, r := range reglas {
		rect, ok := formula.ParseRangeRef(r.Rango)
		if !ok {
			continue
		}
		// Acotado a la rejilla, como los demás recorridos (gráficos, dinámicas…): una
		// regla sobre A1:A9999999 recorría diez millones de celdas en cada edición (FYE-03).
		d.rect = formula.Rect{C1: min(rect.C1, Columnas-1), R1: min(rect.R1, Filas-1), C2: min(rect.C2, Columnas-1), R2: min(rect.R2, Filas-1)}
		switch r.Clase {
		case "", "cell":
			d.celda(r)
		case "formula":
			d.formula(r)
		case "top", "bottom":
			d.extremos(r)
		case "dup", "unique":
			d.repetidos(r)
		case "colorScale":
			d.escala(r)
		case "dataBar":
			d.barras(r)
		case "iconSet":
			d.iconos(r)
		}
	}
	return d.mapa
}

// decorador es lo que comparten las clases de regla mientras se decora: el mapa, cómo
// se lee un valor y el rango de la regla en curso.
type decorador struct {
	mapa        map[Celda]*decoracion
	valor       func(c, f int) formula.Value
	evalFormula func(f string, dc, df int) bool
	rect        formula.Rect
}

func (d *decorador) en(c, f int) *decoracion {
	k := Celda{C: c, F: f}
	x := d.mapa[k]
	if x == nil {
		x = &decoracion{}
		d.mapa[k] = x
	}
	return x
}

// rellenar pone el relleno si ninguna regla anterior lo ha puesto: la primera gana.
func (d *decorador) rellenar(c, f int, color string) {
	if x := d.en(c, f); x.relleno == "" {
		x.relleno = color
	}
}

// pintar es lo que pone una regla que cumple: su relleno y su color de letra, cada canal el
// de la primera regla que lo da.
func (d *decorador) pintar(c, f int, r ReglaCond) {
	if r.Relleno != "" {
		d.rellenar(c, f, r.Relleno)
	}
	if x := d.en(c, f); r.Tinta != "" && x.tinta == "" {
		x.tinta = r.Tinta
	}
}

func (d *decorador) recorrer(fn func(c, f int)) {
	for f := d.rect.R1; f <= d.rect.R2; f++ {
		for c := d.rect.C1; c <= d.rect.C2; c++ {
			fn(c, f)
		}
	}
}

func (d *decorador) numero(c, f int) (float64, bool) {
	n, ok := d.valor(c, f).(float64)
	return n, ok && !math.IsNaN(n) && !math.IsInf(n, 0)
}

// numeros son todos los del rango, para los mínimos, los máximos y los puestos.
func (d *decorador) numeros() []float64 {
	var out []float64
	d.recorrer(func(c, f int) {
		if n, ok := d.numero(c, f); ok {
			out = append(out, n)
		}
	})
	return out
}

func (d *decorador) celda(r ReglaCond) {
	d.recorrer(func(c, f int) {
		if r.pinta() && r.cumple(d.valor(c, f)) {
			d.pintar(c, f, r)
		}
	})
}

func (d *decorador) formula(r ReglaCond) {
	if r.Formula == "" || !r.pinta() {
		return
	}
	d.recorrer(func(c, f int) {
		if d.evalFormula(r.Formula, c-d.rect.C1, f-d.rect.R1) {
			d.pintar(c, f, r)
		}
	})
}

// extremos son los N (o el N %) superiores o inferiores, con los empates del umbral,
// como Excel.
func (d *decorador) extremos(r ReglaCond) {
	nums := d.numeros()
	if !r.pinta() || r.Puesto == 0 || len(nums) == 0 {
		return
	}
	slices.Sort(nums)
	if r.Clase == "top" {
		slices.Reverse(nums)
	}
	cuantos := min(r.Puesto, len(nums))
	if r.Porcien {
		cuantos = max(1, int(math.Ceil(float64(r.Puesto)/100*float64(len(nums)))))
	}
	umbral := nums[cuantos-1]
	d.recorrer(func(c, f int) {
		if n, ok := d.numero(c, f); ok && ((r.Clase == "top" && n >= umbral) || (r.Clase == "bottom" && n <= umbral)) {
			d.pintar(c, f, r)
		}
	})
}

// repetidos son los duplicados o los únicos, comparando lo que se VE.
func (d *decorador) repetidos(r ReglaCond) {
	if !r.pinta() {
		return
	}
	cuenta := map[string]int{}
	d.recorrer(func(c, f int) {
		if t := formula.Display(d.valor(c, f)); t != "" {
			cuenta[t]++
		}
	})
	d.recorrer(func(c, f int) {
		t := formula.Display(d.valor(c, f))
		if n := cuenta[t]; t != "" && ((r.Clase == "dup" && n > 1) || (r.Clase == "unique" && n == 1)) {
			d.pintar(c, f, r)
		}
	})
}

// escala es la escala de colores: el mínimo del rango en el primero, el máximo en el
// último y lo de en medio interpolado.
func (d *decorador) escala(r ReglaCond) {
	nums := d.numeros()
	if len(nums) == 0 || len(r.Escala) < 2 {
		return
	}
	lo, hi := slices.Min(nums), slices.Max(nums)
	d.recorrer(func(c, f int) {
		n, ok := d.numero(c, f)
		if !ok {
			return
		}
		t := 0.5
		if hi != lo {
			t = (n - lo) / (hi - lo)
		}
		d.rellenar(c, f, colorEnEscala(r.Escala, t))
	})
}

// barras son las barras de datos: proporcionales al mayor |valor| del rango, con el
// cero de eje.
func (d *decorador) barras(r ReglaCond) {
	nums := d.numeros()
	if len(nums) == 0 || r.Color == "" {
		return
	}
	lo, hi := min(0, slices.Min(nums)), max(0, slices.Max(nums))
	den := max(math.Abs(lo), math.Abs(hi))
	if den == 0 {
		den = 1
	}
	eje := 0.0
	if hi != lo {
		eje = -lo / (hi - lo) // dónde cae el cero, de 0 a 1
	}
	d.recorrer(func(c, f int) {
		n, ok := d.numero(c, f)
		if !ok || n == 0 {
			return
		}
		if x := d.en(c, f); x.barra == nil {
			x.barra = &barraDatos{frac: math.Abs(n) / den, eje: eje, neg: n < 0, color: r.Color}
		}
	})
}

// iconos son los juegos de iconos: el nivel por tercios entre el mínimo y el máximo.
func (d *decorador) iconos(r ReglaCond) {
	nums := d.numeros()
	if len(nums) == 0 {
		return
	}
	juego := r.Iconos
	if juego == "" {
		juego = "arrows"
	}
	lo, hi := slices.Min(nums), slices.Max(nums)
	d.recorrer(func(c, f int) {
		n, ok := d.numero(c, f)
		if !ok {
			return
		}
		t := 1.0
		if hi != lo {
			t = (n - lo) / (hi - lo)
		}
		nivel := 0
		switch {
		case t >= 2.0/3:
			nivel = 2
		case t >= 1.0/3:
			nivel = 1
		}
		if x := d.en(c, f); x.icono == "" {
			x.icono = juego + ":" + strconv.Itoa(nivel)
		}
	})
}

// colorEnEscala es `colorScaleAt`: el color de una escala de 2 o 3 colores en t ∈ [0, 1].
func colorEnEscala(escala []string, t float64) string {
	t = min(max(t, 0), 1)
	if len(escala) >= 3 {
		if t <= 0.5 {
			return mezclarHex(escala[0], escala[1], t/0.5)
		}
		return mezclarHex(escala[1], escala[2], (t-0.5)/0.5)
	}
	return mezclarHex(escala[0], escala[len(escala)-1], t)
}

// mezclarHex es `lerpHex`: interpola dos #RRGGBB, con el redondeo de `Math.round`.
func mezclarHex(a, b string, t float64) string {
	ca, cb := hexRGB(a), hexRGB(b)
	t = min(max(t, 0), 1)
	out := "#"
	for i := range 3 {
		v := min(max(float64(ca[i])+(float64(cb[i])-float64(ca[i]))*t, 0), 255)
		h := strconv.FormatInt(int64(redondearJS(v)), 16)
		if len(h) < 2 {
			h = "0" + h
		}
		out += h
	}
	return out
}

func hexRGB(s string) [3]int64 {
	var out [3]int64
	for i := range out {
		out[i], _ = strconv.ParseInt(s[1+2*i:3+2*i], 16, 64)
	}
	return out
}

// svgIcono es `iconSetSvg`: el icono de un juego (12×12) en SVG, los mismos trazos que
// la web.
func svgIcono(icono string) string {
	juego, nivelTxt, _ := strings.Cut(icono, ":")
	nivel, _ := strconv.Atoi(nivelTxt)
	nivel = min(max(nivel, 0), 2)
	col := [...]string{"#DC2626", "#D97706", "#16A34A"}[nivel] // bajo, medio, alto
	var dentro string
	switch juego {
	case "traffic":
		dentro = `<circle cx="6" cy="6" r="4.5" fill="` + col + `"/>`
	case "symbols":
		trazo := `fill="none" stroke="` + col + `" stroke-width="1.8" stroke-linecap="round"`
		dentro = [...]string{
			`<path d="M3 3l6 6M9 3l-6 6" ` + trazo + `/>`,
			`<path d="M2.5 6h7" ` + trazo + `/>`,
			`<path d="M2.5 6.3 5 8.8 9.5 3.5" ` + trazo + ` stroke-linejoin="round"/>`,
		}[nivel]
	default:
		dentro = [...]string{
			`<path d="M2 3.5h8L6 9.5Z" fill="` + col + `"/>`,
			`<path d="M2.5 3.5 9.5 6 2.5 8.5Z" fill="` + col + `"/>`,
			`<path d="M6 2.5 10 8.5H2Z" fill="` + col + `"/>`,
		}[nivel]
	}
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 12 12" width="12" height="12">` + dentro + `</svg>`
}
