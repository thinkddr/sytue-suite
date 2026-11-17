// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"regexp"
	"strings"
)

// PartesCelda es `CellParts` de `sheets/format.ts`: lo que envuelve el HTML de una celda
// —alineación, negrita y cursiva, en ese orden— y lo de dentro, que es HTML escapado.
// La comparten el lienzo de Calcus y las descargas del servicio de Docs.
type PartesCelda struct {
	Alinear          string // "", "center" o "right"
	Negrita, Cursiva bool
	Dentro           string
}

var (
	reAlinear = regexp.MustCompile(`(?is)^<div style="text-align:\s*(center|right);?">(.*)</div>$`)
	reNegrita = regexp.MustCompile(`(?is)^<b>(.*)</b>$`)
	reCursiva = regexp.MustCompile(`(?is)^<i>(.*)</i>$`)

	reEtiqueta = regexp.MustCompile(`<[^>]*>`)

	escapador = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	// El orden es el de `stripHtml` y da igual que sea de una pasada: `&amp;` va el
	// último allí, así que lo que decodifica nunca se vuelve a mirar.
	desescapador = strings.NewReplacer("&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")
)

// EscaparHTML es `escapeHtml` de `sheets/format.ts`.
func EscaparHTML(s string) string { return escapador.Replace(s) }

// TextoCelda es `stripHtml` (y `rawText`): quita las etiquetas y decodifica las cinco
// entidades que escribe un `contenteditable`. Es el texto crudo de la celda: el literal
// o la fórmula.
func TextoCelda(html string) string {
	return desescapador.Replace(reEtiqueta.ReplaceAllString(html, ""))
}

// HTML es `renderCellHtml`: lo inverso de LeerCelda. Una celda sin texto es "", con
// formato o sin él, que es como la web decide que una celda está vacía.
func (p PartesCelda) HTML() string {
	out := strings.TrimSpace(p.Dentro)
	if out == "" || out == "<br>" {
		return ""
	}
	if p.Cursiva {
		out = "<i>" + out + "</i>"
	}
	if p.Negrita {
		out = "<b>" + out + "</b>"
	}
	if p.Alinear != "" {
		out = `<div style="text-align: ` + p.Alinear + `;">` + out + "</div>"
	}
	return out
}

// ReescribirCeldaEn es `reescribirCelda` de `sheets/refshift.ts` para una celda que vive
// en la hoja `base`: su fórmula con las referencias movidas, conservando el envoltorio.
// Un literal, o una fórmula que no cambia, sale tal cual. La comparten el lienzo de
// Calcus y la protección del servidor, que comprueba que una celda bloqueada reescrita
// es exactamente esto (office/model/proteccion_ajenas.go).
func (d Desplazamiento) ReescribirCeldaEn(html, base string) string {
	if html == "" {
		return html
	}
	raw := TextoCelda(html)
	if !strings.HasPrefix(raw, "=") {
		return html
	}
	nuevo := d.ReescribirFormulaEn(raw, base)
	if nuevo == raw {
		return html
	}
	p := LeerCelda(html)
	p.Dentro = EscaparHTML(nuevo)
	return p.HTML()
}

// LeerCelda es `parseCell`.
func LeerCelda(html string) PartesCelda {
	p := PartesCelda{Dentro: strings.TrimSpace(html)}
	if m := reAlinear.FindStringSubmatch(p.Dentro); m != nil {
		p.Alinear = "center"
		if strings.EqualFold(m[1], "right") {
			p.Alinear = "right"
		}
		p.Dentro = strings.TrimSpace(m[2])
	}
	if m := reNegrita.FindStringSubmatch(p.Dentro); m != nil {
		p.Negrita, p.Dentro = true, strings.TrimSpace(m[1])
	}
	if m := reCursiva.FindStringSubmatch(p.Dentro); m != nil {
		p.Cursiva, p.Dentro = true, strings.TrimSpace(m[1])
	}
	return p
}
