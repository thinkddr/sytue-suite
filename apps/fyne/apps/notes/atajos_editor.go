// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LA HOJA DE ATAJOS DEL EDITOR (Ctrl/⌘ + /): la `ui.MostrarHojaAtajos` de la suite con lo
// que el cuerpo de una nota entiende, en tres apartados —formato de la selección, estilo
// del texto y lo demás—, incluidos los atajos de Markdown que se escriben («# », «- », «**x**»…). Es
// de escritorio: en un teléfono no hay teclado físico al que enseñárselos, y el botón no
// sale en la barra.

// gruposAtajosEditor son los apartados de la hoja, ya en el idioma de quien lee.
func (e *EditorNota) gruposAtajosEditor() []ui.GrupoDeAtajos {
	t := func(k string) string { return e.tr.T(k, nil) }
	fila := func(desc string, teclas ...string) ui.AtajoDeTeclado {
		return ui.AtajoDeTeclado{Teclas: teclas, Descripcion: desc}
	}
	return []ui.GrupoDeAtajos{
		{Titulo: t("editor.selectionTools"), Atajos: []ui.AtajoDeTeclado{
			fila(t("editor.bold"), ui.AtajoDelSistema("Ctrl+B"), "**x**"),
			fila(t("editor.italic"), ui.AtajoDelSistema("Ctrl+I"), "*x*", "_x_"),
			fila(t("editor.underline"), ui.AtajoDelSistema("Ctrl+U")),
			fila(t("editor.strike"), "~~x~~"),
			fila(t("editor.inlineCode"), "`x`"),
			fila(t("editor.linkTool"), ui.AtajoDelSistema("Ctrl+K"), "[["),
		}},
		{Titulo: t("editor.block"), Atajos: []ui.AtajoDeTeclado{
			fila(t("editor.slashMenu"), "/"),
			fila(t("editor.paragraph"), ui.AtajoDelSistema("Ctrl+Alt+0")),
			fila(t("editor.heading1"), ui.AtajoDelSistema("Ctrl+Alt+1"), "# "),
			fila(t("editor.heading2"), ui.AtajoDelSistema("Ctrl+Alt+2"), "## "),
			fila(t("editor.heading3"), ui.AtajoDelSistema("Ctrl+Alt+3"), "### "),
			fila(t("editor.bulletList"), "- ", "* "),
			fila(t("editor.numberedList"), "1. "),
			fila(t("editor.taskList"), "[ ] "),
			fila(t("editor.quote"), "> "),
			fila(t("editor.codeBlock"), "``` "),
			fila(t("editor.divider"), "---"),
			fila(t("editor.indent"), "Tab"),
			fila(t("editor.outdent"), "Shift+Tab"),
		}},
		{Titulo: t("common.more"), Atajos: []ui.AtajoDeTeclado{
			fila(t("editor.undo"), ui.AtajoDelSistema("Ctrl+Z")),
			fila(t("editor.redo"), ui.AtajoDelSistema("Ctrl+Y"), ui.AtajoDelSistema("Ctrl+Shift+Z")),
			fila(t("editor.find"), ui.AtajoDelSistema("Ctrl+F")),
			fila(t("editor.replace"), ui.AtajoDelSistema("Ctrl+H")),
			fila(t("editor.shortcuts"), ui.AtajoDelSistema("Ctrl+/")),
		}},
	}
}

// verAtajos abre la hoja sobre la ventana del editor.
func (e *EditorNota) verAtajos() {
	c := fyne.CurrentApp().Driver().CanvasForObject(e.zona)
	if c == nil {
		return
	}
	// El diálogo devuelve el foco a quien lo tenía, que es el cuerpo: se sigue
	// escribiendo donde se estaba.
	ui.MostrarHojaAtajos(c, e.tr.T("editor.shortcuts", nil), e.tr.T("ui.close", nil), e.gruposAtajosEditor())
}
