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
)

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
