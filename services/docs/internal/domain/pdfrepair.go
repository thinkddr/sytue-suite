// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Recuperación de un PDF con la estructura rota.
//
// LA TABLA DE REFERENCIAS CRUZADAS ES UN ÍNDICE, Y UN ÍNDICE ES PRESCINDIBLE.
// Dice en qué byte empieza cada objeto, pero los objetos siguen ahí, con su
// cabecera «N G obj» delante. Si el índice miente —una descarga cortada, un
// programa que escribió mal los desplazamientos, un fichero recompuesto a
// mano—, se puede RECONSTRUIR barriendo el fichero entero y anotando dónde está
// cada cabecera. Es lo que hace cualquier visor serio, y por eso abre ficheros
// que otros rechazan.
//
// NO ES UNA HERRAMIENTA, ES UN COMPORTAMIENTO. No hay botón de «reparar»: si el
// índice falla, se reconstruye y el documento se abre. Pedirle al usuario que
// encuentre una herramienta para poder ver su documento es trasladarle un
// problema que no ha causado y que no puede diagnosticar — lo único que él sabe
// es que el fichero «no se abre».
//
// Lo que sí se le dice: que el documento estaba dañado y que la copia que
// guarde saldrá reparada. Eso es información útil; el resto es ruido.
package domain

import (
	"bytes"
	"strconv"
)

// maxObjetosAlBarrer acota la reconstrucción. Un fichero manipulado puede
// declarar millones de cabeceras falsas; a partir de aquí no es un PDF roto, es
// otra cosa.
const maxObjetosAlBarrer = 500_000

// reconstruirXref barre el fichero buscando cabeceras «N G obj» y rehace el
// índice desde cero.
//
// GANA LA ÚLTIMA APARICIÓN de cada objeto, al revés que al seguir la cadena de
// tablas. Es la regla correcta aquí: sin índice fiable no se sabe qué
// actualización incremental es la buena, y la más reciente está FÍSICAMENTE más
// abajo en el fichero porque las actualizaciones se anexan al final.
func (d *PDFDoc) reconstruirXref() bool {
	nuevo := map[int]xrefEntrada{}
	buf := d.buf

	for i := 0; i+3 < len(buf) && len(nuevo) < maxObjetosAlBarrer; {
		j := bytes.Index(buf[i:], []byte("obj"))
		if j < 0 {
			break
		}
		pos := i + j
		i = pos + 3

		// Detrás de «obj» tiene que venir un delimitador o un espacio: sin esto,
		// la palabra «object» dentro de un comentario o de una cadena se tomaría
		// por una cabecera.
		if pos+3 < len(buf) && !esEspacioPDF(buf[pos+3]) && !esDelimitadorPDF(buf[pos+3]) {
			continue
		}
		num, inicio, ok := cabeceraDeObjeto(buf, pos)
		if !ok {
			continue
		}
		nuevo[num] = xrefEntrada{offset: int64(inicio)}
	}

	if len(nuevo) == 0 {
		return false
	}
	d.xref = nuevo
	d.objs = map[int]pdfObj{}
	d.reparado = true

	// El tráiler también puede estar roto o no existir. Se busca el último que
	// haya y, si no hay ninguno utilizable, se deduce el catálogo mirando los
	// objetos: /Type /Catalog es inequívoco.
	if _, ok := d.resolver(d.trailer["Root"]).(pdfDict); !ok {
		d.recuperarTrailer()
	}
	return true
}

// cabeceraDeObjeto lee hacia ATRÁS desde la palabra «obj» los dos enteros que la
// preceden y devuelve el número del objeto y dónde empieza su cabecera. Se hace
// al revés porque el barrido encuentra primero la palabra, y buscar «número
// número obj» hacia delante obligaría a probar en cada byte.
//
// La GENERACIÓN se lee para validar la forma, pero no se devuelve: al
// reconstruir gana la última definición del fichero, así que la generación no
// desempata nada.
func cabeceraDeObjeto(buf []byte, posObj int) (num, inicio int, ok bool) {
	p := posObj
	saltarEspaciosAtras := func() {
		for p > 0 && esEspacioPDF(buf[p-1]) {
			p--
		}
	}
	leerEnteroAtras := func() (int, bool) {
		fin := p
		for p > 0 && buf[p-1] >= '0' && buf[p-1] <= '9' {
			p--
		}
		if p == fin || fin-p > 10 {
			return 0, false
		}
		v, err := strconv.Atoi(string(buf[p:fin]))
		if err != nil {
			return 0, false
		}
		return v, true
	}

	saltarEspaciosAtras()
	if _, ok := leerEnteroAtras(); !ok { // generación
		return 0, 0, false
	}
	saltarEspaciosAtras()
	n, ok := leerEnteroAtras()
	if !ok || n <= 0 {
		return 0, 0, false
	}
	return n, p, true
}

// recuperarTrailer busca el catálogo cuando el tráiler no sirve.
//
// Primero se intenta el último «trailer» del fichero, que suele estar intacto
// aunque los desplazamientos no lo estén. Si tampoco, se recorren los objetos ya
// localizados en busca de /Type /Catalog: es el único que puede serlo.
func (d *PDFDoc) recuperarTrailer() {
	if i := bytes.LastIndex(d.buf, []byte("trailer")); i >= 0 {
		lx := &pdfLexer{buf: d.buf, pos: i + len("trailer")}
		lx.saltarEspacios()
		if dict, err := d.parsearDiccionario(lx); err == nil {
			if td, ok := dict.(pdfDict); ok {
				if _, hay := td["Root"]; hay {
					d.fusionarTrailer(td)
					if _, ok := d.resolver(d.trailer["Root"]).(pdfDict); ok {
						return
					}
				}
			}
		}
	}

	// Último recurso: el objeto que se declara catálogo. Se recorre en orden de
	// número para que el resultado no dependa del recorrido de un mapa.
	nums := make([]int, 0, len(d.xref))
	for n := range d.xref {
		nums = append(nums, n)
	}
	// Orden ascendente sin `sort`: el catálogo suele ser de los primeros y basta
	// con quedarse con el menor que cumpla.
	mejor := -1
	for _, n := range nums {
		o, err := d.objeto(n)
		if err != nil {
			continue
		}
		dict, ok := o.(pdfDict)
		if !ok {
			continue
		}
		if t, _ := d.resolver(dict["Type"]).(pdfName); t != "Catalog" {
			continue
		}
		if mejor < 0 || n < mejor {
			mejor = n
		}
	}
	if mejor >= 0 {
		if d.trailer == nil {
			d.trailer = pdfDict{}
		}
		d.trailer["Root"] = pdfRef{Num: mejor}
	}
}

// EstabaDanado dice si el documento hubo que reconstruirlo para poder abrirlo.
// La interfaz lo usa para avisar de que la copia guardada saldrá reparada, que
// es lo único que le interesa a quien tiene delante un fichero que «no se abría».
func (d *PDFDoc) EstabaDanado() bool { return d.reparado }
