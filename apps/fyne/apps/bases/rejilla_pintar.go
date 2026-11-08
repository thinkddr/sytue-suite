// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// rejillaRenderer pinta la rejilla con el lenguaje visual de aspecto.go: el glifo de tipo
// en cada cabecera, chips del color de su opción, números alineados a la derecha, filas
// alternas y la fila bajo el puntero resaltada.
type rejillaRenderer struct {
	g       *rejilla
	version int

	marco, cabeza, anillo, ancla *canvas.Rectangle
	// resalte es el recuadro de «suelta aquí», encima de todo: las filas tapaban el borde.
	resalte *canvas.Rectangle
	// fondos[fila] es el fondo de la fila: alterno, bajo el puntero o con el foco.
	fondos           []*canvas.Rectangle
	lineas           []*canvas.Rectangle
	glifos           []fyne.CanvasObject
	titulos, numeros []*canvas.Text
	abrir            []*ui.Boton
	// textos[fila][col] es el texto de la celda; nil donde va una casilla o chips.
	textos   [][]*canvas.Text
	casillas map[celda]fyne.CanvasObject
	chips    map[celda][]*ui.Insignia
	masChips map[celda]*ui.Insignia
	objetos  []fyne.CanvasObject

	anchos       []float32
	ancho0, alto float32
	ladoBoton    float32
	estirado     float32
	conEditor    *ui.Entrada
	variante     tema.Variante
}

func (r *rejillaRenderer) Destroy()                     {}
func (r *rejillaRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *rejillaRenderer) poner() {
	if r.version != r.g.version || r.variante != tema.De(fyne.CurrentApp().Settings().ThemeVariant()) {
		r.armar()
	}
}

// armar crea los objetos de la cabecera y de cada fila, y mide las columnas.
func (r *rejillaRenderer) armar() {
	g := r.g
	r.version = g.version
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.variante = v
	tinta, apagada := tema.Color("text", v), tema.Color("text-muted", v)

	r.marco = canvas.NewRectangle(tema.Color("surface", v))
	r.marco.CornerRadius, r.marco.StrokeWidth, r.marco.StrokeColor = tema.RadioLG, tema.Borde, tema.Color("border", v)
	r.cabeza = canvas.NewRectangle(tema.Color("bg-subtle", v))
	// Solo arriba, con la esquina de dentro del marco: entera dejaba una muesca abajo.
	r.cabeza.TopLeftCornerRadius, r.cabeza.TopRightCornerRadius = esquinaDentro, esquinaDentro
	r.anillo = canvas.NewRectangle(color.Transparent)
	r.anillo.StrokeWidth, r.anillo.StrokeColor = tema.AnilloFoco, tema.Color("focus-ring", v)
	r.ancla = canvas.NewRectangle(color.Transparent)
	r.resalte = canvas.NewRectangle(color.Transparent)
	r.resalte.CornerRadius, r.resalte.StrokeWidth, r.resalte.StrokeColor = tema.RadioLG, tema.AnilloFoco, tema.Color("focus-ring", v)
	r.objetos = []fyne.CanvasObject{r.marco, r.cabeza}

	muestra := ui.NuevoBoton("", nil).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Maximizar, 14)
	muestra.SoloIcono = true
	r.ladoBoton = ui.MedidaBotonDe(muestra).Height
	r.alto = 2*tema.Espacio(1.5) + r.ladoBoton

	r.anchos = make([]float32, len(g.Campos))
	r.titulos, r.glifos = r.titulos[:0], r.glifos[:0]
	for i, c := range g.Campos {
		glifo := iconos.Trazo(iconoDeTipo(c.Kind), ladoIconoCabeza, "text-muted", v).Objeto()
		titulo := ui.Texto(c.Name, tema.TextoSM, ui.SemiNegrita, tinta)
		r.glifos, r.titulos = append(r.glifos, glifo), append(r.titulos, titulo)
		r.objetos = append(r.objetos, glifo, titulo)
		r.anchos[i] = ladoIconoCabeza + huecoCabeza + ui.Medir(c.Name, tema.TextoSM, ui.SemiNegrita).Width
	}

	r.fondos, r.lineas, r.numeros, r.abrir, r.textos = r.fondos[:0], r.lineas[:0], r.numeros[:0], r.abrir[:0], r.textos[:0]
	r.casillas = map[celda]fyne.CanvasObject{}
	r.chips = map[celda][]*ui.Insignia{}
	r.masChips = map[celda]*ui.Insignia{}
	anchoNumero := r.armarFilas(tinta, apagada)
	for i := range r.anchos {
		r.anchos[i] = min(max(r.anchos[i]+2*margenCelda, minCelda), maxCelda)
	}
	r.ancho0 = 2*margenFila0 + anchoNumero
	if g.AlAbrir != nil {
		r.ancho0 += tema.Espacio(1) + r.ladoBoton
	}
	r.ancho0 = max(r.ancho0, tema.Espacio(12))
	r.objetos = append(r.objetos, r.ancla, r.anillo, r.resalte)
	r.conEditor = nil
	if g.editando && g.editor != nil {
		r.conEditor = g.editor
		r.objetos = append(r.objetos, g.editor)
	}
	for fila := range g.Registros {
		r.pintarFila(fila)
	}
}

func (r *rejillaRenderer) armarFilas(tinta, apagada color.NRGBA) float32 {
	g, v := r.g, r.variante
	var anchoNumero float32
	for fila, reg := range g.Registros {
		fondo := canvas.NewRectangle(color.Transparent)
		if fila == len(g.Registros)-1 {
			// La última fila no sale por las esquinas de abajo del marco.
			fondo.BottomLeftCornerRadius, fondo.BottomRightCornerRadius = esquinaDentro, esquinaDentro
		}
		linea := canvas.NewRectangle(tema.Color("border", v))
		n := strconv.Itoa(g.Desde + fila + 1)
		numero := ui.Texto(n, tema.TextoXS, ui.Normal, apagada)
		anchoNumero = max(anchoNumero, ui.Medir(n, tema.TextoXS, ui.Normal).Width)
		r.fondos, r.lineas, r.numeros = append(r.fondos, fondo), append(r.lineas, linea), append(r.numeros, numero)
		// Los fondos van debajo de todo lo de la fila.
		r.objetos = append(r.objetos, fondo, linea, numero)
		if g.AlAbrir != nil {
			b := ui.NuevoBoton(g.t("grid.row", i18n.Vars{"n": g.Desde + fila + 1}), func() { g.AlAbrir(reg) }).
				Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Maximizar, 14)
			b.SoloIcono = true
			r.abrir = append(r.abrir, b)
			r.objetos = append(r.objetos, b)
		}
		textos := make([]*canvas.Text, len(g.Campos))
		for col, c := range g.Campos {
			switch {
			case c.Kind == api.BasesKindCheckbox:
				o := casilla(reg.Data[c.ID] == true, v)
				r.casillas[celda{fila, col}] = o
				r.objetos = append(r.objetos, o)
			case conChips(c):
				var ancho float32
				piezas := g.piezas(reg, c)
				for i, p := range piezas {
					ch := ui.NuevaInsignia(p.texto, p.tono)
					r.chips[celda{fila, col}] = append(r.chips[celda{fila, col}], ch)
					r.objetos = append(r.objetos, ch)
					if i > 0 {
						ancho += huecoChips
					}
					ancho += ui.MedidaInsignia(p.texto).Width
				}
				if len(piezas) > 1 {
					mas := ui.NuevaInsignia("", ui.InsigniaNeutra)
					r.masChips[celda{fila, col}] = mas
					r.objetos = append(r.objetos, mas)
				}
				r.anchos[col] = max(r.anchos[col], ancho)
			default:
				completo := g.textoCelda(reg, c)
				textos[col] = ui.Texto(completo, tema.TextoSM, ui.Normal, tinta)
				r.objetos = append(r.objetos, textos[col])
				r.anchos[col] = max(r.anchos[col], ui.Medir(completo, tema.TextoSM, ui.Normal).Width)
			}
		}
		r.textos = append(r.textos, textos)
	}
	return anchoNumero
}

// pintarFila da a una fila su fondo: el de la fila con el foco, el de la fila bajo el
// puntero, el alterno (`even:bg-subtle/40`) o ninguno. Solo cambia un color: no recoloca.
func (r *rejillaRenderer) pintarFila(fila int) {
	if fila < 0 || fila >= len(r.fondos) {
		return
	}
	g, v := r.g, r.variante
	var c color.NRGBA
	switch {
	case g.enfocada && g.foco.fila == fila:
		c = tema.Color("primary", v)
		c.A = 22
	case g.hayEncima && g.filaEncima == fila:
		c = tema.Color("bg-subtle", v)
	case fila%2 == 1:
		c = tema.Color("bg-subtle", v)
		c.A /= 2
	}
	r.fondos[fila].FillColor = c
	r.fondos[fila].Refresh()
}

// casilla es la casilla de una celda: se ve y no se pulsa (`pointer-events-none`); la
// marca Espacio, Intro o el doble clic sobre la celda.
func casilla(marcada bool, v tema.Variante) fyne.CanvasObject {
	caja := canvas.NewRectangle(tema.Color("surface", v))
	caja.CornerRadius, caja.StrokeWidth, caja.StrokeColor = tema.RadioSM, tema.Borde, tema.Color("border-strong", v)
	caja.Resize(fyne.NewSquareSize(ladoCasilla))
	if !marcada {
		return container.NewWithoutLayout(caja)
	}
	caja.FillColor, caja.StrokeColor = tema.Color("primary", v), tema.Color("primary", v)
	tic := iconos.Trazo(iconos.Tic, ladoCasilla-4, "on-primary", v).Objeto()
	tic.Move(fyne.NewPos(2, 2))
	tic.Resize(fyne.NewSquareSize(ladoCasilla - 4))
	return container.NewWithoutLayout(caja, tic)
}

func (r *rejillaRenderer) MinSize() fyne.Size {
	r.poner()
	ancho := r.ancho0
	for _, a := range r.anchos {
		ancho += a
	}
	return fyne.NewSize(ancho, altoCabeza+float32(len(r.g.Registros))*r.alto)
}

// columna es dónde empieza la columna `col` y cuánto mide, ya estirada.
func (r *rejillaRenderer) columna(col int) (x, ancho float32) {
	x = r.ancho0
	for i := 0; i < col; i++ {
		x += r.anchos[i] + r.estirado
	}
	return x, r.anchos[col] + r.estirado
}

// cajaDe es la caja de una celda dentro de la rejilla.
func (r *rejillaRenderer) cajaDe(c celda) (fyne.Position, fyne.Size) {
	if c.col < 0 || c.col >= len(r.anchos) {
		return fyne.Position{}, fyne.Size{}
	}
	x, ancho := r.columna(c.col)
	return fyne.NewPos(x, altoCabeza+float32(c.fila)*r.alto), fyne.NewSize(ancho, r.alto)
}

func (r *rejillaRenderer) celdaEn(p fyne.Position) (celda, bool) {
	if p.Y < altoCabeza || p.X < r.ancho0 || r.alto <= 0 {
		return celda{}, false
	}
	fila := int((p.Y - altoCabeza) / r.alto)
	if fila >= len(r.g.Registros) {
		return celda{}, false
	}
	for col := range r.anchos {
		if x, ancho := r.columna(col); p.X >= x && p.X < x+ancho {
			return celda{fila, col}, true
		}
	}
	return celda{}, false
}

func (r *rejillaRenderer) Layout(t fyne.Size) {
	r.poner()
	g := r.g
	r.estirado = 0
	if sobra := t.Width - r.MinSize().Width; sobra > 0 && len(r.anchos) > 0 {
		r.estirado = sobra / float32(len(r.anchos))
	}
	interior := max(t.Width-2*tema.Borde, 0)
	r.marco.Resize(t)
	r.resalte.Resize(t)
	r.resalte.Hidden = !g.resaltada
	r.cabeza.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.cabeza.Resize(fyne.NewSize(interior, altoCabeza-tema.Borde))
	yGlifo := (altoCabeza - ladoIconoCabeza) / 2
	for i := range g.Campos {
		x, ancho := r.columna(i)
		xg := x + margenCelda
		medido := ui.Truncar(r.titulos[i], g.Campos[i].Name, tema.TextoSM, ui.SemiNegrita, max(ancho-2*margenCelda-ladoIconoCabeza-huecoCabeza, 0))
		if alineaALaDerecha(g.Campos[i]) {
			// `justify-end`: la cabecera de un número va sobre sus cifras.
			xg = x + ancho - margenCelda - medido - huecoCabeza - ladoIconoCabeza
		}
		r.glifos[i].Move(fyne.NewPos(xg, yGlifo))
		r.glifos[i].Resize(fyne.NewSquareSize(ladoIconoCabeza))
		ui.ColocarTexto(r.titulos[i], tema.TextoSM, xg+ladoIconoCabeza+huecoCabeza, tema.Espacio(2))
	}
	yTexto := (r.alto - tema.TextoSM.Linea) / 2
	for fila := range g.Registros {
		y := altoCabeza + float32(fila)*r.alto
		r.fondos[fila].Move(fyne.NewPos(tema.Borde, y))
		r.fondos[fila].Resize(fyne.NewSize(interior, r.alto))
		r.lineas[fila].Move(fyne.NewPos(tema.Borde, y))
		r.lineas[fila].Resize(fyne.NewSize(interior, tema.Borde))
		ui.ColocarTexto(r.numeros[fila], tema.TextoXS, margenFila0, y+(r.alto-tema.TextoXS.Linea)/2)
		if fila < len(r.abrir) {
			b := r.abrir[fila]
			b.Move(fyne.NewPos(r.ancho0-margenFila0-r.ladoBoton, y+(r.alto-r.ladoBoton)/2))
			b.Resize(fyne.NewSquareSize(r.ladoBoton))
		}
		for col, c := range g.Campos {
			x, ancho := r.columna(col)
			cd := celda{fila, col}
			if o, ok := r.casillas[cd]; ok {
				o.Move(fyne.NewPos(x+margenCelda, y+(r.alto-ladoCasilla)/2))
				o.Resize(fyne.NewSquareSize(ladoCasilla))
				continue
			}
			if chips, ok := r.chips[cd]; ok {
				r.colocarChips(chips, r.masChips[cd], x+margenCelda, x+ancho-margenCelda, y)
				continue
			}
			txt := r.textos[fila][col]
			if txt == nil {
				continue
			}
			disponible := ancho - 2*margenCelda
			medido := ui.Truncar(txt, g.textoCelda(g.Registros[fila], c), tema.TextoSM, ui.Normal, disponible)
			xt := x + margenCelda
			if alineaALaDerecha(c) {
				xt = x + ancho - margenCelda - medido
			}
			ui.ColocarTexto(txt, tema.TextoSM, xt, y+yTexto)
		}
	}
	pos, tam := r.cajaDe(g.foco)
	r.ancla.Move(pos)
	r.ancla.Resize(tam)
	r.anillo.Move(pos)
	r.anillo.Resize(tam)
	if g.enfocada && !g.editando && len(g.Registros) > 0 && len(g.Campos) > 0 {
		r.anillo.Show()
	} else {
		r.anillo.Hide()
	}
	if g.editando && g.editor != nil {
		alto := g.editor.MinSize().Height
		g.editor.Move(fyne.NewPos(pos.X+tema.Espacio(1), pos.Y+(tam.Height-alto)/2))
		g.editor.Resize(fyne.NewSize(max(tam.Width-2*tema.Espacio(1), 0), alto))
	}
}

// colocarChips reserva un contador para las opciones que no caben.
func (r *rejillaRenderer) colocarChips(chips []*ui.Insignia, mas *ui.Insignia, x0, x1, y float32) {
	var completo float32
	for i, ch := range chips {
		if i > 0 {
			completo += huecoChips
		}
		completo += ch.MinSize().Width
	}
	if mas != nil {
		mas.Hide()
	}
	x := x0
	vistos := 0
	for i, ch := range chips {
		tam := ch.MinSize()
		cabe := x1 - x
		if completo > x1-x0 && i < len(chips)-1 {
			cabe -= huecoChips + ui.MedidaInsignia("+"+strconv.Itoa(len(chips)-i-1)).Width
		}
		// La primera etiqueta puede truncarse; las siguientes se cuentan si no caben.
		if cabe <= 0 || (tam.Width > cabe && (i > 0 || cabe < 2*ui.MargenInsignia+tema.TextoXS.Tam)) {
			for _, restante := range chips[i:] {
				restante.Hide()
			}
			break
		}
		ch.Show()
		ancho := min(tam.Width, cabe)
		ch.Move(fyne.NewPos(x, y+(r.alto-tam.Height)/2))
		ch.Resize(fyne.NewSize(ancho, tam.Height))
		x += ancho + huecoChips
		vistos++
	}
	if mas != nil && vistos < len(chips) {
		mas.Texto = "+" + strconv.Itoa(len(chips)-vistos)
		tam := mas.MinSize()
		mas.Move(fyne.NewPos(x, y+(r.alto-tam.Height)/2))
		mas.Resize(fyne.NewSize(min(tam.Width, max(x1-x, 0)), tam.Height))
		mas.Refresh()
		mas.Show()
	}
}

func (r *rejillaRenderer) Refresh() {
	var editor *ui.Entrada
	if r.g.editando {
		editor = r.g.editor
	}
	if r.version != r.g.version || r.conEditor != editor || r.variante != tema.De(fyne.CurrentApp().Settings().ThemeVariant()) {
		r.armar()
	} else {
		for fila := range r.g.Registros {
			r.pintarFila(fila)
		}
	}
	r.Layout(r.g.Size())
	canvas.Refresh(r.g)
}
