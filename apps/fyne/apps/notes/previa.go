// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LA VISTA PREVIA DE UN ENLACE A OTRA NOTA (VistaPreviaEnlace.tsx): al pasar el ratón por
// él, una ficha con su título y su principio. Solo con lo que la suite ya resuelve —las
// notas del cuaderno abierto, y si no, la lista de notas del cuaderno del enlace—; nada
// se pide a terceros, así que un enlace a la web no tiene vista previa. Una nota cifrada
// no enseña su contenido.
//
// La ficha es un `flotante` hermano del texto, no una capa: una capa se quedaría con el
// puntero y el enlace dejaría de poderse pulsar.

// VistaPrevia es lo que enseña la ficha. Sin Encontrada, «no puedes ver esta nota».
type VistaPrevia struct {
	Titulo, Texto       string
	Cifrada, Encontrada bool
}

// clavePrevia es la dueña de la ficha en el portal: una sola para todas las prosas, porque
// solo hay un puntero y la pantalla la quita al repintar.
const clavePrevia = "previa"

// retardoPrevia es el de la web: pasar por encima camino de otra cosa no la abre.
const retardoPrevia = 350 * time.Millisecond

// anchoPrevia es el `w-72` de la ficha y rellenoPrevia su `p-3`.
var (
	anchoPrevia   = tema.Espacio(72)
	rellenoPrevia = tema.Espacio(3)
)

var _ desktop.Hoverable = (*Prosa)(nil)

// MouseIn no hace nada: la ficha la abre MouseMoved cuando llega a un enlace.
func (p *Prosa) MouseIn(e *desktop.MouseEvent) { p.MouseMoved(e) }

// MouseMoved abre la ficha del enlace a otra nota que hay bajo el puntero, tras el
// retardo, y la cierra al salir de él.
func (p *Prosa) MouseMoved(e *desktop.MouseEvent) {
	if p.Previa == nil {
		return
	}
	href := p.enlaceEn(e.Position)
	if href == p.hrefPrevia {
		return
	}
	p.cerrarPrevia()
	p.hrefPrevia = href
	if href == "" {
		return
	}
	y := e.Position.Y
	for _, l := range p.maq.Lineas {
		if e.Position.Y >= l.Y && e.Position.Y <= l.Y+l.Alto {
			y = l.Y + l.Alto
		}
	}
	x := e.Position.X
	p.relojPrevia = time.AfterFunc(retardoPrevia, func() {
		fyne.Do(func() { p.abrirPrevia(href, x, y+6) })
	})
}

// MouseOut cierra la ficha.
func (p *Prosa) MouseOut() {
	p.cerrarPrevia()
	p.hrefPrevia = ""
}

func (p *Prosa) cerrarPrevia() {
	if p.relojPrevia != nil {
		p.relojPrevia.Stop()
		p.relojPrevia = nil
	}
	if p.ficha != nil {
		p.ficha = nil
		p.portal.poner(clavePrevia)
		canvas.Refresh(p)
	}
}

// abrirPrevia pide la vista previa y la enseña; si aún no la tiene, la enseña cuando
// llegue (siempre que el puntero siga en el mismo enlace).
func (p *Prosa) abrirPrevia(href string, x, y float32) {
	if p.hrefPrevia != href || p.Previa == nil {
		return
	}
	vp, listo := p.Previa(href, func() { p.abrirPrevia(href, x, y) })
	if !listo {
		return
	}
	p.ficha = fichaPrevia(p.tr, vp)
	if p.portal != nil {
		off := p.portal.desde(p)
		colocarFlotante(p.ficha, x+off.X, y+off.Y, p.portal.c.Size().Width)
		p.portal.poner(clavePrevia, p.ficha)
		return
	}
	colocarFlotante(p.ficha, x, y, max(p.Size().Width, anchoPrevia))
	canvas.Refresh(p)
}

// fichaPrevia arma la ficha: el título en `text-sm font-medium` y el principio en
// `sytue-meta line-clamp-3`.
func fichaPrevia(tr *i18n.Translator, vp VistaPrevia) *flotante {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cabe := anchoPrevia - 2*(rellenoPrevia+tema.Borde)
	var hijos []fyne.CanvasObject
	if !vp.Encontrada {
		hijos = append(hijos, ui.NuevoParrafo(tr.T("linkPreview.unavailable", nil), tema.TextoSM, ui.Normal, "text-muted", v).A(cabe))
	} else {
		titulo := vp.Titulo
		if titulo == "" {
			titulo = tr.T("note.new", nil)
		}
		texto := vp.Texto
		switch {
		case vp.Cifrada:
			texto = tr.T("linkPreview.encrypted", nil)
		case texto == "":
			texto = tr.T("linkPreview.empty", nil)
		}
		hijos = append(hijos,
			ui.NuevoParrafo(titulo, tema.TextoSM, ui.Medio, "text", v).A(cabe).Recortado(1),
			ui.NuevoParrafo(texto, tema.TextoXS, ui.Normal, "text-subtle", v).A(cabe).Recortado(3))
	}
	return nuevoFlotante(container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...), rellenoPrevia)
}

// principioDoc es el texto plano del principio de una nota, para la ficha
// (`principioDe` de la web): sus bloques seguidos, con los blancos plegados.
func principioDoc(d *Documento, n int) string {
	if d == nil {
		return ""
	}
	var partes []string
	for _, b := range d.Bloques {
		partes = append(partes, b.Texto())
	}
	r := []rune(strings.Join(strings.Fields(strings.Join(partes, " ")), " "))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimRight(string(r[:n]), " ") + "…"
}

// vistaDe es la ficha de una nota ya cargada.
func vistaDe(n Nota) VistaPrevia {
	if n.Cifrada && n.Contenido == "" {
		return VistaPrevia{Titulo: n.Titulo, Cifrada: true, Encontrada: true}
	}
	return VistaPrevia{Titulo: n.Titulo, Texto: principioDoc(n.Doc, 180), Encontrada: true}
}

// previaDe es `previa` de NotesApp.tsx: la nota del cuaderno abierto, al instante; la de
// otro, de su lista de notas, que se pide una vez por cuaderno y se guarda. Un enlace que
// no es a una nota no tiene ficha (no vuelve nunca «lista»).
func (p *Pantalla) previaDe(href string, listo func()) (VistaPrevia, bool) {
	cuaderno, id, _, ok := destinoInterno(href, p.AppURL)
	if !ok {
		return VistaPrevia{}, false
	}
	lista := p.notas
	if cuaderno != p.activo.ID {
		guardadas, hay := p.notasDeOtros[cuaderno]
		switch {
		case !hay && (p.svc == nil || !p.hayRed()):
			return VistaPrevia{}, true
		case !hay:
			if p.notasDeOtros == nil {
				p.notasDeOtros = map[string]*[]Nota{}
			}
			p.notasDeOtros[cuaderno] = nil // pedida: no se vuelve a pedir mientras llega
			go func() {
				ctx, cancelar := p.ctx()
				defer cancelar()
				l, err := p.svc.ListNotes(ctx, cuaderno)
				ns := DeNotas(l)
				enHilo(func() {
					if err != nil {
						delete(p.notasDeOtros, cuaderno) // que el siguiente paso lo reintente
					} else {
						p.notasDeOtros[cuaderno] = &ns
					}
					listo()
				})
			}()
			return VistaPrevia{}, false
		case guardadas == nil:
			return VistaPrevia{}, false // en camino
		}
		lista = *guardadas
	}
	for _, n := range lista {
		if n.ID == id {
			return vistaDe(n), true
		}
	}
	return VistaPrevia{}, true
}

// colorResalte es el fondo de lo buscado en un resultado: el del `<mark>` de la nota.
func colorResalte(v tema.Variante) color.NRGBA {
	if v == tema.Oscuro {
		return resaltadoOscuro
	}
	return resaltadoClaro
}
