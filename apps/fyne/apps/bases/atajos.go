// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La hoja de atajos de teclado («?», `lib/atajos.ts` de la web)
//
// La rejilla atendía flechas, Inicio, Fin, RePág, AvPág, Intro, F2, Escape, Espacio,
// Supr, Ctrl+C, Ctrl+V y Alt+flechas, y lo único que lo contaba era una frase bajo la
// tabla con tres de ellas. La hoja es la de toda la suite (`ui.MostrarHojaAtajos`); el
// contenido son estas filas, las mismas y en el mismo orden que `GRUPOS_BASES` de la
// web, cosa que vigila un test.

// filaDeAtajo es una fila de la hoja: sus teclas, ya escritas como las lee quien tiene
// delante el teclado, y la clave de lo que hacen.
type filaDeAtajo struct {
	teclas      func(t func(string) string) []string
	descripcion string
}

// tal son teclas que se dibujan tal cual (flechas, «F2», «?»).
func tal(k ...string) func(func(string) string) []string {
	return func(func(string) string) []string { return k }
}

// nombradas son teclas con nombre, que van por el catálogo («Intro», «Supr»).
func nombradas(claves ...string) func(func(string) string) []string {
	return func(t func(string) string) []string {
		out := make([]string, len(claves))
		for i, c := range claves {
			out[i] = t(c)
		}
		return out
	}
}

// gruposDeBases es la hoja: las filas de `GRUPOS_BASES` de la web, en su orden.
var gruposDeBases = []struct {
	titulo string
	filas  []filaDeAtajo
}{
	{"shortcuts.group.move", []filaDeAtajo{
		{tal("↑", "↓", "←", "→"), "shortcuts.move"},
		{nombradas("shortcuts.key.home", "shortcuts.key.end"), "shortcuts.edges"},
		{nombradas("shortcuts.key.pageUp", "shortcuts.key.pageDown"), "shortcuts.page"},
	}},
	{"shortcuts.group.edit", []filaDeAtajo{
		{func(t func(string) string) []string { return []string{t("shortcuts.key.enter"), "F2"} }, "shortcuts.edit"},
		{nombradas("shortcuts.key.esc"), "shortcuts.cancel"},
		{nombradas("shortcuts.key.space"), "shortcuts.check"},
		{nombradas("shortcuts.key.delete"), "shortcuts.clear"},
		{func(func(string) string) []string { return []string{ui.AtajoDelSistema("Ctrl+C")} }, "shortcuts.copy"},
		{func(func(string) string) []string { return []string{ui.AtajoDelSistema("Ctrl+V")} }, "shortcuts.paste"},
		{tal("Alt+←", "Alt+→"), "shortcuts.moveField"},
		{tal("?"), "shortcuts.help"},
	}},
}

// gruposDeAtajos son los grupos de la hoja en el idioma de la pantalla.
func (p *Pantalla) gruposDeAtajos() []ui.GrupoDeAtajos {
	t := func(clave string) string { return p.t(clave) }
	out := make([]ui.GrupoDeAtajos, 0, len(gruposDeBases))
	for _, g := range gruposDeBases {
		grupo := ui.GrupoDeAtajos{Titulo: p.t(g.titulo)}
		for _, f := range g.filas {
			grupo.Atajos = append(grupo.Atajos, ui.AtajoDeTeclado{Teclas: f.teclas(t), Descripcion: p.t(f.descripcion)})
		}
		out = append(out, grupo)
	}
	return out
}

// verAtajos abre la hoja de «?». Con una capa ya delante (un diálogo, la propia hoja) la
// tecla es de esa capa y no se apila otra, como `esAtajoDeApp` en la web.
func (p *Pantalla) verAtajos() {
	if p.ventana == nil {
		return
	}
	c := p.ventana.Canvas()
	if c.Overlays().Top() != nil {
		return
	}
	ui.MostrarHojaAtajos(c, p.t("shortcuts.title"), p.t("ui.close"), p.gruposDeAtajos())
}

// alTeclear es «?» sin nada enfocado (`SetOnTypedRune` solo llega entonces): con la
// rejilla enfocada la atiende ella, que en una celda de texto la escribe.
func (p *Pantalla) alTeclear(r rune) {
	if r == '?' && p.abierta != nil {
		p.verAtajos()
	}
}

// engancharTeclas pone «?» en el lienzo de la ventana.
func (p *Pantalla) engancharTeclas(c fyne.Canvas) {
	c.SetOnTypedRune(p.alTeclear)
}
