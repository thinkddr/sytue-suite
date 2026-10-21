// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"regexp"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # Lo que se guarda en una celda es HTML, no texto
//
// En la web cada celda es un `contenteditable`, y lo que va al documento es su
// `innerHTML`: «a & b» se guarda como «a &amp; b», y una celda en negrita como
// «<b>10</b>». El texto crudo —el literal o la fórmula— es lo que queda al quitarle
// las etiquetas (`rawText` en `sheets/format.ts`), y es lo único que ve el motor.
//
// Este fichero es el puerto de los cinco ayudantes puros de `format.ts` que traducen
// entre las dos cosas: `escapeHtml`, `stripHtml`, `parseCell`, `renderCellHtml` y
// `isFormula`. Con el mismo orden de envoltorios —alineación, negrita, cursiva— y las
// mismas expresiones, porque `parseCell` y `renderCellHtml` son inversas y de eso
// depende que un conmutador de negrita no deje basura al pulsarlo dos veces.
//
// Antes de esto el lienzo guardaba el texto TAL CUAL: una celda con «<5» salía al
// documento como una etiqueta rota, y la web la pintaba vacía.

// partes es `CellParts`: la alineación, la negrita, la cursiva y lo de dentro, que es
// HTML (escapado), no texto.
type partes struct {
	alinear string // "", "center" o "right"
	b, i    bool
	dentro  string
}

var (
	reEtiqueta = regexp.MustCompile(`<[^>]*>`)

	escapador = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	// El orden es el de `stripHtml` y da igual que sea de una pasada: `&amp;` va el
	// último allí, así que lo que decodifica nunca se vuelve a mirar.
	desescapador = strings.NewReplacer("&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")
)

// escaparHTML es `escapeHtml`.
func escaparHTML(s string) string { return escapador.Replace(s) }

// textoPlano es `stripHtml` (y `rawText`): quita las etiquetas y decodifica las cinco
// entidades que escribe un `contenteditable`.
func textoPlano(html string) string {
	return desescapador.Replace(reEtiqueta.ReplaceAllString(html, ""))
}

// esFormula es `isFormula`: el texto crudo empieza por «=».
func esFormula(html string) bool { return strings.HasPrefix(textoPlano(html), "=") }

// leerCelda es `parseCell`, que vive en el motor porque las descargas del servicio lo
// leen igual.
func leerCelda(html string) partes {
	p := formula.LeerCelda(html)
	return partes{alinear: p.Alinear, b: p.Negrita, i: p.Cursiva, dentro: p.Dentro}
}

// html es `renderCellHtml`: lo inverso de leerCelda. Una celda sin texto es "", con
// formato o sin él, que es como la web decide que una celda está vacía.
func (p partes) html() string {
	out := strings.TrimSpace(p.dentro)
	if out == "" || out == "<br>" {
		return ""
	}
	if p.i {
		out = "<i>" + out + "</i>"
	}
	if p.b {
		out = "<b>" + out + "</b>"
	}
	if p.alinear != "" {
		out = `<div style="text-align: ` + p.alinear + `;">` + out + "</div>"
	}
	return out
}

// conTexto es el HTML de la celda con OTRO texto crudo y el mismo formato: lo que hace
// la web al confirmar la barra de fórmulas (`p.inner = escapeHtml(fx)`).
func conTexto(anterior, texto string) string {
	p := leerCelda(anterior)
	p.dentro = escaparHTML(texto)
	return p.html()
}

// celdaEntendida dice si este lienzo sabe PINTAR y REESCRIBIR esa celda sin cambiarla.
//
// Lo que entiende es el envoltorio canónico —alineación, negrita, cursiva— alrededor
// de texto escapado con las cinco entidades de `stripHtml`. Cualquier otra etiqueta
// (`<span>`, `<br>`, `<u>`…) o entidad (`&eacute;`, `&#39;`) es algo que la web pinta
// de una manera y `rawText` lee de otra, y reescribirlo sería adivinar.
func celdaEntendida(html string) bool {
	dentro := leerCelda(html).dentro
	if strings.Contains(dentro, "<") {
		return false
	}
	for i := strings.IndexByte(dentro, '&'); i >= 0; i = strings.IndexByte(dentro, '&') {
		resto := dentro[i:]
		ok := false
		for _, e := range []string{"&amp;", "&lt;", "&gt;", "&quot;", "&nbsp;"} {
			if strings.HasPrefix(resto, e) {
				ok, dentro = true, resto[len(e):]
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
