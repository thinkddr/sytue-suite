// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Escribir el índice del documento.
//
// Leerlo ya se hacía (`pdfindice.go`); esto es la otra mitad y es lo que
// convierte un PDF ajeno en algo navegable: un escaneo de doscientas páginas o
// un informe exportado de una impresora virtual no traen marcadores, y ponerlos
// es la diferencia entre hojear y consultar.
//
// La lista entra PLANA, con el nivel de cada entrada —es como la enseña la
// interfaz—, y aquí se reconstruye el árbol doblemente enlazado que pide la
// especificación: /First, /Last, /Next, /Prev, /Parent y /Count en cada nodo.
// Equivocarse en uno solo de esos enlaces da un índice que un lector recorre a
// medias, sin ningún error.
package domain

import "fmt"

// maxMarcadoresEscritos acota lo que se acepta escribir.
const maxMarcadoresEscritos = 1000

// EscribirIndicePDF sustituye el índice del documento por el que se le pase.
//
// Una lista vacía QUITA el índice: es la forma de deshacerse del que trae un
// documento mal generado, y no tiene otra vía.
//
// Va por anexo, como el resto: poner un índice no puede tirar una firma.
func EscribirIndicePDF(doc *PDFDoc, marcadores []Marcador) ([]byte, error) {
	if doc == nil || len(doc.buf) == 0 {
		return nil, fmt.Errorf("%w: no hay fichero de partida", ErrNoIncremental)
	}
	if doc.cripto != nil {
		return nil, fmt.Errorf("%w: está cifrado", ErrNoIncremental)
	}
	if doc.reparado {
		return nil, fmt.Errorf("%w: la tabla de referencias estaba rota", ErrNoIncremental)
	}
	if len(marcadores) > maxMarcadoresEscritos {
		return nil, fmt.Errorf("pdf: demasiados marcadores (máximo %d)", maxMarcadoresEscritos)
	}
	raizRef, ok := doc.trailer["Root"].(pdfRef)
	if !ok {
		return nil, fmt.Errorf("%w: el /Root no es una referencia", ErrNoIncremental)
	}
	raiz, ok := doc.resolver(raizRef).(pdfDict)
	if !ok {
		return nil, fmt.Errorf("%w: el catálogo no se pudo leer", ErrNoIncremental)
	}
	xrefAnterior, err := offsetDelUltimoXref(doc.buf)
	if err != nil {
		return nil, err
	}
	limpios, err := marcadoresValidos(doc, marcadores)
	if err != nil {
		return nil, err
	}

	siguiente := doc.maxObjeto() + 1
	reservar := func() int { n := siguiente; siguiente++; return n }
	nuevos := map[int]objetoSalida{}

	catalogo := copiaLlana(raiz)
	if len(limpios) == 0 {
		// Sin marcadores no se escribe un árbol vacío: se quita la clave, que es
		// lo que distingue «no tiene índice» de «tiene uno sin entradas» —un
		// lector enseña el panel vacío en el segundo caso—.
		delete(catalogo, "Outlines")
	} else {
		numRaiz := reservar()
		nodos, nums := construirArbol(doc, limpios, numRaiz, reservar)
		for num, obj := range nodos {
			nuevos[num] = obj
		}
		nuevos[numRaiz] = objetoSalida{valor: raizDelIndice(limpios, nums)}
		catalogo["Outlines"] = pdfRef{Num: numRaiz}
	}
	nuevos[raizRef.Num] = objetoSalida{valor: catalogo}

	return escribirAnexo(doc, nuevos, siguiente, raizRef, xrefAnterior, nil)
}

// marcadoresValidos limpia la lista de entrada.
func marcadoresValidos(d *PDFDoc, ms []Marcador) ([]Marcador, error) {
	out := make([]Marcador, 0, len(ms))
	for _, m := range ms {
		titulo := recortarMetadato(m.Titulo)
		if titulo == "" {
			// Un marcador sin título es una fila en blanco en el panel: no se
			// escribe, pero tampoco tumba los demás.
			continue
		}
		if m.Pagina < 0 || m.Pagina >= d.NumPaginas() {
			return nil, fmt.Errorf("pdf: el marcador %q apunta a la página %d, que no existe",
				titulo, m.Pagina+1)
		}
		nivel := m.Nivel
		if nivel < 0 {
			nivel = 0
		}
		if nivel >= maxProfundidadIndice {
			nivel = maxProfundidadIndice - 1
		}
		// El primero no puede empezar sangrado: un hijo sin padre no tiene dónde
		// colgar, y la especificación no lo admite.
		if len(out) == 0 {
			nivel = 0
		} else if anterior := out[len(out)-1].Nivel; nivel > anterior+1 {
			// Saltarse un nivel («0 luego 2») deja un hueco imposible; se baja al
			// que sí existe en vez de rechazar la lista entera.
			nivel = anterior + 1
		}
		out = append(out, Marcador{Titulo: titulo, Pagina: m.Pagina, Nivel: nivel, Abierto: m.Abierto})
	}
	return out, nil
}

// construirArbol crea un objeto por marcador con todos sus enlaces.
//
// Se hace en dos pasadas —primero los números, después los enlaces— porque cada
// nodo necesita saber el número de su hermano SIGUIENTE, que aún no existe
// cuando se le llega.
func construirArbol(d *PDFDoc, ms []Marcador, numRaiz int, reservar func() int) (map[int]objetoSalida, []int) {
	nums := make([]int, len(ms))
	for i := range ms {
		nums[i] = reservar()
	}

	// padre[i] = índice del marcador padre, o -1 si cuelga de la raíz.
	padre := make([]int, len(ms))
	ultimoDeNivel := map[int]int{}
	for i, m := range ms {
		padre[i] = -1
		if m.Nivel > 0 {
			if p, ok := ultimoDeNivel[m.Nivel-1]; ok {
				padre[i] = p
			}
		}
		ultimoDeNivel[m.Nivel] = i
		// Al bajar de nivel, los niveles más profundos dejan de ser válidos como
		// padres: si no, un marcador de nivel 1 se colgaría de un nieto anterior.
		for n := range ultimoDeNivel {
			if n > m.Nivel {
				delete(ultimoDeNivel, n)
			}
		}
	}

	out := make(map[int]objetoSalida, len(ms))
	for i, m := range ms {
		dict := pdfDict{
			"Title": cadenaUTF16(m.Titulo),
			// /XYZ con nulos = «esta página, sin tocar el zoom ni el desplazamiento».
			// /Fit encajaría la página entera y cambiaría el zoom del lector, que es
			// un efecto que nadie pidió al pulsar un marcador.
			"Dest": pdfArray{pdfRef{Num: d.paginas[m.Pagina].num}, pdfName("XYZ"), nil, nil, nil},
		}
		if padre[i] >= 0 {
			dict["Parent"] = pdfRef{Num: nums[padre[i]]}
		} else {
			dict["Parent"] = pdfRef{Num: numRaiz}
		}
		if h := hermano(ms, padre, i, -1); h >= 0 {
			dict["Prev"] = pdfRef{Num: nums[h]}
		}
		if h := hermano(ms, padre, i, +1); h >= 0 {
			dict["Next"] = pdfRef{Num: nums[h]}
		}
		if pri, ult, n := hijos(ms, padre, i); n > 0 {
			dict["First"] = pdfRef{Num: nums[pri]}
			dict["Last"] = pdfRef{Num: nums[ult]}
			// /Count positivo = abierto (los hijos se ven); negativo = cerrado.
			if m.Abierto {
				dict["Count"] = int64(n)
			} else {
				dict["Count"] = int64(-n)
			}
		}
		out[nums[i]] = objetoSalida{valor: dict}
	}
	// Los números se DEVUELVEN y no se guardan en ningún sitio compartido: esto
	// corre dentro de una petición HTTP y dos a la vez se pisarían.
	return out, nums
}

// raizDelIndice compone el nodo /Outlines.
func raizDelIndice(ms []Marcador, nums []int) pdfDict {
	dict := pdfDict{"Type": pdfName("Outlines")}
	primero, ultimo, n := -1, -1, 0
	for i, m := range ms {
		if m.Nivel != 0 {
			continue
		}
		if primero < 0 {
			primero = i
		}
		ultimo = i
		n++
	}
	if n == 0 {
		return dict
	}
	dict["First"] = pdfRef{Num: nums[primero]}
	dict["Last"] = pdfRef{Num: nums[ultimo]}
	// El /Count de la raíz es el total de entradas VISIBLES al abrir.
	dict["Count"] = int64(n)
	return dict
}

// hermano devuelve el índice del hermano anterior (dir=-1) o siguiente (+1).
func hermano(ms []Marcador, padre []int, i, dir int) int {
	for j := i + dir; j >= 0 && j < len(ms); j += dir {
		if padre[j] == padre[i] {
			return j
		}
		// Al salir del subárbol del padre ya no hay hermanos por ese lado.
		if ms[j].Nivel < ms[i].Nivel {
			return -1
		}
	}
	return -1
}

// hijos devuelve el primero, el último y cuántos hijos DIRECTOS tiene.
func hijos(ms []Marcador, padre []int, i int) (primero, ultimo, n int) {
	primero, ultimo = -1, -1
	for j := i + 1; j < len(ms); j++ {
		if ms[j].Nivel <= ms[i].Nivel {
			break
		}
		if padre[j] != i {
			continue
		}
		if primero < 0 {
			primero = j
		}
		ultimo = j
		n++
	}
	return primero, ultimo, n
}
