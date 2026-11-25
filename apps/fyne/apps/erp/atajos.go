// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La hoja de atajos de teclado («?», `lib/atajos.ts` de la web)
//
// El ERP tenía ⌘K, «/», «n» y Escape y ninguna pantalla que los enseñara. La hoja es la
// de toda la suite (`ui.MostrarHojaAtajos`); el contenido son estas filas, las mismas y
// en el mismo orden que `GRUPOS_ERP` de la web, cosa que vigila un test.

// filaDeAtajo es una fila de la hoja: sus teclas, ya escritas como las lee quien tiene
// delante el teclado, y la clave de lo que hacen.
type filaDeAtajo struct {
	teclas      func(t func(string) string) []string
	descripcion string
}

// tal es una tecla que se dibuja tal cual («/», «n», «?»).
func tal(k string) func(func(string) string) []string {
	return func(func(string) string) []string { return []string{k} }
}

// gruposDelERP es la hoja: las filas de `GRUPOS_ERP` de la web, en su orden.
var gruposDelERP = []struct {
	titulo string
	filas  []filaDeAtajo
}{
	{"shortcuts.group.move", []filaDeAtajo{
		{func(func(string) string) []string { return []string{ui.AtajoDelSistema("Ctrl+K")} }, "shortcuts.palette"},
		{tal("/"), "shortcuts.search"},
	}},
	{"shortcuts.group.actions", []filaDeAtajo{
		{tal("n"), "shortcuts.new"},
		{func(func(string) string) []string { return []string{ui.AtajoDelSistema("Ctrl+S")} }, "shortcuts.save"},
		{func(t func(string) string) []string { return []string{t("shortcuts.key.esc")} }, "shortcuts.close"},
		{tal("?"), "shortcuts.help"},
	}},
}

// gruposDeAtajos son los grupos de la hoja en el idioma de la pantalla.
func (p *Pantalla) gruposDeAtajos() []ui.GrupoDeAtajos {
	t := func(clave string) string { return p.t(clave) }
	out := make([]ui.GrupoDeAtajos, 0, len(gruposDelERP))
	for _, g := range gruposDelERP {
		grupo := ui.GrupoDeAtajos{Titulo: p.t(g.titulo)}
		for _, f := range g.filas {
			grupo.Atajos = append(grupo.Atajos, ui.AtajoDeTeclado{Teclas: f.teclas(t), Descripcion: p.t(f.descripcion)})
		}
		out = append(out, grupo)
	}
	return out
}

// esGuardar dice si un atajo es Ctrl+S (⌘S en un Mac): el `guardar` de `GRUPOS_ERP`.
func esGuardar(s fyne.Shortcut) bool {
	c, ok := s.(*desktop.CustomShortcut)
	return ok && c.KeyName == fyne.KeyS && c.Modifier == fyne.KeyModifierShortcutDefault
}

// atajoGuardar es Ctrl+S en un campo de la ficha: la guarda, como «Guardar». Quien solo
// lee o mira la papelera no tiene nada que guardar, y el atajo se queda sin hacer nada
// (no sigue al editor, que no sabe qué es).
func (f *Ficha) atajoGuardar(s fyne.Shortcut) bool {
	if !esGuardar(s) {
		return false
	}
	if f.AlGuardar != nil && !f.SoloLectura && !f.EnPapelera && !f.Guardando {
		f.AlGuardar()
	}
	return true
}

// verAtajos abre la hoja de «?». Con una capa ya delante (un diálogo, la propia hoja) la
// tecla es de esa capa y no se apila otra, como `esAtajoDeApp` en la web; con la paleta
// abierta su campo tiene el foco y la «?» se escribe en él.
func (p *Pantalla) verAtajos() {
	c := p.lienzo
	if c == nil || p.paleta.Abierta() || c.Overlays().Top() != nil {
		return
	}
	ui.MostrarHojaAtajos(c, p.t("shortcuts.title"), p.t("ui.close"), p.gruposDeAtajos())
}
