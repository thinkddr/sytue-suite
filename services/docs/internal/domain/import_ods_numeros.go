// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/xml"
	"math"
	"strconv"
	"strings"
	"time"
)

// Los FORMATOS DE NÚMERO de ODF (`number:number-style`, `percentage-style`,
// `currency-style`, `date-style` y `time-style`) a los del editor: el preset que les toca
// (número, millares, moneda, porcentaje, fecha) y, si no hay uno, una máscara personalizada
// con los tokens de Excel. Es la vuelta de `odfDataStyle` (export_sheet.go).

// datoODS es una `number:*-style` leída.
type datoODS struct {
	tipo       string // number, percentage, currency, date, time
	decimales  int
	agrupa     bool
	mascara    string // la máscara de Excel equivalente (fechas y horas, y lo que no es preset)
	positivo   string // style:map con value()>=0: el estilo que da el formato de los positivos
	sinMascara bool   // lleva algo que la máscara no sabe decir (notación científica, fracciones)
}

// parseODSFormatos lee los estilos de datos de los documentos dados y devuelve, por nombre,
// el formato de número del editor (nil si es General o no tiene equivalente).
func parseODSFormatos(docs ...[]byte) map[string]*numFmtSpec {
	datos := map[string]*datoODS{}
	for _, doc := range docs {
		leerDatosODS(doc, datos)
	}
	out := make(map[string]*numFmtSpec, len(datos))
	for nombre, d := range datos {
		if p := d.positivo; p != "" && datos[p] != nil {
			d = datos[p] // `N1` es el de los negativos y apunta al de los positivos
		}
		out[nombre] = d.spec()
	}
	return out
}

//nolint:gocognit,gocyclo // recorrido de un solo paso sobre el XML de ODF: un caso por elemento, y trocearlo reparte el estado
func leerDatosODS(doc []byte, out map[string]*datoODS) {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	var actual *datoODS
	var mascara strings.Builder
	var literal strings.Builder
	soltarLiteral := func() {
		if literal.Len() > 0 {
			mascara.WriteString(literalMascara(literal.String()))
			literal.Reset()
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			l := t.Name.Local
			switch l {
			case "body":
				return // los estilos automáticos van siempre antes de <office:body>
			case "number-style", "percentage-style", "currency-style", "date-style", "time-style":
				if len(out) >= maxEstilosHoja {
					return
				}
				actual = &datoODS{tipo: strings.TrimSuffix(l, "-style")}
				out[attrLocal(t, "name")] = actual
				mascara.Reset()
				literal.Reset()
			case "number":
				if actual != nil {
					soltarLiteral()
					actual.decimales = min(atoiDefault(attrLocal(t, "decimal-places"), 0), 8)
					actual.agrupa = attrLocal(t, "grouping") == "true"
					mascara.WriteString(mascaraNumero(actual.decimales, actual.agrupa, atoiDefault(attrLocal(t, "min-integer-digits"), 1)))
				}
			case "text", "currency-symbol":
				if actual != nil {
					texto := textoDeElemento(dec)
					literal.WriteString(texto)
				}
			case "scientific-number", "fraction":
				if actual != nil {
					actual.sinMascara = true
				}
			case "map":
				if actual != nil && strings.Contains(attrLocal(t, "condition"), "value()>=0") {
					actual.positivo = attrLocal(t, "apply-style-name")
				}
			default:
				if actual != nil {
					if tok := tokenFecha(l, t); tok != "" {
						soltarLiteral()
						mascara.WriteString(tok)
					}
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "number-style", "percentage-style", "currency-style", "date-style", "time-style":
				if actual != nil {
					soltarLiteral()
					actual.mascara = mascara.String()
					actual = nil
				}
			}
		}
	}
}

// textoDeElemento lee el texto de un elemento ya abierto hasta su cierre.
func textoDeElemento(dec *xml.Decoder) string {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String()
		}
	}
}

// tokenFecha es el token de máscara de un componente de fecha u hora de ODF.
func tokenFecha(local string, se xml.StartElement) string {
	largo := attrLocal(se, "style") == "long"
	textual := attrLocal(se, "textual") == "true"
	switch local {
	case "day":
		return map[bool]string{true: "dd", false: "d"}[largo]
	case "month":
		switch {
		case textual && largo:
			return "mmmm"
		case textual:
			return "mmm"
		}
		return map[bool]string{true: "mm", false: "m"}[largo]
	case "year":
		return map[bool]string{true: "yyyy", false: "yy"}[largo]
	case "day-of-week":
		return map[bool]string{true: "dddd", false: "ddd"}[largo]
	case "hours":
		return map[bool]string{true: "hh", false: "h"}[largo]
	case "minutes":
		return map[bool]string{true: "mm", false: "m"}[largo]
	case "seconds":
		return map[bool]string{true: "ss", false: "s"}[largo]
	case "am-pm":
		return "AM/PM"
	}
	return ""
}

func mascaraNumero(decimales int, agrupa bool, minEnteros int) string {
	ent := strings.Repeat("0", max(minEnteros, 1))
	if agrupa {
		ent = "#,##0"
	}
	if decimales > 0 {
		ent += "." + strings.Repeat("0", decimales)
	}
	return ent
}

// literalMascara es un literal dentro de una máscara: los separadores habituales van tal
// cual y el resto, entre comillas.
func literalMascara(s string) string {
	if s == "" {
		return ""
	}
	if strings.Trim(s, "/-:. ") == "" {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, "") + `"`
}

// spec es el formato del editor de un estilo de datos: el del código de Excel equivalente,
// que es lo que ya sabe leer el importador de .xlsx (el editor no guarda máscaras propias).
func (d *datoODS) spec() *numFmtSpec {
	if d.sinMascara {
		return nil
	}
	return codeToNumFmtSpec(d.mascara)
}

// --- Valores de fecha y hora ---

// serialDeODS es el número de serie de un `office:date-value` («2026-03-15» o
// «2026-03-15T10:30:00») o de un `office:time-value` («PT12H30M»); false si no se entiende.
func serialDeODS(tipo, valor string) (float64, bool) {
	valor = strings.TrimSpace(valor)
	switch tipo {
	case "date":
		for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, valor); err == nil {
				n := serialDeFecha(t.Year(), int(t.Month()), t.Day())
				if n == 0 {
					return 0, false
				}
				seg := float64(t.Hour()*3600 + t.Minute()*60 + t.Second())
				return math.Round((n+seg/86400)*1e9) / 1e9, true
			}
		}
	case "time":
		return duracionODS(valor)
	}
	return 0, false
}

// duracionODS lee una duración ISO 8601 («PT12H30M15S») como fracción de día.
func duracionODS(v string) (float64, bool) {
	v = strings.TrimPrefix(strings.ToUpper(v), "P")
	dias := 0.0
	if d, resto, ok := strings.Cut(v, "D"); ok && !strings.Contains(d, "T") {
		n, err := strconv.ParseFloat(d, 64)
		if err != nil {
			return 0, false
		}
		dias, v = n, resto
	}
	v = strings.TrimPrefix(v, "T")
	seg := 0.0
	num := ""
	for _, r := range v {
		switch r {
		case 'H', 'M', 'S':
			n, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, false
			}
			seg += n * map[rune]float64{'H': 3600, 'M': 60, 'S': 1}[r]
			num = ""
		default:
			num += string(r)
		}
	}
	if num != "" {
		return 0, false
	}
	return math.Round((dias+seg/86400)*1e9) / 1e9, true
}

// fechaODS es el `office:date-value` de un número de serie; si lleva hora, con ella.
func fechaODS(n float64) (string, bool) {
	t, ok := fechaDeSerial(n)
	if !ok {
		return "", false
	}
	frac := n - math.Floor(n)
	if frac < 1e-9 {
		return t.Format("2006-01-02"), true
	}
	seg := int(math.Round(frac * 86400))
	return t.Add(time.Duration(seg) * time.Second).Format("2006-01-02T15:04:05"), true
}
