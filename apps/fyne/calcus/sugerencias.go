// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las medidas del autocompletado y de la pista (`autoList` y `hintContent` de
// SheetCanvas.tsx). Todo rem salvo los topes de ancho, que la web escribe en px.
//
//	lista: border-border bg-menu-surface max-h-56 w-max max-w-[380px] min-w-[240px]
//	       overflow-auto rounded-lg border py-1 shadow-[var(--shadow-md)]
//	fila:  flex w-full items-baseline gap-2 px-2 py-0.5 text-left text-sm
//	pista: border-border bg-menu-surface w-max max-w-[400px] rounded-lg border px-2 py-1
//	       text-xs shadow-[var(--shadow-md)]
var (
	AnchoMinLista         float32 = 240
	AnchoMaxLista         float32 = 380
	AltoMaxLista                  = tema.Espacio(56)
	AnchoMaxPista         float32 = 400
	RellenoLista                  = tema.Espacio(1)   // py-1
	MargenFilaAC                  = tema.Espacio(2)   // px-2
	RellenoFilaAC                 = tema.Espacio(0.5) // py-0.5
	HuecoFilaAC                   = tema.Espacio(2)   // gap-2
	AltoFilaAC                    = tema.TextoSM.Linea + 2*tema.Espacio(0.5)
	SeparacionEmergenteFx         = tema.Espacio(1) // el `mt-1` bajo la barra
	// La lista de una validación: `mt-0.5 max-h-56 w-max min-w-24`.
	AnchoMinValores           = tema.Espacio(24)
	SeparacionValores         = tema.Espacio(0.5)
	AnchoBotonValores float32 = 18 // `w-[18px]`, y de alto la celda: `h-[24px]`
)

// sugerencias es lo que cuelga bajo la celda o la barra mientras se escribe una fórmula:
// la lista del autocompletado o, si no hay lista, la pista de la función.
//
// Es un widget y no un menú del sistema de diseño porque en la web tampoco lo es: cada
// fila lleva el nombre en `font-medium` y su descripción en `text-xs` al lado, y la
// elegida con `bg-menu-selected`. El fondo es opaco también en modo traslúcido:
// el texto de las celdas no debe mezclarse con las opciones.
type sugerencias struct {
	widget.BaseWidget

	lz *Lienzo
	// lo que se pinta: la lista (items, elegido) o la pista (firma y argumento).
	items   []string
	elegido int
	pista   firma
	arg     int
	desY    float32 // el `overflow-auto` de la lista
	// valores es la lista de una validación (`validDropdown`): las filas en letra
	// normal, sin descripción, y lo pulsado se escribe en la celda.
	valores bool
	// clases son las del autocompletado, en paralelo a items: función, tabla o columna.
	clases []string
}

var (
	_ fyne.Widget       = (*sugerencias)(nil)
	_ fyne.Tappable     = (*sugerencias)(nil)
	_ fyne.Scrollable   = (*sugerencias)(nil)
	_ desktop.Mouseable = (*sugerencias)(nil)
)

func nuevasSugerencias(lz *Lienzo) *sugerencias {
	s := &sugerencias{lz: lz}
	s.ExtendBaseWidget(s)
	return s
}

// poner decide qué se enseña. false si no hay nada que enseñar.
func (s *sugerencias) poner() bool {
	s.items, s.clases, s.pista, s.valores = nil, nil, firma{}, false
	if v := s.lz.valoresAbiertos(); v != nil {
		s.items, s.valores, s.elegido = v.Valores, true, -1
		actual := strings.TrimSpace(s.lz.Libro.Crudo(s.lz.Rejilla.sel.C, s.lz.Rejilla.sel.F))
		for i, x := range v.Valores {
			if x == actual {
				s.elegido = i // `aria-selected` del valor que ya tiene
			}
		}
		return true
	}
	if ac, ok := s.lz.ac(); ok {
		s.items, s.clases, s.elegido = ac.items, ac.clases, min(s.lz.fx.indice, len(ac.items)-1)
		// La elegida siempre a la vista.
		y := float32(s.elegido) * AltoFilaAC
		visible := AltoMaxLista - 2*RellenoLista - 2*tema.Borde
		s.desY = clampF(s.desY, y+AltoFilaAC-visible, y)
		s.desY = max(s.desY, 0)
		return true
	}
	if f, arg, ok := s.lz.pista(); ok {
		s.pista, s.arg = f, arg
		return true
	}
	return false
}

// descripcion es lo que acompaña a cada fila de la lista: el resumen del catálogo de
// una función, y «Tabla» o «Columna» para las estructuradas (`sheet.ac.*`).
func (s *sugerencias) descripcion(i int) string {
	if i < len(s.clases) {
		switch s.clases[i] {
		case acTabla:
			return s.lz.t("sheet.ac.table", nil)
		case acColumna:
			return s.lz.t("sheet.ac.column", nil)
		}
	}
	f, _ := buscarFirma(s.items[i])
	return f.Resumen
}

// MinSize es lo que mide la lista (`w-max` entre los topes) o la pista.
func (s *sugerencias) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	if s.valores {
		var ancho float32
		for _, n := range s.items {
			ancho = max(ancho, ui.Medir(n, tema.TextoSM, ui.Normal).Width)
		}
		return fyne.NewSize(max(ancho+2*MargenFilaAC+2*tema.Borde, AnchoMinValores),
			min(float32(len(s.items))*AltoFilaAC+2*RellenoLista+2*tema.Borde, AltoMaxLista))
	}
	if len(s.items) > 0 {
		var ancho float32
		for i, n := range s.items {
			w := ui.Medir(n, tema.TextoSM, ui.Medio).Width
			if d := s.descripcion(i); d != "" {
				w += HuecoFilaAC + ui.Medir(d, tema.TextoXS, ui.Normal).Width
			}
			ancho = max(ancho, w)
		}
		ancho = min(max(ancho+2*MargenFilaAC+2*tema.Borde, AnchoMinLista), AnchoMaxLista)
		alto := min(float32(len(s.items))*AltoFilaAC+2*RellenoLista+2*tema.Borde, AltoMaxLista)
		return fyne.NewSize(ancho, alto)
	}
	if s.pista.Args == "" {
		return fyne.Size{}
	}
	ancho := s.anchoPista()
	lineas := len(lineasResumen(s.pista.Resumen, ancho))
	return fyne.NewSize(ancho+2*MargenFilaAC+2*tema.Borde,
		2*RellenoLista+2*tema.Borde+tema.TextoXS.Linea+RellenoFilaAC+float32(lineas)*tema.TextoXS.Linea)
}

// anchoPista es el ancho de CONTENIDO de la pista: `w-max`, lo que pida la firma o el
// resumen en una línea, hasta el `max-w-[400px]`.
func (s *sugerencias) anchoPista() float32 {
	cabe := AnchoMaxPista - 2*MargenFilaAC - 2*tema.Borde
	return min(max(s.anchoFirma(), ui.Medir(s.pista.Resumen, tema.TextoXS, ui.Normal).Width), cabe)
}

// lineasResumen reparte el resumen al ancho de la pista, con el reparto de `ui.Parrafo`.
func lineasResumen(resumen string, ancho float32) []string {
	return ui.LineasDe(ui.NuevoParrafoConTinta(resumen, tema.TextoXS, ui.Normal, color.NRGBA{}), ancho)
}

// trozosFirma son los trozos de la primera línea de la pista: el nombre en
// `font-medium`, los paréntesis y cada parámetro, el actual en `font-semibold` y tinta
// normal y los demás en `text-muted`.
func (s *sugerencias) trozosFirma() (textos []string, pesos []ui.Peso, apagados []bool) {
	nombre, params := partirFirma(s.pista.Args)
	activo := min(s.arg, max(0, len(params)-1))
	textos, pesos, apagados = []string{nombre, "("}, []ui.Peso{ui.Medio, ui.Normal}, []bool{false, false}
	for i, p := range params {
		if i > 0 {
			textos, pesos, apagados = append(textos, "; "), append(pesos, ui.Normal), append(apagados, false)
		}
		peso, apagado := ui.Normal, true
		if i == activo {
			peso, apagado = ui.SemiNegrita, false
		}
		textos, pesos, apagados = append(textos, p), append(pesos, peso), append(apagados, apagado)
	}
	return append(textos, ")"), append(pesos, ui.Normal), append(apagados, false)
}

func (s *sugerencias) anchoFirma() float32 {
	textos, pesos, _ := s.trozosFirma()
	var w float32
	for i, t := range textos {
		w += ui.Medir(t, tema.TextoXS, pesos[i]).Width
	}
	return w
}

// MouseDown avisa de que se está pulsando la lista: el editor pierde el foco al pulsar
// y no tiene que dar la fórmula por acabada (en la web, el `preventDefault` del
// `mousedown` hace que ni lo pierda).
func (s *sugerencias) MouseDown(*desktop.MouseEvent) { s.lz.mantenerFx = true }

// MouseUp no hace nada: acepta Tapped.
func (s *sugerencias) MouseUp(*desktop.MouseEvent) {}

// Tapped acepta la función pulsada.
func (s *sugerencias) Tapped(e *fyne.PointEvent) {
	s.lz.mantenerFx = false
	if s.valores {
		if i := int((e.Position.Y - tema.Borde - RellenoLista + s.desY) / AltoFilaAC); i >= 0 && i < len(s.items) {
			s.lz.elegirValor(s.items[i])
		}
		return
	}
	if len(s.items) == 0 || s.lz.fx == nil {
		s.lz.devolverFoco()
		return
	}
	i := int((e.Position.Y - tema.Borde - RellenoLista + s.desY) / AltoFilaAC)
	if i >= 0 && i < len(s.items) {
		s.lz.fx.indice = i
		s.lz.aceptarAC(i)
		s.lz.devolverFoco()
	}
}

// Scrolled desplaza la lista.
func (s *sugerencias) Scrolled(e *fyne.ScrollEvent) {
	total := float32(len(s.items))*AltoFilaAC - (s.Size().Height - 2*RellenoLista - 2*tema.Borde)
	s.desY = clampF(s.desY-e.Scrolled.DY, 0, max(total, 0))
	s.Refresh()
}

// CreateRenderer compone la caja, las filas y la pista. La sombra NO va aquí: la lista
// recorta lo que se sale de su caja (el `overflow-auto`), y la sombra es justo lo que
// se sale; la pone el lienzo detrás, de hermana.
func (s *sugerencias) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &sugerenciasRenderer{
		s:     s,
		caja:  ui.NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioLG),
		marca: canvas.NewRectangle(tema.Color("menu-selected", v)),
	}
	return r
}

type sugerenciasRenderer struct {
	s       *sugerencias
	caja    *ui.Caja
	marca   *canvas.Rectangle
	textos  []*canvas.Text
	objetos []fyne.CanvasObject
}

func (r *sugerenciasRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *sugerenciasRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *sugerenciasRenderer) Destroy()                     {}

// texto da el i-ésimo texto del charco, creándolo si hace falta.
func (r *sugerenciasRenderer) texto(i int) *canvas.Text {
	for len(r.textos) <= i {
		r.textos = append(r.textos, ui.Texto("", tema.TextoSM, ui.Normal, color.NRGBA{}))
	}
	return r.textos[i]
}

func (r *sugerenciasRenderer) Layout(t fyne.Size) {
	s := r.s
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	r.caja.Redimensionar(t)
	r.marca.Hide()
	var n int
	if len(s.items) > 0 {
		n = r.lista(t, v)
	} else {
		n = r.firma(t, v)
	}
	for i := n; i < len(r.textos); i++ {
		r.textos[i].Hide()
	}
	o := make([]fyne.CanvasObject, 0, 3+len(r.textos))
	o = append(o, r.caja.Objetos()...)
	o = append(o, r.marca)
	for _, tx := range r.textos {
		o = append(o, tx)
	}
	r.objetos = o
}

// lista coloca las filas del autocompletado. Las que se salen por el `max-h-56` no se
// pintan: la lista se desplaza con la rueda y la elegida siempre está a la vista.
func (r *sugerenciasRenderer) lista(t fyne.Size, v tema.Variante) int {
	s := r.s
	arriba := tema.Borde + RellenoLista
	abajo := t.Height - tema.Borde - RellenoLista
	n := 0
	for i, nombre := range s.items {
		y := arriba + float32(i)*AltoFilaAC - s.desY
		if y+AltoFilaAC <= arriba || y >= abajo {
			continue
		}
		if i == s.elegido {
			r.marca.FillColor = tema.Color("menu-selected", v)
			r.marca.Move(fyne.NewPos(tema.Borde, max(y, arriba)))
			r.marca.Resize(fyne.NewSize(t.Width-2*tema.Borde, min(y+AltoFilaAC, abajo)-max(y, arriba)))
			r.marca.Show()
		}
		etiqueta := r.texto(n)
		n++
		peso := ui.Medio
		if s.valores {
			peso = ui.Normal
		}
		etiqueta.Color, etiqueta.TextSize, etiqueta.FontSource = tema.Color("text", v), tema.TextoSM.Tam, peso.Cara()
		etiqueta.Text = nombre
		x := tema.Borde + MargenFilaAC
		ui.ColocarTexto(etiqueta, tema.TextoSM, x, y+RellenoFilaAC)
		etiqueta.Show()
		d := s.descripcion(i)
		if d == "" || s.valores {
			continue
		}
		// `items-baseline`: la descripción en `text-xs` comparte la línea base del nombre.
		desc := r.texto(n)
		n++
		desc.Color, desc.TextSize, desc.FontSource = tema.Color("text-muted", v), tema.TextoXS.Tam, ui.Normal.Cara()
		x += ui.Medir(nombre, tema.TextoSM, ui.Medio).Width + HuecoFilaAC
		ui.Truncar(desc, d, tema.TextoXS, ui.Normal, t.Width-tema.Borde-MargenFilaAC-x)
		base := (tema.TextoSM.Linea - tema.TextoXS.Linea) / 2
		ui.ColocarTexto(desc, tema.TextoXS, x, y+RellenoFilaAC+base)
		desc.Show()
	}
	return n
}

// firma coloca la pista: la firma en una línea y el resumen debajo, repartido.
func (r *sugerenciasRenderer) firma(_ fyne.Size, v tema.Variante) int {
	s := r.s
	if s.pista.Args == "" {
		return 0
	}
	textos, pesos, apagados := s.trozosFirma()
	x, y := tema.Borde+MargenFilaAC, tema.Borde+RellenoLista
	n := 0
	for i, tx := range textos {
		ct := r.texto(n)
		n++
		ct.Text, ct.TextSize, ct.FontSource = tx, tema.TextoXS.Tam, pesos[i].Cara()
		ct.Color = tema.Color("text", v)
		if apagados[i] {
			ct.Color = tema.Color("text-muted", v)
		}
		ui.ColocarTexto(ct, tema.TextoXS, x, y)
		ct.Show()
		x += ui.Medir(tx, tema.TextoXS, pesos[i]).Width
	}
	// El resumen, repartido en líneas al ancho de la caja (`max-w-[400px]`).
	y += tema.TextoXS.Linea + RellenoFilaAC
	for _, linea := range lineasResumen(s.pista.Resumen, s.anchoPista()) {
		ct := r.texto(n)
		n++
		ct.Text, ct.TextSize, ct.FontSource, ct.Color = linea, tema.TextoXS.Tam, ui.Normal.Cara(), tema.Color("text-muted", v)
		ui.ColocarTexto(ct, tema.TextoXS, tema.Borde+MargenFilaAC, y)
		ct.Show()
		y += tema.TextoXS.Linea
	}
	return n
}

func (r *sugerenciasRenderer) Refresh() {
	r.Layout(r.s.Size())
	for _, o := range r.objetos {
		o.Refresh()
	}
}
