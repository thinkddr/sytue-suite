// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La segmentación de datos: `SheetSlicer` y `slicer.ts`
//
// Una tarjeta que filtra una COLUMNA del autofiltro de la hoja con sus valores únicos
// como botones: los elegidos se ven, los demás se ocultan. No tiene filtro propio: la
// selección se traduce al `excl` del autofiltro (`excludedFromSelection`), que es la
// única fuente de verdad, así que el desplegable del filtro y la tarjeta nunca se
// contradicen.
//
//	<section class="border-border bg-surface absolute z-10 w-44 overflow-hidden rounded-lg
//	                border shadow-[var(--shadow-md)]">
//	  cabecera h-7 (icono, campo, borrar filtro, quitar)
//	  <ul class="max-h-56 overflow-auto p-1"> botones px-1.5 py-0.5 text-xs
//	  <div class="border-t sytue-meta-dense px-2 py-1">{n} de {m}</div>

var (
	anchoSegmentacion = tema.Espacio(44)  // w-44
	altoMaxListaSeg   = tema.Espacio(56)  // max-h-56
	rellenoListaSeg   = tema.Espacio(1)   // p-1
	margenFilaSegX    = tema.Espacio(1.5) // px-1.5
	margenFilaSegY    = tema.Espacio(0.5) // py-0.5
	huecoFilaSeg      = tema.Espacio(1.5) // gap-1.5
	rellenoAvisoSegY  = tema.Espacio(3)   // py-3
)

// excluidosDeSeleccion es `excludedFromSelection`: los únicos que no están elegidos, en
// su orden.
func excluidosDeSeleccion(unicos []string, elegidos map[string]bool) []string {
	var out []string
	for _, v := range unicos {
		if !elegidos[v] {
			out = append(out, v)
		}
	}
	return out
}

// Segmentaciones son las de la hoja activa.
func (l *Libro) Segmentaciones() []Segmentacion { return l.h().segs }

// AnadirSegmentacion es `sliceradd`: reutiliza el autofiltro de la hoja (o lo crea
// sobre el rango) ensanchándolo si el campo cae fuera, y añade la tarjeta. false y
// `repetida` si ya hay una de esa columna.
func (l *Libro) AnadirSegmentacion(rango string, col int) (ok, repetida bool) {
	r, valido := celdasDeRango(rango)
	if !valido || col < 0 || col >= Columnas {
		return false, false
	}
	h := l.h()
	if slices.ContainsFunc(h.segs, func(s Segmentacion) bool { return s.Pivot == "" && s.Col == col }) {
		return false, true
	}
	af := h.filtro
	nuevo := &Autofiltro{Row: r[1], C1: min(r[0], col), C2: max(r[2], col)}
	if af != nil {
		nuevo.Row, nuevo.C1, nuevo.C2, nuevo.Excl, nuevo.Cond = af.Row, min(af.C1, col), max(af.C2, col), af.Excl, af.Cond
	}
	nuevo.C1, nuevo.C2 = clamp(nuevo.C1, Columnas-1), clamp(nuevo.C2, Columnas-1)
	n := len(h.segs)
	x, y := float64(96+(n%8)*24), float64(48+(n%8)*24)
	l.anotar()
	h = l.h()
	h.filtro = nuevo
	h.segs = append(slices0(h.segs), Segmentacion{ID: nuevoID(), Col: col, X: &x, Y: &y})
	l.version++
	return true, false
}

// MoverSegmentacion es `moveSlicer`.
func (l *Libro) MoverSegmentacion(id string, x, y float64) bool {
	return l.cambiarSegmentacion(id, func(s Segmentacion) (Segmentacion, bool) {
		s.X, s.Y = &x, &y
		return s, true
	})
}

// QuitarSegmentacion es `removeSlicer`. La de un campo que no está en su tabla dinámica
// se lleva su filtro oculto (nada más lo enseña), en el mismo paso de deshacer. Si la
// tabla no se puede reescribir, la tarjeta se queda y `fallo` dice por qué: "protected"
// con la hoja protegida, o el error de EscribirDinamica.
func (l *Libro) QuitarSegmentacion(id string, t func(string, i18n.Vars) string) (quitada bool, fallo string) {
	i := slices.IndexFunc(l.h().segs, func(s Segmentacion) bool { return s.ID == id })
	if i < 0 {
		return false, ""
	}
	s := l.h().segs[i]
	d, hay := l.dinamicaDe(s.Pivot)
	sin, conFiltro := sinFiltroOculto(d, s.Col)
	if !hay || !conFiltro {
		return l.cambiarSegmentacion(id, func(s Segmentacion) (Segmentacion, bool) { return s, false }), ""
	}
	if l.Proteccion() != nil {
		return false, "protected"
	}
	if fallo := l.escribirDinamicas([]analisis.Dinamica{sin}, t); fallo != "" {
		return false, fallo
	}
	// En el paso de deshacer que acaba de anotar escribirDinamicas.
	h := l.h()
	h.segs = slices.Delete(slices0(h.segs), i, i+1)
	if len(h.segs) == 0 {
		h.segs = nil
	}
	l.version++
	return true, ""
}

func (l *Libro) cambiarSegmentacion(id string, f func(Segmentacion) (Segmentacion, bool)) bool {
	h := l.h()
	i := slices.IndexFunc(h.segs, func(s Segmentacion) bool { return s.ID == id })
	if i < 0 {
		return false
	}
	nuevo, sigue := f(h.segs[i])
	l.anotar()
	lista := slices0(h.segs)
	if sigue {
		lista[i] = nuevo
	} else {
		lista = append(lista[:i], lista[i+1:]...)
	}
	if len(lista) == 0 {
		lista = nil
	}
	l.h().segs = lista
	l.version++
	return true
}

// Segmentar es `applySlicerSelection`: la selección de la tarjeta pasa al `excl` del
// autofiltro en UN paso de deshacer.
func (l *Libro) Segmentar(col int, elegidos map[string]bool) bool {
	af := l.h().filtro
	if af == nil {
		return false
	}
	var unicos []string
	for _, e := range l.ValoresFiltro(col) {
		unicos = append(unicos, e.Valor)
	}
	excl := excluidosDeSeleccion(unicos, elegidos)
	nuevo := af.conExcluidos(col, excl)
	l.anotar()
	l.h().filtro = nuevo
	l.version++
	return true
}

// ---- la de una tabla dinámica ----

// dinamicaDe es la tabla dinámica de la hoja activa con ese nombre.
func (l *Libro) dinamicaDe(nombre string) (analisis.Dinamica, bool) {
	for _, d := range l.h().dinamicas {
		if nombre != "" && d.Name == nombre {
			return d, true
		}
	}
	return analisis.Dinamica{}, false
}

// AnadirSegmentacionDinamica es `sliceradd` con `pivot`: una tarjeta para el campo
// `campo` del origen de esa tabla, esté o no en ella (como Excel). No toca el
// autofiltro. false y `repetida` si ya hay una de ese campo en esa tabla.
func (l *Libro) AnadirSegmentacionDinamica(nombre string, campo int) (ok, repetida bool) {
	d, hay := l.dinamicaDe(nombre)
	rejilla, _, leido := l.origenDinamica(d)
	if !hay || !leido || len(rejilla) == 0 || campo < 0 || campo >= len(rejilla[0]) {
		return false, false
	}
	h := l.h()
	if slices.ContainsFunc(h.segs, func(s Segmentacion) bool { return s.Pivot == nombre && s.Col == campo }) {
		return false, true
	}
	n := len(h.segs)
	x, y := float64(96+(n%8)*24), float64(48+(n%8)*24)
	l.anotar()
	h = l.h()
	h.segs = append(slices0(h.segs), Segmentacion{ID: nuevoID(), Col: campo, Pivot: nombre, X: &x, Y: &y})
	l.version++
	return true, false
}

// estadoDinamica es `pivotSlicerView`: el nombre del campo (su encabezado en el
// origen), sus elementos —por su clave, con su rótulo y lo que la tabla oculta— y si
// sigue viva (la tabla está y el campo existe en su origen).
func (l *Libro) estadoDinamica(s Segmentacion, t func(string, i18n.Vars) string) (activa bool, campo string, entradas []EntradaFiltro) {
	campo = t("common.columnN", i18n.Vars{"n": s.Col + 1})
	d, hay := l.dinamicaDe(s.Pivot)
	rejilla, formatoCampo, leido := l.origenDinamica(d)
	if !hay || !leido || len(rejilla) == 0 || s.Col >= len(rejilla[0]) {
		return false, campo, nil
	}
	r := rotulosConFormato(t, formatoCampo)
	if nombre := r.Show(rejilla[0][s.Col], -1); nombre != "" {
		campo = nombre
	}
	ref := refSegmentacion(d.DefDinamica, s.Col)
	claves, rotulos := analisis.ElementosDinamica(rejilla, ref, r)
	for i, k := range claves {
		entradas = append(entradas, EntradaFiltro{Valor: k, Rotulo: rotulos[i], Oculto: slices.Contains(ref.Hidden, k)})
	}
	return true, campo, entradas
}

// SegmentarDinamica es `applyPivotSlicer`: el campo oculta lo que no está elegido y la
// tabla se vuelve a escribir, en UN paso de deshacer.
func (l *Libro) SegmentarDinamica(s Segmentacion, elegidos map[string]bool, t func(string, i18n.Vars) string) bool {
	d, hay := l.dinamicaDe(s.Pivot)
	activa, _, entradas := l.estadoDinamica(s, t)
	if !hay || !activa {
		return false
	}
	unicos := make([]string, len(entradas))
	for i, e := range entradas {
		unicos[i] = e.Valor
	}
	return l.escribirDinamicas([]analisis.Dinamica{conOcultos(d, s.Col, excluidosDeSeleccion(unicos, elegidos))}, t) == ""
}

// refSegmentacion es `pivotSlicerRef`: el campo en filtros, filas o columnas, por ese
// orden (con su agrupación); si no está en la tabla, su filtro oculto, o uno nuevo.
func refSegmentacion(d analisis.DefDinamica, campo int) analisis.CampoDinamica {
	for _, c := range slices.Concat(d.Filters, d.Rows, d.Cols, d.HiddenFilters) {
		if c.Field == campo {
			return c
		}
	}
	return analisis.CampoDinamica{Field: campo}
}

// conOcultos es `pivotWithHidden`: el campo oculta `ocultos` (vacío = todo a la vista)
// en TODOS los sitios en que está; si no está en la tabla, en su filtro oculto. No toca
// las listas de d: son las de la foto de deshacer.
func conOcultos(d analisis.Dinamica, campo int, ocultos []string) analisis.Dinamica {
	var h []string
	if len(ocultos) > 0 {
		h = slices.Clone(ocultos)
	}
	enTabla := false
	poner := func(cs []analisis.CampoDinamica) []analisis.CampoDinamica {
		out := slices.Clone(cs)
		for i := range out {
			if out[i].Field == campo {
				out[i].Hidden, enTabla = h, true
			}
		}
		return out
	}
	d.Rows, d.Cols, d.Filters = poner(d.Rows), poner(d.Cols), poner(d.Filters)
	resto := slices.DeleteFunc(slices.Clone(d.HiddenFilters), func(c analisis.CampoDinamica) bool { return c.Field == campo })
	if !enTabla && h != nil {
		resto = append(resto, analisis.CampoDinamica{Field: campo, Hidden: h})
	}
	d.HiddenFilters = nil
	if len(resto) > 0 {
		d.HiddenFilters = resto
	}
	return d
}

// sinFiltroOculto es `pivotWithoutHiddenFilter`: d sin el filtro oculto del campo;
// false si no tenía.
func sinFiltroOculto(d analisis.Dinamica, campo int) (analisis.Dinamica, bool) {
	resto := slices.DeleteFunc(slices.Clone(d.HiddenFilters), func(c analisis.CampoDinamica) bool { return c.Field == campo })
	if len(resto) == len(d.HiddenFilters) {
		return d, false
	}
	d.HiddenFilters = nil
	if len(resto) > 0 {
		d.HiddenFilters = resto
	}
	return d, true
}

// ---- la tarjeta ----

// TarjetaSegmentacion es la tarjeta de UNA segmentación de la hoja activa.
type TarjetaSegmentacion struct {
	widget.BaseWidget
	lz *Lienzo
	id string

	moviendo bool
	desp     fyne.Delta
	desY     float32 // el desplazamiento de la lista

	borrar, quitar *botonTarjeta
}

var (
	_ fyne.Widget       = (*TarjetaSegmentacion)(nil)
	_ fyne.Draggable    = (*TarjetaSegmentacion)(nil)
	_ fyne.Tappable     = (*TarjetaSegmentacion)(nil)
	_ fyne.Scrollable   = (*TarjetaSegmentacion)(nil)
	_ desktop.Mouseable = (*TarjetaSegmentacion)(nil)
)

func nuevaTarjetaSegmentacion(lz *Lienzo, id string) *TarjetaSegmentacion {
	t := &TarjetaSegmentacion{lz: lz, id: id}
	t.borrar = nuevoBotonTarjeta(iconos.EdFiltroQuitar, lz.t("slicer.clear", nil), func() {
		s, ok := t.segmentacion()
		if !ok {
			return
		}
		todos := map[string]bool{}
		_, _, entradas := t.estado()
		for _, e := range entradas {
			todos[e.Valor] = true
		}
		t.aplicar(s, todos)
	})
	t.quitar = nuevoBotonTarjeta(iconos.EdCerrar, lz.t("slicer.remove", nil), func() {
		lz.quitarSegmentacion(id)
	})
	t.ExtendBaseWidget(t)
	return t
}

func (t *TarjetaSegmentacion) segmentacion() (Segmentacion, bool) {
	for _, s := range t.lz.Libro.Segmentaciones() {
		if s.ID == t.id {
			return s, true
		}
	}
	return Segmentacion{}, false
}

// Posicion es `left`/`top`, con los 96 y 48 de siempre.
func (s Segmentacion) Posicion() (x, y float32) {
	x, y = 96, 48
	if s.X != nil {
		x = float32(*s.X)
	}
	if s.Y != nil {
		y = float32(*s.Y)
	}
	return x, y
}

// estado es lo que pinta la tarjeta: si hay autofiltro que cubra la columna, el nombre
// del campo (su cabecera, o su letra) y las entradas.
func (t *TarjetaSegmentacion) estado() (activa bool, campo string, entradas []EntradaFiltro) {
	s, _ := t.segmentacion()
	l := t.lz.Libro
	if s.Pivot != "" {
		return l.estadoDinamica(s, t.lz.t)
	}
	af := l.Autofiltro()
	campo = formula.NumToCol(s.Col)
	if af == nil {
		return false, campo, nil
	}
	if v := l.textoFiltro(s.Col, af.Row); v != "" {
		campo = v
	}
	if s.Col < af.C1 || s.Col > af.C2 {
		return false, campo, nil
	}
	return true, campo, l.ValoresFiltro(s.Col)
}

// alternar es `toggle`: elige o deja de elegir un valor y reaplica.
func (t *TarjetaSegmentacion) alternar(valor string) {
	s, ok := t.segmentacion()
	if !ok {
		return
	}
	elegidos := map[string]bool{}
	_, _, entradas := t.estado()
	for _, e := range entradas {
		if !e.Oculto {
			elegidos[e.Valor] = true
		}
	}
	elegidos[valor] = !elegidos[valor]
	t.aplicar(s, elegidos)
}

// aplicar lleva la selección al autofiltro o, si es de una tabla dinámica, a la tabla.
func (t *TarjetaSegmentacion) aplicar(s Segmentacion, elegidos map[string]bool) {
	if s.Pivot != "" {
		t.lz.segmentarDinamica(s, elegidos)
		return
	}
	t.lz.segmentar(s.Col, elegidos)
}

func (t *TarjetaSegmentacion) MouseDown(*desktop.MouseEvent) {}
func (t *TarjetaSegmentacion) MouseUp(*desktop.MouseEvent)   {}

// Tapped alterna el valor de la fila pulsada.
func (t *TarjetaSegmentacion) Tapped(e *fyne.PointEvent) {
	z := t.lz.Rejilla.z()
	activa, _, entradas := t.estado()
	if !activa || len(entradas) == 0 {
		return
	}
	arriba := (tema.Borde + altoCabeceraTarjeta + rellenoListaSeg) * z
	i := int((e.Position.Y - arriba + t.desY) / (altoFilaSeg() * z))
	if e.Position.Y >= arriba && i >= 0 && i < len(entradas) {
		t.alternar(entradas[i].Valor)
	}
}

// Scrolled desplaza la lista (`overflow-auto`).
func (t *TarjetaSegmentacion) Scrolled(e *fyne.ScrollEvent) {
	_, _, entradas := t.estado()
	z := t.lz.Rejilla.z()
	total := float32(len(entradas))*altoFilaSeg()*z - (altoMaxListaSeg-2*rellenoListaSeg)*z
	t.desY = clampF(t.desY-e.Scrolled.DY, 0, max(total, 0))
	t.Refresh()
}

// Dragged mueve la tarjeta desde su cabecera.
func (t *TarjetaSegmentacion) Dragged(e *fyne.DragEvent) {
	z := t.lz.Rejilla.z()
	if !t.moviendo {
		if e.Position.Y-e.Dragged.DY >= (tema.Borde+altoCabeceraTarjeta)*z {
			return
		}
		t.moviendo, t.desp = true, fyne.Delta{}
	}
	t.desp.DX += e.Dragged.DX / z
	t.desp.DY += e.Dragged.DY / z
	t.lz.Rejilla.Refresh()
}

func (t *TarjetaSegmentacion) DragEnd() {
	if !t.moviendo {
		return
	}
	t.moviendo = false
	s, ok := t.segmentacion()
	if ok && (t.desp.DX != 0 || t.desp.DY != 0) {
		x, y := s.Posicion()
		t.lz.moverObjeto(t.id, redondearJS(float64(x+t.desp.DX)), redondearJS(float64(y+t.desp.DY)))
	}
	t.desp = fyne.Delta{}
	t.lz.Rejilla.Refresh()
}

func altoFilaSeg() float32 { return tema.TextoXS.Linea + 2*margenFilaSegY }

// medida es el tamaño a zoom 1.
func (t *TarjetaSegmentacion) medida() fyne.Size {
	activa, _, entradas := t.estado()
	alto := 2*tema.Borde + altoCabeceraTarjeta
	if !activa || len(entradas) == 0 {
		alto += 2*rellenoAvisoSegY + tema.Texto2XS.Linea
	} else {
		alto += min(float32(len(entradas))*altoFilaSeg()+2*rellenoListaSeg, altoMaxListaSeg)
		alto += tema.Borde + 2*rellenoLeyendaY + tema.Texto2XS.Linea
	}
	return fyne.NewSize(anchoSegmentacion, alto)
}

func (t *TarjetaSegmentacion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return &tarjetaSegRenderer{
		t: t, sombra: ui.NuevaSombra(tema.SombraMD, tema.RadioLG),
		caja:     ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		lineaCab: canvas.NewRectangle(tema.Color("border", v)), lineaPie: canvas.NewRectangle(tema.Color("border", v)),
		icono: iconos.TrazoEd(iconos.EdSegmentacion, iconoSM, "text-muted", v),
		campo: ui.Texto("", tema.TextoXS, ui.Medio, tema.Color("text", v)),
		aviso: ui.Texto("", tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
		pie:   ui.Texto("", tema.Texto2XS, ui.Normal, tema.Color("text-subtle", v)),
	}
}

type tarjetaSegRenderer struct {
	t                  *TarjetaSegmentacion
	sombra             *ui.Sombra
	caja               *ui.Caja
	lineaCab, lineaPie *canvas.Rectangle
	icono              *iconos.Dibujo
	campo, aviso, pie  *canvas.Text
	fondos             []*canvas.Rectangle
	textos             []*canvas.Text
	tics               []*iconos.Dibujo
	objetos            []fyne.CanvasObject
}

func (r *tarjetaSegRenderer) Destroy()                     {}
func (r *tarjetaSegRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaSegRenderer) MinSize() fyne.Size {
	z := r.t.lz.Rejilla.z()
	m := r.t.medida()
	return fyne.NewSize(m.Width*z, m.Height*z)
}

func (r *tarjetaSegRenderer) Refresh() {
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}

func (r *tarjetaSegRenderer) Layout(tam fyne.Size) {
	t := r.t
	z := t.lz.Rejilla.z()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := tema.Borde * z
	activa, campo, entradas := t.estado()
	r.sombra.Ajustar(fyne.NewPos(0, 0), tam, v)
	r.caja.Redimensionar(tam)
	cab := altoCabeceraTarjeta * z
	caja(r.lineaCab, tema.Color("border", v), b, cab, tam.Width-2*b, b)
	x := b + margenTarjeta*z
	r.icono.Colocar(x, b+(cab-b-iconoSM*z)/2, iconoSM*z)
	x += (iconoSM + huecoCabecera) * z
	lado := ladoBotonTarjeta * z
	der := tam.Width - b - margenTarjeta*z
	oculto := false
	for _, e := range entradas {
		oculto = oculto || e.Oculto
	}
	t.borrar.Inerte = !oculto
	for _, bt := range []*botonTarjeta{t.quitar, t.borrar} {
		der -= lado
		bt.Move(fyne.NewPos(der, b+(cab-b-lado)/2))
		bt.Resize(fyne.NewSize(lado, lado))
		bt.Refresh()
		der -= huecoCabecera * z
	}
	esc := func(e tema.Escalon) tema.Escalon { return tema.Escalon{Tam: e.Tam * z, Linea: e.Linea * z} }
	r.campo.TextSize = tema.TextoXS.Tam * z
	ui.Truncar(r.campo, campo, esc(tema.TextoXS), ui.Medio, max(der-x, 0))
	ui.ColocarTexto(r.campo, esc(tema.TextoXS), x, b+(cab-b-tema.TextoXS.Linea*z)/2)

	o := []fyne.CanvasObject{r.sombra.Objeto()}
	o = append(o, r.caja.Objetos()...)
	o = append(o, r.lineaCab, r.icono.Objeto(), r.campo, t.borrar, t.quitar)
	cuerpo := b + cab
	if !activa || len(entradas) == 0 {
		clave := "slicer.noValues"
		if s, _ := t.segmentacion(); !activa && s.Pivot != "" {
			clave = "slicer.pivotInactive"
		} else if !activa {
			clave = "slicer.inactive"
		}
		r.aviso.TextSize = tema.Texto2XS.Tam * z
		ui.Truncar(r.aviso, t.lz.t(clave, nil), esc(tema.Texto2XS), ui.Normal, tam.Width-2*b-2*margenTarjeta*z)
		ui.ColocarTexto(r.aviso, esc(tema.Texto2XS), b+margenTarjeta*z, cuerpo+rellenoAvisoSegY*z)
		o = append(o, r.aviso)
		r.objetos = o
		return
	}
	o = append(o, r.filas(entradas, cuerpo, tam, z, v)...)
	lista := min(float32(len(entradas))*altoFilaSeg()+2*rellenoListaSeg, altoMaxListaSeg) * z
	pie := cuerpo + lista
	caja(r.lineaPie, tema.Color("border", v), b, pie, tam.Width-2*b, b)
	elegidos := 0
	for _, e := range entradas {
		if !e.Oculto {
			elegidos++
		}
	}
	r.pie.TextSize = tema.Texto2XS.Tam * z
	r.pie.Text = t.lz.t("slicer.count", i18n.Vars{"selected": elegidos, "total": len(entradas)})
	ui.ColocarTexto(r.pie, esc(tema.Texto2XS), b+margenTarjeta*z, pie+b+rellenoLeyendaY*z)
	o = append(o, r.lineaPie, r.pie)
	r.objetos = o
}

// filas pinta los botones de la lista que caben: `bg-selected text-text` los elegidos
// con su tic, `text-text-subtle` los ocultos.
func (r *tarjetaSegRenderer) filas(entradas []EntradaFiltro, arriba float32, tam fyne.Size, z float32, v tema.Variante) []fyne.CanvasObject {
	t := r.t
	b := tema.Borde * z
	alto := altoFilaSeg() * z
	tope := arriba + min(float32(len(entradas))*altoFilaSeg()+2*rellenoListaSeg, altoMaxListaSeg)*z - rellenoListaSeg*z
	y0 := arriba + rellenoListaSeg*z - t.desY
	var out []fyne.CanvasObject
	n := 0
	for i, e := range entradas {
		y := y0 + float32(i)*alto
		if y+alto <= arriba+rellenoListaSeg*z || y >= tope {
			continue
		}
		for len(r.fondos) <= n {
			f := canvas.NewRectangle(color.NRGBA{})
			f.CornerRadius = tema.RadioSM
			r.fondos = append(r.fondos, f)
			r.textos = append(r.textos, ui.Texto("", tema.TextoXS, ui.Normal, color.NRGBA{}))
			r.tics = append(r.tics, iconos.TrazoEd(iconos.EdTic, iconoSM, "text", v))
		}
		f, tx, tic := r.fondos[n], r.textos[n], r.tics[n]
		n++
		x := b + rellenoListaSeg*z
		w := tam.Width - 2*x
		if e.Oculto {
			f.Hide()
			tx.Color = tema.Color("text-subtle", v)
		} else {
			caja(f, tema.Color("selected", v), x, y, w, alto)
			tx.Color = tema.Color("text", v)
		}
		rotulo := e.Rotulo
		if rotulo == "" {
			rotulo = e.Valor
		}
		if rotulo == "" {
			rotulo = t.lz.t("slicer.blanks", nil)
		}
		esc := tema.Escalon{Tam: tema.TextoXS.Tam * z, Linea: tema.TextoXS.Linea * z}
		tx.TextSize = esc.Tam
		cabe := w - 2*margenFilaSegX*z
		if !e.Oculto {
			cabe -= (iconoSM + huecoFilaSeg) * z
			tic.Colocar(x+w-margenFilaSegX*z-iconoSM*z, y+(alto-iconoSM*z)/2, iconoSM*z)
			tic.Ver(true)
			out = append(out, tic.Objeto())
		} else {
			tic.Ver(false)
		}
		ui.Truncar(tx, rotulo, esc, ui.Normal, max(cabe, 0))
		ui.ColocarTexto(tx, esc, x+margenFilaSegX*z, y+margenFilaSegY*z)
		out = append(out, f, tx)
	}
	return out
}

// ---- el lienzo ----

// segmentar aplica la selección de una tarjeta, con el permiso del autofiltro si la hoja
// está protegida (como `filterset`).
func (c *Lienzo) segmentar(col int, elegidos map[string]bool) {
	if c.Rejilla.SoloLectura {
		return
	}
	if p := c.Libro.Proteccion(); p != nil && !p.AutoFilter {
		c.avisarProtegida(c.Rejilla.sel)
		return
	}
	if c.Libro.Segmentar(col, elegidos) {
		c.trasCambio()
	}
}

// segmentarDinamica aplica la selección de la tarjeta de una tabla dinámica: es
// reescribir celdas, así que con la hoja protegida queda vetada como `pivot`.
func (c *Lienzo) segmentarDinamica(s Segmentacion, elegidos map[string]bool) {
	if c.Rejilla.SoloLectura {
		return
	}
	if c.Libro.Proteccion() != nil {
		c.avisarProtegida(c.Rejilla.sel)
		return
	}
	if c.Libro.SegmentarDinamica(s, elegidos, c.t) {
		c.trasCambio()
	}
}

// quitarSegmentacion quita la tarjeta o dice por qué no: la de un campo de fuera de su
// tabla dinámica reescribe la tabla, y la hoja protegida no lo deja.
func (c *Lienzo) quitarSegmentacion(id string) {
	if c.objetosCerrados() {
		return
	}
	quitada, fallo := c.Libro.QuitarSegmentacion(id, c.t)
	switch {
	case quitada:
		c.trasCambio()
	case fallo == "protected":
		c.avisar2(ui.AvisoInfo, "sheet.msg.protectedSheet", "sheet.msg.slicerPivotProtected")
	case fallo != "":
		c.avisarFalloDinamica(fallo)
	}
}

// anadirSegmentacion es el comando `sliceradd`.
func (c *Lienzo) anadirSegmentacion(valor string) bool {
	var req struct {
		Range string `json:"range"`
		Col   any    `json:"col"`
		Pivot string `json:"pivot"`
	}
	if json.Unmarshal([]byte(valor), &req) != nil {
		return false
	}
	col, ok := numeroEntero(req.Col)
	if !ok {
		return false
	}
	if req.Pivot != "" {
		hecho, repetida := c.Libro.AnadirSegmentacionDinamica(req.Pivot, col)
		if repetida && c.AlAviso != nil {
			c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.slicerExists", nil), "")
		}
		return hecho
	}
	hecho, repetida := c.Libro.AnadirSegmentacion(req.Range, col)
	if repetida && c.AlAviso != nil {
		c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.slicerExists", nil), c.t("sheet.msg.slicerExists.detail", i18n.Vars{"col": formula.NumToCol(col)}))
	}
	return hecho
}
