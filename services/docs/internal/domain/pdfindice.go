// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El índice del documento: los marcadores de `/Outlines` (§12.3.3).
//
// Es lo primero que tiene cualquier lector de PDF y lo que hace utilizable un
// informe de doscientas páginas o una norma con cien artículos. Sin él solo
// queda la barra de miniaturas, que sirve para hojear pero no para ir a «el
// anexo III».
//
// Lo que cuesta de verdad no es recorrer el árbol —es una lista doblemente
// enlazada por /First, /Next— sino AVERIGUAR A QUÉ PÁGINA VA cada entrada: el
// destino puede estar escrito de cuatro formas distintas y las tres indirectas
// pasan por tablas que viven en otra parte del documento.
package domain

// Marcador es una entrada del índice, ya aplanada con su nivel.
//
// Se entrega en una lista plana y no en un árbol porque es lo que la interfaz
// pinta —una fila con sangría por nivel— y porque un árbol en JSON obliga a
// quien lo consume a recorrerlo para lo mismo.
type Marcador struct {
	Titulo string `json:"titulo"`
	// Pagina es 0-based; -1 si el destino no se pudo resolver. La entrada se
	// entrega IGUAL: un marcador que no navega sigue diciendo qué hay en el
	// documento, y esconderlo dejaría un índice con huecos sin explicación.
	Pagina int `json:"pagina"`
	// Nivel de sangría, 0 para los de primer nivel.
	Nivel int `json:"nivel"`
	// Abierto es lo que el documento pide: si sus hijos se ven al abrir.
	Abierto bool `json:"abierto,omitempty"`
}

// maxMarcadores corta un índice absurdo (o un árbol con un ciclo que el control
// de profundidad no atrape). Dos mil entradas son más de las que nadie recorre.
const maxMarcadores = 2000

// maxProfundidadIndice acota el anidamiento. Ocho niveles son «Parte > Título >
// Capítulo > Sección > Artículo > Apartado > Párrafo > inciso»: más que
// cualquier norma real.
const maxProfundidadIndice = 8

// IndiceDelDocumento devuelve los marcadores en el orden en que se leen.
//
// Vacío si el documento no tiene índice, que es lo normal en un escaneo o en un
// PDF generado por una impresora virtual.
func (d *PDFDoc) IndiceDelDocumento() []Marcador {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return nil
	}
	arbol, _ := d.resolver(raiz["Outlines"]).(pdfDict)
	if arbol == nil {
		return nil
	}
	if _, ok := d.resolver(arbol["First"]).(pdfDict); !ok {
		return nil
	}
	// El índice de páginas se construye UNA vez: resolver el destino de cada
	// marcador por su cuenta obligaría a recorrer el árbol de páginas por
	// entrada, que en un documento con índice largo es cuadrático.
	porObjeto := d.paginaPorObjeto()

	var out []Marcador
	d.recorrerMarcadores(arbol["First"], 0, porObjeto, &out, map[int]bool{}, 0)
	return out
}

// recorrerMarcadores sigue la cadena /Next y baja por /First.
//
// Recorre REFERENCIAS y no diccionarios ya resueltos: `vistos` guarda números de
// objeto, que es lo único con lo que se puede cortar un ciclo —un `pdfDict` es
// un map de Go y los maps no son comparables, así que no valen como clave—. Un
// /Next que apunta hacia atrás es un documento roto que existe, y sin esto
// cuelga el servicio en vez de dar un índice raro.
func (d *PDFDoc) recorrerMarcadores(ref pdfObj, nivel int,
	porObjeto map[int]int, out *[]Marcador, vistos map[int]bool, pasos int,
) {
	for actualRef := ref; actualRef != nil; {
		if len(*out) >= maxMarcadores || pasos > maxMarcadores {
			return
		}
		pasos++
		if r, ok := actualRef.(pdfRef); ok {
			if vistos[r.Num] {
				return
			}
			vistos[r.Num] = true
		}
		actual, ok := d.resolver(actualRef).(pdfDict)
		if !ok {
			return
		}

		if titulo := textoDelTitulo(d.resolver(actual["Title"])); titulo != "" {
			m := Marcador{Titulo: titulo, Nivel: nivel, Pagina: d.paginaDelDestino(actual, porObjeto)}
			// /Count positivo = el documento quiere sus hijos a la vista.
			if n, ok := comoEntero(d.resolver(actual["Count"])); ok && n > 0 {
				m.Abierto = true
			}
			*out = append(*out, m)
		}

		if nivel+1 < maxProfundidadIndice {
			d.recorrerMarcadores(actual["First"], nivel+1, porObjeto, out, vistos, pasos)
		}
		actualRef = actual["Next"]
	}
}

// textoDelTitulo decodifica el /Title, que viaja como cadena de texto del PDF
// (UTF-16 con BOM o PDFDocEncoding).
func textoDelTitulo(o pdfObj) string {
	s, ok := o.(pdfString)
	if !ok {
		return ""
	}
	return decodificarTextoPDF(s)
}

// paginaDelDestino resuelve a qué página va un marcador. Las cuatro formas que
// permite la especificación, y las tres últimas pasan por tablas de otra parte
// del documento:
//
//	/Dest [3 0 R /Fit]     el array directo — la fácil
//	/Dest (nombre)         cadena, se busca en /Dests del catálogo (PDF 1.1)
//	/Dest /nombre          nombre, misma tabla
//	/A << /S /GoTo /D … >> una ACCIÓN, y su /D es cualquiera de las tres de arriba
//
// Un lector que solo entienda la primera deja sin navegar la mitad de los
// índices que genera Word, que usa nombres.
func (d *PDFDoc) paginaDelDestino(nodo pdfDict, porObjeto map[int]int) int {
	dest := nodo["Dest"]
	if dest == nil {
		if accion, ok := d.resolver(nodo["A"]).(pdfDict); ok {
			if s, _ := d.resolver(accion["S"]).(pdfName); s == "GoTo" {
				dest = accion["D"]
			}
		}
	}
	if dest == nil {
		return -1
	}
	return d.paginaDeDestino(dest, porObjeto, 0)
}

// maxSaltosDestino corta una cadena de nombres que se apuntan entre sí.
const maxSaltosDestino = 4

func (d *PDFDoc) paginaDeDestino(dest pdfObj, porObjeto map[int]int, salto int) int {
	if salto > maxSaltosDestino {
		return -1
	}
	switch v := dest.(type) {
	case pdfRef:
		return d.paginaDeDestino(d.resolver(v), porObjeto, salto+1)
	case pdfArray:
		if len(v) == 0 {
			return -1
		}
		// El primer elemento es la página: una referencia normalmente, o un
		// ÍNDICE de página en los destinos remotos (que aquí no aplican, pero un
		// generador despistado los escribe igual).
		switch p := v[0].(type) {
		case pdfRef:
			if n, ok := porObjeto[p.Num]; ok {
				return n
			}
		default:
			if n, ok := comoInt(v[0]); ok && n >= 0 && n < d.NumPaginas() {
				return n
			}
		}
		return -1
	case pdfName:
		return d.paginaDeDestino(d.destinoConNombre(string(v)), porObjeto, salto+1)
	case pdfString:
		return d.paginaDeDestino(d.destinoConNombre(string(v)), porObjeto, salto+1)
	default:
		return -1
	}
}

// destinoConNombre busca en las DOS tablas donde puede estar: el `/Dests` del
// catálogo (PDF 1.1, un diccionario llano) y el árbol de nombres
// `/Names /Dests` (PDF 1.2+, ordenado y por tramos). Un documento puede traer
// las dos, y Word escribe la segunda.
func (d *PDFDoc) destinoConNombre(nombre string) pdfObj {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return nil
	}
	if tabla, ok := d.resolver(raiz["Dests"]).(pdfDict); ok {
		if v, hay := tabla[pdfName(nombre)]; hay {
			return d.desenvolverDestino(v)
		}
	}
	nombres, _ := d.resolver(raiz["Names"]).(pdfDict)
	if nombres == nil {
		return nil
	}
	arbol, _ := d.resolver(nombres["Dests"]).(pdfDict)
	if arbol == nil {
		return nil
	}
	return d.desenvolverDestino(d.buscarEnArbolDeNombres(arbol, nombre, 0))
}

// desenvolverDestino acepta el destino tal cual o dentro de un diccionario con
// /D, que es como lo envuelve el árbol de nombres.
func (d *PDFDoc) desenvolverDestino(v pdfObj) pdfObj {
	if dict, ok := d.resolver(v).(pdfDict); ok {
		return dict["D"]
	}
	return v
}

// buscarEnArbolDeNombres recorre el árbol de §7.9.6: cada nodo tiene hojas
// (/Names, pares nombre-valor ORDENADOS) o hijos (/Kids con su rango /Limits).
//
// Se respeta el /Limits para no bajar por ramas que no pueden contenerlo: en un
// documento con miles de destinos, ignorarlo convierte cada búsqueda en un
// recorrido completo.
func (d *PDFDoc) buscarEnArbolDeNombres(nodo pdfDict, nombre string, prof int) pdfObj {
	if prof > maxProfundidadIndice {
		return nil
	}
	if lim, ok := d.resolver(nodo["Limits"]).(pdfArray); ok && len(lim) >= 2 {
		bajo := comoCadena(d.resolver(lim[0]))
		alto := comoCadena(d.resolver(lim[1]))
		if bajo != "" && nombre < bajo {
			return nil
		}
		if alto != "" && nombre > alto {
			return nil
		}
	}
	if pares, ok := d.resolver(nodo["Names"]).(pdfArray); ok {
		for i := 0; i+1 < len(pares); i += 2 {
			if comoCadena(d.resolver(pares[i])) == nombre {
				return pares[i+1]
			}
		}
	}
	hijos, _ := d.resolver(nodo["Kids"]).(pdfArray)
	for _, k := range hijos {
		hijo, ok := d.resolver(k).(pdfDict)
		if !ok {
			continue
		}
		if v := d.buscarEnArbolDeNombres(hijo, nombre, prof+1); v != nil {
			return v
		}
	}
	return nil
}

// paginaPorObjeto mapea el número de objeto de cada página a su índice.
func (d *PDFDoc) paginaPorObjeto() map[int]int {
	out := make(map[int]int, len(d.paginas))
	for i, p := range d.paginas {
		if p.num != 0 {
			out[p.num] = i
		}
	}
	return out
}

// comoCadena saca el texto de un valor que puede venir como cadena o como
// nombre. Los destinos con nombre usan las dos formas según la versión del PDF.
func comoCadena(o pdfObj) string {
	switch v := o.(type) {
	case pdfString:
		return string(v)
	case pdfName:
		return string(v)
	default:
		return ""
	}
}
