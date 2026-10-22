// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"path"
	"strconv"
	"strings"
)

// Importación de los segmentadores (slicers) de Excel a `sheetSlicer`.
//
// Referencias (implementación original, sin tomar código de nadie):
//   - [MS-XLSX] «Extensions to SpreadsheetML»: CT_SlicerCaches (extLst del libro, URIs
//     {BBE1A952-AA13-448e-AADC-164F8A28A991} para los de tabla dinámica y
//     {46BE6895-7355-4a93-B00E-2C351335B9C9} para los de tabla), CT_SlicerCacheDefinition
//     (`name`, `sourceName`, `pivotTables`, `data/tabular`), CT_TableSlicerCache
//     (`tableId`, `column`: el id de la columna de la tabla; extensión {2F2917AC-…}),
//     CT_SlicerList (extLst de la hoja) y CT_Slicers/CT_Slicer (`name`, `cache`).
//   - ECMA-376 Parte 1 §18.5.1.2 (table, `tableColumn@id`), §18.2.5 (definedName: Excel
//     escribe un `Slicer_*` con valor `#N/A` por cada caché) y §20.5.2 (anclas del dibujo).
//   - ECMA-376 Parte 2 (OPC): las partes se resuelven por relaciones, no por nombre.
//
// El modelo de Sytue no guarda la selección del segmentador: vive en el autofiltro (tablas)
// o en los elementos ocultos de la dinámica, y ambos ya se importan por su lado. Aquí solo
// se reconstruye «qué segmenta y dónde está la tarjeta».
//
// Entrada hostil: los límites van en constantes, una parte que se pasa se ignora entera y
// cualquier referencia rota (caché sin tabla, tabla o dinámica que no existe, columna fuera
// de la tabla) descarta ESE segmentador, nunca el libro.
const (
	maxSegmentadores    = 100     // por libro; Excel no pasa de unas decenas en uso real
	maxParteSegmentador = 1 << 20 // bytes de cada parte de caché, de lista o de dibujo
)

// parteAcotada es la parte `p`, o nil si falta o es mayor que el tope.
func parteAcotada(files map[string][]byte, p string) []byte {
	if b := files[p]; len(b) <= maxParteSegmentador {
		return b
	}
	return nil
}

// cacheSegmentador es lo que se lee de un slicerCacheDefinition.
type cacheSegmentador struct {
	Name       string `xml:"name,attr"`
	SourceName string `xml:"sourceName,attr"`
	Pivots     []struct {
		Name string `xml:"name,attr"`
	} `xml:"pivotTables>pivotTable"`
	Tabla *struct {
		TableID *int `xml:"tableId,attr"`
		Column  *int `xml:"column,attr"`
	} `xml:"extLst>ext>tableSlicerCache"`
}

// tablaDeSegmentador es una tabla de la hoja tal como la necesita un segmentador.
type tablaDeSegmentador struct {
	ref  rango
	cols []struct {
		ID   int    `xml:"id,attr"`
		Name string `xml:"name,attr"`
	}
}

// attachXlsxSlicers añade a las hojas los segmentadores de Excel, que cita la parte del libro
// (`libro`). Va DESPUÉS de las dinámicas (comprueba que existen) y no pisa ninguno que ya
// traiga la extensión de Sytue.
func attachXlsxSlicers(files map[string][]byte, libro string, model *sheetModel, sheetParts []string) {
	var wb struct {
		Caches []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"extLst>ext>slicerCaches>slicerCache"`
	}
	if xml.Unmarshal(files[libro], &wb) != nil || len(wb.Caches) == 0 {
		return
	}
	wbRels := parseRels(files[relsPath(libro)])
	caches := map[string]cacheSegmentador{}
	for _, c := range wb.Caches {
		if len(caches) >= maxSegmentadores {
			break
		}
		var def cacheSegmentador
		raw := parteAcotada(files, destinoRel(libro, wbRels[c.RID]))
		if len(raw) > 0 && xml.Unmarshal(raw, &def) == nil && def.Name != "" {
			caches[def.Name] = def
		}
	}
	if len(caches) == 0 {
		return
	}
	pivotes := partesDeDinamicas(files, sheetParts)
	total := 0
	for i, part := range sheetParts {
		if i >= len(model.Sheets) {
			break
		}
		sd := &model.Sheets[i]
		nuevos := segmentadoresDeHoja(files, part, caches, sd, pivotes)
		for _, n := range nuevos {
			if total >= maxSegmentadores || existeSegmentador(sd.Slicers, n) {
				continue
			}
			sd.Slicers = append(sd.Slicers, n)
			total++
		}
	}
}

func existeSegmentador(lista []sheetSlicer, n sheetSlicer) bool {
	for _, s := range lista {
		if s.Col == n.Col && s.Pivot == n.Pivot {
			return true
		}
	}
	return false
}

// partesDeDinamicas es la parte de cada tabla dinámica por su nombre.
func partesDeDinamicas(files map[string][]byte, sheetParts []string) map[string]string {
	out := map[string]string{}
	for _, part := range sheetParts {
		for _, r := range parseRelsTyped(files[path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")]) {
			if !strings.HasSuffix(r.Type, "/pivotTable") {
				continue
			}
			p := destinoRel(part, r.Target)
			var t struct {
				Name string `xml:"name,attr"`
			}
			if raw := parteAcotada(files, p); len(raw) > 0 && xml.Unmarshal(raw, &t) == nil && t.Name != "" {
				out[t.Name] = p
			}
		}
	}
	return out
}

// segmentadoresDeHoja lee los de la hoja `part` (su `slicerList`) y deja el autofiltro de
// `sd` cubriendo la columna de cada uno de tabla, como hace el editor al crearlos.
func segmentadoresDeHoja(files map[string][]byte, part string, caches map[string]cacheSegmentador, sd *sheetData, pivotes map[string]string) []sheetSlicer {
	var ws struct {
		Drawing struct {
			ID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"drawing"`
		Listas []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"extLst>ext>slicerList>slicer"`
	}
	raw := files[part]
	if len(raw) == 0 || xml.Unmarshal(raw, &ws) != nil || len(ws.Listas) == 0 {
		return nil
	}
	rels := relsDe(files, part)
	tablas := tablasDeHoja(files, part)
	pos := map[string][2]float64{}
	if ws.Drawing.ID != "" {
		pos = posicionesDeSegmentadores(files, destinoRel(part, rels[ws.Drawing.ID]))
	}
	var out []sheetSlicer
	for _, l := range ws.Listas {
		var lista struct {
			Segs []struct {
				Name  string `xml:"name,attr"`
				Cache string `xml:"cache,attr"`
			} `xml:"slicer"`
		}
		b := parteAcotada(files, destinoRel(part, rels[l.RID]))
		if len(b) == 0 || xml.Unmarshal(b, &lista) != nil {
			continue
		}
		for _, s := range lista.Segs {
			if len(out) >= maxSegmentadores {
				return out
			}
			c, ok := caches[s.Cache]
			if !ok {
				continue // caché que no existe
			}
			seg, ok := segmentadorDeCache(files, c, sd, tablas, pivotes)
			if !ok {
				continue
			}
			seg.ID = idNuevo()
			if p, ok := pos[s.Name]; ok {
				x, y := p[0], p[1]
				seg.X, seg.Y = &x, &y
			}
			out = append(out, seg)
		}
	}
	return out
}

// segmentadorDeCache resuelve a qué apunta una caché: una columna de una tabla de la
// hoja o un campo de una tabla dinámica del libro. No sale si lo que segmenta no existe.
func segmentadorDeCache(files map[string][]byte, c cacheSegmentador, sd *sheetData, tablas map[int]tablaDeSegmentador, pivotes map[string]string) (sheetSlicer, bool) {
	if c.Tabla != nil && c.Tabla.TableID != nil {
		t, ok := tablas[*c.Tabla.TableID]
		if !ok {
			return sheetSlicer{}, false
		}
		idx := columnaDeTabla(t, c)
		if idx < 0 || t.ref.c1-1+idx > t.ref.c2-1 {
			return sheetSlicer{}, false
		}
		col := t.ref.c1 - 1 + idx
		cubrirConAutofiltro(sd, t.ref, col)
		return sheetSlicer{Col: col}, true
	}
	for _, p := range c.Pivots {
		if !tieneDinamica(sd, p.Name) {
			continue
		}
		var cache xlsxCacheXML
		if cd, ok := xlsxPivotCacheOf(files, pivotes[p.Name]); ok {
			cache = cd
		}
		for k, f := range cache.Fields {
			if f.Name == c.SourceName && c.SourceName != "" {
				return sheetSlicer{Col: k, Pivot: p.Name}, true
			}
		}
	}
	return sheetSlicer{}, false
}

// columnaDeTabla es el índice de la columna que segmenta la caché: por el id de
// tableSlicerCache@column y, si no cuadra, por el nombre (sourceName); -1 si no hay.
func columnaDeTabla(t tablaDeSegmentador, c cacheSegmentador) int {
	for k, col := range t.cols {
		if c.Tabla.Column != nil && col.ID == *c.Tabla.Column {
			return k
		}
	}
	for k, col := range t.cols {
		if c.SourceName != "" && col.Name == c.SourceName {
			return k
		}
	}
	return -1
}

func tieneDinamica(sd *sheetData, nombre string) bool {
	for _, p := range sd.Pivots {
		if p.Name == nombre {
			return true
		}
	}
	return false
}

// cubrirConAutofiltro hace que el autofiltro de la hoja cubra la columna segmentada: si no
// hay, lo crea sobre la tabla; si hay, lo extiende (un autofiltro por hoja, como el editor).
func cubrirConAutofiltro(sd *sheetData, t rango, col int) {
	if sd.AutoFilter == nil {
		sd.AutoFilter = &sheetAutoFilter{Row: t.r1 - 1, C1: t.c1 - 1, C2: t.c2 - 1}
		return
	}
	sd.AutoFilter.C1 = min(sd.AutoFilter.C1, col)
	sd.AutoFilter.C2 = max(sd.AutoFilter.C2, col)
}

// tablasDeHoja son las tablas de la hoja por su `id` (el que cita tableSlicerCache).
func tablasDeHoja(files map[string][]byte, part string) map[int]tablaDeSegmentador {
	out := map[int]tablaDeSegmentador{}
	for _, r := range parseRelsTyped(files[path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")]) {
		if !strings.HasSuffix(r.Type, "/table") {
			continue
		}
		var t struct {
			ID   string `xml:"id,attr"`
			Ref  string `xml:"ref,attr"`
			Cols []struct {
				ID   int    `xml:"id,attr"`
				Name string `xml:"name,attr"`
			} `xml:"tableColumns>tableColumn"`
		}
		raw := parteAcotada(files, destinoRel(part, r.Target))
		if len(raw) == 0 || xml.Unmarshal(raw, &t) != nil {
			continue
		}
		id, err := strconv.Atoi(t.ID)
		if err != nil {
			continue
		}
		if ref, ok := leerRango(t.Ref); ok {
			out[id] = tablaDeSegmentador{ref: ref, cols: t.Cols}
		}
	}
	return out
}

// posicionesDeSegmentadores es la posición en px de cada tarjeta del dibujo, por el
// `name` de su `sle:slicer` (ECMA-376 §20.5.2.x para las anclas; el marco va dentro de un
// mc:AlternateContent que solo entiende Excel 2010+).
func posicionesDeSegmentadores(files map[string][]byte, dibujo string) map[string][2]float64 {
	type ancla struct {
		From anclaXlsx `xml:"from"`
		Pos  *struct {
			X int64 `xml:"x,attr"`
			Y int64 `xml:"y,attr"`
		} `xml:"pos"`
		Segs []struct {
			Name string `xml:"name,attr"`
		} `xml:"graphicFrame>graphic>graphicData>slicer"`
	}
	// Excel envuelve el ancla entera en mc:AlternateContent/mc:Choice; otros la dejan suelta.
	var d struct {
		Uno     []ancla `xml:"oneCellAnchor"`
		Dos     []ancla `xml:"twoCellAnchor"`
		Abso    []ancla `xml:"absoluteAnchor"`
		AltUno  []ancla `xml:"AlternateContent>Choice>oneCellAnchor"`
		AltDos  []ancla `xml:"AlternateContent>Choice>twoCellAnchor"`
		AltAbso []ancla `xml:"AlternateContent>Choice>absoluteAnchor"`
	}
	out := map[string][2]float64{}
	raw := parteAcotada(files, dibujo)
	if len(raw) == 0 || xml.Unmarshal(raw, &d) != nil {
		return out
	}
	for tipo, lista := range [][]ancla{d.Uno, d.Dos, d.Abso, d.AltUno, d.AltDos, d.AltAbso} {
		for _, a := range lista {
			x, y := a.From.px()
			if tipo%3 == 2 && a.Pos != nil {
				x, y = emuAPx(a.Pos.X), emuAPx(a.Pos.Y)
			}
			for _, s := range a.Segs {
				out[s.Name] = [2]float64{float64(int(x)), float64(int(y))}
			}
		}
	}
	return out
}

// sinNombresDeSegmentador quita de los nombres definidos los `Slicer_*` con `#N/A` que
// Excel escribe por cada caché: no son rangos del usuario.
func sinNombresDeSegmentador(defs []xlsxDefinedName) []xlsxDefinedName {
	out := defs[:0:0]
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "Slicer_") && strings.HasPrefix(strings.TrimSpace(d.Ref), "#N/A") {
			continue
		}
		out = append(out, d)
	}
	return out
}
