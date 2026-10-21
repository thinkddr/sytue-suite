// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Editar las propiedades del documento: título, autor, asunto y palabras clave
// (§14.3.3, el diccionario /Info).
//
// Se leían desde el principio —`Huella` las usa para avisar antes de compartir—
// pero no se podían cambiar, y es la mitad que hace falta: un PDF exportado de
// Word sale con el título «Documento1» y el nombre de quien lo escribió, y así
// llega al cliente.
//
// No confundir con SanearMetadatosPDF, que los BORRA todos. Aquí se corrigen.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// Metadatos son las propiedades editables del documento.
//
// Un campo vacío BORRA el que hubiera; para dejarlo como está no se manda la
// clave. Es la única forma de poder quitar el nombre del autor sin tener que
// borrar todo lo demás.
type Metadatos struct {
	Titulo   *string `json:"titulo,omitempty"`
	Autor    *string `json:"autor,omitempty"`
	Asunto   *string `json:"asunto,omitempty"`
	Palabras *string `json:"palabras,omitempty"`
}

// maxLargoMetadato acota cada campo. Un título de más de mil caracteres no es
// un título, y el /Info se lee entero en cada apertura.
const maxLargoMetadato = 1000

// EditarMetadatosPDF devuelve el documento con las propiedades cambiadas.
//
// Escribe por ANEXO, así que una firma anterior sigue verificando su rango: los
// metadatos son de lo que más se corrige justo antes de mandar algo, y
// reescribir el fichero por un título tiraría la firma.
//
// AVISO documentado y no resuelto: si el documento lleva metadatos XMP
// (`/Root /Metadata`), estos NO se tocan y pueden contradecir al /Info. Un
// lector moderno prefiere el XMP, así que el cambio podría no verse. Se
// devuelve `conXMP` para que la interfaz lo diga en vez de que el usuario
// descubra que su corrección no aparece.
func EditarMetadatosPDF(doc *PDFDoc, m Metadatos) (salida []byte, conXMP bool, err error) {
	if doc == nil || len(doc.buf) == 0 {
		return nil, false, fmt.Errorf("%w: no hay fichero de partida", ErrNoIncremental)
	}
	if doc.cripto != nil {
		return nil, false, fmt.Errorf("%w: está cifrado", ErrNoIncremental)
	}
	if doc.reparado {
		return nil, false, fmt.Errorf("%w: la tabla de referencias estaba rota", ErrNoIncremental)
	}
	if m.Titulo == nil && m.Autor == nil && m.Asunto == nil && m.Palabras == nil {
		return nil, false, fmt.Errorf("pdf: no se indicó ninguna propiedad que cambiar")
	}
	raizRef, ok := doc.trailer["Root"].(pdfRef)
	if !ok {
		return nil, false, fmt.Errorf("%w: el /Root no es una referencia", ErrNoIncremental)
	}
	xrefAnterior, err := offsetDelUltimoXref(doc.buf)
	if err != nil {
		return nil, false, err
	}

	// Se parte del /Info que hubiera: cambiar el título no puede borrar la fecha
	// de creación ni el productor.
	info := pdfDict{}
	if ya, ok := doc.resolver(doc.trailer["Info"]).(pdfDict); ok {
		info = copiaLlana(ya)
	}
	aplicar := func(clave pdfName, v *string) {
		if v == nil {
			return
		}
		texto := recortarMetadato(*v)
		if texto == "" {
			delete(info, clave)
			return
		}
		info[clave] = cadenaUTF16(texto)
	}
	aplicar("Title", m.Titulo)
	aplicar("Author", m.Autor)
	aplicar("Subject", m.Asunto)
	aplicar("Keywords", m.Palabras)
	// Quien produjo la última versión del fichero es Sytue, y decirlo es lo
	// correcto: el /Producer describe la herramienta, no al autor.
	info["Producer"] = cadenaUTF16("Sytue PDF")
	info["ModDate"] = pdfString(fechaPDF(time.Now().UTC()))

	siguiente := doc.maxObjeto() + 1
	numInfo := siguiente
	siguiente++
	nuevos := map[int]objetoSalida{numInfo: {valor: info}}

	if raiz, ok := doc.resolver(raizRef).(pdfDict); ok {
		_, conXMP = raiz["Metadata"]
	}
	salida, err = escribirAnexo(doc, nuevos, siguiente, raizRef, xrefAnterior,
		pdfDict{"Info": pdfRef{Num: numInfo}})
	return salida, conXMP, err
}

// recortarMetadato limpia y acota un valor.
func recortarMetadato(s string) string {
	s = strings.TrimSpace(s)
	// Los controles no aportan nada en un título y sí sirven para ensuciar la
	// línea de un visor que lo pinte tal cual.
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r != 0x7f {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())
	if len([]rune(s)) > maxLargoMetadato {
		s = string([]rune(s)[:maxLargoMetadato])
	}
	return s
}
