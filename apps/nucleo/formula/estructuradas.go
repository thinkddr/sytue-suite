// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"slices"
	"strconv"
	"strings"
)

// TablaResoluble es `ResolvableTable` de `sheets/table.ts`: una tabla con formato ya
// leída —su hoja, su rectángulo, si lleva cabecera y fila de totales— y los nombres de
// sus columnas. La comparten el lienzo de Calcus y las descargas del servicio de Docs,
// que resuelven `Tabla[Columna]` igual.
type TablaResoluble struct {
	Nombre            string
	Hoja              string
	Rect              Rect
	Cabecera, Totales bool
	Columnas          []string
}

// cuerpo son las filas de datos; la segunda es menor que la primera si no hay ninguna.
func (t *TablaResoluble) cuerpo() (int, int) {
	f1, f2 := t.Rect.R1, t.Rect.R2
	if t.Cabecera {
		f1++
	}
	if t.Totales {
		f2--
	}
	return f1, f2
}

// NuevaTablaResoluble es `collectResolvableTables` para una tabla: `rango` en A1,
// `cabecera` da el texto crudo de una celda de su hoja. false si el rango no se lee.
func NuevaTablaResoluble(nombre, hoja, rango string, conCabecera, conTotales bool, cabecera func(c, f int) string) (TablaResoluble, bool) {
	r, ok := ParseRangeRef(rango)
	if !ok {
		return TablaResoluble{}, false
	}
	crudos := make([]string, r.C2-r.C1+1)
	if conCabecera {
		for k := range crudos {
			crudos[k] = cabecera(r.C1+k, r.R1)
		}
	}
	return TablaResoluble{
		Nombre: nombre, Hoja: hoja, Rect: r, Cabecera: conCabecera, Totales: conTotales,
		Columnas: NombresCabecera(crudos),
	}, true
}

// NombresCabecera es `uniqueHeaderNames`: el texto de cada cabecera recortado, «ColumnaN»
// si está vacía y un número detrás si se repite (sin distinguir mayúsculas).
func NombresCabecera(crudos []string) []string {
	out := make([]string, 0, len(crudos))
	usados := map[string]bool{}
	for i, r := range crudos {
		n := jsTrim(r)
		if n == "" {
			n = "Columna" + strconv.Itoa(i+1)
		}
		if usados[strings.ToLower(n)] {
			k := 2
			for usados[strings.ToLower(n+strconv.Itoa(k))] {
				k++
			}
			n += strconv.Itoa(k)
		}
		usados[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out
}

// ResolverEstructurada es `resolveStructuredRef`: el rango de `Tabla[Columna]` visto
// desde la celda `aqui`. nil sin error es «fuera de toda tabla» (el motor da #REF!).
//
//nolint:gocyclo // un caso por especificador (#Datos, #Cabeceras, #Totales, #Todo, @), como `resolveStructuredRef`
func ResolverEstructurada(tablas []TablaResoluble, q TableQuery, aqui Here) (*RefRange, Error) {
	var def *TablaResoluble
	if q.Table == "" {
		for i := range tablas {
			t := &tablas[i]
			if t.Hoja == aqui.Sheet && aqui.Col >= t.Rect.C1 && aqui.Col <= t.Rect.C2 && aqui.Row >= t.Rect.R1 && aqui.Row <= t.Rect.R2 {
				def = t
				break
			}
		}
		if def == nil {
			return nil, ""
		}
	} else {
		for i := range tablas {
			if strings.EqualFold(tablas[i].Nombre, q.Table) {
				def = &tablas[i]
				break
			}
		}
		if def == nil {
			return nil, ErrName
		}
	}
	c1, c2 := def.Rect.C1, def.Rect.C2
	if q.HasColumn {
		i := slices.IndexFunc(def.Columnas, func(n string) bool { return strings.EqualFold(n, q.Column) })
		if i < 0 {
			return nil, ErrRef
		}
		c1, c2 = def.Rect.C1+i, def.Rect.C1+i
	}
	cuerpo1, cuerpo2 := def.cuerpo()
	var f1, f2 int
	switch q.Area {
	case AreaHeaders:
		if !def.Cabecera {
			return nil, ErrRef
		}
		f1, f2 = def.Rect.R1, def.Rect.R1
	case AreaTotals:
		if !def.Totales {
			return nil, ErrRef
		}
		f1, f2 = def.Rect.R2, def.Rect.R2
	case AreaAll:
		f1, f2 = def.Rect.R1, def.Rect.R2
	default:
		f1, f2 = cuerpo1, cuerpo2
		if f2 < f1 {
			return nil, ErrRef
		}
	}
	if q.ThisRow {
		if aqui.Sheet != def.Hoja || aqui.Row < cuerpo1 || aqui.Row > cuerpo2 {
			return nil, ErrValue
		}
		f1, f2 = aqui.Row, aqui.Row
	}
	return &RefRange{Sheet: def.Hoja, C1: c1, R1: f1, C2: c2, R2: f2}, ""
}
