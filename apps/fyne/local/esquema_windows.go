// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package local

import "golang.org/x/sys/windows/registry"

// hayAppPara mira si el esquema está registrado: `rundll32 url.dll` no falla cuando no
// lo está (enseña su propio diálogo), así que probar a abrir no lo dice.
func hayAppPara(esquema string) (hay, sabido bool) {
	for _, raiz := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(raiz, `Software\Classes\`+esquema+`\shell\open\command`, registry.QUERY_VALUE)
		if err == nil {
			_ = k.Close()
			return true, true
		}
	}
	return false, true
}
