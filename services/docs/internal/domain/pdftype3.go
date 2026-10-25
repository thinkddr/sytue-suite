// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Fuentes de tipo 3, que son las raras y las que más desconciertan cuando no se
// dibujan.
//
// UNA FUENTE DE TIPO 3 NO TIENE CONTORNOS: cada glifo es un FLUJO DE CONTENIDO
// (§9.6.5). Donde una TrueType guarda curvas, esta guarda «pinta un rectángulo
// aquí y una línea allá», con los mismos operadores que una página. Por eso no se
// puede reemitir como `@font-face` ni sustituir por una familia parecida: no es
// una tipografía, es un programa de dibujo por carácter.
//
// Quién las usa: TeX antiguo con fuentes de mapa de bits, generadores que
// vectorizan símbolos propios, planos de CAD con su simbología, y programas de
// laboratorio que dibujan sus unidades. Aparecen poco, pero cuando aparecen son
// TODO el contenido de la página — un documento con una fórmula en tipo 3 sin
// dibujar no es un documento con un hueco, es una página en blanco.
//
// Antes de esto se reservaba el avance y no se pintaba nada: el texto caía en su
// sitio y no se veía. Ahora se ejecuta el flujo de cada glifo con el mismo
// intérprete que dibuja la página, que es lo que evita tener dos.
package domain

// maxGlifosTipo3PorCadena acota cuántos glifos se dibujan de una sola cadena.
// Cada uno es un flujo de contenido, así que una línea de mil caracteres son mil
// ejecuciones: hay que poder pintar una fórmula, no un libro entero en tipo 3.
const maxGlifosTipo3PorCadena = 512

// type3 es lo que hace falta para dibujar los glifos de una fuente de tipo 3.
type type3 struct {
	// matriz lleva del espacio del glifo al del texto. NO es 1/1000 como en las
	// demás fuentes: la declara el documento y puede ser cualquier cosa.
	matriz matriz
	// procs es el /CharProcs: nombre de glifo → flujo que lo dibuja.
	procs pdfDict
	// codificacion traduce el código del carácter al nombre de su glifo, que es
	// la única vía: en tipo 3 /Encoding /Differences no es un adorno, es
	// obligatorio para poder encontrar el dibujo.
	codificacion map[byte]pdfName
	// recursos son los del glifo. Un glifo puede usar imágenes y otras fuentes.
	recursos pdfDict
}

// leerType3 rellena los datos de la fuente si es de tipo 3; devuelve nil si no.
func (l *lienzo) leerType3(fd pdfDict) *type3 {
	if s, _ := l.d.resolver(fd["Subtype"]).(pdfName); s != "Type3" {
		return nil
	}
	t := &type3{
		// La identidad NO es un valor por defecto razonable aquí —el espacio del
		// glifo suele ser de miles de unidades—, pero es mejor que no pintar: sin
		// /FontMatrix el documento está roto y esto al menos dibuja algo.
		matriz:       matriz{0.001, 0, 0, 0.001, 0, 0},
		codificacion: map[byte]pdfName{},
	}
	if fm, ok := l.d.resolver(fd["FontMatrix"]).(pdfArray); ok && len(fm) >= 6 {
		for i := 0; i < 6; i++ {
			t.matriz[i] = comoReal(l.d.resolver(fm[i]))
		}
	}
	t.procs, _ = l.d.resolver(fd["CharProcs"]).(pdfDict)
	t.recursos, _ = l.d.resolver(fd["Resources"]).(pdfDict)
	if t.procs == nil {
		return nil // sin dibujos no hay nada que hacer
	}

	l.leerCodificacionType3(fd, t.codificacion)
	if len(t.codificacion) == 0 {
		return nil // no hay forma de saber qué dibujo va con qué código
	}
	return t
}

// leerCodificacionType3 usa solo /Differences: en tipo 3 no hay codificación
// base que valga, porque los nombres de glifo los inventa el documento.
func (l *lienzo) leerCodificacionType3(fd pdfDict, codificacion map[byte]pdfName) {
	enc, ok := l.d.resolver(fd["Encoding"]).(pdfDict)
	if !ok {
		return
	}
	difs, ok := l.d.resolver(enc["Differences"]).(pdfArray)
	if !ok {
		return
	}
	codigo := 0
	for _, e := range difs {
		switch v := l.d.resolver(e).(type) {
		case int64:
			if v < 0 || v >= 256 {
				codigo = 256
			} else {
				codigo = int(v)
			}
		case float64:
			if v >= 0 && v < 256 {
				codigo = int(v)
			} else {
				codigo = 256
			}
		case pdfName:
			if codigo >= 0 && codigo < 256 {
				codificacion[byte(codigo)] = v
			}
			codigo++
		}
	}
}

// dibujarGlifosType3 pinta uno a uno los glifos de la cadena y devuelve si pintó
// alguno.
//
// El bucle avanza la matriz de texto glifo a glifo, al contrario que el camino
// normal —que emite un `<text>` por cadena—: aquí cada glifo es un dibujo
// independiente y necesita su propia transformación.
func (e *ejecucion) dibujarGlifosType3(f *fuentePDF, codigos pdfString) bool {
	t := f.type3
	if t == nil || e.l.prof > maxProfundidadXObject {
		return false
	}
	// Modo 3 y 7 son invisibles: el avance ya lo hace quien llama, y ejecutar los
	// flujos para no pintar nada sería trabajo tirado.
	if e.estado.modoRender == 3 || e.estado.modoRender == 7 {
		return false
	}

	pintado := false
	for i, codigo := range codigos {
		if i >= maxGlifosTipo3PorCadena {
			break
		}
		nombre, hay := t.codificacion[codigo]
		if !hay {
			e.avanzarUnGlifo(f, codigo)
			continue
		}
		flujo, ok := e.l.d.resolver(t.procs[nombre]).(*pdfStream)
		if !ok {
			e.avanzarUnGlifo(f, codigo)
			continue
		}
		contenido, err := e.l.d.datosDeFlujo(flujo)
		if err != nil {
			e.avanzarUnGlifo(f, codigo)
			continue
		}

		// La matriz del glifo: su propio espacio → texto → página. El tamaño de
		// fuente y la escala horizontal entran aquí y no en el avance, porque el
		// dibujo se escala con ellos.
		escala := matriz{e.estado.tamano * e.estado.escalaH, 0, 0, e.estado.tamano, 0, e.estado.subida}
		sub := e.estado
		sub.ctm = t.matriz.por(escala).por(e.tm).por(e.estado.ctm)
		// La fuente NO se hereda: un glifo que use texto declara la suya, y dejar
		// la de tipo 3 puesta abriría una recursión de un glifo dibujándose a sí
		// mismo.
		sub.fuente = nil

		recursos := t.recursos
		if recursos == nil {
			recursos = e.recursos // heredar los de la página, como un formulario
		}
		e.l.prof++
		e.l.ejecutar(contenido, recursos, sub)
		e.l.prof--
		pintado = true

		e.avanzarUnGlifo(f, codigo)
	}
	return pintado
}

// avanzarUnGlifo mueve la matriz de texto lo que ocupa UN código.
//
// Está aparte porque el avance tiene que ocurrir tanto si el glifo se pintó como
// si no: un dibujo que falta deja un hueco, no descoloca el resto de la línea.
func (e *ejecucion) avanzarUnGlifo(f *fuentePDF, codigo byte) {
	g := f.glifoDe(uint32(codigo), codigo == 32)
	a := g.ancho/1000*e.estado.tamano + e.estado.espCar
	if g.esEspacioSimple {
		a += e.estado.espPal
	}
	e.tm = matriz{1, 0, 0, 1, a * e.estado.escalaH, 0}.por(e.tm)
}
