// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: NoteInfo.tsx, la «Información» del «…» de una nota: palabras,
// caracteres, minutos de lectura, creada, editada, cuaderno y cuántas notas apuntan aquí.
//
// Las cuentas son las de `htmltext.Stats` sobre `htmltext.Flat`, el espejo de
// `estadisticas(textoDeNota(…))` de la web, con los mismos vectores
// (apps/nucleo/htmltext/testdata/estadisticas.json): los mismos números en las dos.

// sinDato es lo que se pinta cuando no hay con qué contar: una nota cifrada sin su clave
// diría «0 palabras», y eso mentiría.
const sinDato = "—"

// filasInformacion son las filas del diálogo, rótulo y valor, en el orden de la web.
// `enlazan` es lo que va en la última: «…» mientras se pide.
func (p *Pantalla) filasInformacion(n Nota, enlazan string) [][2]string {
	t := p.tr.T
	palabras, caracteres, lectura := sinDato, sinDato, sinDato
	// En un cuaderno cifrado se cuenta el claro que ya está en memoria (deNota); cerrado
	// no hay texto, y no se cuenta.
	if !p.bloqueado && (!n.Cifrada || p.clave != nil) {
		e := htmltext.Stats(htmltext.Flat(n.Contenido))
		palabras = i18n.FormatNumber(p.loc, float64(e.Palabras))
		caracteres = i18n.FormatNumber(p.loc, float64(e.Caracteres))
		lectura = t("noteInfo.readingValue", i18n.Vars{"count": e.Minutos})
	}
	creada, editada := sinDato, sinDato
	if !n.Creada.IsZero() {
		creada = i18n.FormatDateTime(p.loc, n.Creada.Local())
	}
	if !n.Tocada.IsZero() {
		editada = i18n.FormatDateTime(p.loc, n.Tocada.Local())
	}
	filas := [][2]string{
		{t("noteInfo.words", nil), palabras},
		{t("noteInfo.characters", nil), caracteres},
		{t("noteInfo.reading", nil), lectura},
		{t("noteInfo.created", nil), creada},
		{t("noteInfo.edited", nil), editada},
		{t("noteInfo.notebook", nil), p.nombreCuaderno(n.Cuaderno)},
	}
	// Una tarea dice en qué está, como NoteInfo.tsx.
	if n.Tipo == "todo" {
		estado := "noteInfo.task.open"
		switch {
		case n.Cancelada:
			estado = "noteInfo.task.cancelled"
		case n.Marcada:
			estado = "noteInfo.task.done"
		}
		filas = append(filas, [2]string{t("noteInfo.task", nil), t(estado, nil)})
	}
	return append(filas,
		// Lo que sale de la nota hacia otras notas y ficheros, junto a lo que entra.
		[2]string{t("noteInfo.links", nil), i18n.FormatNumber(p.loc, float64(len(n.Enlaces)))},
		[2]string{t("noteInfo.backlinks", nil), enlazan})
}

// nombreCuaderno es el título del cuaderno de la nota, que casi siempre es el abierto.
func (p *Pantalla) nombreCuaderno(id string) string {
	for _, c := range p.cuadernos {
		if c.ID == id && c.Titulo != "" {
			return c.Titulo
		}
	}
	if p.activo.ID == id && p.activo.Titulo != "" {
		return p.activo.Titulo
	}
	return sinDato
}

// dialogoInformacion abre «Información». Las notas que apuntan aquí se piden AL ABRIR,
// como en dialogoRetroenlaces: pedirlas al pintar el lienzo serían cuarenta peticiones.
func (p *Pantalla) dialogoInformacion(n Nota) {
	t := p.tr.T
	cuerpo := container.NewStack()
	d := ui.NuevoDialogo(t("note.info", nil), cuerpo).Con(ui.DialogoSM, nil)
	pintar := func(enlazan string) {
		cuerpo.Objects = []fyne.CanvasObject{p.cuerpoInformacion(n, enlazan)}
		cuerpo.Refresh()
		d.Refresh()
	}
	pintar("…")
	p.mostrar(d)
	if p.svc == nil || !p.hayRed() {
		pintar(sinDato)
		return
	}
	p.iniciar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		notas, err := p.svc.LinkedNotes(ctx, "note", n.ID, limiteRetroenlaces)
		p.enHilo(func() {
			if err != nil {
				pintar(sinDato)
				return
			}
			pintar(t("notebook.notes", i18n.Vars{"count": len(notas)}))
		})
	})
}

// cuerpoInformacion es el de NoteInfo.tsx: de qué nota (`sytue-meta mb-3`) y la lista
// `grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-base`, rótulo apagado a la izquierda
// y valor a la derecha.
func (p *Pantalla) cuerpoInformacion(n Nota, enlazan string) fyne.CanvasObject {
	v, t := p.variante(), p.tr.T
	titulo := n.Titulo
	if titulo == "" {
		titulo = t("note.new", nil)
	}
	datos := p.filasInformacion(n, enlazan)
	filas := make([]fyne.CanvasObject, 0, len(datos))
	for _, f := range datos {
		filas = append(filas, container.New(&filaDato{hueco: 4 * 0.25 * tema.Rem},
			lineaTok(f[0], tema.TextoBase, ui.Normal, "text-muted", v),
			lineaTok(f[1], tema.TextoBase, ui.Normal, "text", v)))
	}
	return container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem},
		lineaTok(titulo, tema.TextoXS, ui.Normal, "text-subtle", v),
		container.New(ui.Columna{Hueco: 2 * 0.25 * tema.Rem}, filas...))
}

// filaDato pone el rótulo a su ancho a la izquierda y el valor pegado a la derecha; si
// no cabe, el valor se recorta (lineaTok) y el rótulo no.
type filaDato struct{ hueco float32 }

func (f *filaDato) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+f.hueco+b.Width, max(a.Height, b.Height))
}

func (f *filaDato) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(a.Width, s.Height))
	ancho := min(b.Width, max(0, s.Width-a.Width-f.hueco))
	objs[1].Move(fyne.NewPos(s.Width-ancho, 0))
	objs[1].Resize(fyne.NewSize(ancho, s.Height))
}
