// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package dominio

import (
	"bytes"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// El EVALUADOR EN MEMORIA de una vista (PLAN-EDICION-LOCAL.md §6.12, B-02): lo mismo que
// el servicio traduce a SQL en `services/bases/internal/store/filtros.go`, aplicado a
// registros que ya están en memoria (Sytue Office, sin base de datos). Cada rama copia
// la semántica de su SQL, con sus NULL incluidos:
//
//   - `r.data ->> campo` es el texto del valor; un campo ausente o `null` es NULL.
//   - `empty` / `notempty` son IS NULL / IS NOT NULL (un texto vacío NO es NULL).
//   - Una comparación con NULL es falsa, salvo `neq`, que es IS DISTINCT FROM.
//   - `contains` es `position(lower(v) in lower(coalesce(col, '')))`: sin comodines.
//   - El orden deja los NULL al final en las dos direcciones, un texto ordena por sus
//     MaxClaveOrden primeras letras y los empates se deshacen por antigüedad y por id.
//
// Los vectores de VectoresDeVista son los de las pruebas del SQL del servidor
// (`store_integration_test.go`, TestVistasYFiltros y TestBusquedaYOrdenAdHoc): los dos
// lados han de dar lo mismo.

// FiltrarYOrdenar devuelve los registros que cumplen la vista, en su orden. No toca los
// registros que recibe. Un filtro o un orden sobre un campo que ya no existe se ignora,
// como en el servicio.
func FiltrarYOrdenar(campos []Campo, registros []Registro, cfg ConfigVista) []Registro {
	porID := make(map[string]Campo, len(campos))
	for _, c := range campos {
		porID[c.ID] = c.Efectivo() // una fórmula se filtra y ordena como su resultado
	}
	out := make([]Registro, 0, len(registros))
	busqueda := strings.TrimSpace(cfg.Search)
	for _, r := range registros {
		if cumpleFiltros(porID, cfg.Filters, r) && (busqueda == "" || cumpleBusqueda(campos, porID, busqueda, r)) {
			out = append(out, r)
		}
	}
	var claves []Orden
	for _, o := range cfg.Sort {
		if _, ok := porID[o.Field]; ok {
			claves = append(claves, o)
		}
	}
	slices.SortStableFunc(out, func(a, b Registro) int {
		for _, o := range claves {
			c := porID[o.Field]
			if n := compararClave(c, a, b, o.Dir == "desc"); n != 0 {
				return n
			}
		}
		if n := a.CreatedAt.Compare(b.CreatedAt); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// textoDe es `r.data ->> campo`: el texto del valor, o ok=false si es NULL.
func textoDe(r Registro, campo string) (string, bool) {
	raw := bytes.TrimSpace(r.Data[campo])
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return s, true
	}
	return string(raw), true
}

// numeroDe es `col::numeric`: exacto, como el numeric de Postgres.
func numeroDe(s string) (*big.Rat, bool) {
	n, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	return n, ok
}

func cumpleFiltros(porID map[string]Campo, filtros []Filtro, r Registro) bool {
	for _, f := range filtros {
		c, ok := porID[f.Field]
		if !ok {
			continue
		}
		cumple, aplica := cumpleFiltro(c, f, r)
		if aplica && !cumple {
			return false
		}
	}
	return true
}

// cumpleFiltro dice si r cumple f; aplica=false es un filtro que el SQL tampoco escribe
// (un operador o un valor que no se entiende).
//
//nolint:gocognit,gocyclo // un caso por operador y tipo, en el orden de construirConsulta.
func cumpleFiltro(c Campo, f Filtro, r Registro) (cumple, aplica bool) {
	if c.Kind == TipoRelacion {
		switch f.Op {
		case "notempty":
			return len(r.Links[c.ID]) > 0, true
		case "empty":
			return len(r.Links[c.ID]) == 0, true
		}
		return false, false
	}
	col, hay := textoDe(r, c.ID)
	switch f.Op {
	case "empty":
		return !hay, true
	case "notempty":
		return hay, true
	}
	v, ok := valorDeFiltro(c, f)
	if !ok {
		return false, false
	}
	switch {
	case c.Kind == TipoMultiple && f.Op == "contains":
		var lista []any
		if json.Unmarshal(r.Data[c.ID], &lista) != nil {
			return false, true
		}
		return slices.Contains(lista, any(v)), true
	case c.Kind == TipoCasilla:
		b := hay && col == "true"
		return strconv.FormatBool(b) == v, true
	case f.Op == "contains":
		return buscar.Contiene(col, v), true
	case c.Kind == TipoNumero && (f.Op == "eq" || f.Op == "neq"):
		igual := false
		if hay {
			a, okA := numeroDe(col)
			b, okB := numeroDe(v)
			igual = okA && okB && a.Cmp(b) == 0
		}
		if f.Op == "eq" {
			return igual, true
		}
		return !igual, true
	case f.Op == "eq":
		return hay && col == v, true
	case f.Op == "neq":
		return !hay || col != v, true
	}
	var cmp int
	if !hay {
		_, valido := map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true}[f.Op]
		return false, valido
	}
	if c.Kind == TipoNumero {
		a, okA := numeroDe(col)
		b, okB := numeroDe(v)
		if !okA || !okB {
			return false, true
		}
		cmp = a.Cmp(b)
	} else { // fechas ISO: el orden de texto es el cronológico
		cmp = strings.Compare(col, v)
	}
	switch f.Op {
	case "gt":
		return cmp > 0, true
	case "gte":
		return cmp >= 0, true
	case "lt":
		return cmp < 0, true
	case "lte":
		return cmp <= 0, true
	}
	return false, false
}

// valorDeFiltro es `valorFiltro` del servicio: el valor del filtro como texto.
func valorDeFiltro(c Campo, f Filtro) (string, bool) {
	switch c.Kind { //nolint:exhaustive // el resto de tipos lleva un texto
	case TipoNumero:
		var n float64
		if json.Unmarshal(f.Value, &n) != nil {
			return "", false
		}
		return strconv.FormatFloat(n, 'f', -1, 64), true
	case TipoCasilla:
		var b bool
		if json.Unmarshal(f.Value, &b) != nil {
			return "", false
		}
		return strconv.FormatBool(b), true
	}
	var s string
	if json.Unmarshal(f.Value, &s) != nil {
		return "", false
	}
	return s, true
}

// cumpleBusqueda es `condicionBusqueda`: el texto en alguno de los campos con valor
// escrito; enlaces, adjuntos y casillas no cuentan, y sin campos buscables no casa nada.
func cumpleBusqueda(campos []Campo, porID map[string]Campo, texto string, r Registro) bool {
	aguja := buscar.Plegar(texto)
	for _, c0 := range campos {
		c := porID[c0.ID]
		switch c.Kind { //nolint:exhaustive // el resto lleva un valor con texto
		case TipoRelacion, TipoAdjunto, TipoCasilla:
			continue
		}
		col, _ := textoDe(r, c.ID)
		if strings.Contains(buscar.Plegar(col), aguja) {
			return true
		}
	}
	return false
}

// compararClave compara a y b por un campo, con los NULL al final en las dos
// direcciones (NULLS LAST).
func compararClave(c Campo, a, b Registro, desc bool) int {
	va, okA := claveDe(c, a)
	vb, okB := claveDe(c, b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return 1
	case !okB:
		return -1
	}
	// claveDe da el mismo tipo a las dos (es el mismo campo): las comprobaciones de
	// tipo de vb no fallan, y si fallaran las dos claves contarían como iguales.
	var n int
	switch x := va.(type) {
	case *big.Rat:
		if y, ok := vb.(*big.Rat); ok {
			n = x.Cmp(y)
		}
	case bool:
		if y, ok := vb.(bool); ok {
			n = compararBool(x, y)
		}
	case string:
		if y, ok := vb.(string); ok {
			n = strings.Compare(x, y)
		}
	}
	if desc {
		return -n
	}
	return n
}

// compararBool ordena false antes que true, como boolean en SQL.
func compararBool(x, y bool) int {
	switch {
	case x == y:
		return 0
	case !x:
		return -1
	default:
		return 1
	}
}

// claveDe es la expresión del ORDER BY: numeric, boolean o el principio del texto.
func claveDe(c Campo, r Registro) (any, bool) {
	col, hay := textoDe(r, c.ID)
	if !hay {
		return nil, false
	}
	switch c.Kind { //nolint:exhaustive // el resto se ordena como texto
	case TipoNumero:
		n, ok := numeroDe(col)
		return n, ok
	case TipoCasilla:
		return col == "true", col == "true" || col == "false"
	}
	if utf8.RuneCountInString(col) > MaxClaveOrden {
		col = string([]rune(col)[:MaxClaveOrden])
	}
	return col, true
}
