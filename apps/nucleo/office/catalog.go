// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package office comparte el catálogo original de Sytue entre web y Fyne.
// catalog.json es la fuente; sync-office-catalog.sh publica su copia de build web.
package office

import (
	_ "embed"
	"encoding/json"
)

//go:embed catalog.json
var data []byte

// Export es una descarga. El servidor vuelve a autorizar cada petición.
type Export struct {
	ID     string            `json:"id"`
	Online bool              `json:"online"`
	Labels map[string]string `json:"labels"`
	// Option es un interruptor de la descarga (hoy solo «PDF accesible»): una entrada con
	// tic justo debajo, que no descarga nada. nil: la descarga no tiene opciones.
	Option *Option `json:"option,omitempty"`
}

// Option es un interruptor de una descarga: su id es el campo de la petición de exportación.
type Option struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// Available impide ofrecer descargas offline o mientras otra está en curso.
func (e Export) Available(offline, busy bool) bool {
	return !busy && (!e.Online || !offline)
}

// Exports devuelve una copia independiente para cada pantalla.
func Exports() []Export {
	var catalog struct {
		Version int      `json:"version"`
		Exports []Export `json:"exports"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil || catalog.Version != 1 {
		panic("invalid Office command catalog")
	}
	return catalog.Exports
}

// Formats es el conjunto y orden de formatos admitidos por los clientes.
func Formats() []string {
	commands := Exports()
	formats := make([]string, 0, len(commands))
	for _, command := range commands {
		formats = append(formats, command.ID)
	}
	return formats
}
