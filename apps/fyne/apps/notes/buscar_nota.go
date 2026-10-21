// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Buscar en la nota» (Ctrl+F dentro del editor), el de RichText.tsx: una barra bajo la
// de formato con la caja, la cuenta «2 de 5» y anterior, siguiente y cerrar. Intro va a
// la siguiente, Mayús+Intro a la anterior y Escape cierra dejando seleccionada la
// coincidencia en la que estaba. Con la regla de búsqueda de la suite: sin acentos ni
// mayúsculas.
//
// La coincidencia actual se enseña como SELECCIÓN del cuerpo (que se pinta aunque el foco
// esté en la caja); la web pinta además las demás con la API de resaltados de CSS.
// ponytail: aquí solo se ve la actual; pintar todas pide un velo más en zonaRenderer.

type buscadorNota struct {
	campo    *ui.Entrada
	cuenta   *canvas.Text
	fondo    *canvas.Rectangle
	filete   *canvas.Rectangle
	barra    *fyne.Container
	hallados [][2]Pos
	i        int
}

// coincidenciasDoc son las de q en el documento, en orden, bloque a bloque.
func coincidenciasDoc(d *Documento, q string) [][2]Pos {
	var out [][2]Pos
	for bi, b := range d.Bloques {
		if !editable(b) {
			continue
		}
		for _, r := range buscar.Rangos([]rune(b.Texto()), q) {
			out = append(out, [2]Pos{{bi, r[0]}, {bi, r[1]}})
		}
	}
	return out
}

func (e *EditorNota) abrirBuscador() {
	if e.buscador == nil {
		e.buscador = e.armarBuscador()
	}
	if e.haySeleccion() {
		e.buscador.campo.PonerTexto(e.Seleccionado())
	}
	e.buscarEnNota(e.buscador.campo.Texto(), 0)
	e.Refresh()
	llamar0(e.AlCrecer)
	if c := fyne.CurrentApp().Driver().CanvasForObject(e.zona); c != nil {
		c.Focus(e.buscador.campo)
	}
}

func (e *EditorNota) armarBuscador() *buscadorNota {
	t := e.tr.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := &buscadorNota{
		campo:  ui.NuevaEntrada(t("find.placeholder", nil)).Con(ui.CampoSM),
		cuenta: ui.Texto("", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
	}
	b.campo.Rotulo = t("find.placeholder", nil)
	b.campo.AlCambiar(func(s string) { e.buscarEnNota(s, 0) })
	b.campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			paso := 1
			if e.shift {
				paso = -1
			}
			e.buscarEnNota(b.campo.Texto(), b.i+paso)
			return true
		case fyne.KeyEscape:
			e.cerrarBuscador()
			return true
		default:
		}
		return false
	}
	atajo := func(rotulo, teclas string) string {
		return t("editor.shortcut", i18n.Vars{"label": rotulo, "keys": teclas})
	}
	anterior := nuevoBotonBarra("", iconos.ChevronArriba, atajo(t("find.previous", nil), "Shift+Enter"),
		func() { e.buscarEnNota(b.campo.Texto(), b.i-1) })
	siguiente := nuevoBotonBarra("", iconos.ChevronAbajo, atajo(t("find.next", nil), "Enter"),
		func() { e.buscarEnNota(b.campo.Texto(), b.i+1) })
	cerrar := nuevoBotonBarra("", iconos.Cerrar, t("find.close", nil), e.cerrarBuscador)
	cuenta := container.New(&textoVivo{t: b.cuenta, e: tema.TextoXS}, b.cuenta)
	fila := container.New(&filaElastica{hueco: tema.Espacio(1.5)}, b.campo, cuenta, anterior, siguiente, cerrar)
	b.barra = container.NewStack(b.fondo,
		container.NewBorder(nil, container.New(&altoFijo{alto: tema.Borde}, b.filete), nil, nil,
			container.New(ui.Margen{Izq: PXBarraEd, Der: PXBarraEd, Arr: PYBarraEd, Abj: PYBarraEd}, fila)))
	return b
}

// buscarEnNota busca q y lleva la selección a la coincidencia i (dando la vuelta).
func (e *EditorNota) buscarEnNota(q string, i int) {
	b := e.buscador
	if b == nil {
		return
	}
	b.hallados = coincidenciasDoc(e.doc, q)
	n := len(b.hallados)
	b.i = 0
	if n > 0 {
		b.i = ((i % n) + n) % n
		e.ancla, e.cur = b.hallados[b.i][0], b.hallados[b.i][1]
	}
	switch {
	case n > 0:
		b.cuenta.Text = e.tr.T("find.count", i18n.Vars{"current": strconv.Itoa(b.i + 1), "total": strconv.Itoa(n)})
	case q != "":
		b.cuenta.Text = e.tr.T("find.none", nil)
	default:
		b.cuenta.Text = ""
	}
	b.cuenta.Refresh()
	b.barra.Refresh()
	e.repintar()
	if n > 0 && e.AlMoverCursor != nil {
		_, arriba, alto := e.zona.cajaCursor()
		e.AlMoverCursor(PYZonaEd+arriba, alto)
	}
}

// cerrarBuscador quita la barra y devuelve el foco al cuerpo con la coincidencia
// seleccionada.
func (e *EditorNota) cerrarBuscador() {
	if e.buscador == nil {
		return
	}
	e.buscador = nil
	e.Refresh()
	llamar0(e.AlCrecer)
	e.reenfocar()
}

// textoVivo coloca un `canvas.Text` cuyo contenido cambia: mide lo que lleve AHORA.
type textoVivo struct {
	t *canvas.Text
	e tema.Escalon
}

func (l *textoVivo) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(ui.Medir(l.t.Text, l.e, ui.Normal).Width, l.e.Linea)
}

func (l *textoVivo) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(l.t, l.e, 0, (t.Height-l.e.Linea)/2)
}
