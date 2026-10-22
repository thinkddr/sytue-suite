// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// CAMPOS DE FORMULARIO AL CENSURAR.
//
// Decisión del dueño (2026-10-16), «conservar los de fuera»: se conservan los
// campos cuyos widgets quedan TODOS fuera de las zonas censuradas, como ya se
// hacía con las demás anotaciones; un campo con algún widget que toca una zona
// se quita entero, valor incluido; el XFA se sigue quitando. Antes se quitaba el
// /AcroForm entero y los campos de fuera desaparecían al verlos (un widget sin
// formulario no lo pinta nadie), pero sus objetos seguían en el fichero.
//
// Lo que podría delatar lo censurado:
//   - un campo de fuera cuyo valor DEPENDE de uno quitado —un total calculado,
//     un formato— se quita también: su /AA lleva JavaScript que lo nombra;
//   - un padre que conserva hijos no arrastra a los quitados: sus /Kids pierden
//     la referencia y el objeto no se copia (sin esto, el DNI de un campo quitado
//     viajaba por el /Parent de su hermano);
//   - los campos de firma no vuelven al formulario y pierden su valor (la firma,
//     que censurar invalida, con el certificado de quien firmó); su widget de
//     fuera se queda, como cualquier anotación.
package domain

import (
	"strings"
)

const (
	maxProfCamposCensura   = 32
	maxAccionesJSCensura   = 64
	maxVueltasDependencias = 16
)

// campoCensura es un campo TERMINAL del formulario: el que tiene valor y widgets.
type campoCensura struct {
	num       int // objeto del campo; 0 si va directo dentro de su padre
	nombre    string
	dict      pdfDict
	firma     bool
	widgets   []int // objetos de sus widgets (él mismo si va fundido con el suyo)
	ancestros []int // objetos de los campos que lo contienen
	quitado   bool
}

// formularioCensura es lo que censurar hace con el formulario de origen.
type formularioCensura struct {
	campos     []*campoCensura
	excluidos  map[int]bool // objetos del origen que no viajan
	quitados   int
	porCalculo int // de los quitados, los que dependían de otro quitado
}

// formularioAlCensurar decide qué campos se quitan con las zonas de cada página
// (ya en coordenadas absolutas).
func (d *PDFDoc) formularioAlCensurar(zonas map[int][]AreaRedaccion) *formularioCensura {
	f := &formularioCensura{excluidos: map[int]bool{}}
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	forma, _ := d.resolver(raiz["AcroForm"]).(pdfDict)
	campos, _ := d.resolver(forma["Fields"]).(pdfArray)
	if len(campos) == 0 {
		return f
	}
	vistos := map[int]bool{}
	for _, c := range campos {
		d.recogerCampos(f, c, "", "", nil, vistos, 0)
	}

	pagina := map[int]int{} // widget → página
	for i, p := range d.paginas {
		annots, _ := d.resolver(p.dict["Annots"]).(pdfArray)
		for _, a := range annots {
			if r, ok := a.(pdfRef); ok {
				pagina[r.Num] = i
			}
		}
	}
	for _, c := range f.campos {
		c.quitado = d.widgetEnZona(c, pagina, zonas)
		// Una firma de fuera deja su widget —lo que se ve—, pero no su valor:
		// censurar la invalida, y su /V lleva el certificado de quien firmó.
		if v, ok := c.dict["V"].(pdfRef); ok && c.firma && !c.quitado {
			f.excluidos[v.Num] = true
		}
	}
	f.quitarDependientes(d)

	// Lo que no viaja: los campos quitados, sus widgets y los padres que se
	// quedan sin ningún campo conservado.
	conservan := map[int]bool{}
	for _, c := range f.campos {
		if c.quitado {
			f.quitados++
			if c.num > 0 {
				f.excluidos[c.num] = true
			}
			for _, w := range c.widgets {
				f.excluidos[w] = true
			}
			continue
		}
		for _, a := range c.ancestros {
			conservan[a] = true
		}
	}
	for _, c := range f.campos {
		for _, a := range c.ancestros {
			if !conservan[a] {
				f.excluidos[a] = true
			}
		}
	}
	return f
}

// recogerCampos recorre el árbol de campos (§12.7.4) y apunta los terminales.
func (d *PDFDoc) recogerCampos(f *formularioCensura, ref pdfObj, prefijo, ft string,
	ancestros []int, vistos map[int]bool, prof int,
) {
	num := numDeRef(ref)
	if prof > maxProfCamposCensura || (num > 0 && vistos[num]) {
		return
	}
	if num > 0 {
		vistos[num] = true
	}
	dict, ok := d.resolver(ref).(pdfDict)
	if !ok {
		return
	}
	nombre := prefijo
	if t, ok := d.resolver(dict["T"]).(pdfString); ok {
		if nombre != "" {
			nombre += "."
		}
		nombre += decodificarTextoPDF(t)
	}
	if v, ok := d.resolver(dict["FT"]).(pdfName); ok {
		ft = string(v)
	}
	var widgets []int
	var hijos []pdfObj
	if kids, ok := d.resolver(dict["Kids"]).(pdfArray); ok {
		for _, k := range kids {
			kd, ok := d.resolver(k).(pdfDict)
			if !ok {
				continue
			}
			if _, esCampo := kd["T"]; esCampo {
				hijos = append(hijos, k)
			} else if n := numDeRef(k); n > 0 {
				widgets = append(widgets, n) // un widget sin nombre es de este campo
			}
		}
	}
	if sub, _ := d.resolver(dict["Subtype"]).(pdfName); sub == "Widget" && num > 0 {
		widgets = append(widgets, num) // campo y widget fundidos
	}
	if len(hijos) == 0 || len(widgets) > 0 {
		f.campos = append(f.campos, &campoCensura{
			num: num, nombre: nombre, dict: dict, firma: ft == "Sig",
			widgets: widgets, ancestros: append([]int(nil), ancestros...),
		})
	}
	if num > 0 {
		ancestros = append(ancestros, num)
	}
	for _, h := range hijos {
		d.recogerCampos(f, h, nombre, ft, ancestros, vistos, prof+1)
	}
}

// widgetEnZona dice si algún widget del campo toca una zona censurada de su
// página. Uno sin /Rect en una página con zonas no se puede comprobar: se quita.
func (d *PDFDoc) widgetEnZona(c *campoCensura, pagina map[int]int, zonas map[int][]AreaRedaccion) bool {
	for _, w := range c.widgets {
		p, enPagina := pagina[w]
		if !enPagina || len(zonas[p]) == 0 {
			continue
		}
		wd, _ := d.resolver(pdfRef{Num: w}).(pdfDict)
		r, ok := d.resolver(wd["Rect"]).(pdfArray)
		if !ok || len(r) < 4 {
			return true
		}
		var v [4]float64
		for i := range v {
			v[i] = comoReal(d.resolver(r[i]))
		}
		caja := cajaPDF{x1: min(v[0], v[2]), y1: min(v[1], v[3]), x2: max(v[0], v[2]), y2: max(v[1], v[3])}
		if caja.tocaAlguna(zonas[p]) {
			return true
		}
	}
	return false
}

// quitarDependientes quita los campos conservados cuyo JavaScript (/AA: cálculo,
// formato, validación, pulsación) nombra a uno quitado: su valor puede ser un
// total o una copia del dato censurado. Hasta que no cambie nada, porque quitar
// uno puede arrastrar al que dependía de él.
func (f *formularioCensura) quitarDependientes(d *PDFDoc) {
	for vuelta := 0; vuelta < maxVueltasDependencias; vuelta++ {
		var nombres []string
		for _, c := range f.campos {
			if c.quitado && !c.firma && c.nombre != "" {
				nombres = append(nombres, c.nombre)
			}
		}
		if len(nombres) == 0 {
			return
		}
		cambio := false
		for _, c := range f.campos {
			if c.quitado {
				continue
			}
			js := d.javascriptDeCampo(c)
			for _, n := range nombres {
				if strings.Contains(js, `"`+n+`"`) || strings.Contains(js, `'`+n+`'`) {
					c.quitado, cambio = true, true
					f.porCalculo++
					break
				}
			}
		}
		if !cambio {
			return
		}
	}
}

// javascriptDeCampo junta el JavaScript de las /AA del campo y de sus widgets.
func (d *PDFDoc) javascriptDeCampo(c *campoCensura) string {
	var sb strings.Builder
	dicts := []pdfDict{c.dict}
	for _, w := range c.widgets {
		if wd, ok := d.resolver(pdfRef{Num: w}).(pdfDict); ok {
			dicts = append(dicts, wd)
		}
	}
	n := 0
	for _, dict := range dicts {
		aa, _ := d.resolver(dict["AA"]).(pdfDict)
		for _, k := range clavesOrdenadas(aa) {
			accion := d.resolver(aa[k])
			for accion != nil && n < maxAccionesJSCensura {
				n++
				ad, ok := accion.(pdfDict)
				if !ok {
					break
				}
				switch js := d.resolver(ad["JS"]).(type) {
				case pdfString:
					sb.WriteString(decodificarTextoPDF(js))
				case *pdfStream:
					if b, err := d.datosDeFlujo(js); err == nil {
						sb.Write(b)
					}
				}
				sb.WriteByte('\n')
				accion = d.resolver(ad["Next"])
			}
		}
	}
	return sb.String()
}

// formularioCensurado es el /AcroForm de la salida: los campos de arriba que
// viajan (sin firmas ni los quitados) y lo demás del original salvo el XFA.
// nil si no queda ningún campo.
func (e *PDFEnsamblador) formularioCensurado(doc *PDFDoc, f *formularioCensura) pdfObj {
	raiz, _ := doc.resolver(doc.trailer["Root"]).(pdfDict)
	forma, _ := doc.resolver(raiz["AcroForm"]).(pdfDict)
	campos, _ := doc.resolver(forma["Fields"]).(pdfArray)
	firmas := map[int]bool{}
	for _, c := range f.campos {
		if c.firma && c.num > 0 {
			firmas[c.num] = true
		}
	}
	var quedan pdfArray
	for _, c := range campos {
		if n := numDeRef(c); n > 0 && !f.excluidos[n] && !firmas[n] {
			quedan = append(quedan, c)
		}
	}
	if len(quedan) == 0 {
		return nil
	}
	out := pdfDict{}
	for _, k := range clavesOrdenadas(forma) {
		switch k {
		case "XFA", "Fields":
			continue
		}
		out[k] = e.copiarValor(doc, forma[k])
	}
	out["Fields"] = e.copiarValor(doc, quedan)
	return out
}
