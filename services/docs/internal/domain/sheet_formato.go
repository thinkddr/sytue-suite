// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// El FORMATO de una hoja de Calcus —estilos de celda, formato condicional y validaciones
// de datos— tal y como lo guarda el editor (`CellStyle`, `CondRule` y `DataValidation` de
// `packages/office/src/editor/types.ts`). El servicio no lo interpreta al guardar (doc:sheet es
// un blob), pero sí al exportar a .xlsx/.ods y al importar de ellos: ahí se traduce a lo
// que entienden Excel y LibreOffice (cellXfs, dxfs, conditionalFormatting, dataValidations
// en OOXML; estilos de celda, calcext:conditional-formats y table:content-validations en
// ODF) y de vuelta.

// Topes al importar (entrada no confiable): un fichero hostil no hace crecer sin medida
// el modelo ni escribe un millón de celdas vacías por un relleno de columna entera.
const (
	maxEstilosHoja      = 20000
	maxReglasHoja       = 2000
	maxValidacionesHoja = 2000
	// maxCeldasVacias acota las celdas vacías que se escriben solo por su formato.
	maxCeldasVacias = 20000
	// Límites de la rejilla de Excel: lo que no cabe no se escribe.
	maxFilasHoja    = 1048576
	maxColumnasHoja = 16384
)

// formatoHoja es el formato de una SheetData; va embebido en sheetData.
type formatoHoja struct {
	Styles      map[string]cellStyle `json:"styles,omitempty"`
	CondFormats []condRule           `json:"condFormats,omitempty"` //nolint:tagliatelle // el nombre lo fija el editor
	Validations []dataValidation     `json:"validations,omitempty"`
	medidasHoja
	esquemaHoja
	escenariosHoja
	combinadasHoja
}

type cellBorder struct {
	T bool `json:"t,omitempty"`
	R bool `json:"r,omitempty"`
	B bool `json:"b,omitempty"`
	L bool `json:"l,omitempty"`
	// TC… son el color de cada lado (#RRGGBB) y TS… su trazo (estilosBorde); vacíos, el
	// color de serie del editor y la línea fina. Son de 2026-10: un borde de antes no los
	// lleva y se ve como siempre.
	TC string `json:"tc,omitempty"`
	RC string `json:"rc,omitempty"`
	BC string `json:"bc,omitempty"`
	LC string `json:"lc,omitempty"`
	TS string `json:"ts,omitempty"`
	RS string `json:"rs,omitempty"`
	BS string `json:"bs,omitempty"`
	LS string `json:"ls,omitempty"`
}

// estilosBorde son los trazos de un borde que el editor pinta además de la línea fina:
// en CSS, 2px y 3px sólidos, discontinuo, de puntos y doble.
var estilosBorde = map[string]bool{"medium": true, "thick": true, "dashed": true, "dotted": true, "double": true}

// trazoDeXlsx es el trazo del editor de un `style` de borde de OOXML (ECMA-376 §18.18.3):
// los discontinuos de cualquier grosor son «dashed» y el «hair», de puntos; "" la línea fina.
func trazoDeXlsx(s string) string {
	switch s {
	case "medium", "thick", "dashed", "dotted", "double":
		return s
	case "hair":
		return "dotted"
	case "mediumDashed", "dashDot", "mediumDashDot", "dashDotDot", "mediumDashDotDot", "slantDashDot":
		return "dashed"
	}
	return ""
}

// cellStyle es `CellStyle`. La protección de celda (`locked`, `hidden`) la cumple el servidor
// (proteccion.go) y viaja en cellXfs (`<protection>`) y en `style:cell-protect`.
type cellStyle struct {
	Font   string      `json:"font,omitempty"`
	Size   int         `json:"size,omitempty"`
	Color  string      `json:"color,omitempty"`
	Fill   string      `json:"fill,omitempty"`
	U      bool        `json:"u,omitempty"`
	S      bool        `json:"s,omitempty"`
	Valign string      `json:"valign,omitempty"`
	Wrap   bool        `json:"wrap,omitempty"`
	Border *cellBorder `json:"border,omitempty"`
	// Locked es false en una celda desbloqueada (ausente = bloqueada, como en Excel); Hidden
	// oculta su fórmula con la hoja protegida.
	Locked *bool `json:"locked,omitempty"`
	Hidden bool  `json:"hidden,omitempty"`
}

func (s cellStyle) vacio() bool {
	return s.Font == "" && s.Size == 0 && s.Color == "" && s.Fill == "" && !s.U && !s.S &&
		s.Valign == "" && !s.Wrap && (s.Border == nil || *s.Border == cellBorder{}) &&
		s.Locked == nil && !s.Hidden
}

// desbloqueada: la celda se puede editar con la hoja protegida.
func (s cellStyle) desbloqueada() bool { return s.Locked != nil && !*s.Locked }

type condRule struct {
	Range   string   `json:"range"`
	Kind    string   `json:"kind,omitempty"`
	Op      string   `json:"op,omitempty"`
	Value   string   `json:"value,omitempty"`
	Fill    string   `json:"fill,omitempty"`
	Formula string   `json:"formula,omitempty"`
	Rank    int      `json:"rank,omitempty"`
	Percent bool     `json:"percent,omitempty"`
	Color   string   `json:"color,omitempty"`
	Scale   []string `json:"scale,omitempty"`
	Icons   string   `json:"icons,omitempty"`
	// TextColor es el color de letra (#RRGGBB) de una regla que pinta la celda: con su
	// relleno, o solo él («en rojo si es negativo»). De 2026-10: una regla de antes no lo lleva.
	TextColor string `json:"textColor,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
}

type dataValidation struct {
	Range   string   `json:"range"`
	Type    string   `json:"type,omitempty"`
	Values  []string `json:"values,omitempty"`
	Op      string   `json:"op,omitempty"`
	A       string   `json:"a,omitempty"`
	B       string   `json:"b,omitempty"`
	Integer bool     `json:"integer,omitempty"`
	Formula string   `json:"formula,omitempty"`
	Error   string   `json:"error,omitempty"`
	Strict  bool     `json:"strict,omitempty"`
	// listaDe es el origen de una lista que viene de un rango o de un nombre («$A$1:$A$9»);
	// el importador lo resuelve con las celdas del libro y no llega al modelo.
	listaDe string
}

var reColorHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// colorHex normaliza «#rrggbb» a «#RRGGBB»; "" si no es un color del editor.
func colorHex(s string) string {
	if !reColorHex.MatchString(s) {
		return ""
	}
	return strings.ToUpper(s)
}

// argb es el color de OOXML («FFRRGGBB») de un «#RRGGBB».
func argb(hex string) string { return "FF" + strings.ToUpper(strings.TrimPrefix(hex, "#")) }

// hexSinAlfa pasa «FF1F3864» o «1F3864» a «#1F3864»; "" si no lo es.
func hexSinAlfa(v string) string {
	v = strings.TrimSpace(v)
	if len(v) == 8 {
		v = v[2:]
	}
	if len(v) != 6 {
		return ""
	}
	return colorHex("#" + v)
}

// parseNumEs es `parseEsNumberStrict` del editor: coma decimal si aparece, y si no, punto
// decimal («1.234» se lee 1,234); solo lo que parece un número.
func parseNumEs(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsFunc(s, func(r rune) bool { return !strings.ContainsRune("0123456789 .,+-", r) }) {
		return 0, false
	}
	c := strings.Map(func(r rune) rune {
		if strings.ContainsRune("0123456789.,-", r) {
			return r
		}
		return -1
	}, s)
	if c == "" || c == "-" {
		return 0, false
	}
	if strings.LastIndexByte(c, ',') > strings.LastIndexByte(c, '.') {
		c = strings.ReplaceAll(strings.ReplaceAll(c, ".", ""), ",", ".")
	} else {
		c = strings.ReplaceAll(c, ",", "")
	}
	n, err := strconv.ParseFloat(c, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// textoNumEs es un número como lo teclea el editor en es-ES: coma decimal y sin miles.
func textoNumEs(n float64) string {
	return strings.Replace(strconv.FormatFloat(n, 'f', -1, 64), ".", ",", 1)
}

// numOOXML es un número como va en una fórmula de OOXML: punto decimal.
func numOOXML(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

var epocaExcel = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// serialDeFecha es el número de serie (sistema 1900, con el día fantasma del 29-feb) de un
// día; 0 si es anterior a 1900-01-01.
func serialDeFecha(y, m, d int) float64 {
	dias := math.Round(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).Sub(epocaExcel).Hours() / 24)
	if dias < 1 {
		return 0
	}
	if dias < 61 {
		dias-- // antes del 1-mar-1900 el sistema 1900 va un día por detrás
	}
	return dias
}

// fechaDeSerial es el día de un número de serie.
func fechaDeSerial(n float64) (time.Time, bool) {
	dias := int(math.Floor(n))
	if dias < 1 || dias > 2958465 {
		return time.Time{}, false
	}
	if dias < 61 {
		dias++
	}
	return epocaExcel.AddDate(0, 0, dias), true
}

var reFechaEs = regexp.MustCompile(`^(\d{1,4})[/.-](\d{1,2})[/.-](\d{1,4})$`)

// parseFechaEs es `parseEsDate`: «dd/mm/aaaa» (o «aaaa-mm-dd») o un número suelto, que ya
// es un serial.
func parseFechaEs(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	m := reFechaEs.FindStringSubmatch(s)
	if m == nil {
		return parseNumEs(s)
	}
	a, _ := strconv.Atoi(m[1])
	mes, _ := strconv.Atoi(m[2])
	b, _ := strconv.Atoi(m[3])
	y, d := b, a
	if len(m[1]) == 4 {
		y, d = a, b
	}
	if y < 1900 || y > 9999 || mes < 1 || mes > 12 || d < 1 || d > 31 {
		return 0, false
	}
	if t := time.Date(y, time.Month(mes), d, 0, 0, 0, 0, time.UTC); t.Day() != d {
		return 0, false // 31/02 no existe
	}
	n := serialDeFecha(y, mes, d)
	return n, n > 0
}

// textoFechaEs es el serial como «dd/mm/aaaa»; si no es un día, el número tal cual.
func textoFechaEs(n float64) string {
	if t, ok := fechaDeSerial(n); ok {
		return t.Format("02/01/2006")
	}
	return textoNumEs(n)
}

// valido dice si el rango cabe en la rejilla de Excel; lo demás es basura de una entrada
// hostil y se descarta.
func (r rango) valido() bool {
	return r.c1 >= 1 && r.r1 >= 1 && r.c2 <= maxColumnasHoja && r.r2 <= maxFilasHoja
}

// partesSqref lee un `sqref` de OOXML («A1:A3 C1 E5:F9») a rangos válidos.
func partesSqref(s string) []rango {
	var out []rango
	for _, p := range strings.Fields(s) {
		if r, ok := leerRango(p); ok && r.valido() {
			out = append(out, r)
		}
	}
	return out
}

// envolverCelda es `renderCellHtml`: el texto (ya escapado) dentro de cursiva, negrita y
// alineación, en ese orden.
func envolverCelda(dentro string, negrita, cursiva bool, alinear string) string {
	if dentro == "" {
		return ""
	}
	if cursiva {
		dentro = "<i>" + dentro + "</i>"
	}
	if negrita {
		dentro = "<b>" + dentro + "</b>"
	}
	if alinear == "center" || alinear == "right" {
		dentro = `<div style="text-align: ` + alinear + `;">` + dentro + `</div>`
	}
	return dentro
}
