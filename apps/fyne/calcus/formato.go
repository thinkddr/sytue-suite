// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"regexp"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/numfmt"
)

// # El formato de una celda vive FUERA de su HTML
//
// Es el criterio de la web (`SheetData.formats` y `SheetData.styles`): el texto crudo de
// la celda sigue siendo la fuente de verdad y el formato solo se aplica al PINTAR. La
// negrita, la cursiva y la alineación horizontal son la excepción histórica y van en el
// envoltorio del HTML (`celda.go`); todo lo demás —formato de número, fuente, color,
// relleno, bordes— va en estos dos mapas.
//
// Este fichero es el puerto de lo que `sheets/format.ts` hace con ellos: los presets
// de número en es-ES, el saneado de lo que llega del documento y el catálogo de fuentes
// y tamaños de la cinta.

// FormatoNum es `NumFmt`: el formato de número de una celda.
type FormatoNum struct {
	ID string `json:"id"`
	// Decimales es `decimals`: nil son los del preset.
	Decimales *int `json:"decimals,omitempty"`
	// Mascara es `mask`, solo con `custom`: una máscara al estilo de Excel.
	Mascara string `json:"mask,omitempty"`
}

// Los ids de NUMFMT_IDS, en su orden.
var idsFormato = []string{"general", "number", "thousands", "currency", "percent", "date", "custom"}

// PresetsFormato es NUMFMT_PRESETS: lo que ofrece el selector de la cinta, con su clave
// de catálogo. `custom` no está: se escribe en su propio menú.
var PresetsFormato = []struct{ ID, Clave string }{
	{"general", "numfmt.general"},
	{"number", "numfmt.number"},
	{"thousands", "sheet.mask.thousands"},
	{"currency", "numfmt.currency"},
	{"percent", "sheet.mask.percent"},
	{"date", "numfmt.date"},
}

// decimalesPorDefecto es DEFAULT_DECIMALS (criterio de Excel).
func decimalesPorDefecto(id string) int {
	switch id {
	case "number", "thousands", "currency":
		return 2
	}
	return 0
}

// decimales son los del formato o, sin ellos, los del preset.
func (f FormatoNum) decimales() int {
	if f.Decimales != nil {
		return *f.Decimales
	}
	return decimalesPorDefecto(f.ID)
}

// formatoCrudo es un NumFmt tal y como llega del documento, sin fiarse de los tipos.
type formatoCrudo struct {
	ID        string   `json:"id"`
	Decimales *float64 `json:"decimals"`
	Mascara   *string  `json:"mask"`
}

// sanear es `sanitizeNumFmt`: false si no es un formato. `custom` exige una máscara no
// vacía (hasta 200 caracteres) y los decimales se truncan a 0..8.
func (c formatoCrudo) sanear() (FormatoNum, bool) {
	if !contiene(idsFormato, c.ID) {
		return FormatoNum{}, false
	}
	if c.ID == "custom" {
		if c.Mascara == nil || strings.TrimSpace(*c.Mascara) == "" {
			return FormatoNum{}, false
		}
		return FormatoNum{ID: "custom", Mascara: cortarUTF16(*c.Mascara, 200)}, true
	}
	f := FormatoNum{ID: c.ID}
	if c.Decimales != nil && !math.IsNaN(*c.Decimales) && !math.IsInf(*c.Decimales, 0) {
		d := min(8, max(0, int(math.Trunc(*c.Decimales))))
		f.Decimales = &d
	}
	return f, true
}

// textoConFormato es `formatCellValue`: los números pasan por el formato (o por el
// General del motor) y el resto —texto, booleano, error— sale tal cual. El color es el
// de un `[Rojo]` de máscara, o "".
func textoConFormato(v formula.Value, f *FormatoNum) (texto, color string) {
	n, ok := v.(float64)
	if !ok || f == nil {
		return formula.Display(v), ""
	}
	return aplicarFormato(*f, n)
}

// aplicarFormato es `applyNumFmt` (más el color de `numFmtInnerHtml`). Vive en
// `apps/nucleo/numfmt` desde que el servicio exporta las hojas con sus valores: el mismo
// número tiene que salir igual en la celda y en la descarga.
func aplicarFormato(f FormatoNum, n float64) (texto, color string) {
	return numfmt.Aplicar(f.ID, f.decimales(), f.Mascara, n)
}

// Lo de las máscaras que el lienzo usa suelto.
var (
	conMascara  = numfmt.ConMascara
	redondearJS = numfmt.RedondearJS
)

// epocaFechas es el serial 0 del sistema de fechas 1900, el 31-12-1899: lo usa la
// validación de fechas tecleadas.
var epocaFechas = time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)

// ---- el formato visual ----

// Estilo es `CellStyle`: el formato visual de una celda. Es un VALOR: se sustituye
// entero, nunca se toca en su sitio, porque las fotos de deshacer comparten los
// punteros de dentro.
type Estilo struct {
	Fuente   string `json:"font,omitempty"`
	Tam      int    `json:"size,omitempty"` // en pt
	Tinta    string `json:"color,omitempty"`
	Relleno  string `json:"fill,omitempty"`
	Subraya  bool   `json:"u,omitempty"`
	Tacha    bool   `json:"s,omitempty"`
	Vertical string `json:"valign,omitempty"` // "top", "middle" o "bottom"
	Ajustar  bool   `json:"wrap,omitempty"`
	Bordes   Bordes `json:"border,omitzero"`
	// Bloqueada es `locked`: solo se guarda el `false` (desbloqueada), que es lo único
	// que dice algo con la hoja protegida. Oculta es `hidden`: esconder la fórmula.
	Bloqueada *bool `json:"locked,omitempty"`
	Oculta    bool  `json:"hidden,omitempty"`
	// Tema no es del editor sino del servicio: dice que esta celda la vistió la marca de la
	// organización, para que «Actualizar a la marca actual» la encuentre. Se conserva tal cual.
	Tema string `json:"tema,omitempty"`
}

// Bordes son los lados de `CellBorder` y, de cada uno, su color (TC…, #RRGGBB) y su trazo
// (TS…, uno de Trazos): los trae un libro importado; vacíos, la línea fina de serie.
type Bordes struct {
	T  bool   `json:"t,omitempty"`
	R  bool   `json:"r,omitempty"`
	B  bool   `json:"b,omitempty"`
	L  bool   `json:"l,omitempty"`
	TC string `json:"tc,omitempty"`
	RC string `json:"rc,omitempty"`
	BC string `json:"bc,omitempty"`
	LC string `json:"lc,omitempty"`
	TS string `json:"ts,omitempty"`
	RS string `json:"rs,omitempty"`
	BS string `json:"bs,omitempty"`
	LS string `json:"ls,omitempty"`
}

// Trazos son los de `BorderStroke`: los que se pintan además de la línea fina.
var Trazos = []string{"medium", "thick", "dashed", "dotted", "double"}

// bordesDe es `sanitizeBorder`: los lados que valen `true` y, de esos, el color `#RRGGBB` y
// el trazo conocido.
func bordesDe(m map[string]any) Bordes {
	lado := func(k string) (bool, string, string) {
		if m[k] != true {
			return false, "", ""
		}
		c, _ := m[k+"c"].(string)
		if !reHex.MatchString(c) {
			c = ""
		}
		s, _ := m[k+"s"].(string)
		if !contiene(Trazos, s) {
			s = ""
		}
		return true, c, s
	}
	var b Bordes
	b.T, b.TC, b.TS = lado("t")
	b.R, b.RC, b.RS = lado("r")
	b.B, b.BC, b.BS = lado("b")
	b.L, b.LC, b.LS = lado("l")
	return b
}

// crudo son los bordes como los recibe bordesDe.
func (b Bordes) crudo() map[string]any {
	m := map[string]any{"t": b.T, "r": b.R, "b": b.B, "l": b.L}
	for k, v := range map[string]string{"tc": b.TC, "rc": b.RC, "bc": b.BC, "lc": b.LC, "ts": b.TS, "rs": b.RS, "bs": b.BS, "ls": b.LS} {
		if v != "" {
			m[k] = v
		}
	}
	return m
}

// vacio dice si el estilo no lleva nada, que es cuando la web lo quita del mapa.
func (e Estilo) vacio() bool { return e == Estilo{} }

// Fuentes es FONT_STACKS, en su orden: las seis de la cinta. La primera es lo que
// enseña el selector cuando la celda no tiene fuente.
var Fuentes = []string{"Inter", "Arial", "Georgia", "Times New Roman", "Courier New", "JetBrains Mono"}

// TamanosFuente es FONT_SIZES, en pt.
var TamanosFuente = []int{8, 9, 10, 11, 12, 14, 16, 18, 20, 24, 28, 36, 48, 72}

var reHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// estiloCrudo es un CellStyle tal y como llega, sin fiarse de los tipos: un `size` con
// decimales, un `u` que no es booleano… La web no rechaza el documento por eso: se
// queda con lo que vale.
type estiloCrudo struct {
	Fuente   any `json:"font"`
	Tam      any `json:"size"`
	Tinta    any `json:"color"`
	Relleno  any `json:"fill"`
	Subraya  any `json:"u"`
	Tacha    any `json:"s"`
	Vertical any `json:"valign"`
	Ajustar  any `json:"wrap"`
	Bordes   any `json:"border"`
	Bloq     any `json:"locked"`
	Oculta   any `json:"hidden"`
	Tema     any `json:"tema"`
}

// sanear es `sanitizeCellStyle`: solo familias del catálogo, colores `#RRGGBB`, el
// tamaño truncado a 6..96 y los booleanos que valen `true` (o el `false` de `locked`).
// false si no queda nada.
func (c estiloCrudo) sanear() (Estilo, bool) {
	var e Estilo
	if s, ok := c.Fuente.(string); ok && contiene(Fuentes, s) {
		e.Fuente = s
	}
	if n, ok := c.Tam.(float64); ok && !math.IsNaN(n) && !math.IsInf(n, 0) {
		e.Tam = min(96, max(6, int(math.Trunc(n))))
	}
	if s, ok := c.Tinta.(string); ok && reHex.MatchString(s) {
		e.Tinta = s
	}
	if s, ok := c.Relleno.(string); ok && reHex.MatchString(s) {
		e.Relleno = s
	}
	e.Subraya, e.Tacha, e.Ajustar, e.Oculta = c.Subraya == true, c.Tacha == true, c.Ajustar == true, c.Oculta == true
	if s, ok := c.Vertical.(string); ok && (s == "top" || s == "middle" || s == "bottom") {
		e.Vertical = s
	}
	if m, ok := c.Bordes.(map[string]any); ok {
		e.Bordes = bordesDe(m)
	}
	if c.Bloq == false {
		f := false
		e.Bloqueada = &f
	}
	if e.vacio() {
		return e, false
	}
	// `tema` solo acompaña a un estilo de verdad, como en la web (`sanitizeCellStyle` y
	// luego la marca): una celda sin más que la marca no gana estilo.
	e.Tema, _ = c.Tema.(string)
	return e, true
}

// crudo devuelve el estilo como lo recibiría `sanear`, para aplicarle un parche.
func (e Estilo) crudo() estiloCrudo {
	c := estiloCrudo{}
	if e.Fuente != "" {
		c.Fuente = e.Fuente
	}
	if e.Tam != 0 {
		c.Tam = float64(e.Tam)
	}
	if e.Tinta != "" {
		c.Tinta = e.Tinta
	}
	if e.Relleno != "" {
		c.Relleno = e.Relleno
	}
	c.Subraya, c.Tacha, c.Ajustar, c.Oculta = e.Subraya, e.Tacha, e.Ajustar, e.Oculta
	if e.Vertical != "" {
		c.Vertical = e.Vertical
	}
	if e.Bordes != (Bordes{}) {
		c.Bordes = e.Bordes.crudo()
	}
	if e.Bloqueada != nil {
		c.Bloq = *e.Bloqueada
	}
	return c
}

// Parche es el `Partial<CellStyle>` del comando `style`: cada campo que se pone
// sustituye al del estilo; Quitar* lo borra (el `null` de la web).
type Parche func(*estiloCrudo)

// Los parches que manda la cinta.
func conTinta(s string) Parche   { return func(c *estiloCrudo) { c.Tinta = s } }
func conRelleno(s string) Parche { return func(c *estiloCrudo) { c.Relleno = s } }
func conSubraya(b bool) Parche   { return func(c *estiloCrudo) { c.Subraya = b } }
func conTacha(b bool) Parche     { return func(c *estiloCrudo) { c.Tacha = b } }
func conBordes(b *Bordes) Parche {
	return func(c *estiloCrudo) {
		if b == nil {
			c.Bordes = nil
			return
		}
		c.Bordes = b.crudo()
	}
}

// fusionar es `mergeCellStyle`: el parche sobre el estilo y, otra vez, el saneado.
func fusionar(base Estilo, p Parche) (Estilo, bool) {
	c := base.crudo()
	p(&c)
	return c.sanear()
}

// ---- utilidades ----

func contiene(lista []string, s string) bool {
	for _, x := range lista {
		if x == s {
			return true
		}
	}
	return false
}

// cortarUTF16 es `s.slice(0, n)` de JavaScript: n unidades UTF-16, no bytes. Sin
// partir un par sustituto, que JavaScript sí partiría y dejaría un carácter roto.
func cortarUTF16(s string, n int) string {
	u := 0
	for i, r := range s {
		ancho := 1
		if r > 0xffff {
			ancho = 2
		}
		if u+ancho > n {
			return s[:i]
		}
		u += ancho
	}
	return s
}
