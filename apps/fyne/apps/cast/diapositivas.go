// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Porta apps/frontend/apps/cast/src/components/Diapositivas.tsx (pieza de la página
// del vídeo, cuyo marcador de paridad lleva ver.go).

package cast

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Presentar con Cast» (Sytue Diapos), lo que ve quien mira: en directo, en qué
// diapositiva va quien presenta; en la grabación, un capítulo por cada cambio de
// diapositiva para saltar a ella. Solo viaja la posición —número y título—: el
// contenido de la presentación y las notas de quien presenta no salen de Diapos.

// altoMaxCapitulos es el `max-h-56` de la lista de capítulos: más larga, se desplaza.
const altoMaxCapitulos = 14 * tema.Rem

// BloqueDiapositivas es lo que va entre el reproductor y el título. nil cuando no hay
// nada que enseñar: sin presentación, o una grabación sin marcas.
func BloqueDiapositivas(v Vista, p *api.CastPresentacion, enDirecto bool, alCapitulo func(ms int64)) fyne.CanvasObject {
	if p == nil {
		return nil
	}
	if enDirecto {
		return diapositivaEnCurso(v, *p)
	}
	if len(p.Marks) == 0 {
		return nil
	}
	return capitulos(v, p.Marks, alCapitulo)
}

// diapositivaEnCurso es el `<p aria-live="polite">` de la web: «Diapositiva 3 de 12» en
// semi-negrita y « · Título» apagado, en un recuadro hundido.
func diapositivaEnCurso(v Vista, p api.CastPresentacion) fyne.CanvasObject {
	actual := v.Traduce("watch.slides.now", i18n.Vars{"n": p.Index + 1, "total": p.Total})
	hijos := []fyne.CanvasObject{ui.NuevoRotulo(actual, tema.TextoSM, ui.SemiNegrita, "text")}
	if p.Title != "" {
		titulo := ui.NuevoRotulo(" · "+p.Title, tema.TextoSM, ui.Normal, "text-muted")
		titulo.Truncar = true
		hijos = append(hijos, titulo)
	}
	fila := container.New(ui.FilaAuto{Derecha: len(hijos), Partir: true, Elastico: len(hijos)}, hijos...)
	caja := ui.NuevaCaja(tema.Color("surface-sunken", v.Variante), transparente(), tema.RadioMD)
	return container.New(&recuadroFino{caja: caja, x: tema.Espacio(3), y: tema.Espacio(2)}, append(caja.Objetos(), fila)...)
}

// capitulos es la sección «Diapositivas» de la grabación: cada marca con su hora, y
// pulsarla lleva el vídeo a ese instante.
func capitulos(v Vista, marcas []api.CastMarca, alCapitulo func(ms int64)) fyne.CanvasObject {
	filas := make([]ui.Bloque, 0, len(marcas))
	alto := float32(0)
	for _, m := range marcas {
		hora := Duracion(time.Duration(m.AtMS) * time.Millisecond)
		texto := v.Traduce("watch.slides.chapter", i18n.Vars{"n": m.Index + 1})
		if m.Title != "" {
			texto += " · " + m.Title
		}
		titulo := ui.NuevoRotulo(texto, tema.TextoSM, ui.Normal, "text")
		titulo.Truncar = true
		contenido := container.New(capituloLayout{},
			ui.NuevoRotulo(hora, tema.TextoSM, ui.Normal, "text-muted"), titulo)
		ms := m.AtMS
		f := ui.NuevaFilaPulsable(hora+" · "+texto, contenido, func() {
			if alCapitulo != nil {
				alCapitulo(ms)
			}
		})
		filas = append(filas, ui.Bloque{Obj: f})
		alto += ui.MedidaFilaPulsable(f).Height
	}
	var lista fyne.CanvasObject = ui.PilaBloques(filas...)
	if alto > altoMaxCapitulos {
		s := container.NewVScroll(lista)
		s.Content = container.New(&alAnchoDe{s: s}, lista)
		lista = container.New(&altoFijo{alto: altoMaxCapitulos}, s)
	}
	cabecera := ui.NuevoParrafo(v.Traduce("watch.slides.title"), tema.TextoSM, ui.SemiNegrita, "text", v.Variante)
	return ui.PilaBloques(
		ui.Bloque{Obj: cabecera, Margen: tema.Espacio(2)},
		ui.Bloque{Obj: lista},
	)
}

// capituloLayout: la hora en su columna de `w-12` y el título detrás con `gap-3`, que
// se lleva el resto y se recorta (`min-w-0 truncate`).
type capituloLayout struct{}

const anchoHoraCapitulo = 3 * tema.Rem

func (capituloLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(anchoHoraCapitulo+tema.Espacio(3), max(objs[0].MinSize().Height, objs[1].MinSize().Height))
}

func (capituloLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := anchoHoraCapitulo + tema.Espacio(3)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(anchoHoraCapitulo, t.Height))
	objs[1].Move(fyne.NewPos(x, 0))
	objs[1].Resize(fyne.NewSize(max(t.Width-x, 0), t.Height))
}
