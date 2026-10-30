// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"math"
	"strconv"
	"strings"
)

// # iWork 2013 y posteriores: presentaciones .key
//
// Tipos y campos observados en los cuatro .key de 2013 en adelante del corpus:
//
//   - 1, la raíz (objeto 1): 2 → la presentación. (Un .numbers también tiene la raíz de
//     tipo 1, pero sin el campo 2.)
//   - 2, la presentación: 3 {2 → cada nodo de diapositiva, en orden}; 4 tamaño {1 ancho,
//     2 alto} en puntos.
//   - 4, un nodo: 2 → su diapositiva.
//   - 5, una diapositiva: 6 → el marcador del cuerpo; 7 → cada objeto visible, en orden
//     (un marcador que no está aquí está oculto); 27 → la nota del orador.
//   - 7, un marcador (título, cuerpo, número): 1 {1 la forma, 2 → su almacén de texto}.
//   - 2011, un cuadro de texto: 1 la forma; 2 → su almacén.
//   - 6000, una tabla (como en .numbers).
//   - 15, una nota: 1 → su almacén.
//
// La forma lleva su dibujo en 1, y el dibujo su geometría en 1: {1 posición {1 x, 2 y},
// 2 tamaño {1 ancho, 2 alto}}, en puntos.

const (
	tipoIWAPresentacion = 2
	tipoIWANodo         = 4
	tipoIWADiapositiva  = 5
	tipoIWAMarcador     = 7
	tipoIWANota         = 15
)

// esKeynoteIWA dice si el documento es una presentación (y no una hoja .numbers).
func esKeynoteIWA(doc documentoIWA) bool {
	return doc.de(doc.de(1, tipoIWADocumento).ref(2), tipoIWAPresentacion) != nil
}

func nodosDiapositivasIWA(doc documentoIWA, show pb) []uint64 {
	var nodos []uint64
	show.msg(3).cada(2, func(n campoPB) { nodos = append(nodos, n.b.uint(1)) })
	if len(nodos) > 0 {
		return nodos
	}
	var recoger func(id uint64, prof int)
	recoger = func(id uint64, prof int) {
		nd := doc.de(id, tipoIWANodo)
		if nd == nil || prof > maxHerenciaIWA {
			return
		}
		if nd.ref(2) != 0 {
			nodos = append(nodos, id)
		}
		nd.cada(1, func(c campoPB) { recoger(c.b.uint(1), prof+1) })
	}
	recoger(show.msg(3).ref(1), 0)
	return nodos
}

func (l *lectorIWA) cajaImagenDiapositivaIWA(idObj uint64, idCaja string, ancho, alto float64) (slideBox, bool) {
	src, _, _, pos, tm := l.imagenIWA(idObj)
	if src == "" {
		l.omitido = true
		return slideBox{}, false
	}
	x, y, w, h := pctIWA(pos.f32(1), ancho), pctIWA(pos.f32(2), alto), pctIWA(tm.f32(1), ancho), pctIWA(tm.f32(2), alto)
	if w <= 0 || h <= 0 {
		x, y, w, h = 10, 20, 40, 40
	}
	return slideBox{ID: idCaja, Kind: "image", Src: src, X: x, Y: y, W: w, H: h}, true
}

// importKeynoteIWA convierte una presentación .key de 2013 en adelante. nil si no hay contenido.
func importKeynoteIWA(doc documentoIWA, p *paqueteIWork) (m *slidesModel, omitido bool) {
	show := doc.de(doc.de(1, tipoIWADocumento).ref(2), tipoIWAPresentacion)
	tam := show.msg(4)
	ancho, alto := float64(tam.f32(1)), float64(tam.f32(2))
	if !(ancho > 0 && alto > 0) || math.IsInf(ancho, 0) || math.IsInf(alto, 0) {
		ancho, alto = 1024, 768
	}
	l := nuevoLectorIWA(doc, p, false)
	m = &slidesModel{Size: tamanoDiapositiva(ancho*2.54/72, alto*2.54/72)}
	texto := false
	for _, idNodo := range nodosDiapositivasIWA(doc, show) {
		d := doc.de(doc.de(idNodo, tipoIWANodo).ref(2), tipoIWADiapositiva)
		if d == nil {
			continue
		}
		sl := slideT{ID: "s" + strconv.Itoa(len(m.Slides)+1), Boxes: []slideBox{}}
		cuerpo := d.ref(6)
		d.cada(7, func(r campoPB) {
			idObj := r.b.uint(1)
			idCaja := sl.ID + "b" + strconv.Itoa(len(sl.Boxes)+1)
			if doc[idObj].tipo == tipoIWAImagen {
				if b, ok := l.cajaImagenDiapositivaIWA(idObj, idCaja, ancho, alto); ok {
					texto = true
					sl.Boxes = append(sl.Boxes, b)
				}
				return
			}
			if doc[idObj].tipo == tipoIWAComentarioInfo {
				return
			}
			bs, forma := l.objetoDiapositivaIWA(idObj, cuerpo)
			if !conTexto(bs) {
				return
			}
			texto = true
			g := forma.msg(1).msg(1)
			pos, tm := g.msg(1), g.msg(2)
			sl.Boxes = append(sl.Boxes, slideBox{
				ID: idCaja, Kind: "text", HTML: blocksToHTML(bs),
				X: pctIWA(pos.f32(1), ancho), Y: pctIWA(pos.f32(2), alto),
				W: pctIWA(tm.f32(1), ancho), H: pctIWA(tm.f32(2), alto),
			})
		})
		var notas []string
		idNota := doc.de(d.ref(27), tipoIWANota).ref(1)
		if idNota == 0 {
			idNota = d.ref(27)
		}
		for _, p := range l.parrafos(idNota) {
			notas = append(notas, p.plain())
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

func (l *lectorIWA) objetoDiapositivaIWA(id, cuerpo uint64) (bs []xblock, forma pb) {
	o := l.doc[id]
	switch o.tipo {
	case tipoIWAMarcador:
		forma = o.datos.msg(1).msg(1)
		paras := l.parrafos(o.datos.msg(1).ref(2))
		if id == cuerpo {
			for i := range paras {
				if paras[i].list == "" {
					paras[i].list = "bullet"
				}
			}
		}
		return bloquesDeParrafos(paras), forma
	case tipoIWACuadro:
		return bloquesDeParrafos(l.parrafos(o.datos.ref(2))), o.datos.msg(1)
	case tipoIWATabla:
		if t := l.tablaInfoIWA(o.datos); t != nil {
			return []xblock{{table: t.xtable()}}, o.datos
		}
		return nil, o.datos
	case tipoIWAGrafico, tipoIWAGrafico2:
		if t, f := l.graficoIWA(id); t != nil {
			return []xblock{{table: t.xtable()}}, f
		}
		l.omitido = true
		return nil, nil
	default:
		l.omitido = true
		return nil, nil
	}
}

// pctIWA es una longitud en puntos como % del lado del lienzo.
func pctIWA(v float32, lado float64) float64 {
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return roundPct(f / lado * 100)
}

func bloquesDeParrafos(paras []xpara) []xblock {
	bs := make([]xblock, len(paras))
	for i := range paras {
		bs[i] = xblock{para: &paras[i]}
	}
	return bs
}
