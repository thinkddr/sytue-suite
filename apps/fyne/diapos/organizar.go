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
