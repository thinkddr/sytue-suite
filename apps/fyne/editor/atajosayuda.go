// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// # La hoja «Atajos de teclado» (`editor/atajosAyuda.ts`)
//
// Los atajos solo se descubrían en la pista de cada botón, y uno que no se ve no existe.
// La hoja NO es una lista escrita a mano: sale del registro común (`office.Comandos`),
// agrupada como la cinta (pestaña › grupo) y en el mismo orden que en la web, más los del
// marco nativo, que no son comandos. Un atajo nuevo en el registro aparece aquí y en la
// web sin tocar nada. Se abre con Ctrl/Cmd+/ y desde «Vista › Mostrar», en los tres.

// AtajoAyuda es la combinación que abre (y cierra) la hoja, escrita como en el registro.
const AtajoAyuda = "Ctrl+/"

// FilaAyuda es una fila de la hoja: la combinación, como en el registro, y la clave de
// catálogo de lo que hace. ID es el comando del que sale; los del marco no tienen.
type FilaAyuda struct {
	Keys, LabelKey, ID string
}

// GrupoAyuda es un apartado: un grupo de la cinta dentro de su pestaña.
type GrupoAyuda struct {
	Tab, Group string
	Filas      []FilaAyuda
}

// RotuloDelValor es `ROTULO_DEL_VALOR` de la web: los atajos que despachan un comando con
// valor dicen ese valor, no el rótulo del comando (Ctrl+X no es «Copiar»).
var RotuloDelValor = map[string]string{
	"copy:cut":     "shortcut.cut",
	"paste:values": "shortcut.pasteValues",
	"heading:p":    "style.normal",
	"heading:h1":   "style.h1",
	"heading:h2":   "style.h2",
	"heading:h3":   "style.h3",
}

// AtajosDelMarco son los del marco NATIVO, los que atiende [Contexto.AtajoDelEditor]: aquí
// Ctrl+P imprime (en la web lo hace el navegador) y no hay F6, que es del cromo web. Si uno
// cambia allí, cambia aquí (`TestLaHojaDiceLosAtajosDelMarco`).
var AtajosDelMarco = []FilaAyuda{
	{Keys: "Ctrl+P", LabelKey: "shortcut.print"},
	{Keys: "Ctrl++", LabelKey: "view.zoomIn"},
	{Keys: "Ctrl+-", LabelKey: "view.zoomOut"},
	{Keys: "Ctrl+0", LabelKey: "shortcut.zoomReset"},
	{Keys: AtajoAyuda, LabelKey: "view.shortcuts"},
}

// AtajosDeDocus son los del marco que solo atiende Docus: Ctrl/Cmd+Mayús+C abre el recuento
// ([Contexto.AtajoRecuento]); en Calcus y Diapos no es de nadie. `ATAJOS_DE_DOCUS` de la web.
var AtajosDeDocus = []FilaAyuda{
	{Keys: "Ctrl+Shift+C", LabelKey: "wordcount.title"},
}

// AtajosDelFolio son los que en Fyne solo atiende el folio de Docus con el foco
// (`docus/escribir.go`, `Folio.atajo`): guardar ya y abrir el panel Buscar. En la web son
// del marco y valen en los tres; aquí Calcus y Diapos no los tienen, así que la hoja solo
// los enseña en un documento, delante de los del marco y en el orden de la web.
var AtajosDelFolio = []FilaAyuda{
	{Keys: "Ctrl+S", LabelKey: "shortcut.save"},
	{Keys: "Ctrl+F", LabelKey: "shortcut.find"},
}

// AtajosDelRegistro es `atajosDelRegistro` de la web: los atajos del registro que existen en
// ese tipo de documento, por pestaña en su orden y, dentro, por grupo y comando en el orden
// del registro. Van TODOS, también los que atiende el propio lienzo (deshacer).
func AtajosDelRegistro(kind string) []GrupoAyuda {
	ids := office.IDsEnOrden()
	pestanas := office.PestanasDe(kind)
	grupos := make([]GrupoAyuda, 0, len(pestanas))
	for _, p := range pestanas {
		var orden []string
		filas := map[string][]FilaAyuda{}
		for _, id := range ids {
			def, _ := office.BuscarComando(id)
			if def.Tab != p.ID || len(def.Shortcuts) == 0 || !def.Existe(kind) {
				continue
			}
			if _, ya := filas[def.Group]; !ya {
				orden = append(orden, def.Group)
			}
			for _, a := range def.Shortcuts {
				rotulo := def.LabelKey
				if a.Value != "" {
					if r, ok := RotuloDelValor[id+":"+a.Value]; ok {
						rotulo = r
					}
				}
				filas[def.Group] = append(filas[def.Group], FilaAyuda{Keys: a.Keys, LabelKey: rotulo, ID: id})
			}
		}
		for _, g := range orden {
			grupos = append(grupos, GrupoAyuda{Tab: p.ID, Group: g, Filas: filas[g]})
		}
	}
	return grupos
}

// Las teclas se escriben con [TextoAtajo] (`atajos.go`), el `textoAtajo` de la web.

// GruposDeAtajos es la hoja lista para pintar: el marco, el registro del tipo abierto y al
// final los propios del producto ([Contexto.AtajosPropios]).
func (c *Contexto) GruposDeAtajos() []ui.GrupoDeAtajos {
	return c.gruposDeAtajos(TextoAtajo)
}

func (c *Contexto) gruposDeAtajos(texto func(string, func(string) string) string) []ui.GrupoDeAtajos {
	tr := func(k string) string { return c.Traducir(k) }
	fila := func(f FilaAyuda) ui.AtajoDeTeclado {
		return ui.AtajoDeTeclado{Teclas: []string{texto(f.Keys, tr)}, Descripcion: tr(f.LabelKey)}
	}
	marco := ui.GrupoDeAtajos{Titulo: tr("shortcut.group.editor")}
	delMarco := AtajosDelMarco
	if c.Tipo == Documento {
		delMarco = append(append(slices.Clone(AtajosDelFolio), AtajosDelMarco...), AtajosDeDocus...)
	}
	for _, f := range delMarco {
		marco.Atajos = append(marco.Atajos, fila(f))
	}
	grupos := []ui.GrupoDeAtajos{marco}
	kind := c.Tipo.kindDe()
	rotulos := map[string]string{}
	for _, p := range office.PestanasDe(kind) {
		rotulos[p.ID] = tr(p.LabelKey)
	}
	for _, g := range AtajosDelRegistro(kind) {
		gr := ui.GrupoDeAtajos{Titulo: rotulos[g.Tab] + " › " + tr(g.Group)}
		for _, f := range g.Filas {
			gr.Atajos = append(gr.Atajos, fila(f))
		}
		grupos = append(grupos, gr)
	}
	if c.AtajosPropios != nil {
		grupos = append(grupos, c.AtajosPropios()...)
	}
	return grupos
}

// AlternarAtajos abre la hoja sobre el lienzo o, si ya está abierta, la cierra: Ctrl+/
// otra vez la quita, como en la web. Escape y «Cerrar» la cierran y devuelven el foco.
func (c *Contexto) AlternarAtajos(lienzo fyne.Canvas) {
	if c.hojaAtajos != nil && c.hojaAtajos.Abierto() {
		c.hojaAtajos.Cerrar()
		c.hojaAtajos = nil
		return
	}
	c.hojaAtajos = ui.MostrarHojaAtajos(lienzo, c.Traducir("view.shortcuts"), c.Traducir("ui.close"), c.GruposDeAtajos())
}

// AtajoAyudaPulsado es Ctrl/Cmd+/ para la hoja: dice si era ese atajo y, si lo era y hay
// quien la abra ([Contexto.Atajos], que pone el [Marco]), la abre o la cierra. Sin nadie
// que la abra no lo da por atendido: el lienzo (la rejilla de Calcus suelta, en sus
// pruebas) puede tener su propia hoja.
func (c *Contexto) AtajoAyudaPulsado(s *desktop.CustomShortcut) bool {
	if s == nil || s.KeyName != fyne.KeySlash || s.Modifier != fyne.KeyModifierShortcutDefault {
		return false
	}
	if c == nil || c.Atajos == nil {
		return false
	}
	c.Atajos()
	return true
}
