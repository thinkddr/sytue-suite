// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// PDF: el volcado para IMPRIMIR o archivar, no para restaurar.
//
// Es el formato que se manda a quien no va a abrir nada nuestro y el que aguanta
// como copia impresa: paginado, con el título arriba y cada nota debajo. Lo que
// NO es —y conviene decirlo aquí porque parece que sí— es un formato del que se
// pueda volver: no lleva volcado embebido. Para eso está el HTML.
//
// Usa el mismo escritor que Sytue Docs (platform/pkg/pdfsimple), que salió de
// allí justo para esto: dos copias del ensamblado de objetos, xref y trailer
// serían dos sitios donde se desalinea un offset.

// Tamaños en puntos. Se nombran para que la jerarquía se lea de un vistazo y no
// haya números sueltos repartidos por la función.
const (
	pdfTituloCuaderno = 22.0
	pdfTituloNota     = 14.0
	pdfCuerpo         = 11.0
	pdfMenor          = 9.0
)

// NotebookAPDF vuelca el cuaderno como PDF paginado.
func NotebookAPDF(nb Notebook, notas []Note) []byte {
	w := pdfsimple.Nuevo()
	w.Texto(tituloDe(nb.Title, "Cuaderno sin título"), pdfsimple.FuenteNegrita, pdfTituloCuaderno)
	w.Hueco(10)

	for _, n := range enOrdenDePantalla(notas) {
		w.Texto(tituloDe(n.Title, "Nota sin título"), pdfsimple.FuenteNegrita, pdfTituloNota)
		w.Hueco(4)

		switch {
		case n.ContentCipher != "":
			// En cursiva y dicho, no omitido: quien imprima el cuaderno tiene que
			// ver que ahí HABÍA algo que no se puede imprimir, o contará mal lo que
			// tiene.
			w.Texto("[Nota cifrada de extremo a extremo: su contenido no puede imprimirse.]",
				pdfsimple.FuenteCursiva, pdfCuerpo)
		case strings.TrimSpace(n.Content) != "":
			// El contenido es HTML; al PDF va su TEXTO. Se reutiliza el mismo
			// conversor que el volcado a `.txt`, que ya respeta los cortes de línea
			// de párrafos y listas: sin él, la nota entera saldría como un renglón.
			for _, linea := range strings.Split(TextoDeHTML(n.Content), "\n") {
				if strings.TrimSpace(linea) == "" {
					w.Hueco(4)
					continue
				}
				w.Texto(linea, pdfsimple.FuenteNormal, pdfCuerpo)
			}
		default:
			w.Texto("(vacía)", pdfsimple.FuenteCursiva, pdfCuerpo)
		}

		if e := estadosEnLinea(n); e != "" {
			w.Hueco(3)
			w.Texto(e, pdfsimple.FuenteCursiva, pdfMenor)
		}
		for _, l := range n.Links {
			w.Texto("→ "+enlaceEnLinea(l), pdfsimple.FuenteNormal, pdfMenor)
		}
		w.Hueco(14)
	}
	return w.Bytes()
}

// estadosEnLinea deja los estados en una sola línea, con su etiqueta delante: en
// papel no hay color que distinga «Alta» de «Alta» de otra etiqueta.
func estadosEnLinea(n Note) string {
	nombres := make([]string, 0, len(n.States))
	for _, s := range n.States {
		estado := strings.TrimSpace(s.Name)
		if estado == "" {
			continue
		}
		if tag := strings.TrimSpace(s.TagName); tag != "" {
			estado = tag + ": " + estado
		}
		nombres = append(nombres, estado)
	}
	if len(nombres) == 0 {
		return ""
	}
	return "Etiquetas: " + strings.Join(nombres, ", ")
}

// enlaceEnLinea escribe un enlace de forma que siga sirviendo en papel: la URL
// entera, porque ahí no se puede pulsar nada.
func enlaceEnLinea(l NoteLink) string {
	rotulo := strings.TrimSpace(l.Label)
	if rotulo == "" {
		rotulo = l.TargetID
	}
	if u := strings.TrimSpace(l.URL); u != "" {
		return rotulo + " — " + u
	}
	return rotulo + " (" + l.TargetKind + ": " + l.TargetID + ")"
}
