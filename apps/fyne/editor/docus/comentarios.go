// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El panel «Comentarios» es el de `editor` (comentarios.go), que Calcus también monta;
// aquí queda lo propio del folio: el ancla `<mark data-comment-id>` y las menciones.

// Los nombres de antes, que el panel de revisión y otros siguen usando.
var (
	anchoCuerpoPanel = editor.AnchoCuerpoPanel
	conArriba        = editor.ConArriba
	lineaBase        = editor.LineaBase
)

type (
	filaAcciones      = editor.FilaAcciones
	tarjetaComentario = editor.TarjetaComentario
	comentarios       = editor.Comentarios
)

func (a *App) panelComentarios(ab *abierto) fyne.CanvasObject {
	if ab.comentarios == nil {
		ab.comentarios = editor.NuevosComentarios(editor.FuenteComentarios{
			T: a.t, Loc: a.loc, Svc: a.Svc, ID: ab.id, Lanzar: a.lanzar, Plazo: a.Plazo, Avisos: a.avisos,
			SinRed: func() bool { return ab.ctx.SinRed },
			Menciones: func(area *ui.AreaTexto) (fyne.CanvasObject, func(string)) {
				m := nuevasMenciones(a, area)
				return m.caja, m.cambio
			},
			Resaltar:    ab.folio.ResaltarAncla,
			QuitarAncla: ab.folio.QuitarAncla,
		})
	}
	return ab.comentarios.Objeto()
}

// ---- el ancla en el folio ----

// Anclar envuelve la selección en el `<mark data-comment-id>` del comentario, bloque a
// bloque, y devuelve lo citado con los blancos juntos (`range.toString()` normalizado),
// o "" si no hay selección de texto.
func (f *Folio) Anclar(id string) string {
	a, b := f.orden()
	cita := strings.Join(strings.Fields(strings.ReplaceAll(f.textoSeleccionado(), " ", " ")), " ")
	if !f.haySeleccion() || cita == "" || !f.editable() {
		return ""
	}
	f.porBloques(a, b, func(bl rico.Bloque, e *rico.Edicion) bool {
		i := rico.Indice(f.bloques, bl.Lugar)
		desde, hasta := 0, e.Largo()
		if i == a.b {
			desde = a.p
		}
		if i == b.b {
			hasta = b.p
		}
		if desde >= hasta {
			return false
		}
		e.Anclar(desde, hasta, id)
		return true
	})
	f.reestructurado()
	return cita
}

// QuitarAncla quita el resaltado de un comentario del folio (al resolverlo, al borrarlo o
// si no se pudo crear), como `removeCommentMark`.
func (f *Folio) QuitarAncla(id string) {
	if id == "" || !f.editable() {
		return
	}
	antes := f.doc.HTML()
	if f.doc.QuitarAncla(id) {
		f.hist.atras = append(f.hist.atras, paso{antes, f.cur, f.anc})
		f.reestructurado()
	}
}

// ResaltarAncla pinta lo anclado a un comentario con el azul de la web (`#bfdbfe`, el
// `::highlight(sytue-comment-anchor)` del panel) y lo lleva a la vista; "" lo quita.
func (f *Folio) ResaltarAncla(id string) {
	f.ancla = nil
	for i, b := range f.bloques {
		if desde, hasta, ok := b.Ancla(id); ok && id != "" {
			f.ancla = append(f.ancla, coincidencia{i, desde, hasta})
		}
	}
	f.pag.Refresh()
}

// comentarAnclado es «Nuevo comentario» con el cuerpo ya escrito: ancla la selección (si
// la hay) y crea el hilo con el ancla `{ mark, text }`; si el servicio falla, el
// resaltado se quita, que no quede huérfano. Es `nuevoComentario` de Colaboracion.tsx.
func (a *App) comentarAnclado(ab *abierto, cuerpo string) {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	marca := hex.EncodeToString(b)
	cita := ab.folio.Anclar(marca)
	a.lanzar(func() {
		c, cancelar := context.WithTimeout(context.Background(), a.Plazo)
		defer cancelar()
		var err error
		if cita != "" {
			_, err = a.Svc.CreateAnchoredComment(c, ab.id, cuerpo, marca, cita)
		} else {
			_, err = a.Svc.CreateComment(c, ab.id, "", cuerpo)
		}
		fyne.Do(func() {
			if err != nil {
				ab.folio.QuitarAncla(marca)
				a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.t("comments.addError"), ""))
				return
			}
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, a.t("comments.added"), ""))
			// Se abre el panel para ver el hilo recién creado.
			ab.ctx.Panel = editor.PanelComentarios
			ab.marco.Recomponer()
			if ab.comentarios != nil {
				ab.comentarios.Recargar()
			}
		})
	})
}
