// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"html"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// HTMLDeBloques es lo que `escribirBloques` del panel del asistente deja en el folio: los
// títulos, los párrafos y las listas que ha escrito el modelo, con su negrita y su
// cursiva. Se arma desde los bloques de tipos cerrados de `ia.BloquesDe` y con el texto
// ESCAPADO: la respuesta es de un tercero y no se inyecta nada suyo.
func HTMLDeBloques(bloques []ia.Bloque) string {
	var b strings.Builder
	lista := ""
	cerrarLista := func() {
		if lista != "" {
			b.WriteString("</" + lista + ">")
			lista = ""
		}
	}
	for _, bl := range bloques {
		etiqueta := string(bl.Tipo)
		if bl.Tipo == ia.Punto {
			quiere := "ul"
			if bl.Ordenada {
				quiere = "ol"
			}
			if lista != quiere {
				cerrarLista()
				b.WriteString("<" + quiere + ">")
				lista = quiere
			}
		} else {
			cerrarLista()
		}
		b.WriteString("<" + etiqueta + ">")
		for _, t := range bl.Trozos {
			texto := html.EscapeString(t.Texto)
			if t.Cursiva {
				texto = "<em>" + texto + "</em>"
			}
			if t.Negrita {
				texto = "<strong>" + texto + "</strong>"
			}
			b.WriteString(texto)
		}
		b.WriteString("</" + etiqueta + ">")
	}
	cerrarLista()
	return b.String()
}
