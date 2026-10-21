// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// # El canalón del esquema
//
// Con grupos, la web abre un canalón a la izquierda de las cabeceras de fila (`RGUT`) y
// otro encima de las de columna (`CGUT`): un carril de 14 px por nivel, con la línea del
// grupo (`bg-border-strong`, 1 px) a lo largo de su detalle y el botón +/− en la fila o
// columna de su resumen, la de después del grupo. En la esquina, la barra de niveles
// 1…N+1. Todo corre el origen de las celdas sin cambiar su tamaño.
//
// Medidas, de SheetCanvas.tsx con 1 rem = 14 px:
//
//	carril       OUTLINE_STEP = 14, más OUTLINE_PAD = 5 de aire
//	+/−          h-3 w-3 (10,5) rounded-[2px] border bg-surface text-2xs leading-none
//	niveles      h-3.5 w-3.5 (12,25), gap-px entre ellos; la barra, justify-end px-0.5
//	celda        bg-bg-subtle border-r border-b, como las cabeceras

// PasoEsquema y AireEsquema son `OUTLINE_STEP` y `OUTLINE_PAD`.
const (
	PasoEsquema float32 = 14
	AireEsquema float32 = 5
)

var (
	ladoConmutador = tema.Espacio(3)   // h-3 w-3
	ladoNivel      = tema.Espacio(3.5) // h-3.5 w-3.5
	margenNiveles  = tema.Espacio(0.5) // px-0.5
)

// RadioBotonEsquema es el `rounded-[2px]` de los botones del canalón.
const RadioBotonEsquema float32 = 2

// anchoCanal es `RGUT`/`CGUT` para tantos niveles: 0 sin esquema.
func anchoCanal(niveles int) float32 {
	if niveles == 0 {
		return 0
	}
	return float32(niveles)*PasoEsquema + AireEsquema
}

// canal es el ancho del canalón de filas y el alto del de columnas, en px de la hoja.
func (r *Rejilla) canal() (cx, cy float32) {
	return anchoCanal(analisis.Niveles(r.L.GruposFilas())), anchoCanal(analisis.Niveles(r.L.GruposCols()))
}

// conmutadorEn es `laneToggle`: el grupo de ese nivel cuyo resumen es la unidad u (la de
// después del grupo, o la última del grupo si llega al final de la rejilla).
func conmutadorEn(gs []analisis.Grupo, u, nivel, tope int) *analisis.Grupo {
	for i, g := range gs {
		if g.Level == nivel && (g.End+1 == u || (u == tope-1 && g.End == u)) {
			return &gs[i]
		}
	}
	return nil
}

// enGrupo es `laneLine`: la unidad está en el detalle de algún grupo de ese nivel.
func enGrupo(gs []analisis.Grupo, u, nivel int) bool {
	for _, g := range gs {
		if g.Level == nivel && u >= g.Start && u <= g.End {
			return true
		}
	}
	return false
}

// botonPintado es un botón del canalón: la caja con su borde (dos rectángulos, como
// `ui.Caja`, para que la línea no salga a media fuerza) y su rótulo.
type botonPintado struct {
	fuera, dentro *canvas.Rectangle
	rotulo        *canvas.Text
}

func nuevoBotonPintado() *botonPintado {
	return &botonPintado{
		fuera: canvas.NewRectangle(Papel), dentro: canvas.NewRectangle(Papel),
		rotulo: ui.Texto("", tema.Texto2XS, ui.Normal, Tinta),
	}
}

func (b *botonPintado) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{b.fuera, b.dentro, b.rotulo}
}

func (b *botonPintado) esconder() {
	for _, o := range b.objetos() {
		o.Hide()
	}
}

// pintar pone el botón de lado `lado` (ya con el zoom) en (x, y) con su rótulo centrado.
func (b *botonPintado) pintar(v tema.Variante, x, y, lado float32, texto string, z float32) {
	bo := tema.Borde * z
	b.fuera.CornerRadius, b.dentro.CornerRadius = RadioBotonEsquema*z, max(RadioBotonEsquema-tema.Borde, 0)*z
	caja(b.fuera, tema.Color("border", v), x, y, lado, lado)
	caja(b.dentro, tema.Color("surface", v), x+bo, y+bo, lado-2*bo, lado-2*bo)
	esc := tema.Escalon{Tam: tema.Texto2XS.Tam * z, Linea: tema.Texto2XS.Tam * z} // leading-none
	b.rotulo.Text, b.rotulo.TextSize, b.rotulo.FontSource = texto, esc.Tam, tema.Regular
	b.rotulo.Color = tema.Color("text-muted", v)
	w := medirCon(b.rotulo, texto)
	ui.ColocarTexto(b.rotulo, esc, x+(lado-w)/2, y+(lado-esc.Linea)/2)
	b.rotulo.Show()
}

// zonaCanal es un botón pintado y lo que manda al pulsarlo. Lo que caiga a la izquierda
// de minX o por encima de minY está tapado por las cabeceras y no se pulsa.
type zonaCanal struct {
	pos        fyne.Position
	tam        fyne.Size
	minX, minY float32
	valor      string
}

// canalPintado son las primitivas del canalón. `cuerpo` va sobre las celdas y bajo las
// cabeceras —lo que se desplaza pasa por debajo de ellas—; `encima`, la esquina del
// canalón de columnas y las barras de niveles, sobre todo lo demás.
type canalPintado struct {
	filas, cols  []*cabeceraPintada
	lineas       []*canvas.Rectangle
	conmutadores []*botonPintado
	esquina      *cabeceraPintada
	niveles      []*botonPintado
	zonas        []zonaCanal
}

func (k *canalPintado) cuerpo() []fyne.CanvasObject {
	var o []fyne.CanvasObject
	for _, c := range k.filas {
		o = append(o, c.objetos()...)
	}
	for _, c := range k.cols {
		o = append(o, c.objetos()...)
	}
	for _, l := range k.lineas {
		o = append(o, l)
	}
	for _, b := range k.conmutadores {
		o = append(o, b.objetos()...)
	}
	return o
}

func (k *canalPintado) encima() []fyne.CanvasObject {
	var o []fyne.CanvasObject
	if k.esquina != nil {
		o = append(o, k.esquina.objetos()...)
	}
	for _, b := range k.niveles {
		o = append(o, b.objetos()...)
	}
	return o
}

// valorEsquema es el JSON de un botón: conmutar un grupo o enseñar hasta un nivel.
func valorEsquema(eje, op string, g *analisis.Grupo, nivel int) string {
	m := map[string]any{"axis": eje, "op": op, "level": nivel}
	if g != nil {
		m["start"], m["end"], m["level"] = g.Start, g.End, g.Level
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// usoCanal lleva la cuenta de las primitivas del canalón que se usan en un pintado: lo
// que sobre se esconde al final y, si un charco crece, hay que recomponer.
type usoCanal struct {
	k          *canalPintado
	v          tema.Variante
	nl, nb, nn int
	crecio     bool
}

func (u *usoCanal) linea() *canvas.Rectangle {
	if u.nl == len(u.k.lineas) {
		u.k.lineas, u.crecio = append(u.k.lineas, canvas.NewRectangle(Papel)), true
	}
	u.nl++
	return u.k.lineas[u.nl-1]
}

func (u *usoCanal) conmutador() *botonPintado {
	if u.nb == len(u.k.conmutadores) {
		u.k.conmutadores, u.crecio = append(u.k.conmutadores, nuevoBotonPintado()), true
	}
	u.nb++
	return u.k.conmutadores[u.nb-1]
}

func (u *usoCanal) nivel() *botonPintado {
	if u.nn == len(u.k.niveles) {
		u.k.niveles, u.crecio = append(u.k.niveles, nuevoBotonPintado()), true
	}
	u.nn++
	return u.k.niveles[u.nn-1]
}

func (u *usoCanal) celda(pool *[]*cabeceraPintada, i int) *cabeceraPintada {
	for len(*pool) <= i {
		*pool, u.crecio = append(*pool, nuevaCabecera(u.v)), true
	}
	return (*pool)[i]
}

// pintarCanal coloca el canalón de filas, el de columnas y sus barras de niveles.
func (rr *rejillaRenderer) pintarCanal(v tema.Variante, vw vista, cols, fils []int) {
	r, k := rr.r, &rr.canal
	k.zonas = k.zonas[:0]
	u := &usoCanal{k: k, v: v}
	gf, gc := r.L.GruposFilas(), r.L.GruposCols()
	nf, nc := analisis.Niveles(gf), analisis.Niveles(gc)
	rr.pintarCanalFilas(u, vw, fils, gf, nf)
	rr.pintarCanalCols(u, vw, cols, gc, nc)
	rr.pintarNiveles(u, vw, nf, nc)
	for _, l := range k.lineas[u.nl:] {
		l.Hide()
	}
	for _, bt := range k.conmutadores[u.nb:] {
		bt.esconder()
	}
	for _, bt := range k.niveles[u.nn:] {
		bt.esconder()
	}
	if u.crecio {
		rr.recomponer()
	}
}

// pintarCanalFilas es el de filas: una celda por fila, un carril por nivel, en
// horizontal.
func (rr *rejillaRenderer) pintarCanalFilas(u *usoCanal, vw vista, fils []int, gf []analisis.Grupo, nf int) {
	k, z, v := u.k, vw.z, u.v
	b, paso, lado := tema.Borde*z, PasoEsquema*z, ladoConmutador*z
	usadas := 0
	if nf > 0 {
		for _, f := range fils {
			c := u.celda(&k.filas, usadas)
			usadas++
			y, alto := vw.y(f), vw.alto()
			rr.cabecera(c, v, false, false, false, "", 0, y, vw.cx*z, alto, z)
			for l := range nf {
				x := float32(l) * paso
				if g := conmutadorEn(gf, f, l+1, Filas); g != nil {
					bx, by := x+(paso-lado)/2, y+(alto-b-lado)/2
					u.conmutador().pintar(v, bx, by, lado, signoEsquema(g.Collapsed), z)
					k.zonas = append(k.zonas, zonaCanal{
						fyne.NewPos(bx, by), fyne.NewSize(lado, lado), 0, vw.oy(),
						valorEsquema("row", "toggle", g, 0),
					})
				} else if enGrupo(gf, f, l+1) {
					caja(u.linea(), tema.Color("border-strong", v), x+(paso-b)/2, y, b, alto-b)
				}
			}
		}
	}
	for i := usadas; i < len(k.filas); i++ {
		k.filas[i].esconder()
	}
}

// pintarCanalCols es el de columnas: una celda por columna, un carril por nivel, en
// vertical.
func (rr *rejillaRenderer) pintarCanalCols(u *usoCanal, vw vista, cols []int, gc []analisis.Grupo, nc int) {
	k, z, v := u.k, vw.z, u.v
	b, paso, lado := tema.Borde*z, PasoEsquema*z, ladoConmutador*z
	usadas := 0
	if nc > 0 {
		for _, c := range cols {
			cp := u.celda(&k.cols, usadas)
			usadas++
			x, ancho := vw.x(c), vw.ancho()
			rr.cabecera(cp, v, false, false, false, "", x, 0, ancho, vw.cy*z, z)
			for l := range nc {
				y := float32(l) * paso
				if g := conmutadorEn(gc, c, l+1, Columnas); g != nil {
					bx, by := x+(ancho-b-lado)/2, y+(paso-lado)/2
					u.conmutador().pintar(v, bx, by, lado, signoEsquema(g.Collapsed), z)
					k.zonas = append(k.zonas, zonaCanal{
						fyne.NewPos(bx, by), fyne.NewSize(lado, lado), vw.ox(), 0,
						valorEsquema("col", "toggle", g, 0),
					})
				} else if enGrupo(gc, c, l+1) {
					caja(u.linea(), tema.Color("border-strong", v), x, y+(paso-b)/2, ancho-b, b)
				}
			}
		}
	}
	for i := usadas; i < len(k.cols); i++ {
		k.cols[i].esconder()
	}
}

// pintarNiveles pone la esquina del canalón de columnas y, en cada esquina, su barra
// de niveles 1…N+1, pegada a la derecha.
func (rr *rejillaRenderer) pintarNiveles(u *usoCanal, vw vista, nf, nc int) {
	k, z, v := u.k, vw.z, u.v
	b := tema.Borde * z
	if k.esquina == nil {
		k.esquina, u.crecio = nuevaCabecera(v), true
	}
	if nc > 0 {
		rr.cabecera(k.esquina, v, false, false, false, "", 0, 0, vw.ox(), vw.cy*z, z)
	} else {
		k.esquina.esconder()
	}
	barra := func(eje string, niveles int, y0, alto float32) {
		ln, hueco := ladoNivel*z, 1*z // gap-px
		x := vw.ox() - b - margenNiveles*z - float32(niveles+1)*ln - float32(niveles)*hueco
		y := y0 + (alto-b-ln)/2
		for n := 1; n <= niveles+1; n++ {
			u.nivel().pintar(v, x, y, ln, strconv.Itoa(n), z)
			k.zonas = append(k.zonas, zonaCanal{fyne.NewPos(x, y), fyne.NewSize(ln, ln), 0, 0, valorEsquema(eje, "level", nil, n)})
			x += ln + hueco
		}
	}
	if nf > 0 {
		barra("row", nf, vw.cy*z, vw.alto())
	}
	if nc > 0 {
		barra("col", nc, 0, vw.cy*z)
	}
}

// signoEsquema es el rótulo del +/−: «+» contraído, «−» abierto.
func signoEsquema(contraido bool) string {
	if contraido {
		return "+"
	}
	return "−"
}

// esquemaEn dice qué botón del canalón hay bajo un punto y el comando que manda. Mira lo
// último que se pintó: pulsar es pulsar lo que se ve.
func (r *Rejilla) esquemaEn(p fyne.Position) (string, bool) {
	if r.r == nil {
		return "", false
	}
	zs := r.r.canal.zonas
	for i := len(zs) - 1; i >= 0; i-- {
		z := zs[i]
		if dentro(p, z.pos, z.tam) && p.X >= z.minX && p.Y >= z.minY {
			return z.valor, true
		}
	}
	return "", false
}
