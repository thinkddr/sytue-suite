// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image/color"
	"strconv"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// El punto del puntero láser: el rojo de CONTENT_PALETTE (#DC2626), que es el de la luz de
// un láser, con su halo del mismo color translúcido (`LASER` y `LASER_HALO` de la web).
var (
	rojoLaser = color.NRGBA{R: 0xdc, G: 0x26, B: 0x26, A: 0xff}
	haloLaser = color.NRGBA{R: 0xdc, G: 0x26, B: 0x26, A: 0x99}
)

// AnchoSelectorAvance es el `width={132}` del selector del avance automático.
const AnchoSelectorAvance float32 = 132

// ladoLaser y ladoHalo son el diámetro del punto (14 px) y del resplandor que lo rodea.
const (
	ladoLaser   float32 = 14
	ladoHalo    float32 = 26
	umbralRueda float32 = 20 // una muesca de rueda en Fyne gira 25 (10 en el Mac)
	calmaRueda          = 350 * time.Millisecond
)

// alternarLaser enciende o apaga el puntero láser (L).
func (p *Presentacion) alternarLaser() {
	p.Laser = !p.Laser
	if !p.Laser {
		p.laserEn = nil
	}
	p.Refresh()
}

// MouseIn, MouseMoved y MouseOut llevan el punto del láser con el ratón.
func (p *Presentacion) MouseIn(e *desktop.MouseEvent) { p.MouseMoved(e) }

func (p *Presentacion) MouseMoved(e *desktop.MouseEvent) {
	if !p.Laser {
		return
	}
	pos := e.Position
	p.laserEn = &pos
	p.Refresh()
}

func (p *Presentacion) MouseOut() {
	if p.laserEn != nil {
		p.laserEn = nil
		p.Refresh()
	}
}

// Cursor esconde el del sistema con el láser encendido: el punto rojo ES el puntero.
func (p *Presentacion) Cursor() desktop.Cursor {
	if p.Laser {
		return desktop.HiddenCursor
	}
	return desktop.DefaultCursor
}

// Scrolled es la rueda: abajo, la siguiente, como en PowerPoint. Una muesca basta, el
// panel táctil acumula y tras cada paso hay un rato de calma para no saltar diez.
func (p *Presentacion) Scrolled(e *fyne.ScrollEvent) {
	if p.ayudaAbierta() {
		return
	}
	ahora := time.Now()
	if ahora.Sub(p.ultimaRueda) < calmaRueda {
		return
	}
	dir, resto := PasoDeRueda(p.rueda, -e.Scrolled.DY, umbralRueda)
	p.rueda = resto
	if dir == 0 {
		return
	}
	p.ultimaRueda = ahora
	if p.Pantalla != Encendida {
		p.encender()
		return
	}
	p.Avanzar(dir)
}

// ArmarAvance vuelve a contar el avance automático desde ahora: lo llama cada cambio de
// posición, y quien abre la presentación al empezar. Con la pantalla apagada o la lista
// de teclas abierta no pasa: se espera otra vuelta.
func (p *Presentacion) ArmarAvance() {
	if p.parar != nil {
		p.parar()
		p.parar = nil
	}
	if p.Avance <= 0 {
		return
	}
	programar := p.Programar
	if programar == nil {
		programar = programarDeVerdad
	}
	p.parar = programar(time.Duration(p.Avance)*time.Second, func() {
		p.parar = nil
		if p.Pantalla == Encendida && !p.ayudaAbierta() {
			p.Ir(AvanceAutomatico(p.M.Diapos, p.Diapo, p.Paso))
		}
		if p.parar == nil { // Ir no ha rearmado: no se movió, o no se podía
			p.ArmarAvance()
		}
	})
}

// programarDeVerdad corre `f` en el hilo de la interfaz dentro de `d`.
func programarDeVerdad(d time.Duration, f func()) func() {
	t := time.AfterFunc(d, func() { fyne.Do(f) })
	return func() { t.Stop() }
}

// Lo que la tanda del 2026-11-25 añade a presentar (`slides/presentar.ts`): el avance
// automático en bucle, la rueda, la pausa del cronómetro del moderador y las notas del
// orador como texto. Con los mismos vectores que la web
// (`packages/office/test/vectores/diapos-herramientas.json`).

// ClaveAvance guarda en el Almacen los segundos del avance automático (vacío, a mano).
// Es de quien presenta en este dispositivo, como en la web: no viaja con el documento.
const ClaveAvance = "diapos.avance"

// SegundosAvance son los que se ofrecen; 0 es «a mano».
var SegundosAvance = []int{0, 5, 10, 20, 30, 60}

// LeerAvance son los segundos guardados en `a`, o 0 si no hay o no es uno de los ofrecidos.
func LeerAvance(a Almacen) int {
	if a == nil {
		return 0
	}
	n, err := strconv.Atoi(a.Leer(ClaveAvance))
	if err != nil {
		return 0
	}
	for _, s := range SegundosAvance {
		if s == n {
			return n
		}
	}
	return 0
}

// AvanceAutomatico es `avanceAutomatico`: la posición de la flecha y, al acabar, la
// primera diapositiva que se enseña, como PowerPoint «examinada en un quiosco».
func AvanceAutomatico(diapos []Diapo, diapo, paso int) (int, int) {
	d, p := Avanzar(diapos, diapo, paso, 1)
	if d != diapo || p != paso {
		return d, p
	}
	if n := SiguienteVisible(Saltables(diapos), -1, 1); n != SinDestino {
		return n, 0
	}
	return 0, 0
}

// PasoDeRueda es `pasoDeRueda`: lo girado hacia abajo (negativo, hacia arriba) se acumula
// y, al pasar del umbral, se pasa una diapositiva y se empieza de cero.
func PasoDeRueda(acumulado, abajo, umbral float32) (dir int, resto float32) {
	total := acumulado + abajo
	switch {
	case total >= umbral:
		return 1, 0
	case total <= -umbral:
		return -1, 0
	default:
		return 0, total
	}
}

// Cronometro es el del moderador: cuándo empezó y, si está en pausa, desde cuándo (cero
// si no lo está).
type Cronometro struct {
	Inicio, Pausa time.Time
}

// Transcurrido es lo que lleva a `ahora`, sin lo que ha pasado en pausa.
func (c Cronometro) Transcurrido(ahora time.Time) time.Duration {
	hasta := ahora
	if !c.Pausa.IsZero() {
		hasta = c.Pausa
	}
	return max(0, hasta.Sub(c.Inicio))
}

// Alternar pausa o reanuda: al reanudar, el inicio se corre lo que duró la pausa.
func (c Cronometro) Alternar(ahora time.Time) Cronometro {
	if c.Pausa.IsZero() {
		return Cronometro{Inicio: c.Inicio, Pausa: ahora}
	}
	return Cronometro{Inicio: c.Inicio.Add(ahora.Sub(c.Pausa))}
}

// NotasComoTexto es `notasComoTexto`: cada diapositiva con su título y sus notas debajo,
// separadas por una línea en blanco; las líneas sin el blanco del final y sin las vacías
// de los extremos. Las ocultas también: quien presenta las lleva.
func NotasComoTexto(diapos []Diapo, titulo func(n int) string) string {
	if len(diapos) == 0 {
		return ""
	}
	bloques := make([]string, len(diapos))
	for i, d := range diapos {
		lineas := strings.Split(strings.ReplaceAll(strings.ReplaceAll(d.Notas, "\r\n", "\n"), "\r", "\n"), "\n")
		for j, l := range lineas {
			lineas[j] = strings.TrimRightFunc(l, unicode.IsSpace)
		}
		for len(lineas) > 0 && lineas[0] == "" {
			lineas = lineas[1:]
		}
		for len(lineas) > 0 && lineas[len(lineas)-1] == "" {
			lineas = lineas[:len(lineas)-1]
		}
		bloques[i] = strings.Join(append([]string{titulo(i + 1)}, lineas...), "\n")
	}
	return strings.Join(bloques, "\n\n") + "\n"
}
