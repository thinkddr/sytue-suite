// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ficheros adjuntos dentro del PDF (§7.11.4).
//
// Un PDF puede llevar otros ficheros dentro: el XML de una factura electrónica
// —que es como funcionan Factur-X y ZUGFeRD—, la hoja de cálculo de la que sale
// una tabla, o el original de un informe convertido. Sin panel de adjuntos esos
// ficheros son invisibles: están en el documento, ocupan su tamaño y nadie
// puede sacarlos.
//
// Viven en dos sitios y hay que mirar los dos: el árbol de nombres
// `/Root /Names /EmbeddedFiles` (lo normal) y las anotaciones `/FileAttachment`
// de cada página, que es como los pega Acrobat cuando se arrastra un fichero
// sobre una página.
package domain

import (
	"fmt"
	"strings"
)

// Adjunto describe un fichero incrustado, sin su contenido.
type Adjunto struct {
	// Nombre es el del fichero tal y como lo declara el documento, ya saneado:
	// puede traer rutas y es de donde saldría un «../» al guardarlo.
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion,omitempty"`
	TamanoBytes int    `json:"tamano_bytes"`
	// Tipo es el MIME que DECLARA el documento. Es informativo y no se usa para
	// servir el fichero: ver ExtraerAdjunto.
	Tipo string `json:"tipo,omitempty"`
	// Pagina es donde está pegado si vino de una anotación; -1 si cuelga del
	// documento entero, que es lo normal en una factura electrónica.
	Pagina int `json:"pagina"`
}

// maxAdjuntos acota un documento con cientos de ficheros dentro.
const maxAdjuntos = 200

// AdjuntosDelDocumento lista los ficheros incrustados.
func (d *PDFDoc) AdjuntosDelDocumento() []Adjunto {
	var out []Adjunto
	vistos := map[string]bool{}

	d.recorrerFilespecs(func(spec pdfDict, pagina int) bool {
		if len(out) >= maxAdjuntos {
			return false // parar: ya hay de sobra
		}
		a, ok := d.adjuntoDe(spec, pagina)
		if !ok || vistos[a.Nombre] {
			return true
		}
		vistos[a.Nombre] = true
		out = append(out, a)
		return true
	})
	return out
}

// recorrerFilespecs visita cada /Filespec del documento y para si `f` devuelve
// falso. `pagina` es -1 cuando el adjunto cuelga del documento entero.
//
// Los adjuntos viven en DOS sitios y hay que mirar los dos: el árbol de nombres
// del catálogo —por donde viaja el XML de una factura electrónica— y las
// anotaciones /FileAttachment, que es el clip que se ve en una página. Mirar
// solo uno deja fuera la mitad de los documentos, y por eso el recorrido está
// aquí una vez y no copiado en cada función que lo necesita.
func (d *PDFDoc) recorrerFilespecs(f func(spec pdfDict, pagina int) bool) {
	if !d.filespecsDelCatalogo(f) {
		return
	}
	d.filespecsDeLasPaginas(f)
}

// filespecsDelCatalogo recorre el árbol de nombres /EmbeddedFiles, que es por
// donde viaja el XML de una factura electrónica. Devuelve falso si `f` pidió
// parar.
func (d *PDFDoc) filespecsDelCatalogo(f func(spec pdfDict, pagina int) bool) bool {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return true
	}
	nombres, _ := d.resolver(raiz["Names"]).(pdfDict)
	if nombres == nil {
		return true
	}
	arbol, _ := d.resolver(nombres["EmbeddedFiles"]).(pdfDict)
	if arbol == nil {
		return true
	}
	for _, v := range d.hojasDelArbolDeNombres(arbol, 0) {
		spec, ok := d.resolver(v).(pdfDict)
		if !ok {
			continue
		}
		// -1: cuelga del documento entero, no de una página.
		if !f(spec, -1) {
			return false
		}
	}
	return true
}

// filespecsDeLasPaginas recorre las anotaciones /FileAttachment, que es el clip
// que se ve sobre una página.
func (d *PDFDoc) filespecsDeLasPaginas(f func(spec pdfDict, pagina int) bool) {
	for i := range d.paginas {
		anots, _ := d.resolver(d.paginas[i].dict["Annots"]).(pdfArray)
		for _, a := range anots {
			ad, ok := d.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			if sub, _ := d.resolver(ad["Subtype"]).(pdfName); sub != "FileAttachment" {
				continue
			}
			spec, ok := d.resolver(ad["FS"]).(pdfDict)
			if !ok {
				continue
			}
			if !f(spec, i) {
				return
			}
		}
	}
}

// adjuntoDe lee un /Filespec (§7.11.3).
func (d *PDFDoc) adjuntoDe(spec pdfDict, pagina int) (Adjunto, bool) {
	ef, _ := d.resolver(spec["EF"]).(pdfDict)
	if ef == nil {
		return Adjunto{}, false
	}
	st := d.flujoDelAdjunto(ef)
	if st == nil {
		return Adjunto{}, false
	}

	// /UF es la versión en Unicode y manda sobre /F, que es la heredada.
	nombre := textoDelTitulo(d.resolver(spec["UF"]))
	if nombre == "" {
		nombre = textoDelTitulo(d.resolver(spec["F"]))
	}
	nombre = nombreDeAdjuntoSeguro(nombre)
	if nombre == "" {
		return Adjunto{}, false
	}

	a := Adjunto{
		Nombre:      nombre,
		Descripcion: textoDelTitulo(d.resolver(spec["Desc"])),
		Pagina:      pagina,
	}
	if sub, _ := d.resolver(st.Dict["Subtype"]).(pdfName); sub != "" {
		// El /Subtype de un fichero incrustado es el MIME con las barras
		// escapadas como #2F, y el lexer ya las ha decodificado.
		a.Tipo = string(sub)
	}
	// El tamaño declarado (/Params /Size) es orientativo; el real solo se sabe
	// descomprimiendo, y eso no se hace para pintar una lista.
	if params, _ := d.resolver(st.Dict["Params"]).(pdfDict); params != nil {
		if n, ok := comoEntero(d.resolver(params["Size"])); ok && n > 0 {
			a.TamanoBytes = int(n)
		}
	}
	if a.TamanoBytes == 0 {
		a.TamanoBytes = len(st.Raw)
	}
	return a, true
}

// flujoDelAdjunto saca el flujo del /EF, que puede venir en /F o en /UF.
func (d *PDFDoc) flujoDelAdjunto(ef pdfDict) *pdfStream {
	for _, k := range []pdfName{"F", "UF", "DOS", "Mac", "Unix"} {
		if st, ok := d.resolver(ef[k]).(*pdfStream); ok {
			return st
		}
	}
	return nil
}

// ExtraerAdjunto devuelve el contenido de un fichero incrustado.
//
// El nombre se compara ya saneado, que es como se entrega en la lista: aceptar
// el nombre crudo del documento permitiría pedir uno con «../» que la lista
// nunca enseñó.
func (d *PDFDoc) ExtraerAdjunto(nombre string) ([]byte, error) {
	// Se busca por el nombre YA saneado, en los dos lados de la comparación: si
	// se buscara por el crudo, un adjunto llamado `../../etc/passwd` se podría
	// pedir escribiendo esa misma ruta.
	nombre = nombreDeAdjuntoSeguro(nombre)
	if nombre == "" {
		return nil, fmt.Errorf("pdf: no se indicó qué adjunto extraer")
	}

	var encontrado *pdfStream
	d.recorrerFilespecs(func(spec pdfDict, _ int) bool {
		n := textoDelTitulo(d.resolver(spec["UF"]))
		if n == "" {
			n = textoDelTitulo(d.resolver(spec["F"]))
		}
		if nombreDeAdjuntoSeguro(n) != nombre {
			return true
		}
		if ef, _ := d.resolver(spec["EF"]).(pdfDict); ef != nil {
			encontrado = d.flujoDelAdjunto(ef)
		}
		return encontrado == nil // en cuanto se tiene, no hay más que mirar
	})
	if encontrado == nil {
		return nil, fmt.Errorf("pdf: el documento no lleva un adjunto llamado %q", nombre)
	}
	return d.datosDeFlujo(encontrado)
}

// hojasDelArbolDeNombres devuelve los VALORES de todas las hojas. Es el recorrido
// completo del árbol de §7.9.6, hermano de `buscarEnArbolDeNombres`: aquel busca
// una clave y puede podar por /Limits, este los quiere todos.
func (d *PDFDoc) hojasDelArbolDeNombres(nodo pdfDict, prof int) []pdfObj {
	if prof > maxProfundidadIndice {
		return nil
	}
	var out []pdfObj
	if pares, ok := d.resolver(nodo["Names"]).(pdfArray); ok {
		for i := 1; i < len(pares); i += 2 {
			out = append(out, pares[i])
		}
	}
	hijos, _ := d.resolver(nodo["Kids"]).(pdfArray)
	for _, k := range hijos {
		if hijo, ok := d.resolver(k).(pdfDict); ok {
			out = append(out, d.hojasDelArbolDeNombres(hijo, prof+1)...)
		}
		if len(out) > maxAdjuntos {
			break
		}
	}
	return out
}

// nombreDeAdjuntoSeguro deja el nombre en algo que se pueda guardar.
//
// El nombre lo escribe el documento y puede traer lo que sea: `../../.ssh/id_rsa`
// para escribir fuera del sitio previsto, separadores de Windows, o caracteres
// de control para partir una cabecera HTTP donde se ponga. Se queda con el
// último tramo y se limpia; si no queda nada utilizable, el adjunto no se
// entrega, porque un fichero que no se puede nombrar tampoco se puede guardar.
func nombreDeAdjuntoSeguro(n string) string {
	n = strings.TrimSpace(n)
	// Los dos separadores: un PDF hecho en Windows escribe `\` y uno de Unix `/`.
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	var b strings.Builder
	for _, r := range n {
		switch {
		case r < 0x20 || r == 0x7f: // controles: fuera
		case r == '"' || r == ':' || r == '*' || r == '?' || r == '<' || r == '>' || r == '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	n = strings.Trim(b.String(), " .")
	if len(n) > 200 {
		n = n[:200]
	}
	return n
}
