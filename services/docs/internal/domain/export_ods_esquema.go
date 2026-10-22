// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"slices"
	"strings"
)

// El esquema (sheet_esquema.go) en ODF: grupos `table:table-row-group` y
// `table:table-column-group` anidados, con `table:display="false"` el colapsado.

// grupoODS es un grupo en unidades 1-based, ambos extremos incluidos.
type grupoODS struct {
	ini, fin  int
	colapsado bool
}

// gruposAnidados deja solo los grupos que se anidan bien (ODF no admite que se crucen),
// ordenados de fuera adentro. Los que se cruzan con uno ya aceptado se descartan.
func gruposAnidados(gs []sheetGroup, limite int) []grupoODS {
	var out, abiertos []grupoODS
	for _, g := range sanearGrupos(gs, limite) {
		n := grupoODS{g.Start + 1, g.End + 1, g.Collapsed}
		for len(abiertos) > 0 && abiertos[len(abiertos)-1].fin < n.ini {
			abiertos = abiertos[:len(abiertos)-1]
		}
		if len(abiertos) > 0 && abiertos[len(abiertos)-1].fin < n.fin {
			continue
		}
		abiertos = append(abiertos, n)
		out = append(out, n)
	}
	return out
}

// pilaODS abre y cierra los grupos de un eje según avanza la unidad.
type pilaODS struct {
	pend     []grupoODS // por abrir, de fuera adentro
	abiertos []grupoODS
	tag      string
}

func nuevaPilaODS(gs []grupoODS, tag string) *pilaODS { return &pilaODS{pend: gs, tag: tag} }

// avanzar cierra los grupos que terminaron antes de `u` y abre los que empiezan en `u`.
func (p *pilaODS) avanzar(u int, b *strings.Builder) {
	for len(p.abiertos) > 0 && p.abiertos[len(p.abiertos)-1].fin < u {
		p.abiertos = p.abiertos[:len(p.abiertos)-1]
		b.WriteString(`</table:` + p.tag + `>`)
	}
	for len(p.pend) > 0 && p.pend[0].ini <= u {
		g := p.pend[0]
		p.pend = p.pend[1:]
		p.abiertos = append(p.abiertos, g)
		b.WriteString(`<table:` + p.tag)
		if g.colapsado {
			b.WriteString(` table:display="false"`)
		}
		b.WriteString(`>`)
	}
}

func (p *pilaODS) cerrar(b *strings.Builder) {
	for range p.abiertos {
		b.WriteString(`</table:` + p.tag + `>`)
	}
	p.abiertos = nil
}

// cortes son las unidades (1-based) donde empieza un grupo o donde acaba uno más uno,
// ordenadas: los bloques de unidades vacías repetidas se parten ahí.
func cortes(gs []grupoODS) []int {
	out := make([]int, 0, 2*len(gs))
	for _, g := range gs {
		out = append(out, g.ini, g.fin+1)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// trozos parte [desde, hasta] por los cortes, devolviendo pares (inicio, fin).
func trozos(desde, hasta int, cs []int) [][2]int {
	var out [][2]int
	ini := desde
	for _, c := range cs {
		if c > ini && c <= hasta {
			out = append(out, [2]int{ini, c - 1})
			ini = c
		}
	}
	return append(out, [2]int{ini, hasta})
}

// ultimaDe es la última unidad que cubre algún grupo (0 si no hay).
func ultimaDe(gs []grupoODS) int {
	n := 0
	for _, g := range gs {
		n = max(n, g.fin)
	}
	return n
}

// colapsoODS es el ` table:visibility="collapse"` de una fila o columna oculta por un grupo
// colapsado: LibreOffice esconde por ahí (el `display="false"` del grupo solo lo escribe).
func colapsoODS(gs []sheetGroup, u int) string {
	if _, oculta := nivelDe(gs, u); oculta {
		return ` table:visibility="collapse"`
	}
	return ""
}
