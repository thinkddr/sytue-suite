// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package voz

import "strings"

// leer es `say`: el texto por la entrada (una línea de órdenes tiene tope, un documento
// no) y la voz del idioma si la hay.
func leer(texto, idioma string, alAcabar func()) (func(), error) {
	var args []string
	if lista, err := orden("say", "-v", "?").Output(); err == nil {
		if v := vozDe(string(lista), idioma); v != "" {
			args = append(args, "-v", v)
		}
	}
	o := orden("say", append(args, "-f", "-")...)
	o.Stdin = strings.NewReader(texto)
	return leerCon(o, alAcabar, nil)
}
