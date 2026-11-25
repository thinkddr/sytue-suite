// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// # Ctrl, Alt y Meta con un teclado físico en el móvil
//
// En el escritorio, una tecla pulsada con Ctrl, Alt o Super llega como atajo
// (`desktop.CustomShortcut`) y nunca como la tecla sola. En el móvil, con un teclado
// físico, el driver de Fyne hasta `v2.8.2-sytue.10` descartaba los modificadores
// (`typeDownCanvas`, y Android ni los leía): Ctrl+↑ y Alt+↑ llegaban como ↑ a secas. Lo
// arregla el parche 0023 del fork (`apps/fyne/upstream/0023-mobile-modifier-shortcuts.patch`).
//
// Hasta que la suite use un fork con ese parche, quien da a ↑ (o a otra tecla que con
// un modificador significa otra cosa) un sentido propio no puede fiarse de ella en el
// móvil: Rooms editaba el último mensaje con Ctrl+↑ y con Alt+↑, que en el escritorio
// cambia de canal.

// VersionForkModificadores es la primera etiqueta del fork de Fyne con el parche 0023.
// Si se etiqueta con otro número, se cambia aquí: TestLaVersionDeFyneSaleDelBinario
// comprueba que la que se lee es la de `go.mod`.
const VersionForkModificadores = "v2.8.2-sytue.11"

// ModificadoresEnElMovil dice si una tecla con Ctrl, Alt o Meta llega distinta de la
// tecla sola: siempre en el escritorio, y en el móvil solo con el fork que los entrega.
func ModificadoresEnElMovil() bool { return !esMovil() || forkConModificadores() }

var forkConModificadores = sync.OnceValue(func() bool {
	return VersionConModificadores(versionFyne())
})

// versionFyne es la versión de Fyne con la que se compiló el binario: la del fork que
// la sustituye (`replace` de `go.mod`). Vacía si no se sabe, como en un `go.work` que
// apunta a un clon local.
func versionFyne() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, m := range info.Deps {
		if m.Path != "fyne.io/fyne/v2" {
			continue
		}
		if m.Replace != nil {
			return m.Replace.Version
		}
		return m.Version
	}
	return ""
}

// VersionConModificadores dice si esa versión del fork ya trae el parche 0023: la
// misma base y un número de etiqueta igual o mayor que [VersionForkModificadores], o
// una base posterior que se etiquete desde el fork (`-sytue.`). Lo que no se sabe
// leer, no: la guarda se queda puesta.
func VersionConModificadores(v string) bool {
	base, n, ok := versionFork(v)
	minBase, minN, _ := versionFork(VersionForkModificadores)
	if !ok {
		return false
	}
	if c := compararBase(base, minBase); c != 0 {
		return c > 0
	}
	return n >= minN
}

// versionFork parte «v2.8.2-sytue.11» en su base «v2.8.2» y el 11.
func versionFork(v string) (base string, n int, ok bool) {
	base, etiqueta, ok := strings.Cut(v, "-sytue.")
	if !ok {
		return "", 0, false
	}
	n, err := strconv.Atoi(etiqueta)
	return base, n, err == nil
}

// compararBase compara dos versiones «vX.Y.Z» número a número. Lo que no se sabe leer
// cuenta como menor.
func compararBase(a, b string) int {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
