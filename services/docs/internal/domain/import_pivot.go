// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"path"
	"strconv"
	"strings"
)

// Las TABLAS DINÁMICAS de un .xlsx o un .ods, hacia dentro: su definición pasa a la
// hoja (`pivots`) para que se pueda actualizar y editar. Las celdas ya vienen como
// celdas. Si el fichero lo hizo Sytue, la definición entera está en su extensión y
// manda; si no, se reconstruye de las partes normalizadas (export_pivot.go).

// fieldIndex es el índice de un campo por su nombre en el origen (la fila de
// encabezados), o -1.
func fieldIndex(sd sheetData, source, name string) int {
	r1, c1, _, c2, ok := parseA1Range(source)
	if !ok {
		return -1
	}
	for c := c1; c <= c2; c++ {
		if strings.EqualFold(strings.TrimSpace(stripHTML(sd.Cells[A1Ref(r1, c)])), strings.TrimSpace(name)) {
			return c - c1
		}
	}
	return -1
}

// ---- OOXML ----

type xlsxPivotXML struct {
	Name     string `xml:"name,attr"`
	RowGrand string `xml:"rowGrandTotals,attr"`
	ColGrand string `xml:"colGrandTotals,attr"`
	Location struct {
		Ref          string `xml:"ref,attr"`
		RowPageCount int    `xml:"rowPageCount,attr"`
	} `xml:"location"`
	PivotFields []struct {
		DefaultSubtotal string `xml:"defaultSubtotal,attr"`
		Items           []struct {
			X int    `xml:"x,attr"`
			H string `xml:"h,attr"`
			T string `xml:"t,attr"`
		} `xml:"items>item"`
	} `xml:"pivotFields>pivotField"`
	RowFieldsA []struct {
		X int `xml:"x,attr"`
	} `xml:"rowFields>field"`
	ColFieldsA []struct {
		X int `xml:"x,attr"`
	} `xml:"colFields>field"`
	PageFields []struct {
		Fld int `xml:"fld,attr"`
	} `xml:"pageFields>pageField"`
	DataFields []struct {
		Name       string `xml:"name,attr"`
		Fld        int    `xml:"fld,attr"`
		Subtotal   string `xml:"subtotal,attr"`
		ShowDataAs string `xml:"showDataAs,attr"`
	} `xml:"dataFields>dataField"`
	Ext []struct {
		URI   string `xml:"uri,attr"`
		Inner []byte `xml:",innerxml"`
	} `xml:"extLst>ext"`
}

type xlsxCacheXML struct {
	Source struct {
		Ref   string `xml:"ref,attr"`
		Sheet string `xml:"sheet,attr"`
	} `xml:"cacheSource>worksheetSource"`
	Fields []struct {
		Name   string `xml:"name,attr"`
		Shared struct {
			Items []struct {
				XMLName xml.Name
				V       string `xml:"v,attr"`
			} `xml:",any"`
		} `xml:"sharedItems"`
	} `xml:"cacheFields>cacheField"`
}

// attachXlsxPivots lee las tablas dinámicas de cada hoja del paquete. `sheetParts`
// es la parte de cada hoja del modelo, en su orden.
func attachXlsxPivots(files map[string][]byte, model *sheetModel, sheetParts []string) {
	for i, part := range sheetParts {
		dir, base := path.Split(part)
		rels := parseRelsTyped(files[dir+"_rels/"+base+".rels"])
		for _, r := range rels {
			if !strings.HasSuffix(r.Type, "/pivotTable") {
				continue
			}
			tab := path.Clean(dir + r.Target)
			if p, ok := xlsxPivotFrom(files, tab, model, i); ok {
				model.Sheets[i].Pivots = append(model.Sheets[i].Pivots, p)
			}
		}
	}
}

type relTyped struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

func parseRelsTyped(data []byte) []relTyped {
	var rels struct {
		Rel []relTyped `xml:"Relationship"`
	}
	_ = xml.Unmarshal(data, &rels)
	return rels.Rel
}

// xlsxPivotFrom lee una pivotTableDefinition (y su caché) como tabla de la hoja i.
func xlsxPivotFrom(files map[string][]byte, part string, model *sheetModel, i int) (pivotSpec, bool) {
	var t xlsxPivotXML
	if xml.Unmarshal(files[part], &t) != nil {
		return pivotSpec{}, false
	}
	if p, ok := xlsxPivotFromExt(t); ok {
		return p, true
	}
	c, ok := xlsxPivotCacheOf(files, part)
	if !ok {
		return pivotSpec{}, false
	}
	p := pivotSpec{Name: t.Name, Source: strings.ToUpper(c.Source.Ref)}
	if c.Source.Sheet != "" && c.Source.Sheet != sheetName(model, i) {
		p.SourceSheet = c.Source.Sheet
	}
	// La esquina: la de la ubicación, más arriba si hay filtros del informe.
	r1, c1, r2, c2, ok := parseA1Range(t.Location.Ref)
	if !ok {
		return pivotSpec{}, false
	}
	if n := len(t.PageFields); n > 0 {
		r1 = max(1, r1-n-1)
	}
	p.Anchor, p.Range = A1Ref(r1, c1), A1Ref(r1, c1)+":"+A1Ref(r2, c2)
	no := false
	if t.RowGrand == "0" {
		p.GrandRows = &no
	}
	if t.ColGrand == "0" {
		p.GrandCols = &no
	}
	xlsxPivotAxes(&p, t, c)
	xlsxPivotValues(&p, t, c)
	return p, p.Name != ""
}

// xlsxPivotFromExt es la definición de Sytue de la extensión, si el fichero la lleva.
func xlsxPivotFromExt(t xlsxPivotXML) (pivotSpec, bool) {
	for _, e := range t.Ext {
		if e.URI != pivotExtURI {
			continue
		}
		var inner struct {
			Def string `xml:"pivot"`
		}
		if xml.Unmarshal(append(append([]byte("<x>"), e.Inner...), "</x>"...), &inner) != nil || inner.Def == "" {
			continue
		}
		var p pivotSpec
		if json.Unmarshal([]byte(inner.Def), &p) == nil && p.Name != "" {
			return p, true
		}
	}
	return pivotSpec{}, false
}

// xlsxPivotCacheOf es la caché de la tabla, por su relación.
func xlsxPivotCacheOf(files map[string][]byte, part string) (xlsxCacheXML, bool) {
	dir, base := path.Split(part)
	var c xlsxCacheXML
	for _, r := range parseRelsTyped(files[dir+"_rels/"+base+".rels"]) {
		if strings.HasSuffix(r.Type, "/pivotCacheDefinition") {
			ok := xml.Unmarshal(files[path.Clean(dir+r.Target)], &c) == nil && c.Source.Ref != ""
			return c, ok
		}
	}
	return c, false
}

// xlsxPivotAxes son los campos de filas, columnas y filtros con sus elementos ocultos,
// y si lleva subtotales.
func xlsxPivotAxes(p *pivotSpec, t xlsxPivotXML, c xlsxCacheXML) {
	hidden := func(f int) []string {
		if f < 0 || f >= len(t.PivotFields) || f >= len(c.Fields) {
			return nil
		}
		var out []string
		for _, it := range t.PivotFields[f].Items {
			if items := c.Fields[f].Shared.Items; it.H == "1" && it.T == "" && it.X >= 0 && it.X < len(items) {
				out = append(out, xlsxSharedKey(items[it.X].XMLName.Local, items[it.X].V))
			}
		}
		return out
	}
	for _, f := range t.RowFieldsA {
		if f.X >= 0 {
			p.Rows = append(p.Rows, pivotFieldSpec{Field: f.X, Hidden: hidden(f.X)})
		}
	}
	for _, f := range t.ColFieldsA {
		if f.X >= 0 { // -2 es el pseudocampo «Valores»
			p.Cols = append(p.Cols, pivotFieldSpec{Field: f.X, Hidden: hidden(f.X)})
		}
	}
	for _, f := range t.PageFields {
		p.Filters = append(p.Filters, pivotFieldSpec{Field: f.Fld, Hidden: hidden(f.Fld)})
	}
	// Un campo sin eje con elementos ocultos también filtra: el de una segmentación.
	enEje := pivotAxisFields(*p)
	for f := range t.PivotFields {
		if h := hidden(f); !enEje[f] && len(h) > 0 {
			p.HiddenFilters = append(p.HiddenFilters, pivotFieldSpec{Field: f, Hidden: h})
		}
	}
	for _, f := range t.PivotFields {
		if f.DefaultSubtotal == "0" {
			no := false
			p.Subtotals = &no
		}
	}
}

// xlsxPivotValues son los campos de valores. Un nombre que es el del campo del origen
// (con el espacio que pide Excel) es el de por defecto.
func xlsxPivotValues(p *pivotSpec, t xlsxPivotXML, c xlsxCacheXML) {
	for _, d := range t.DataFields {
		v := pivotValueSpec{Field: d.Fld, Fn: "sum", Name: d.Name}
		for k, x := range pivotFnXlsx {
			if x == d.Subtotal {
				v.Fn = k
			}
		}
		for k, x := range pivotShowXlsx {
			if x == d.ShowDataAs {
				v.Show = k
			}
		}
		if d.Fld >= 0 && d.Fld < len(c.Fields) && strings.TrimSpace(d.Name) == c.Fields[d.Fld].Name {
			v.Name = ""
		}
		p.Values = append(p.Values, v)
	}
}

// xlsxSharedKey es la clave de Sytue de un elemento compartido de la caché.
func xlsxSharedKey(tipo, v string) string {
	switch tipo {
	case "n":
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
	case "m":
		return ""
	}
	return v
}

// ---- ODF ----

// attachOdsPivots lee las <table:data-pilot-table> de content.xml.
func attachOdsPivots(content []byte, model *sheetModel) {
	if !bytes.Contains(content, []byte("data-pilot-table")) {
		return
	}
	dec := xml.NewDecoder(bytes.NewReader(content))
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "data-pilot-table" {
			continue
		}
		p, hoja, ok := odsPivotFrom(dec, se, model)
		if !ok {
			continue
		}
		for i := range model.Sheets {
			if sheetName(model, i) == hoja {
				model.Sheets[i].Pivots = append(model.Sheets[i].Pivots, p)
			}
		}
	}
}

// odsSplitAddress parte «Hoja.A1:Hoja.C10» (o «'Mi hoja'.A1») en hoja y rango A1.
func odsSplitAddress(a string) (hoja, rng string) {
	parte := func(s string) (string, string) {
		s = strings.TrimPrefix(s, "$")
		i := strings.LastIndex(s, ".")
		if i < 0 {
			return "", strings.ReplaceAll(s, "$", "")
		}
		h := strings.ReplaceAll(s[:i], "$", "")
		if strings.HasPrefix(h, "'") && strings.HasSuffix(h, "'") {
			h = strings.ReplaceAll(h[1:len(h)-1], "''", "'")
		}
		return h, strings.ReplaceAll(s[i+1:], "$", "")
	}
	a1, a2, hay := strings.Cut(a, ":")
	hoja, r1 := parte(a1)
	if !hay {
		return hoja, r1
	}
	_, r2 := parte(a2)
	return hoja, r1 + ":" + r2
}

// odsPivotFrom lee una tabla del piloto de datos (consume el elemento).
func odsPivotFrom(dec *xml.Decoder, se xml.StartElement, model *sheetModel) (pivotSpec, string, bool) {
	hoja, destino := odsSplitAddress(attrLocal(se, "target-range-address"))
	if def := attrLocal(se, "pivot"); def != "" {
		var p pivotSpec
		if json.Unmarshal([]byte(def), &p) == nil && p.Name != "" {
			_ = dec.Skip()
			return p, hoja, true
		}
	}
	p := pivotSpec{Name: attrLocal(se, "name")}
	odsReadGrandTotal(&p, attrLocal(se, "grand-total"))
	var origen sheetData
	var fuente string
	for {
		tok, err := dec.Token()
		if err != nil {
			return pivotSpec{}, "", false
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "source-cell-range":
				fuente, origen = odsReadSource(&p, e, hoja, model)
			case "data-pilot-field":
				odsPivotField(dec, e, &p, origen)
				continue
			}
		case xml.EndElement:
			if e.Name.Local == "data-pilot-table" {
				r1, c1, _, _, ok := parseA1Range(destino)
				if !ok || fuente == "" || p.Name == "" {
					return pivotSpec{}, "", false
				}
				p.Anchor, p.Range = A1Ref(r1, c1), strings.ToUpper(destino)
				return p, hoja, true
			}
		}
	}
}

// odsReadGrandTotal es lo inverso de odsGrandTotal.
func odsReadGrandTotal(p *pivotSpec, g string) {
	no := false
	switch g {
	case "none":
		p.GrandRows, p.GrandCols = &no, &no
	case "row":
		p.GrandCols = &no
	case "column":
		p.GrandRows = &no
	}
}

// odsReadSource lee el origen: su rango, su hoja (si no es la de la tabla) y la hoja
// del modelo con la que se resuelven los nombres de los campos.
func odsReadSource(p *pivotSpec, e xml.StartElement, hoja string, model *sheetModel) (string, sheetData) {
	h, r := odsSplitAddress(attrLocal(e, "cell-range-address"))
	p.Source = strings.ToUpper(r)
	if h != hoja {
		p.SourceSheet = h
	}
	for i := range model.Sheets {
		if sheetName(model, i) == h {
			return h, model.Sheets[i]
		}
	}
	return h, sheetData{}
}

// odsPivotField lee un <table:data-pilot-field> en la tabla (consume el elemento).
func odsPivotField(dec *xml.Decoder, se xml.StartElement, p *pivotSpec, origen sheetData) {
	nombre, orientacion := attrLocal(se, "source-field-name"), attrLocal(se, "orientation")
	f := fieldIndex(origen, p.Source, nombre)
	var ocultos []string
	sinSubtotales := false
	for depth := 1; depth > 0; {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch e := tok.(type) {
		case xml.StartElement:
			depth++
			if e.Name.Local == "data-pilot-member" && attrLocal(e, "display") == "false" {
				ocultos = append(ocultos, attrLocal(e, "name"))
			}
			if e.Name.Local == "data-pilot-subtotals" {
				sinSubtotales = true // si trae alguno, se vuelve a poner abajo
			}
			if e.Name.Local == "data-pilot-subtotal" {
				sinSubtotales = false
			}
		case xml.EndElement:
			depth--
		}
	}
	if f < 0 {
		return
	}
	no := false
	if sinSubtotales && orientacion != "data" {
		p.Subtotals = &no
	}
	campo := pivotFieldSpec{Field: f, Hidden: ocultos}
	switch orientacion {
	case "row":
		p.Rows = append(p.Rows, campo)
	case "column":
		p.Cols = append(p.Cols, campo)
	case "page":
		p.Filters = append(p.Filters, campo)
	case "hidden":
		if len(ocultos) > 0 { // el de una segmentación: filtra sin estar en la tabla
			p.HiddenFilters = append(p.HiddenFilters, campo)
		}
	case "data":
		v := pivotValueSpec{Field: f, Fn: "sum"}
		fn := attrLocal(se, "function")
		for k, x := range pivotFnOds {
			if x == fn {
				v.Fn = k
			}
		}
		p.Values = append(p.Values, v)
	}
}
