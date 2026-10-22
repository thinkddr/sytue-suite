// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Responder a un comentario.
//
// Un documento que se revisa entre varios no se comenta: se DISCUTE. Sin
// respuestas, la única forma de contestar a un «esto habría que revisarlo» es
// otra nota suelta al lado, y a la tercera nadie sabe qué contesta a qué.
//
// La especificación lo resuelve con /IRT (§12.5.6.2): una anotación normal que
// apunta a la que responde, más /RT /R para decir que es una réplica y no un
// grupo. Acrobat y Foxit pintan con eso el hilo, así que un documento comentado
// aquí se lee igual allí.
//
// Va por ANEXO y no reensamblando: responder a un comentario no puede tirar una
// firma, y quien revisa un contrato firmado es justo quien más comenta.
package domain

import (
	"fmt"
	"time"
)

// maxLargoRespuesta acota el texto de una réplica. Es un comentario, no un
// documento adjunto.
const maxLargoRespuesta = 8000

// ResponderAAnotacionPDF añade una réplica a la anotación número `indice` de la
// lista que devuelve AnotacionesDelDocumento.
//
// El índice es el de ESA lista y no el del array /Annots de la página: son
// distintos porque los enlaces y las emergentes no se enumeran. Usar el otro
// haría que la respuesta colgara de un comentario que no es.
func ResponderAAnotacionPDF(doc *PDFDoc, indice int, texto, autor string) ([]byte, error) {
	return anadirReplica(doc, indice, texto, autor, "")
}

// EstadosDeRevision son los que admite /State con /StateModel /Review
// (§12.5.6.4). Son los mismos que enseña Acrobat, y por eso se escriben en
// inglés: cambiarlos por su traducción haría que otro lector no los entendiera.
var EstadosDeRevision = map[string]bool{
	"Accepted": true, "Rejected": true, "Cancelled": true, "Completed": true, "None": true,
}

// MarcarComentarioPDF pone el estado de revisión de un comentario.
//
// Es lo que hace usable una revisión de cuarenta comentarios: sin poder cerrar
// los resueltos, la segunda vuelta obliga a releerlos todos para saber cuáles
// quedan. La especificación lo guarda como una réplica que en vez de texto lleva
// /State, así que es el mismo mecanismo que responder.
func MarcarComentarioPDF(doc *PDFDoc, indice int, estado, autor string) ([]byte, error) {
	if !EstadosDeRevision[estado] {
		return nil, fmt.Errorf("pdf: %q no es un estado de revisión", estado)
	}
	return anadirReplica(doc, indice, "", autor, estado)
}

// anadirReplica escribe una anotación colgada de otra: una respuesta con texto,
// o una marca de estado. Las dos son lo mismo para el formato y por eso están
// aquí juntas; separarlas duplicaría el anexo entero.
func anadirReplica(doc *PDFDoc, indice int, texto, autor, estado string) ([]byte, error) {
	if doc == nil || len(doc.buf) == 0 {
		return nil, fmt.Errorf("%w: no hay fichero de partida", ErrNoIncremental)
	}
	if doc.cripto != nil {
		return nil, fmt.Errorf("%w: está cifrado", ErrNoIncremental)
	}
	if doc.reparado {
		return nil, fmt.Errorf("%w: la tabla de referencias estaba rota", ErrNoIncremental)
	}
	texto = recortarMetadato(texto)
	// Una marca de estado no lleva texto —el estado ES el mensaje—, pero una
	// respuesta sin nada escrito no dice nada y no se escribe.
	if texto == "" && estado == "" {
		return nil, fmt.Errorf("pdf: una respuesta sin texto no dice nada")
	}
	if len([]rune(texto)) > maxLargoRespuesta {
		return nil, fmt.Errorf("pdf: la respuesta pasa de %d caracteres", maxLargoRespuesta)
	}

	padreRef, padre, pagina, ok := doc.anotacionNumero(indice)
	if !ok {
		return nil, fmt.Errorf("pdf: no hay ningún comentario número %d", indice+1)
	}
	xrefAnterior, err := offsetDelUltimoXref(doc.buf)
	if err != nil {
		return nil, err
	}
	raizRef, ok := doc.trailer["Root"].(pdfRef)
	if !ok {
		return nil, fmt.Errorf("%w: el /Root no es una referencia", ErrNoIncremental)
	}

	siguiente := doc.maxObjeto() + 1
	numRespuesta := siguiente
	siguiente++

	// La réplica va DONDE EL PADRE: si tuviera su propio sitio en la página,
	// aparecería como un globo suelto encima del texto de alguien.
	rect := pdfArray{0.0, 0.0, 0.0, 0.0}
	if r, ok := doc.resolver(padre["Rect"]).(pdfArray); ok && len(r) >= 4 {
		rect = pdfArray{r[0], r[1], r[2], r[3]}
	}

	respuesta := pdfDict{
		"Type":    pdfName("Annot"),
		"Subtype": pdfName("Text"),
		"Rect":    rect,
		// Bit 3 = imprimir, como el resto: un hilo que desaparece al imprimir es
		// la queja clásica de quien revisa en papel.
		"F":        int64(4),
		"Contents": cadenaUTF16(texto),
		"M":        pdfString(fechaPDF(time.Now().UTC())),
		"IRT":      padreRef,
		// /R = réplica. El otro valor, /Group, agrupa anotaciones que van juntas
		// pero NO son una conversación, y el lector las pinta distinto.
		"RT": pdfName("R"),
		// Cerrada: un hilo de diez réplicas abriría diez globos sobre la página.
		"Open": false,
	}
	if a := recortarMetadato(autor); a != "" {
		respuesta["T"] = cadenaUTF16(a)
	}
	if estado != "" {
		// /StateModel es obligatorio junto a /State: sin él, un lector no sabe si
		// «Completed» habla de la revisión o de si alguien la ha marcado.
		respuesta["State"] = pdfString(estado)
		respuesta["StateModel"] = pdfString("Review")
	}

	nuevos := map[int]objetoSalida{numRespuesta: {valor: respuesta}}

	// La página tiene que ENUMERAR la respuesta: un objeto de anotación que no
	// está en ningún /Annots no lo enseña ningún lector, y el fichero engordaría
	// sin que la réplica existiera para nadie.
	pagRef, pagDict, ok := doc.refDePagina(pagina)
	if !ok {
		return nil, fmt.Errorf("%w: la página %d no se pudo localizar", ErrNoIncremental, pagina+1)
	}
	copia := copiaLlana(pagDict)
	anots, _ := doc.resolver(pagDict["Annots"]).(pdfArray)
	copia["Annots"] = append(append(pdfArray{}, anots...), pdfRef{Num: numRespuesta})
	nuevos[pagRef.Num] = objetoSalida{valor: copia}

	return escribirAnexo(doc, nuevos, siguiente, raizRef, xrefAnterior, nil)
}

// anotacionNumero localiza la anotación `indice` de la lista de comentarios.
//
// Recorre con el MISMO criterio que AnotacionesDelDocumento —fuera enlaces,
// emergentes y campos de formulario—, porque si los dos contaran distinto el
// índice significaría una anotación en el panel y otra aquí.
func (d *PDFDoc) anotacionNumero(indice int) (ref pdfRef, dict pdfDict, pagina int, ok bool) {
	if indice < 0 {
		return pdfRef{}, nil, 0, false
	}
	n := 0
	for i := 0; i < d.NumPaginas(); i++ {
		p, err := d.Pagina(i)
		if err != nil {
			continue
		}
		anots, _ := d.resolver(p.dict["Annots"]).(pdfArray)
		for _, a := range anots {
			ad, ok := d.resolver(a).(pdfDict)
			if !ok {
				continue
			}
			if esAnotacionDeComentario(d, ad) {
				if n == indice {
					// Solo se puede responder a una anotación que sea un OBJETO
					// propio: a una escrita en línea dentro del array no se le
					// puede apuntar con /IRT.
					r, esRef := a.(pdfRef)
					return r, ad, i, esRef
				}
				n++
			}
		}
	}
	return pdfRef{}, nil, 0, false
}

// esAnotacionDeComentario dice si una anotación cuenta como comentario.
//
// Un enlace no es de nadie, una emergente es la ventanita de otra —contarla
// duplicaría cada nota— y un widget es un campo de formulario.
//
// Una marca de estado tampoco: no dice nada, dice algo SOBRE otra. Si contara,
// aceptar un comentario correría el índice de todos los de abajo y la siguiente
// respuesta colgaría del que no es. Este criterio es el único que hay, y por eso
// leer, borrar y responder lo comparten.
func esAnotacionDeComentario(d *PDFDoc, ad pdfDict) bool {
	sub, _ := d.resolver(ad["Subtype"]).(pdfName)
	if sub == "Link" || sub == "Popup" || sub == "Widget" {
		return false
	}
	_, marcaDeEstado := d.resolver(ad["State"]).(pdfString)
	return !marcaDeEstado
}

// refDePagina devuelve la referencia y el diccionario de una página.
func (d *PDFDoc) refDePagina(i int) (pdfRef, pdfDict, bool) {
	if i < 0 || i >= len(d.paginas) {
		return pdfRef{}, nil, false
	}
	return pdfRef{Num: d.paginas[i].num}, d.paginas[i].dict, true
}
