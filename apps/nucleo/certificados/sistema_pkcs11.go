// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo && (linux || darwin || windows) && !android && !ios

package certificados

import (
	"log/slog"
	"sort"
)

func init() { fuentes = append(fuentes, tarjetas) }

// tarjetas abre cada módulo PKCS#11 instalado: el lector no se va a ir.
func tarjetas() []Almacen {
	mods := ModulosPKCS11()
	rutas := make([]string, 0, len(mods))
	for r := range mods {
		rutas = append(rutas, r)
	}
	sort.Strings(rutas)
	var out []Almacen
	for _, r := range rutas {
		p, err := AbrirPKCS11(r, mods[r])
		if err != nil {
			slog.Info("certificados: módulo PKCS#11 que no carga", "ruta", r, "error", err)
			continue
		}
		out = append(out, p)
	}
	return out
}
