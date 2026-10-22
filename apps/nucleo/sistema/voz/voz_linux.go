// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package voz

import "strings"

// leer es `spd-say`: en modo tubería (`-e`) el texto va por la entrada y no por la línea
// de órdenes, que tiene tope; `-w` espera a que acabe, que es lo que dice cuándo se ha
// terminado de leer. Llega como UNA línea (ver [Leer]): un solo mensaje.
//
// Callar mata a `spd-say` y le pide a Speech Dispatcher que pare lo que está diciendo
// (`-S`): el mensaje ya es suyo y seguiría sonando.
func leer(texto, idioma string, alAcabar func()) (func(), error) {
	args := []string{"-e", "-w"}
	if l := lengua(idioma); l != "" {
		args = append(args, "-l", l)
	}
	o := orden("spd-say", args...)
	o.Stdin = strings.NewReader(texto + "\n")
	return leerCon(o, alAcabar, func() { _ = orden("spd-say", "-S").Run() })
}
