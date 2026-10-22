// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	xhtml "golang.org/x/net/html"
)

// La MATRIZ DE FIDELIDAD de la importación y la exportación ofimáticas. Cada fichero
// del corpus (testdata/fidelidad) se importa, se exporta al mismo formato y se vuelve a
// importar; y cada característica se mide con sus sondas sobre el modelo del editor en
// los dos momentos:
//
//   - import: cuántas sondas se cumplen tras abrir el fichero de fuera.
//   - export: de esas, cuántas siguen cumpliéndose tras guardar y volver a abrir.
//
// Todas cumplidas es «completa», ninguna «perdida» y el resto «parcial». Si al importar
// no llega nada, la exportación no se puede medir: «—».
//
// El resultado vive en docs/architecture.md (sección «Fidelidad OOXML/ODF») y
// TestMatrizDeFidelidad falla si lo medido no coincide con lo escrito. Para reescribirla
// (en el frontal, nunca en el portátil):
//
//	SYTUE_FIDELIDAD_ESCRIBIR=1 go test ./services/docs/internal/domain -run TestMatrizDeFidelidad

// nivel es el grado de conservación de una característica.
type nivel int

const (
	sinMedir nivel = iota
	perdida
	parcial
	completa
)

func (n nivel) String() string {
	switch n {
	case perdida:
		return "perdida"
	case parcial:
		return "parcial"
	case completa:
		return "completa"
	default:
		return "—"
	}
}

// vista es el modelo del editor ya leído: el HTML de un documento, o el JSON de una hoja
// o una presentación (genérico, para que una sonda pueda mirar un campo que el modelo de
// Go todavía no declara).
type vista struct {
	crudo string
	html  *xhtml.Node
	json  any
}

type sonda struct {
	nombre string
	ok     func(v *vista) bool
}

type caracteristica struct {
	nombre string
	kind   Kind
	// nota explica una pérdida que no es del importador: el modelo no tiene dónde guardarla.
	nota   string
	sondas []sonda
}

// muestra es un fichero del corpus: de LibreOffice (generado desde fuentes/ con
// generar.sh) o con el sabor de Microsoft (XML escrito a mano en office/, que se
// empaqueta al vuelo).
type muestra struct {
	columna string
	ruta    string // relativa a testdata/fidelidad
	ext     string
	formato BlobFormat
}

var corpus = []muestra{
	{"DOCX · Word", "office/documento-word", "docx", FormatOOXML},
	{"DOCX · LibreOffice", "libreoffice/documento.docx", "docx", FormatOOXML},
	{"ODT · LibreOffice", "libreoffice/documento.odt", "odt", FormatODF},
	{"XLSX · Excel", "office/hoja-excel", "xlsx", FormatOOXML},
	{"XLSX · LibreOffice", "libreoffice/hoja.xlsx", "xlsx", FormatOOXML},
	{"ODS · LibreOffice", "libreoffice/hoja.ods", "ods", FormatODF},
	{"PPTX · PowerPoint", "office/presentacion-powerpoint", "pptx", FormatOOXML},
	{"PPTX · LibreOffice", "libreoffice/presentacion.pptx", "pptx", FormatOOXML},
	{"ODP · LibreOffice", "libreoffice/presentacion.odp", "odp", FormatODF},
}

// --- Sondas de documento (HTML del editor) ---

func textoDe(n *xhtml.Node) string {
	var b strings.Builder
	var rec func(*xhtml.Node)
	rec = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

func valorAttr(n *xhtml.Node, k string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val, true
		}
	}
	return "", false
}

// buscar dice si algún elemento cumple f.
func buscar(n *xhtml.Node, f func(*xhtml.Node) bool) bool {
	if n == nil {
		return false
	}
	if n.Type == xhtml.ElementNode && f(n) {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if buscar(c, f) {
			return true
		}
	}
	return false
}

// elem es la sonda «hay un <tag> (uno de los nombrados) que contiene el texto y cumple
// los atributos»: «attr» exige que exista; «attr=valor», que contenga ese valor (sin
// distinguir mayúsculas).
func elem(tags, texto string, attrs ...string) func(*vista) bool {
	return func(v *vista) bool {
		return buscar(v.html, func(n *xhtml.Node) bool {
			if !strings.Contains(" "+tags+" ", " "+n.Data+" ") || !strings.Contains(textoDe(n), texto) {
				return false
			}
			for _, a := range attrs {
				k, want, conValor := strings.Cut(a, "=")
				got, ok := valorAttr(n, k)
				if !ok || (conValor && !strings.Contains(strings.ToLower(got), strings.ToLower(want))) {
					return false
				}
			}
			return true
		})
	}
}

// dentro es la sonda «hay un <tag> con el texto que cuelga de un <padre>».
func dentro(padre, tag, texto string) func(*vista) bool {
	return func(v *vista) bool {
		return buscar(v.html, func(n *xhtml.Node) bool {
			if n.Data != tag || !strings.Contains(textoDe(n), texto) {
				return false
			}
			for p := n.Parent; p != nil; p = p.Parent {
				if p.Type == xhtml.ElementNode && p.Data == padre {
					return true
				}
			}
			return false
		})
	}
}

// hermanos es la sonda «los dos <li> con estos textos son hijos de la MISMA lista»: una
// numerada que se parte en dos al cerrar la sublista vuelve a empezar por 1.
func hermanos(a, b string) func(*vista) bool {
	return func(v *vista) bool {
		return buscar(v.html, func(n *xhtml.Node) bool {
			if n.Data != "ol" {
				return false
			}
			var hayA, hayB bool
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xhtml.ElementNode && c.Data == "li" {
					t := textoDe(c)
					hayA = hayA || strings.HasPrefix(t, a)
					hayB = hayB || strings.HasPrefix(t, b)
				}
			}
			return hayA && hayB
		})
	}
}

func enElModelo(texto string) func(*vista) bool {
	return func(v *vista) bool { return strings.Contains(v.crudo, texto) }
}

// --- Sondas de hoja y presentación (JSON genérico) ---

func campo(x any, claves ...string) any {
	for _, k := range claves {
		m, ok := x.(map[string]any)
		if !ok {
			return nil
		}
		x = m[k]
	}
	return x
}

func cadena(x any) string {
	switch t := x.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

func lista(x any) []any {
	l, _ := x.([]any)
	return l
}

// contiene dice si el JSON de x contiene el texto, sin distinguir mayúsculas.
func contiene(x any, texto string) bool {
	if x == nil {
		return false
	}
	b, _ := json.Marshal(x)
	return strings.Contains(strings.ToLower(string(b)), strings.ToLower(texto))
}

func hoja(v *vista, nombre string) any {
	for _, h := range lista(campo(v.json, "sheets")) {
		if cadena(campo(h, "name")) == nombre {
			return h
		}
	}
	return nil
}

// celda es el texto crudo de la celda (sin HTML), como lo ve el motor.
func celda(v *vista, h, ref string) string { return stripHTML(cadena(campo(hoja(v, h), "cells", ref))) }

// normFormula normaliza una fórmula para comparar: sin espacios, sin `$` ni comillas de hoja,
// en mayúsculas.
func normFormula(s string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "$", "", "'", "").Replace(s))
}

func esFormula(h, ref, want string) func(*vista) bool {
	return func(v *vista) bool { return normFormula(celda(v, h, ref)) == normFormula(want) }
}

func esValor(h, ref, want string) func(*vista) bool {
	return func(v *vista) bool { return strings.EqualFold(celda(v, h, ref), want) }
}

func formatoEs(ref, id string) func(*vista) bool {
	return func(v *vista) bool { return cadena(campo(hoja(v, "Ventas"), "formats", ref, "id")) == id }
}

// algunObjeto es la sonda «alguno de los objetos de la lista `clave` de la hoja contiene
// todos los textos» (en su JSON).
func algunObjeto(clave string, textos ...string) func(*vista) bool {
	return func(v *vista) bool {
		for _, o := range lista(campo(hoja(v, "Ventas"), clave)) {
			todos := true
			for _, t := range textos {
				todos = todos && contiene(o, t)
			}
			if todos {
				return true
			}
		}
		return false
	}
}

var reNegritaDelPlan = regexp.MustCompile(`<(strong|b)>(<span[^>]*>)?reducir un 20 %`)

func cajas(v *vista) []any {
	diapos := lista(campo(v.json, "slides"))
	out := make([]any, 0, 4*len(diapos))
	for _, d := range diapos {
		out = append(out, lista(campo(d, "boxes"))...)
	}
	return out
}

func caja(f func(c any) bool) func(*vista) bool {
	return func(v *vista) bool {
		for _, c := range cajas(v) {
			if f(c) {
				return true
			}
		}
		return false
	}
}

func cajaCon(texto string) func(c any) bool {
	return func(c any) bool { return strings.Contains(cadena(campo(c, "html")), texto) }
}

// cercaDe dice si el número de x está a menos de `tol` de `quiero`.
func cercaDe(x any, quiero, tol float64) bool {
	n := numero(x)
	return n != 0 && n > quiero-tol && n < quiero+tol
}

func numero(x any) float64 {
	f, _ := x.(float64)
	return f
}

// animacionDe dice si la caja de la diapositiva 2 con ese texto lleva una animación de ese
// tipo y ese disparador («» = al hacer clic).
func animacionDe(v *vista, texto, tipo, disparador string) bool {
	for _, c := range lista(campo(diapo(v, 1), "boxes")) {
		if !cajaCon(texto)(c) {
			continue
		}
		for _, a := range lista(campo(c, "anims")) {
			if cadena(campo(a, "type")) == tipo && cadena(campo(a, "trigger")) == disparador {
				return true
			}
		}
	}
	return false
}

// conectorDe es el primer conector de la diapositiva 2.
func conectorDe(v *vista) any {
	if l := lista(campo(diapo(v, 1), "connectors")); len(l) > 0 {
		return l[0]
	}
	return nil
}

func diapo(v *vista, i int) any {
	if l := lista(campo(v.json, "slides")); i < len(l) {
		return l[i]
	}
	return nil
}

// --- Las características ---

var caracteristicas = []caracteristica{
	{nombre: "Títulos y cita", kind: KindDocument, sondas: []sonda{
		{"título 1", elem("h1", "Informe trimestral de la cooperativa")},
		{"título 2", elem("h2", "Resultados del huerto")},
		{"título 3", elem("h3", "Detalle por parcela")},
		{"cita", elem("blockquote", "La tierra no se hereda")},
	}},
	{nombre: "Formato de carácter", kind: KindDocument, sondas: []sonda{
		{"negrita", elem("strong b", "cosecha récord")},
		{"cursiva", elem("em i", "con matices")},
		{"subrayado", elem("u", "subrayado")},
		{"tachado", elem("s strike", "precio antiguo")},
		{"color", elem("span", "en rojo", "style=c0392b")},
	}},
	{nombre: "Alineación", kind: KindDocument, sondas: []sonda{
		{"centrada", elem("p", "Texto centrado", "style=center")},
		{"a la derecha", elem("p", "Texto a la derecha", "style=right")},
		{"justificada", elem("p", "Texto justificado", "style=justify")},
	}},
	{nombre: "Enlaces", kind: KindDocument, sondas: []sonda{
		{"enlace", elem("a", "web de la cooperativa", "href=https://www.sytue.eu/cooperativa")},
	}},
	{nombre: "Listas", kind: KindDocument, sondas: []sonda{
		{"viñetas", dentro("ul", "li", "Tomates")},
		{"numerada", dentro("ol", "li", "Preparar la tierra")},
		{"anidada", dentro("li", "li", "Abonar el bancal")},
		{"numeración seguida tras la sublista", hermanos("Preparar la tierra", "Sembrar")},
	}},
	{nombre: "Tablas", kind: KindDocument, sondas: []sonda{
		{"celdas", dentro("table", "td", "1200")},
		{"celda combinada", elem("td th", "Total anual", "colspan=2")},
		{"celda combinada vertical", elem("td th", "Zona sur", "rowspan=2")},
	}},
	{nombre: "Imágenes", kind: KindDocument, sondas: []sonda{
		{"imagen embebida", elem("img", "", "src=data:image/")},
		{"tamaño", elem("img", "", "data-w")},
	}},
	{nombre: "Cabecera y pie", kind: KindDocument, sondas: []sonda{
		{"cabecera", elem("div", "Cooperativa La Vega", "data-doc-header")},
		{"pie", elem("div", "Pie del informe", "data-doc-footer")},
	}},
	{nombre: "Notas al pie y finales", kind: KindDocument, sondas: []sonda{
		{"llamada", elem("sup", "", "data-footnote-ref")},
		{"nota al pie", dentro("section", "li", "báscula municipal")},
		{"nota final", dentro("section", "li", "libro de registro")},
		{"tipo de nota final", elem("sup", "", "data-note-class=endnote")},
	}},
	{nombre: "Comentarios", kind: KindDocument, sondas: []sonda{
		{"ancla", elem("mark", "1200", "data-comment-id")},
		{"texto del comentario", enElModelo("Revisar esta cifra con tesorería")},
	}},
	{nombre: "Control de cambios", kind: KindDocument, sondas: []sonda{
		{"inserción", elem("ins", "añadido en revisión", "data-track")},
		{"autoría", elem("ins", "añadido en revisión", "data-author=Lucía Prieto")},
		{"borrado", elem("del", "texto retirado", "data-track")},
	}},

	{nombre: "Tipos de valor", kind: KindSpreadsheet, sondas: []sonda{
		{"número", esValor("Ventas", "B2", "1200")},
		{"texto", esValor("Ventas", "A2", "Norte")},
		{"booleano", esValor("Ventas", "D2", "TRUE")},
		{"fecha", func(v *vista) bool {
			return celda(v, "Ventas", "E2") == "46096" && formatoEs("E2", "date")(v)
		}},
	}},
	{nombre: "Fórmulas", kind: KindSpreadsheet, sondas: []sonda{
		{"fórmula", esFormula("Ventas", "C2", "=B2*2")},
		{"fórmula compartida", esFormula("Ventas", "C3", "=B3*2")},
		{"entre hojas", esFormula("Resumen", "B1", "=SUM(Ventas!B2:B4)")},
		{"con nombre definido", esFormula("Resumen", "B4", "=B1/Objetivo")},
	}},
	{nombre: "Formatos de número", kind: KindSpreadsheet, sondas: []sonda{
		{"moneda", formatoEs("F2", "currency")},
		{"porcentaje", formatoEs("G2", "percent")},
		{"decimales", func(v *vista) bool {
			return cadena(campo(hoja(v, "Ventas"), "formats", "H2", "decimals")) == "3"
		}},
	}},
	{nombre: "Estilo de celda", kind: KindSpreadsheet, sondas: []sonda{
		{"negrita", func(v *vista) bool { return strings.Contains(cadena(campo(hoja(v, "Ventas"), "cells", "A1")), "<b>") }},
		{"relleno", func(v *vista) bool { return contiene(campo(hoja(v, "Ventas"), "styles", "A1", "fill"), "ddebf7") }},
		{"color de texto", func(v *vista) bool { return contiene(campo(hoja(v, "Ventas"), "styles", "A1", "color"), "1f3864") }},
	}},
	{nombre: "Celdas combinadas", kind: KindSpreadsheet, sondas: []sonda{
		{"rango combinado", func(v *vista) bool {
			m, _ := campo(hoja(v, "Ventas"), "merges").([]any)
			return len(m) == 1 && cadena(m[0]) == "A6:C6"
		}},
	}},
	{nombre: "Anchos y altos", kind: KindSpreadsheet, sondas: []sonda{
		{"ancho de columna", func(v *vista) bool { return cercaDe(campo(hoja(v, "Ventas"), "colWidths", "0"), 150, 4) }},
		{"alto de fila", func(v *vista) bool { return cercaDe(campo(hoja(v, "Ventas"), "rowHeights", "0"), 30, 3) }},
	}},
	{nombre: "Validación de datos", kind: KindSpreadsheet, sondas: []sonda{
		{"lista", algunObjeto("validations", "A2:A4", "Sur")},
		{"número entero", algunObjeto("validations", "B2:B4", `"integer":true`)},
	}},
	{nombre: "Formato condicional", kind: KindSpreadsheet, sondas: []sonda{
		{"regla de celda", algunObjeto("condFormats", "B2:B4", `"op":"gt"`, "1000")},
		{"relleno", algunObjeto("condFormats", "B2:B4", "fde9d9")},
	}},
	{nombre: "Esquema (agrupar)", kind: KindSpreadsheet, sondas: []sonda{
		{"filas agrupadas y colapsadas", func(v *vista) bool {
			g := lista(campo(hoja(v, "Análisis"), "rowGroups"))
			return len(g) == 1 && numero(campo(g[0], "start")) == 2 && numero(campo(g[0], "end")) == 3 &&
				numero(campo(g[0], "level")) == 1 && campo(g[0], "collapsed") == true
		}},
		{"columnas agrupadas y colapsadas", func(v *vista) bool {
			g := lista(campo(hoja(v, "Análisis"), "colGroups"))
			return len(g) == 1 && numero(campo(g[0], "start")) == 1 && numero(campo(g[0], "end")) == 2 && campo(g[0], "collapsed") == true
		}},
	}},
	{nombre: "Autofiltro", kind: KindSpreadsheet, sondas: []sonda{
		{"rango del filtro", func(v *vista) bool {
			return numero(campo(hoja(v, "Análisis"), "autofilter", "c2")) == 3 && campo(hoja(v, "Análisis"), "autofilter", "row") == float64(0)
		}},
		{"condición «mayor que»", func(v *vista) bool {
			c := campo(hoja(v, "Análisis"), "autofilter", "cond", "1")
			return cadena(campo(c, "op")) == "gt" && cadena(campo(c, "a")) == "5"
		}},
	}},
	{nombre: "Escenarios", kind: KindSpreadsheet, sondas: []sonda{
		{"escenario con su valor", func(v *vista) bool {
			e := lista(campo(hoja(v, "Análisis"), "scenarios"))
			return len(e) == 1 && cadena(campo(e[0], "name")) == "Alto" && cadena(campo(e[0], "values", "B2")) == "10"
		}},
	}},
	{nombre: "Tablas", kind: KindSpreadsheet, sondas: []sonda{
		{"tabla con nombre", algunObjeto("tables", "TablaVentas", "A1:H4")},
	}},
	{nombre: "Nombres definidos", kind: KindSpreadsheet, sondas: []sonda{
		{"nombre", func(v *vista) bool { return contiene(campo(v.json, "names"), "Objetivo") }},
	}},
	{nombre: "Varias hojas", kind: KindSpreadsheet, sondas: []sonda{
		{"hojas en orden", func(v *vista) bool {
			l := lista(campo(v.json, "sheets"))
			return len(l) == 3 && cadena(campo(l[0], "name")) == "Ventas" && cadena(campo(l[1], "name")) == "Resumen" &&
				cadena(campo(l[2], "name")) == "Análisis"
		}},
	}},

	{nombre: "Texto", kind: KindPresentation, sondas: []sonda{
		{"título", caja(cajaCon("Plan de riego 2027"))},
		{"negrita", caja(func(c any) bool {
			// LibreOffice pone el color en cada tramo: el span puede ir dentro de la negrita.
			return reNegritaDelPlan.MatchString(cadena(campo(c, "html")))
		})},
		{"posición del marcador", caja(func(c any) bool {
			x, y := numero(campo(c, "x")), numero(campo(c, "y"))
			return cajaCon("Plan de riego 2027")(c) && x > 6 && x < 8.5 && y > 8 && y < 11
		})},
	}},
	{nombre: "Formas", kind: KindPresentation, sondas: []sonda{
		{"rectángulo", caja(func(c any) bool {
			return cadena(campo(c, "shape")) == "rect" && cajaCon("Depósito")(c) && contiene(campo(c, "fill"), "2e86ab")
		})},
		{"elipse", caja(func(c any) bool { return cadena(campo(c, "shape")) == "ellipse" && cajaCon("Pozo")(c) })},
	}},
	{nombre: "Imágenes", kind: KindPresentation, sondas: []sonda{
		{"imagen embebida", caja(func(c any) bool {
			return cadena(campo(c, "kind")) == "image" && strings.HasPrefix(cadena(campo(c, "src")), "data:image/")
		})},
	}},
	{nombre: "Notas del ponente", kind: KindPresentation, sondas: []sonda{
		{"notas", func(v *vista) bool {
			return strings.Contains(cadena(campo(diapo(v, 0), "notes")), "presupuesto del pozo")
		}},
	}},
	{nombre: "Tema y patrón", kind: KindPresentation, sondas: []sonda{
		{"caja del patrón", func(v *vista) bool { return contiene(campo(v.json, "master"), "Cooperativa La Vega") }},
		{"fondo", func(v *vista) bool { return contiene(v.json, "f4f1ea") }},
	}},
	{nombre: "Transiciones", kind: KindPresentation, sondas: []sonda{
		{"fundido", func(v *vista) bool { return cadena(campo(diapo(v, 1), "transition")) == "fade" }},
		{"deslizamiento", func(v *vista) bool { return cadena(campo(diapo(v, 2), "transition")) == "slide" }},
	}},
	{nombre: "Fondo y patrón por diapositiva", kind: KindPresentation, sondas: []sonda{
		{"fondo propio", func(v *vista) bool { return contiene(campo(diapo(v, 2), "bg"), "dfe7d5") }},
		{"patrón oculto", func(v *vista) bool { return cadena(campo(diapo(v, 2), "hideMaster")) == "true" }},
	}},
	{nombre: "Animaciones", kind: KindPresentation, sondas: []sonda{
		{"entrada al hacer clic", func(v *vista) bool { return animacionDe(v, "Depósito", "fade", "") }},
		{"a la vez que la anterior", func(v *vista) bool { return animacionDe(v, "Pozo", "spin", "with") }},
	}},
	{nombre: "Conectores", kind: KindPresentation, sondas: []sonda{
		{"anclado a sus dos formas", func(v *vista) bool {
			c := conectorDe(v)
			return cadena(campo(c, "from", "boxId")) != "" && cadena(campo(c, "to", "boxId")) != ""
		}},
		{"punta de flecha", func(v *vista) bool { return cadena(campo(conectorDe(v), "endArrow")) == "true" }},
	}},
	{nombre: "Diseños", kind: KindPresentation, sondas: []sonda{
		{"diseño con sus marcadores", func(v *vista) bool {
			for _, l := range lista(campo(v.json, "master", "layouts")) {
				if len(lista(campo(l, "boxes"))) > 0 && cadena(campo(l, "name")) != "" {
					return true
				}
			}
			return false
		}},
		{"diseño de la diapositiva", func(v *vista) bool { return cadena(campo(diapo(v, 0), "layout")) != "" }},
	}},
	{nombre: "Vídeo y subtítulos", kind: KindPresentation, nota: "las muestras de LibreOffice no llevan vídeo: Impress no guarda pistas de subtítulos de un vídeo, y un .vtt que viaja con él solo se lee si el paquete lo pone junto al medio con su mismo nombre", sondas: []sonda{
		{"vídeo", caja(func(c any) bool { return cadena(campo(c, "kind")) == "media" })},
		{"pista de subtítulos con su idioma", caja(func(c any) bool {
			for _, tr := range lista(campo(c, "tracks")) {
				if cadena(campo(tr, "lang")) == "es-ES" && contiene(campo(tr, "vtt"), "asamblea") {
					return true
				}
			}
			return false
		})},
	}},
	{nombre: "Tamaño de diapositiva", kind: KindPresentation, sondas: []sonda{
		{"4:3", func(v *vista) bool { return cadena(campo(v.json, "size", "preset")) == "4:3" }},
	}},
}

// --- El recorrido fichero → modelo → fichero → modelo ---

// leerMuestra lee el fichero del corpus; un directorio (los de sabor Microsoft) se
// empaqueta en un ZIP en memoria.
func leerMuestra(t *testing.T, m muestra) []byte {
	t.Helper()
	ruta := filepath.Join("testdata", "fidelidad", m.ruta)
	info, err := os.Stat(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		b, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = filepath.WalkDir(ruta, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(ruta, p)
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func claveDeContenido(k Kind) string {
	switch k {
	case KindSpreadsheet:
		return "doc:sheet"
	case KindPresentation:
		return "doc:slides"
	default:
		return "doc:html"
	}
}

func leerVista(t *testing.T, res *ImportResult) *vista {
	t.Helper()
	v := &vista{crudo: res.Content}
	if res.Kind == KindDocument {
		// Los comentarios no van en el HTML: la sonda de su texto mira también los guardados.
		for _, c := range res.Comments {
			v.crudo += "\n" + c.Text
		}
		n, err := xhtml.Parse(strings.NewReader(res.Content))
		if err != nil {
			t.Fatal(err)
		}
		v.html = n
		return v
	}
	if err := json.Unmarshal([]byte(res.Content), &v.json); err != nil {
		t.Fatal(err)
	}
	return v
}

// comentariosGuardados son los comentarios importados tal como los dejaría guardados un
// cliente: una fila por comentario, de quien importa, con la autoría original delante del
// cuerpo (Text), y el ancla {mark, text}.
func comentariosGuardados(imp []ImportedComment) []Comment {
	out := make([]Comment, 0, len(imp))
	for _, c := range imp {
		fila := Comment{ID: "db-" + c.ID, AuthorName: "Quien importa", Body: c.Text, Resolved: c.Resolved, CreatedAt: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)}
		if c.ParentID != "" {
			p := "db-" + c.ParentID
			fila.ParentID = &p
		}
		if c.Anchored {
			fila.Anchor, _ = json.Marshal(map[string]string{"mark": c.ID, "text": c.Quote})
		}
		out = append(out, fila)
	}
	return out
}

// medida es lo que se midió de una característica en una muestra.
type medida struct {
	imp, exp           nivel
	fallaImp, fallaExp []string // sondas que no se cumplen en cada momento
}

func medirMuestra(t *testing.T, m muestra) map[string]medida {
	t.Helper()
	nombre := "muestra." + m.ext
	res, err := ImportWithComments(nombre, leerMuestra(t, m))
	if err != nil {
		t.Fatalf("%s no se importa: %v", m.columna, err)
	}
	antes := leerVista(t, res)
	data, _, ext, err := ExportWithComments(res.Kind, "", stateWithContent(t, claveDeContenido(res.Kind), res.Content), m.formato, comentariosGuardados(res.Comments))
	if err != nil {
		t.Fatalf("%s no se exporta: %v", m.columna, err)
	}
	res2, err := ImportWithComments("vuelta."+ext, data)
	if err != nil {
		t.Fatalf("%s no se reimporta tras exportar: %v", m.columna, err)
	}
	despues := leerVista(t, res2)

	out := map[string]medida{}
	for _, c := range caracteristicas {
		if c.kind != res.Kind {
			continue
		}
		var md medida
		pasan := 0
		siguen := 0
		for _, s := range c.sondas {
			if !s.ok(antes) {
				md.fallaImp = append(md.fallaImp, s.nombre)
				continue
			}
			pasan++
			if s.ok(despues) {
				siguen++
			} else {
				md.fallaExp = append(md.fallaExp, s.nombre)
			}
		}
		md.imp = grado(pasan, len(c.sondas))
		if pasan > 0 {
			md.exp = grado(siguen, pasan)
		}
		out[c.nombre] = md
	}
	return out
}

func grado(si, de int) nivel {
	switch si {
	case de:
		return completa
	case 0:
		return perdida
	default:
		return parcial
	}
}

// --- La matriz en Markdown ---

func matrizMarkdown(t *testing.T) string {
	t.Helper()
	medidas := make([]map[string]medida, len(corpus))
	for i, m := range corpus {
		medidas[i] = medirMuestra(t, m)
	}
	var b strings.Builder
	for _, g := range []struct {
		kind   Kind
		titulo string
	}{{KindDocument, "Documento (Docus)"}, {KindSpreadsheet, "Hoja de cálculo (Calcus)"}, {KindPresentation, "Presentación (Diapos)"}} {
		var cols []int
		for i := range corpus {
			if _, ok := medidas[i][primeraDe(g.kind)]; ok {
				cols = append(cols, i)
			}
		}
		fmt.Fprintf(&b, "**%s** (import / export):\n\n| Característica |", g.titulo)
		for _, i := range cols {
			b.WriteString(" " + corpus[i].columna + " |")
		}
		b.WriteString("\n| --- |" + strings.Repeat(" --- |", len(cols)) + "\n")
		var perdidas []string
		for _, c := range caracteristicas {
			if c.kind != g.kind {
				continue
			}
			b.WriteString("| " + c.nombre + " |")
			for _, i := range cols {
				md := medidas[i][c.nombre]
				fmt.Fprintf(&b, " %s / %s |", md.imp, md.exp)
				if len(md.fallaImp)+len(md.fallaExp) > 0 && c.nota == "" {
					p := "- " + c.nombre + ", " + corpus[i].columna + ":"
					if len(md.fallaImp) > 0 {
						p += " al importar se pierde " + strings.Join(md.fallaImp, ", ") + "."
					}
					if len(md.fallaExp) > 0 {
						p += " Al exportar se pierde " + strings.Join(md.fallaExp, ", ") + "."
					}
					perdidas = append(perdidas, p)
				}
			}
			b.WriteString("\n")
			if c.nota != "" {
				perdidas = append(perdidas, "- "+c.nombre+": "+c.nota+".")
			}
		}
		if len(perdidas) > 0 {
			b.WriteString("\nPérdidas:\n\n" + strings.Join(perdidas, "\n") + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func primeraDe(k Kind) string {
	for _, c := range caracteristicas {
		if c.kind == k {
			return c.nombre
		}
	}
	return ""
}

const (
	marcaInicio = "<!-- matriz-fidelidad:inicio (generada: no editar a mano) -->"
	marcaFin    = "<!-- matriz-fidelidad:fin -->"
)

// TestMatrizDeFidelidad mide el corpus y exige que docs/architecture.md diga lo mismo.
// Un cambio que gane (o pierda) fidelidad tiene que llevar su matriz al día.
func TestMatrizDeFidelidad(t *testing.T) {
	medida := matrizMarkdown(t)
	ruta := filepath.Join("..", "..", "..", "..", "docs", "architecture.md")
	doc, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	ini := bytes.Index(doc, []byte(marcaInicio))
	fin := bytes.Index(doc, []byte(marcaFin))
	if ini < 0 || fin < ini {
		t.Fatalf("docs/architecture.md no tiene las marcas de la matriz de fidelidad")
	}
	escrita := strings.TrimSpace(string(doc[ini+len(marcaInicio) : fin]))
	if normalizarMatriz(escrita) == normalizarMatriz(medida) {
		return
	}
	if os.Getenv("SYTUE_FIDELIDAD_ESCRIBIR") == "1" {
		nuevo := string(doc[:ini+len(marcaInicio)]) + "\n\n" + medida + "\n\n" + string(doc[fin:])
		if err := os.WriteFile(ruta, []byte(nuevo), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("matriz reescrita en %s", ruta)
		return
	}
	t.Errorf("la matriz de fidelidad de docs/architecture.md no es la medida. Reescríbela con\n"+
		"  SYTUE_FIDELIDAD_ESCRIBIR=1 go test ./services/docs/internal/domain -run TestMatrizDeFidelidad\n"+
		"y revisa el diff. Medida:\n\n%s", medida)
}

// Prettier alinea columnas y ensancha los separadores de las tablas. Es
// presentación, no una diferencia de fidelidad: comparar el contenido de cada
// celda conserva el guardián sin pelear con format:check.
func normalizarMatriz(md string) string {
	lineas := strings.Split(strings.TrimSpace(md), "\n")
	for i, linea := range lineas {
		if !strings.HasPrefix(linea, "|") {
			continue
		}
		celdas := strings.Split(linea, "|")
		for j, celda := range celdas {
			celda = strings.TrimSpace(celda)
			if celda != "" && strings.Trim(celda, "-") == "" {
				celda = "---"
			}
			celdas[j] = celda
		}
		lineas[i] = strings.Join(celdas, "|")
	}
	return strings.Join(lineas, "\n")
}

func TestNormalizarMatrizSoloIgnoraAlineacionMarkdown(t *testing.T) {
	medida := "| Formato | Estado |\n| --- | --- |\n| DOCX | completa |"
	formateada := "| Formato    | Estado   |\n| ---------- | -------- |\n| DOCX       | completa |"
	if normalizarMatriz(medida) != normalizarMatriz(formateada) {
		t.Fatal("el formateo cambia una matriz equivalente")
	}
	if normalizarMatriz(medida) == normalizarMatriz(strings.ReplaceAll(formateada, "completa", "perdida")) {
		t.Fatal("el guardián ocultó una pérdida real")
	}
}
