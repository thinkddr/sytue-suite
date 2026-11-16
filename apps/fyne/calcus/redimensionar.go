// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Cambiar a mano el ancho de una columna o el alto de una fila, como la web: arrastrando
// el borde de su cabecera, con doble clic en él para ajustarla a lo escrito, o con los
// comandos `colwidth`/`rowheight` de la cinta (Inicio › Celdas), que piden el valor
// exacto. Lo que se guarda es lo de siempre: `colWidths`/`rowHeights`, solo lo que se
// aparta de la medida de serie.

// Los comandos de las medidas, con el nombre y el valor de la web: «{px}» o «auto».
const (
	CmdAnchoCol editor.Comando = "colwidth"
	CmdAltoFila editor.Comando = "rowheight"
)

// MedioAsaMedida es medio `w-1` del asa del borde de una cabecera (`-right-0.5`: va
// centrada en la raya), en px de maqueta.
const MedioAsaMedida float32 = 2

// Medidas son los anchos y altos propios de la hoja activa (px de maqueta, clave el índice
// 0-based). Son de la foto: no se tocan, se sustituyen.
func (l *Libro) Medidas() (anchos, altos map[string]float64) { return l.h().anchos, l.h().altos }

// MedidaDe es lo que mide la columna (col) o la fila i de la hoja activa, oculta o no.
func (l *Libro) MedidaDe(col bool, i int) float32 {
	if col {
		return medidaDe(l.h().anchos, i, AnchoCelda, AnchoMax)
	}
	return medidaDe(l.h().altos, i, AltoCelda, AltoMax)
}

// PonerMedidas es `medidasPedidas`: pone a `indices` la medida pedida (unos px, o la de
// autoajuste de cada uno). Devuelve si cambió algo; sin cambio no se anota, para que
// deshacer no gaste un paso en nada.
func (l *Libro) PonerMedidas(col bool, indices []int, p medidaPedida) bool {
	h := l.h()
	serie, tope, medidas := AltoCelda, AltoMax, h.altos
	if col {
		serie, tope, medidas = AnchoCelda, AnchoMax, h.anchos
	}
	nuevas := medidas
	if p.auto {
		for _, i := range indices {
			px := float64(l.medidaAjustada(col, i))
			nuevas = conMedidas(nuevas, []int{i}, &px, serie, tope)
		}
	} else {
		px := p.px
		nuevas = conMedidas(medidas, indices, &px, serie, tope)
	}
	if mismasMedidas(medidas, nuevas) {
		return false
	}
	l.anotar()
	l.version++
	if col {
		h.anchos = nuevas
	} else {
		h.altos = nuevas
	}
	return true
}

// mismasMedidas compara dos mapas de medidas: nil y vacío son lo mismo.
func mismasMedidas(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// medidaAjustada es el autoajuste de la columna o la fila i (`usadoEn` y `anchoAjustado`
// o `altoAjustado` de la web): lo que mide el texto más ancho que se pinta en la columna,
// con su letra, o la línea de la letra mayor de la fila. Las combinadas que abarcan más
// de una unidad no cuentan, como en Excel.
func (l *Libro) medidaAjustada(col bool, i int) float32 {
	h := l.h()
	abarcaVarias := combinadasDeVarias(h.fusiones, col)
	var anchos []float32
	var tamanos []float64
	for k, html := range h.celdas {
		if (col && k.C != i) || (!col && k.F != i) || abarcaVarias(k.C, k.F) {
			continue
		}
		if col {
			if ancho, ok := l.anchoDelTexto(k.C, k.F); ok {
				anchos = append(anchos, ancho)
			}
		} else if e := l.Estilo(k.C, k.F); html != "" && e.Tam > 0 {
			tamanos = append(tamanos, float64(e.Tam))
		}
	}
	if col {
		return anchoAjustado(anchos, AnchoCelda)
	}
	return altoAjustado(tamanos, AltoCelda)
}

// combinadasDeVarias dice si una celda cae en una combinada que abarca más de una columna
// (col) o más de una fila.
func combinadasDeVarias(fusiones []string, col bool) func(c, f int) bool {
	var rs []formula.Rect
	for _, m := range fusiones {
		if r, ok := formula.ParseRangeRef(m); ok && (col && r.C1 != r.C2 || !col && r.R1 != r.R2) {
			rs = append(rs, r)
		}
	}
	return func(c, f int) bool {
		return slices.ContainsFunc(rs, func(r formula.Rect) bool { return c >= r.C1 && c <= r.C2 && f >= r.R1 && f <= r.R2 })
	}
}

// anchoDelTexto es lo que mide lo que se pinta en la celda, con su letra (`medirTexto`).
func (l *Libro) anchoDelTexto(c, f int) (float32, bool) {
	texto, _ := l.Pintura(c, f)
	if texto == "" {
		return 0, false
	}
	e := l.Estilo(c, f)
	negrita, cursiva, _ := l.Formato(c, f)
	cara, estilo := caraCelda(e.Fuente, negrita, cursiva)
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(texto, escalonCelda(e, 1).Tam, estilo, cara)
	return tam.Width, true
}

// mandarMedida ejecuta `colwidth` y `rowheight` sobre TODA la selección, como Excel. Un
// valor que no se lee avisa y no hace nada. false si no es uno de ellos.
func (c *Lienzo) mandarMedida(cmd editor.Comando, valor string, c1, f1, c2, f2 int) bool {
	if cmd != CmdAnchoCol && cmd != CmdAltoFila {
		return false
	}
	p, ok := leerMedida(valor)
	if !ok {
		if c.AlAviso != nil {
			c.AlAviso(ui.AvisoAdvertencia, c.t("sheet.resize.invalid", nil), "")
		}
		return false
	}
	col := cmd == CmdAnchoCol
	a, b := f1, f2
	if col {
		a, b = c1, c2
	}
	indices := make([]int, 0, b-a+1)
	for i := a; i <= b; i++ {
		indices = append(indices, i)
	}
	return c.Libro.PonerMedidas(col, indices, p)
}

// pedirMedida es `pedirMedida` de InicioSheets: pregunta la medida (px, o «auto» para
// ajustarla a lo escrito) con la de la celda ancla puesta, y la manda.
func (c *Lienzo) pedirMedida(ctx *editor.Contexto, cmd editor.Comando) {
	// Con la hoja protegida sin permiso de formato, se avisa antes de preguntar nada:
	// Mandar lo vetaría igual, pero tras teclear la medida en balde.
	if c.Libro.vetado(cmd) {
		c.avisarProtegida(c.Rejilla.sel)
		return
	}
	col := cmd == CmdAnchoCol
	actual := c.Libro.MedidaDe(col, map[bool]int{true: c.Rejilla.sel.C, false: c.Rejilla.sel.F}[col])
	clave := "sheet.rowHeight.prompt"
	if col {
		clave = "sheet.colWidth.prompt"
	}
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c.Rejilla)
	if lienzo == nil {
		return
	}
	ui.Pregunta(ctx.Traducir(clave), strconv.FormatFloat(float64(actual), 'f', -1, 32),
		ctx.Traducir("ui.cancel"), ctx.Traducir("ui.save"), ctx.Traducir("ui.required"), func(v string) {
			if strings.TrimSpace(v) != "" {
				ctx.Mandar(cmd, v)
			}
		}).Mostrar(lienzo)
}

// botonMedida es el «Ancho de columna…» o el «Alto de fila…» de Inicio › Celdas.
func (c *Lienzo) botonMedida(ctx *editor.Contexto, cmd editor.Comando) *editor.BotonCinta {
	icono, clave := iconos.EdAltoFila, "sheet.rowHeight"
	if cmd == CmdAnchoCol {
		icono, clave = iconos.EdAnchoCol, "sheet.colWidth"
	}
	b := editor.NuevoBotonCinta(icono, ctx.Traducir(clave), func() { c.pedirMedida(ctx, cmd) })
	b.Inerte = ctx.SoloLectura
	return b
}

// ---- el asa del borde de la cabecera ----

// gestoMedida es un arrastre del asa: qué unidad, lo que medía al empezar, dónde se pulsó
// y lo que mide ahora (la previa, que la rejilla ya pinta con esa medida).
type gestoMedida struct {
	col    bool
	i      int
	base   float32
	inicio fyne.Position
	px     float32
}

// asaMedidaEn dice si bajo un punto está el asa del borde final de la cabecera de una
// columna (col) o de una fila, y de cuál. Con el borde entre dos, manda la de antes: es
// la que se redimensiona, como en la web, donde el asa es de la cabecera de la izquierda.
func (r *Rejilla) asaMedidaEn(p fyne.Position) (col bool, i int, ok bool) {
	vw := r.vista()
	z, cx, cy := vw.z, vw.cx*vw.z, vw.cy*vw.z
	medio := MedioAsaMedida * z
	altoCab, anchoCab := AltoCelda*z, AnchoCabecera*z
	switch {
	case p.Y >= cy && p.Y < cy+altoCab && p.X >= vw.ox()-medio:
		if c, ok := r.bordeMasCercano(true, p.X, medio); ok {
			return true, c, true
		}
	case p.X >= cx && p.X < cx+anchoCab && p.Y >= vw.oy()-medio:
		if f, ok := r.bordeMasCercano(false, p.Y, medio); ok {
			return false, f, true
		}
	}
	return false, 0, false
}

// bordeMasCercano busca la columna (o la fila) cuyo borde final cae a menos de `medio` de
// la coordenada: la que hay debajo o la que se ve justo antes.
func (r *Rejilla) bordeMasCercano(col bool, v, medio float32) (int, bool) {
	vw := r.vista()
	e, total, pos, medida := vw.filas, r.Fils, vw.y, vw.alto
	en := r.filaEn
	if col {
		e, total, pos, medida = vw.cols, r.Cols, vw.x, vw.ancho
		en = r.colEn
	}
	i := en(v)
	if i < 0 || i >= total {
		return 0, false
	}
	if fin := pos(i) + medida(i); v >= fin-medio && v <= fin+medio {
		return i, true
	}
	// La de antes es la que se VE a la izquierda (o encima), no la del puesto anterior:
	// con paneles inmovilizados y desplazado, esa queda tapada por la última fija.
	if e.puesto(i) > 0 && v <= pos(i)+medio {
		if antes := en(pos(i) - medio); antes != i && antes >= 0 && antes < total {
			return antes, true
		}
	}
	return 0, false
}

// empezarMedida arranca el arrastre si se pulsa en el asa (y se puede editar).
func (r *Rejilla) empezarMedida(p fyne.Position) bool {
	if r.SoloLectura || r.editando {
		return false
	}
	col, i, ok := r.asaMedidaEn(p)
	if !ok {
		return false
	}
	base := r.L.MedidaDe(col, i)
	r.medida = &gestoMedida{col: col, i: i, base: base, inicio: p, px: base}
	return true
}

// arrastrarMedida sigue el arrastre: la medida nueva, acotada y redondeada como la web, se
// pinta ya mientras se arrastra.
func (r *Rejilla) arrastrarMedida(p fyne.Position) {
	g := r.medida
	d := (p.X - g.inicio.X) / r.z()
	tope := AnchoMax
	if !g.col {
		d, tope = (p.Y-g.inicio.Y)/r.z(), AltoMax
	}
	px := float32(math.Round(float64(min(tope, max(MinMedida, g.base+d)))))
	if px != g.px {
		g.px = px
		r.Refresh()
	}
}

// soltarMedida cierra el arrastre: si cambió, la guarda.
func (r *Rejilla) soltarMedida() {
	g := r.medida
	r.medida = nil
	if g.px != g.base {
		r.redimensionar(g.col, g.i, medidaPedida{px: float64(g.px)})
	}
	r.Refresh()
}

// unidadesDelGesto son las unidades a las que va un gesto sobre la cabecera i: toda la
// selección si i está en ella y cubre columnas (o filas) ENTERAS —el gesto de Excel—; si
// no, solo esa.
func (r *Rejilla) unidadesDelGesto(col bool, i int) []int {
	c1, f1, c2, f2 := r.Rango()
	enteras, a, b := c1 == 0 && c2 >= r.Cols-1, f1, f2
	if col {
		enteras, a, b = f1 == 0 && f2 >= r.Fils-1, c1, c2
	}
	if !enteras || i < a || i > b {
		return []int{i}
	}
	out := make([]int, 0, b-a+1)
	for k := a; k <= b; k++ {
		out = append(out, k)
	}
	return out
}

// redimensionar aplica un gesto de la cabecera (arrastre o doble clic). Con la hoja
// protegida, solo si se concedió el formato, como los comandos.
func (r *Rejilla) redimensionar(col bool, i int, p medidaPedida) {
	cmd := CmdAltoFila
	if col {
		cmd = CmdAnchoCol
	}
	if r.L.vetado(cmd) {
		if r.AlProtegida != nil {
			r.AlProtegida(r.sel)
		}
		return
	}
	if r.L.PonerMedidas(col, r.unidadesDelGesto(col, i), p) {
		r.desX, r.desY = clampF(r.desX, 0, r.maxDesX()), clampF(r.desY, 0, r.maxDesY())
		if r.AlEditar != nil {
			r.AlEditar()
		}
	}
	r.Refresh()
}

// cursorMedida es el cursor del asa bajo el puntero, o false si no hay asa.
func (r *Rejilla) cursorMedida() (desktop.Cursor, bool) {
	if r.medida != nil {
		return cursorDeEje(r.medida.col), true
	}
	if r.SoloLectura || r.puntero == nil {
		return nil, false
	}
	if col, _, ok := r.asaMedidaEn(*r.puntero); ok {
		return cursorDeEje(col), true
	}
	return nil, false
}

func cursorDeEje(col bool) desktop.Cursor {
	if col {
		return desktop.HResizeCursor
	}
	return desktop.VResizeCursor
}

// conPrevia son las medidas del eje con la del arrastre en curso ya puesta.
func (r *Rejilla) conPrevia(col bool, medidas map[string]float64) map[string]float64 {
	g := r.medida
	if g == nil || g.col != col {
		return medidas
	}
	serie, tope := AltoCelda, AltoMax
	if col {
		serie, tope = AnchoCelda, AnchoMax
	}
	// A todas las que va a ir al soltar: con columnas enteras seleccionadas, la previa
	// enseña ya cómo quedan todas, no solo la del borde que se arrastra.
	px := float64(g.px)
	return conMedidas(medidas, r.unidadesDelGesto(col, g.i), &px, serie, tope)
}
