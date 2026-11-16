// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"math"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Arrastrar un evento a otro día en el MES, como el `arrastreMes` de CalendarGrid.tsx.
// Solo la rejilla horaria arrastraba y el mes es donde se replanifica casi todo. Se
// arrastra la píldora o la barra; el desplazamiento son los días entre la celda donde se
// cogió y la celda donde se suelta, así que una barra de varios días cogida por su
// tercer día cae con ese día bajo el puntero. Guarda por el mismo `AlMoverEvento` que la
// rejilla horaria (`App.mover`): pregunta el alcance en una serie, no mueve una
// suscripción y trae «Deshacer».
//
// Solo en ESCRITORIO, por lo mismo que `cuerpoArrastrable`: en una pantalla táctil
// deslizar tiene que desplazar la vista, y un objeto `fyne.Draggable` se quedaría el gesto.

// UmbralArrastreMes es lo que tiene que viajar el puntero para que pulsar deje de ser un
// clic (vista rápida) y pase a ser un arrastre: el `UMBRAL_ARRASTRE_PX` de la web.
const UmbralArrastreMes float32 = 4

// arrastreMes es el gesto en curso: qué ocurrencia, de qué celda y hasta cuál.
type arrastreMes struct {
	o               Ocurrencia
	origen, destino int
	recorrido       float32
	movido          bool
}

// pildoraArrastrable es la píldora de escritorio: además de pulsarse, se arrastra.
type pildoraArrastrable struct{ pildora }

// barraArrastrable es la barra de escritorio: además de pulsarse, se arrastra.
type barraArrastrable struct {
	barraEvento
	m *Mes
}

var (
	_ fyne.Draggable = (*pildoraArrastrable)(nil)
	_ fyne.Draggable = (*barraArrastrable)(nil)
)

func (p *pildoraArrastrable) Dragged(e *fyne.DragEvent) { p.m.arrastrar(p.o, p, e) }
func (p *pildoraArrastrable) DragEnd()                  { p.m.soltar(p) }
func (b *barraArrastrable) Dragged(e *fyne.DragEvent)   { b.m.arrastrar(b.t.Item, b, e) }
func (b *barraArrastrable) DragEnd()                    { b.m.soltar(b) }

// arrastrable dice si una ocurrencia se puede arrastrar en esta rejilla.
func (m *Mes) arrastrable(o Ocurrencia) bool {
	return m.AlMover != nil && !o.SoloLectura && !ui.EsTactil()
}

// celdaEn es el índice de la celda bajo un punto de la rejilla. Fuera de ella se pega al
// borde, como `celdaEnPunto` de la web.
func (m *Mes) celdaEn(p fyne.Position) int {
	t := m.Size()
	acotar := func(v float32, hi int) int {
		return max(0, min(hi, int(math.Floor(float64(v)))))
	}
	col := 0
	if t.Width > 0 {
		col = acotar(p.X/t.Width*7, 6)
	}
	fila := 0
	if alto := AltoSemana(t.Height); alto > 0 {
		fila = acotar((p.Y-AltoDiasSemana-tema.Borde)/alto, 5)
	}
	return fila*7 + col
}

// enRejilla pasa un punto del objeto arrastrado a coordenadas de la rejilla.
func (m *Mes) enRejilla(o fyne.CanvasObject, p fyne.Position) fyne.Position {
	d := fyne.CurrentApp().Driver()
	return d.AbsolutePositionForObject(o).Add(p).Subtract(d.AbsolutePositionForObject(m))
}

// arrastrar empieza o sigue el gesto: el primer evento decide la celda de origen por
// dónde se PULSÓ (el punto de ahora menos lo arrastrado), como el cuerpo horario.
func (m *Mes) arrastrar(o Ocurrencia, desde fyne.CanvasObject, e *fyne.DragEvent) {
	p := m.enRejilla(desde, e.Position)
	if m.arr == nil {
		ini := m.celdaEn(p.Subtract(e.Dragged))
		m.arr = &arrastreMes{o: o, origen: ini, destino: ini}
	}
	a := m.arr
	a.recorrido += float32(math.Hypot(float64(e.Dragged.DX), float64(e.Dragged.DY)))
	a.movido = a.movido || a.recorrido >= UmbralArrastreMes
	a.destino = m.celdaEn(p)
	if a.movido {
		m.resaltarDestino()
	}
}

// soltar cierra el gesto. Sin arrastre real es un clic: abre la vista rápida, que es lo
// que habría hecho el `Tapped` que Fyne no entrega tras un arrastre.
func (m *Mes) soltar(ancla fyne.CanvasObject) {
	a := m.arr
	m.arr = nil
	if a == nil {
		return
	}
	m.resaltarDestino()
	if !a.movido {
		if m.AlAbrir != nil {
			m.AlAbrir(a.o, ancla)
		}
		return
	}
	dias := a.destino - a.origen
	if dias == 0 || m.AlMover == nil {
		return
	}
	inicio, fin := DesplazarDias(a.o, dias, m.zona())
	// Se pinta ya en su día nuevo mientras se guarda; la recarga trae lo guardado (o lo
	// devuelve a su sitio si se cancela el alcance de una serie o falla el guardado).
	for i, e := range m.Eventos {
		if e.Event.ID != a.o.Event.ID || !e.Start.Equal(a.o.Start) {
			continue
		}
		movida := make([]Ocurrencia, len(m.Eventos))
		copy(movida, m.Eventos)
		movida[i].Start, movida[i].End = inicio, fin
		m.Eventos = movida
		break
	}
	m.Refresh()
	m.AlMover(a.o, inicio, fin)
}

// CancelarArrastre retira el resaltado sin mover nada.
func (m *Mes) CancelarArrastre() {
	if m.arr == nil {
		return
	}
	m.arr = nil
	m.resaltarDestino()
}

// destinoArrastre son las celdas que ocuparía la ocurrencia si se soltara ahora.
func (m *Mes) destinoArrastre(dias []time.Time) (desde, hasta int, ok bool) {
	a := m.arr
	if a == nil || !a.movido {
		return 0, 0, false
	}
	loc := m.zona()
	o := a.o
	o.Start, o.End = DesplazarDias(a.o, a.destino-a.origen, loc)
	ini, fin := ocurrenciaInstantes(o, loc)
	return Tramar(dias, ini, fin, loc)
}

// resaltarDestino marca las celdas de destino (`bg-primary/10` en la web) y repinta solo
// las que cambian.
func (m *Mes) resaltarDestino() {
	r := m.renderer
	if r == nil {
		return
	}
	dias := make([]time.Time, len(r.celdas))
	for i, c := range r.celdas {
		dias[i] = c.dia
	}
	desde, hasta, ok := m.destinoArrastre(dias)
	for i, c := range r.celdas {
		es := ok && desde <= i && i <= hasta
		if c.destino != es {
			c.destino = es
			c.Refresh()
		}
	}
}

// DesplazarDias es el inicio y el fin de la ocurrencia movida `dias` días, con la misma
// hora de pared y la misma duración. Un día completo es FLOTANTE (el servicio lo guarda a
// las 00:00 UTC) y se desplaza en UTC; uno con hora, en la zona de visualización, para
// que un cambio de hora por medio no lo corra una hora. Sin fin, sigue sin fin.
func DesplazarDias(o Ocurrencia, dias int, loc *time.Location) (inicio, fin time.Time) {
	en := loc
	if o.Event.AllDay {
		en = time.UTC
	}
	inicio = o.Start.In(en).AddDate(0, 0, dias)
	if !o.End.IsZero() {
		fin = o.End.In(en).AddDate(0, 0, dias)
	}
	return inicio, fin
}
