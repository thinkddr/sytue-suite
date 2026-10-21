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

// importKeynoteIWA convierte una presentación .key de 2013 en adelante. nil si no hay texto.
func importKeynoteIWA(doc documentoIWA) (m *slidesModel, omitido bool) {
	show := doc.de(doc.de(1, tipoIWADocumento).ref(2), tipoIWAPresentacion)
	tam := show.msg(4)
	ancho, alto := float64(tam.f32(1)), float64(tam.f32(2))
	if !(ancho > 0 && alto > 0) || math.IsInf(ancho, 0) || math.IsInf(alto, 0) {
		ancho, alto = 1024, 768
	}
	l := &lectorIWA{doc: doc}
	m = &slidesModel{Size: tamanoDiapositiva(ancho*2.54/72, alto*2.54/72)}
	texto := false
	show.msg(3).cada(2, func(n campoPB) {
		d := doc.de(doc.de(n.b.uint(1), tipoIWANodo).ref(2), tipoIWADiapositiva)
		if d == nil {
			return
		}
		sl := slideT{ID: "s" + strconv.Itoa(len(m.Slides)+1), Boxes: []slideBox{}}
		cuerpo := d.ref(6)
		d.cada(7, func(r campoPB) {
			id := r.b.uint(1)
			o := doc[id]
			var bs []xblock
			var forma pb
			switch o.tipo {
			case tipoIWAMarcador:
				forma = o.datos.msg(1).msg(1)
				paras := l.parrafos(o.datos.msg(1).ref(2))
				if id == cuerpo {
					// ponytail: los puntos del cuerpo salen todos en el primer nivel; el
					// nivel de cada uno no se ha comprobado aún en ningún fichero.
					for i := range paras {
						paras[i].list = "bullet"
					}
				}
				bs = bloquesDeParrafos(paras)
			case tipoIWACuadro:
				forma = o.datos.msg(1)
				bs = bloquesDeParrafos(l.parrafos(o.datos.ref(2)))
			case tipoIWATabla:
				forma = o.datos
				if t := l.tablaIWA(o.datos.ref(2)); t != nil {
					bs = []xblock{{table: t.xtable()}}
				}
			default:
				l.omitido = true
				return
			}
			if !conTexto(bs) {
				return
			}
			texto = true
			g := forma.msg(1).msg(1)
			pos, tm := g.msg(1), g.msg(2)
			sl.Boxes = append(sl.Boxes, slideBox{
				ID: sl.ID + "b" + strconv.Itoa(len(sl.Boxes)+1), Kind: "text", HTML: blocksToHTML(bs),
				X: pctIWA(pos.f32(1), ancho), Y: pctIWA(pos.f32(2), alto),
				W: pctIWA(tm.f32(1), ancho), H: pctIWA(tm.f32(2), alto),
			})
		})
		var notas []string
		for _, p := range l.parrafos(doc.de(d.ref(27), tipoIWANota).ref(1)) {
			notas = append(notas, p.plain())
		}
		sl.Notes = strings.TrimSpace(strings.Join(notas, "\n"))
		texto = texto || sl.Notes != ""
		m.Slides = append(m.Slides, sl)
	})
	if !texto {
		return nil, l.omitido
	}
	return m, l.omitido
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
