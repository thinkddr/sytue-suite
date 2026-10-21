// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"slices"
	"sort"
)

// Las operaciones de los tres paneles de debajo del escenario, sobre la caja elegida:
// los datos del gráfico, los elementos del diagrama y la secuencia de animaciones. Son
// las de SlidesCanvas.tsx una a una (`setChartCell`, `addSmartItem`, `addBoxAnim`…).

// ---- gráfico ----

// columnasRejilla es el ancho de la rejilla: la fila más larga, y al menos dos.
func columnasRejilla(g *Grafico) int {
	n := 0
	for _, f := range g.Rejilla {
		n = max(n, len(f))
	}
	return max(2, n)
}

func (m *Modelo) grafico(i int, id string) *Grafico {
	if c := m.caja(i, id); c != nil {
		return c.Grafico
	}
	return nil
}

// PonerCelda es `setChartCell`: escribe una celda rellenando los huecos.
func (m *Modelo) PonerCelda(i int, id string, f, c int, v string) {
	g := m.grafico(i, id)
	if g == nil || f < 0 || c < 0 {
		return
	}
	for len(g.Rejilla) <= f {
		g.Rejilla = append(g.Rejilla, []string{})
	}
	for len(g.Rejilla[f]) <= c {
		g.Rejilla[f] = append(g.Rejilla[f], "")
	}
	g.Rejilla[f][c] = v
}

// AñadirFila es `addChartRow`: una fila vacía del ancho de la rejilla.
func (m *Modelo) AñadirFila(i int, id string) {
	if g := m.grafico(i, id); g != nil {
		g.Rejilla = append(g.Rejilla, make([]string, columnasRejilla(g)))
	}
}

// AñadirColumna es `addChartCol`.
func (m *Modelo) AñadirColumna(i int, id string) {
	if g := m.grafico(i, id); g != nil {
		for f := range g.Rejilla {
			g.Rejilla[f] = append(g.Rejilla[f], "")
		}
	}
}

// QuitarFila es `delChartRow`: la última, si quedan más de dos.
func (m *Modelo) QuitarFila(i int, id string) {
	if g := m.grafico(i, id); g != nil && len(g.Rejilla) > 2 {
		g.Rejilla = g.Rejilla[:len(g.Rejilla)-1]
	}
}

// QuitarColumna es `delChartCol`: la última, si quedan más de dos.
func (m *Modelo) QuitarColumna(i int, id string) {
	g := m.grafico(i, id)
	if g == nil {
		return
	}
	n := 0
	for _, f := range g.Rejilla {
		n = max(n, len(f))
	}
	if n <= 2 {
		return
	}
	for f := range g.Rejilla {
		g.Rejilla[f] = g.Rejilla[f][:min(len(g.Rejilla[f]), n-1)]
	}
}

// ---- diagrama ----

func (m *Modelo) diagrama(i int, id string) *Diagrama {
	if c := m.caja(i, id); c != nil {
		return c.Diagrama
	}
	return nil
}

// nivelDe es el `level ?? 0` de un elemento.
func nivelDe(e Elemento) float64 {
	if e.Nivel == nil {
		return 0
	}
	return *e.Nivel
}

// PonerTextoElemento es `setSmartText`.
func (m *Modelo) PonerTextoElemento(i int, id string, k int, texto string) {
	if d := m.diagrama(i, id); d != nil && k >= 0 && k < len(d.Elementos) {
		d.Elementos[k].Texto = texto
	}
}

// AñadirElemento es `addSmartItem`: al final, con la sangría del último.
func (m *Modelo) AñadirElemento(i int, id string) {
	d := m.diagrama(i, id)
	if d == nil {
		return
	}
	nuevo := Elemento{}
	if n := len(d.Elementos); n > 0 && nivelDe(d.Elementos[n-1]) > 0 {
		nivel := nivelDe(d.Elementos[n-1])
		nuevo.Nivel = &nivel
	}
	d.Elementos = append(d.Elementos, nuevo)
}

// QuitarElemento es `removeSmartItem`: siempre queda uno.
func (m *Modelo) QuitarElemento(i int, id string, k int) {
	if d := m.diagrama(i, id); d != nil && len(d.Elementos) > 1 && k >= 0 && k < len(d.Elementos) {
		d.Elementos = slices.Delete(d.Elementos, k, k+1)
	}
}

// MoverElemento es `moveSmartItem`: lo cambia por su vecino.
func (m *Modelo) MoverElemento(i int, id string, k, dir int) {
	d := m.diagrama(i, id)
	if d == nil || k < 0 || k >= len(d.Elementos) || k+dir < 0 || k+dir >= len(d.Elementos) {
		return
	}
	d.Elementos[k], d.Elementos[k+dir] = d.Elementos[k+dir], d.Elementos[k]
}

// SangrarElemento es `indentSmartItem`: ±1 nivel sin saltar más de uno bajo el
// anterior; el primero es la raíz y no se sangra. Nivel 0 = sin campo.
func (m *Modelo) SangrarElemento(i int, id string, k, delta int) {
	d := m.diagrama(i, id)
	if d == nil || k <= 0 || k >= len(d.Elementos) {
		return
	}
	actual := nivelDe(d.Elementos[k])
	nuevo := min(max(actual+float64(delta), 0), nivelDe(d.Elementos[k-1])+1)
	switch nuevo {
	case actual:
	case 0:
		d.Elementos[k].Nivel = nil
	default:
		d.Elementos[k].Nivel = &nuevo
	}
}

// ---- animaciones ----

// cambiarAnims reescribe la secuencia de una caja.
func (m *Modelo) cambiarAnims(i int, id string, f func([]Anim) []Anim) {
	if c := m.caja(i, id); c != nil {
		*c = conAnims(*c, f(AnimsDe(*c)))
	}
}

// PonerPuntosMovimiento es `setMotionPoints`: la animación `k` de la caja pasa a ser
// una trayectoria personalizada con esos vértices.
func (m *Modelo) PonerPuntosMovimiento(i int, id string, k int, pts []Punto) {
	m.cambiarAnims(i, id, func(as []Anim) []Anim {
		if k >= 0 && k < len(as) {
			as[k].Tipo, as[k].Puntos = "motion-custom", pts
		}
		return as
	})
}

// AñadirAnim es `addBoxAnim`: al final de la secuencia de la diapositiva, con
// disparador «al hacer clic», y el orden renumerado.
func (m *Modelo) AñadirAnim(i int, id, tipo string) {
	if _, ok := categoriaAnim[tipo]; !ok || !m.valida(i) {
		return
	}
	var ultimo float64
	for _, c := range m.Diapos[i].Cajas {
		for _, a := range AnimsDe(c) {
			ultimo = max(ultimo, a.Orden)
		}
	}
	m.cambiarAnims(i, id, func(as []Anim) []Anim {
		return append(as, Anim{Tipo: tipo, Orden: ultimo + 1, Disparo: "click"})
	})
	m.Diapos[i].Cajas = NormalizarOrden(m.Diapos[i].Cajas)
}

// PonerEfecto es `setAnimEffect`.
func (m *Modelo) PonerEfecto(i int, id string, k int, tipo string) {
	if _, ok := categoriaAnim[tipo]; !ok {
		return
	}
	m.cambiarAnims(i, id, func(as []Anim) []Anim {
		if k >= 0 && k < len(as) {
			as[k].Tipo = tipo
		}
		return as
	})
}

// PonerDisparo es `setAnimTrigger`.
func (m *Modelo) PonerDisparo(i int, id string, k int, disparo string) {
	if !disparadores[disparo] {
		return
	}
	m.cambiarAnims(i, id, func(as []Anim) []Anim {
		if k >= 0 && k < len(as) {
			as[k].Disparo = disparo
		}
		return as
	})
}

// QuitarAnim es `removeAnimAt`, con el orden renumerado.
func (m *Modelo) QuitarAnim(i int, id string, k int) {
	if !m.valida(i) {
		return
	}
	m.cambiarAnims(i, id, func(as []Anim) []Anim {
		if k >= 0 && k < len(as) {
			return slices.Delete(as, k, k+1)
		}
		return as
	})
	m.Diapos[i].Cajas = NormalizarOrden(m.Diapos[i].Cajas)
}

// MoverAnim es `reorderAnimOrder`: cambia de sitio en la secuencia global la de ese
// `order` con su vecina.
func (m *Modelo) MoverAnim(i int, orden float64, dir int) {
	if !m.valida(i) {
		return
	}
	sec := entradasDe(m.Diapos[i].Cajas)
	sort.SliceStable(sec, func(a, b int) bool { return sec[a].anim.Orden < sec[b].anim.Orden })
	k := slices.IndexFunc(sec, func(e entradaAnim) bool { return e.anim.Orden == orden })
	j := k + dir
	if k < 0 || j < 0 || j >= len(sec) {
		return
	}
	oi, oj := sec[k].anim.Orden, sec[j].anim.Orden
	for n := range m.Diapos[i].Cajas {
		c := &m.Diapos[i].Cajas[n]
		as := AnimsDe(*c)
		if len(as) == 0 {
			continue
		}
		for x := range as {
			switch as[x].Orden {
			case oi:
				as[x].Orden = oj
			case oj:
				as[x].Orden = oi
			}
		}
		*c = conAnims(*c, as)
	}
	m.Diapos[i].Cajas = NormalizarOrden(m.Diapos[i].Cajas)
}
