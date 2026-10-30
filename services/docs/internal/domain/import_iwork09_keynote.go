// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
)

// # iWork '09: presentaciones .key
//
// `key:presentation` > `key:size[sfa:w, sfa:h]` (en puntos) y `key:slide-list` >
// `key:slide`, cada una con:
//
//   - `key:title-placeholder` y `key:body-placeholder` > `key:text` > `sf:text-storage`: el
//     título y los puntos del cuerpo (`sf:p[sf:list-level]`, 1 el primer nivel). Su
//     geometría en la diapositiva es ficticia (100×100): la caja sale en el sitio de
//     siempre del título y del cuerpo.
//   - `key:page` > `sf:layers` > `sf:layer` > `sf:drawables`: los objetos, con
//     `sf:geometry` > `sf:position[sfa:x, sfa:y]` y `sf:size[sfa:w, sfa:h]`. De ellos se
//     convierten las formas con texto (`sf:shape` > `sf:text` > `sf:text-storage`) y las
//     tablas; las notas adhesivas (`sf:sticky-note`) son comentarios y no salen.
//   - `key:notes` > `sf:text-storage`: las notas del orador.
//
// Las diapositivas del tema (`key:theme-list`) son el maestro: no se convierten.

// importKeynote09 convierte una presentación .key '09. nil si no hay contenido.
func importKeynote09(raiz *nodo09, p *paqueteIWork) (m *slidesModel, omitido bool) {
	l := &lector09{paquete: p, est: leerEstilos09(raiz), binarios: leerBinarios09(raiz)}
	ancho, alto := 1024.0, 768.0
	if s := raiz.hijo("key:size"); s != nil {
		w, ew := strconv.ParseFloat(s.attr["sfa:w"], 64)
		h, eh := strconv.ParseFloat(s.attr["sfa:h"], 64)
		if ew == nil && eh == nil && w > 0 && h > 0 {
			ancho, alto = w, h
		}
	}
	m = &slidesModel{Size: tamanoDiapositiva(ancho*2.54/72, alto*2.54/72)}
	texto := false
	for _, s := range raiz.hijo("key:slide-list").hijosDe() {
		if s.nombre != "key:slide" {
			continue
		}
		sl := slideT{ID: "s" + strconv.Itoa(len(m.Slides)+1), Boxes: []slideBox{}}
		caja := func(bs []xblock, x, y, w, h float64) {
			if !conTexto(bs) {
				return
			}
			texto = true
			sl.Boxes = append(sl.Boxes, slideBox{
				ID: sl.ID + "b" + strconv.Itoa(len(sl.Boxes)+1), Kind: "text", HTML: blocksToHTML(bs),
				X: roundPct(x), Y: roundPct(y), W: roundPct(w), H: roundPct(h),
			})
		}
		caja(l.bloques(s.hijo("key:title-placeholder").hijo("key:text").hijo("sf:text-storage")), 5, 4, 90, 16)
		l.vinetas = true
		caja(l.bloques(s.hijo("key:body-placeholder").hijo("key:text").hijo("sf:text-storage")), 5, 24, 90, 68)
		l.vinetas = false
		s.hijo("key:page").recorrer(func(n *nodo09) bool {
			return l.dibujoDiapositiva09(n, ancho, alto, &sl, &texto, caja)
		})
		var notas []string
		for _, b := range l.bloques(s.hijo("key:notes").hijo("sf:text-storage")) {
			if b.para != nil {
				notas = append(notas, b.para.plain())
			}
		}
		sl.Notes = strings.TrimSpace(strings.Join(notas, "\n"))
		texto = texto || sl.Notes != ""
		m.Slides = append(m.Slides, sl)
	}
	if !texto {
		return nil, l.omitido
	}
	return m, l.omitido
}

func (l *lector09) dibujoDiapositiva09(n *nodo09, ancho, alto float64, sl *slideT, texto *bool, caja func([]xblock, float64, float64, float64, float64)) bool {
	switch n.nombre {
	case "sf:shape", "sf:tabular-info":
		x, y, w, h := geometria09(n, ancho, alto)
		if t := leerTabla09(primerModelo09(n), l.est); t != nil {
			caja([]xblock{{table: t.xtable()}}, x, y, w, h)
		} else if st := primerAlmacen09(n); st != nil {
			caja(l.bloques(st), x, y, w, h)
		}
		return false
	case "sf:image", "sf:media":
		if src, _, _ := l.imagenNodo09(n); src != "" {
			x, y, w, h := geometria09(n, ancho, alto)
			*texto = true
			sl.Boxes = append(sl.Boxes, slideBox{
				ID: sl.ID + "b" + strconv.Itoa(len(sl.Boxes)+1), Kind: "image", Src: src,
				X: roundPct(x), Y: roundPct(y), W: roundPct(w), H: roundPct(h),
			})
		} else {
			l.omitido = true
		}
		return false
	case "sf:chart-info":
		if t := leerGrafico09(n); t != nil {
			x, y, w, h := geometria09(n, ancho, alto)
			caja([]xblock{{table: t.xtable()}}, x, y, w, h)
		} else {
			l.omitido = true
		}
		return false
	case "sf:movie":
		l.omitido = true
		return false
	case "sf:sticky-note":
		return false
	}
	return true
}

// primerAlmacen09 es el primer `sf:text-storage` del subárbol, o nil.
func primerAlmacen09(n *nodo09) *nodo09 {
	var st *nodo09
	n.recorrer(func(x *nodo09) bool {
		if st == nil && x.nombre == "sf:text-storage" {
			st = x
		}
		return st == nil
	})
	return st
}

// geometria09 es la caja de un objeto en % de la diapositiva.
func geometria09(n *nodo09, ancho, alto float64) (x, y, w, h float64) {
	g := n.hijo("sf:geometry")
	num := func(e *nodo09, k string) float64 {
		f, _ := strconv.ParseFloat(e.attr[k], 64)
		return f
	}
	p, s := g.hijo("sf:position"), g.hijo("sf:size")
	if p == nil || s == nil {
		return 10, 10, 80, 20
	}
	return num(p, "sfa:x") / ancho * 100, num(p, "sfa:y") / alto * 100, num(s, "sfa:w") / ancho * 100, num(s, "sfa:h") / alto * 100
}
