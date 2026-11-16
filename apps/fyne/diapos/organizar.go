// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"math"
	"slices"
	"sort"
)

// Organizar objetos: `slides/organize.ts`, pura. Todo trabaja en % del lienzo sobre las
// cajas de la diapositiva (el orden del array es el orden en Z: más al final, más
// arriba), y devuelve un array NUEVO con las cajas no tocadas intactas.

// rect es `Rect`: x/y/w/h en % del lienzo.
type rect struct{ x, y, w, h float64 }

func rectDe(c Caja) rect { return rect{c.X, c.Y, c.W, c.H} }

// r3 redondea a milésimas, como la web.
func r3(n float64) float64 { return math.Round(n*1000) / 1000 }

// acotarPos es `clampPos`: la caja no se sale por ningún borde.
func acotarPos(v, tam float64) float64 { return min(100-tam, max(0, v)) }

// envolvente es `boundingBox`.
func envolvente(rs []rect) rect {
	if len(rs) == 0 {
		return rect{}
	}
	x1, y1, x2, y2 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, r := range rs {
		x1, y1 = min(x1, r.x), min(y1, r.y)
		x2, y2 = max(x2, r.x+r.w), max(y2, r.y+r.h)
	}
	return rect{x1, y1, x2 - x1, y2 - y1}
}

func conjunto(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func partir(cajas []Caja, sel map[string]bool) (dentro, fuera []Caja) {
	for _, c := range cajas {
		if sel[c.ID] {
			dentro = append(dentro, c)
		} else {
			fuera = append(fuera, c)
		}
	}
	return dentro, fuera
}

// ExpandirGrupos es `expandGroups`: mover, alinear o animar una caja de un grupo actúa
// sobre el grupo entero. En el orden del array y sin ids inexistentes.
func ExpandirGrupos(cajas []Caja, ids []string) []string {
	sel := conjunto(ids)
	grupos := map[string]bool{}
	for _, c := range cajas {
		if sel[c.ID] && c.Grupo != "" {
			grupos[c.Grupo] = true
		}
	}
	var out []string
	for _, c := range cajas {
		if sel[c.ID] || (c.Grupo != "" && grupos[c.Grupo]) {
			out = append(out, c.ID)
		}
	}
	return out
}

// ReordenarZ es `reorderZ`: "front" y "back" llevan el bloque al extremo; "forward" y
// "backward" lo pasan al otro lado del vecino inmediato. Una selección dispersa se
// compacta.
func ReordenarZ(cajas []Caja, ids []string, op string) []Caja {
	sel := conjunto(ids)
	elegidas, otras := partir(cajas, sel)
	if len(elegidas) == 0 || len(otras) == 0 {
		return cajas
	}
	switch op {
	case "front":
		return append(otras, elegidas...)
	case "back":
		return append(elegidas, otras...)
	}
	lo, hi := len(cajas), -1
	for i, c := range cajas {
		if sel[c.ID] {
			lo, hi = min(lo, i), max(hi, i)
		}
	}
	out := make([]Caja, 0, len(cajas))
	switch op {
	case "forward":
		if hi >= len(cajas)-1 {
			return cajas
		}
		vecino := cajas[hi+1].ID
		for _, c := range otras {
			out = append(out, c)
			if c.ID == vecino {
				out = append(out, elegidas...)
			}
		}
	case "backward":
		if lo <= 0 {
			return cajas
		}
		vecino := cajas[lo-1].ID
		for _, c := range otras {
			if c.ID == vecino {
				out = append(out, elegidas...)
			}
			out = append(out, c)
		}
	default:
		return cajas
	}
	return out
}

// Alinear es `alignBoxes`: respecto a la envolvente de la selección o, con
// `alLienzo`, al lienzo entero. Bordes: left, hcenter, right, top, vmiddle, bottom.
func Alinear(cajas []Caja, ids []string, borde string, alLienzo bool) []Caja {
	sel := conjunto(ids)
	elegidas, _ := partir(cajas, sel)
	if len(elegidas) == 0 {
		return cajas
	}
	ref := rect{0, 0, 100, 100}
	if !alLienzo {
		rs := make([]rect, len(elegidas))
		for i, c := range elegidas {
			rs[i] = rectDe(c)
		}
		ref = envolvente(rs)
	}
	out := slices.Clone(cajas)
	for i := range out {
		c := &out[i]
		if !sel[c.ID] {
			continue
		}
		switch borde {
		case "left":
			c.X = r3(acotarPos(ref.x, c.W))
		case "hcenter":
			c.X = r3(acotarPos(ref.x+(ref.w-c.W)/2, c.W))
		case "right":
			c.X = r3(acotarPos(ref.x+ref.w-c.W, c.W))
		case "top":
			c.Y = r3(acotarPos(ref.y, c.H))
		case "vmiddle":
			c.Y = r3(acotarPos(ref.y+(ref.h-c.H)/2, c.H))
		case "bottom":
			c.Y = r3(acotarPos(ref.y+ref.h-c.H, c.H))
		}
	}
	return out
}

// Distribuir es `distributeBoxes`: huecos iguales entre bordes consecutivos a lo largo
// del eje ("h" o "v"); los extremos quedan fijos. Hacen falta tres cajas o más.
func Distribuir(cajas []Caja, ids []string, eje string) []Caja {
	sel := conjunto(ids)
	elegidas, _ := partir(cajas, sel)
	if len(elegidas) < 3 {
		return cajas
	}
	inicio := func(c Caja) float64 {
		if eje == "h" {
			return c.X
		}
		return c.Y
	}
	tam := func(c Caja) float64 {
		if eje == "h" {
			return c.W
		}
		return c.H
	}
	orden := slices.Clone(elegidas)
	sort.SliceStable(orden, func(a, b int) bool {
		ia, ib := inicio(orden[a]), inicio(orden[b])
		if ia != ib {
			return ia < ib
		}
		return ia+tam(orden[a])/2 < ib+tam(orden[b])/2
	})
	primera, ultima := orden[0], orden[len(orden)-1]
	tramo := inicio(ultima) + tam(ultima) - inicio(primera)
	suma := 0.0
	for _, c := range orden {
		suma += tam(c)
	}
	hueco := (tramo - suma) / float64(len(orden)-1)
	en := map[string]float64{}
	cursor := inicio(primera)
	for _, c := range orden {
		en[c.ID] = r3(cursor)
		cursor += tam(c) + hueco
	}
	out := slices.Clone(cajas)
	for i := range out {
		if p, ok := en[out[i].ID]; ok {
			if eje == "h" {
				out[i].X = p
			} else {
				out[i].Y = p
			}
		}
	}
	return out
}

// Agrupar es `groupBoxes`: las cajas bajo un `groupId` común, compactadas a un rango
// contiguo en Z donde estaba su miembro más al frente. Necesita dos o más.
func Agrupar(cajas []Caja, ids []string, grupo string) []Caja {
	sel := conjunto(ids)
	if elegidas, _ := partir(cajas, sel); len(elegidas) < 2 {
		return cajas
	}
	marcadas := slices.Clone(cajas)
	for i := range marcadas {
		if sel[marcadas[i].ID] {
			marcadas[i].Grupo = grupo
		}
	}
	return compactarBloque(marcadas, sel)
}

// compactarBloque es `compact`.
func compactarBloque(cajas []Caja, sel map[string]bool) []Caja {
	elegidas, otras := partir(cajas, sel)
	hi := -1
	for i, c := range cajas {
		if sel[c.ID] {
			hi = i
		}
	}
	debajo := 0
	for i, c := range cajas {
		if !sel[c.ID] && i < hi {
			debajo++
		}
	}
	out := make([]Caja, 0, len(cajas))
	out = append(out, otras[:debajo]...)
	out = append(out, elegidas...)
	return append(out, otras[debajo:]...)
}

// Desagrupar es `ungroupBoxes`: quita el grupo a todas las cajas de los grupos tocados.
func Desagrupar(cajas []Caja, ids []string) []Caja {
	sel := conjunto(ids)
	grupos := map[string]bool{}
	for _, c := range cajas {
		if sel[c.ID] && c.Grupo != "" {
			grupos[c.Grupo] = true
		}
	}
	if len(grupos) == 0 {
		return cajas
	}
	out := slices.Clone(cajas)
	for i := range out {
		if out[i].Grupo != "" && grupos[out[i].Grupo] {
			out[i].Grupo = ""
		}
	}
	return out
}

// ---- Guías inteligentes (ajuste magnético al arrastrar) ----

// Ancla de una caja en un eje: borde inicial, centro o borde final (`AnclaGuia`).
const (
	anclaIni    = "ini"
	anclaCentro = "centro"
	anclaFin    = "fin"
)

var anclasGuia = []string{anclaIni, anclaCentro, anclaFin}

// Guia es `Guia`: línea vertical (eje "x", en la abscisa Pos, de Desde a Hasta en y)
// u horizontal (eje "y"). Solo de vista: nunca se guarda en el modelo.
type Guia struct {
	Eje   string  `json:"eje"`
	Pos   float64 `json:"pos"`
	Desde float64 `json:"desde"`
	Hasta float64 `json:"hasta"`
}

// AjusteGuias es `AjusteGuias`: corrección a sumar al delta y guías visibles.
type AjusteGuias struct {
	DX     float64 `json:"dx"`
	DY     float64 `json:"dy"`
	Lineas []Guia  `json:"lineas"`
}

func enAncla(ini, lado float64, a string) float64 {
	switch a {
	case anclaIni:
		return ini
	case anclaCentro:
		return ini + lado/2
	}
	return ini + lado
}

// epsGuia absorbe el ruido de coma flotante al comparar distancias a las guías.
const epsGuia = 1e-6

// proyeccionGuia lee una caja en el eje e: posición y lado en ese eje, y posición y
// lado en el otro (por donde se extiende la línea guía).
func proyeccionGuia(e string, r rect) (p, lado, q, ladoQ float64) {
	if e == "x" {
		return r.x, r.w, r.y, r.h
	}
	return r.y, r.h, r.x, r.w
}

// mejorAjusteGuia es la corrección más pequeña en el eje e que pega alguna ancla
// propia de mov a un borde o centro de los objetivos, dentro de tol (+Inf si no hay).
// En empate gana el primero: el lienzo, luego otros en su orden y la primera ancla.
func mejorAjusteGuia(e string, mov rect, objetivos []rect, tol float64, propias []string) float64 {
	mp, ml, _, _ := proyeccionGuia(e, mov)
	mejor := math.Inf(1)
	for _, o := range objetivos {
		op, ol, _, _ := proyeccionGuia(e, o)
		for _, ao := range anclasGuia {
			t := enAncla(op, ol, ao)
			for _, am := range propias {
				d := t - enAncla(mp, ml, am)
				if math.Abs(d) <= tol+epsGuia && math.Abs(d) < math.Abs(mejor)-epsGuia {
					mejor = d
				}
			}
		}
	}
	return mejor
}

// coincideAncla dice si t cae sobre alguna ancla propia de la caja ya corregida.
func coincideAncla(t, mp, ml float64, propias []string) bool {
	for _, am := range propias {
		if math.Abs(t-enAncla(mp, ml, am)) <= epsGuia*1000 {
			return true
		}
	}
	return false
}

// lineasDeGuia son las guías del eje e sobre fin (mov ya corregido en los dos ejes,
// para que la línea llegue a donde queda la caja): una por posición, con la extensión
// que cubre fin y todos los objetivos que coinciden, ordenadas por posición.
func lineasDeGuia(e string, fin rect, objetivos []rect, propias []string) []Guia {
	mp, ml, mq, mlq := proyeccionGuia(e, fin)
	var lineas []Guia
	idx := map[float64]int{}
	for _, o := range objetivos {
		op, ol, oq, olq := proyeccionGuia(e, o)
		for _, ao := range anclasGuia {
			t := r3(enAncla(op, ol, ao))
			if !coincideAncla(t, mp, ml, propias) {
				continue
			}
			desde, hasta := r3(min(oq, mq)), r3(max(oq+olq, mq+mlq))
			if i, ok := idx[t]; ok {
				lineas[i].Desde, lineas[i].Hasta = min(lineas[i].Desde, desde), max(lineas[i].Hasta, hasta)
				continue
			}
			idx[t] = len(lineas)
			lineas = append(lineas, Guia{Eje: e, Pos: t, Desde: desde, Hasta: hasta})
		}
	}
	sort.SliceStable(lineas, func(i, j int) bool { return lineas[i].Pos < lineas[j].Pos })
	return lineas
}

// correccionEnEje es la corrección del eje e (+Inf si no pega). propias vacías son
// las tres anclas.
func correccionEnEje(e string, mov rect, objetivos []rect, tol float64, propias []string) float64 {
	if len(propias) == 0 {
		propias = anclasGuia
	}
	return mejorAjusteGuia(e, mov, objetivos, tol, propias)
}

// lineasEnEje son las guías del eje e (ninguna si no pegó).
func lineasEnEje(e string, fin rect, objetivos []rect, corr float64, propias []string) []Guia {
	if math.IsInf(corr, 1) {
		return nil
	}
	if len(propias) == 0 {
		propias = anclasGuia
	}
	return lineasDeGuia(e, fin, objetivos, propias)
}

// sinInf es la corrección, o 0 si no pegó.
func sinInf(corr float64) float64 {
	if math.IsInf(corr, 1) {
		return 0
	}
	return corr
}

// guiasDeAjuste es el espejo de `guiasDeAjuste` (@sytue/escena, organizar.ts): misma
// salida para los vectores de testdata/guias.vectores.json. anclasX/anclasY vacías
// son las tres anclas.
func guiasDeAjuste(mov rect, otros []rect, lienzo rect, tolX, tolY float64, anclasX, anclasY []string) AjusteGuias {
	objetivos := append([]rect{lienzo}, otros...)
	cx := correccionEnEje("x", mov, objetivos, tolX, anclasX)
	cy := correccionEnEje("y", mov, objetivos, tolY, anclasY)
	fin := rect{mov.x + sinInf(cx), mov.y + sinInf(cy), mov.w, mov.h}
	lx := lineasEnEje("x", fin, objetivos, cx, anclasX)
	ly := lineasEnEje("y", fin, objetivos, cy, anclasY)
	return AjusteGuias{DX: r3(sinInf(cx)), DY: r3(sinInf(cy)), Lineas: append(append([]Guia{}, lx...), ly...)}
}
