// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"runtime"

	"fyne.io/fyne/v2"
)

// # Volver a la app y dejarla en segundo plano
//
// Fyne admite UN receptor por app para cada cosa (`Lifecycle.SetOnEnteredForeground` y
// `SetOnExitedForeground`): el segundo que se apunta borra al primero sin avisar. La barra
// de la suite ya lo usaba para volver a pedir la ficha, y Cast lo necesita en el móvil
// para pausar el vídeo al salir; con el receptor único, el que llegara último dejaba al
// otro sin enterarse. Aquí se reparten: la sesión se apunta UNA vez en Abrir y avisa a
// todos los que se hayan apuntado con AlVolver y AlFondo, en el orden en que llegaron.
//
// En escritorio «volver» es que la ventana recibe el foco y «fondo» que lo pierde; en el
// móvil, que la app pasa a primer plano o sale de él.

// AlVolver apunta a quien quiera enterarse de que la app vuelve a primer plano. Se le
// llama en el hilo de la interfaz.
func (s *Suite) AlVolver(f func()) {
	if f == nil {
		return
	}
	s.mu.Lock()
	s.alVolver = append(s.alVolver, f)
	s.mu.Unlock()
}

// AlFondo apunta a quien quiera enterarse de que la app deja el primer plano: en el
// móvil, que el sistema la manda al fondo (y puede matarla). Se le llama en el hilo de
// la interfaz.
func (s *Suite) AlFondo(f func()) {
	if f == nil {
		return
	}
	s.mu.Lock()
	s.alFondo = append(s.alFondo, f)
	s.mu.Unlock()
}

// engancharVida pone a la sesión como el único receptor de Fyne.
func (s *Suite) engancharVida(a fyne.App) {
	lc := a.Lifecycle()
	lc.SetOnEnteredForeground(func() { s.repartir(true) })
	lc.SetOnExitedForeground(func() { s.repartir(false) })
}

// repartir llama a los apuntados, sin el cerrojo: uno de ellos puede apuntar a otro.
func (s *Suite) repartir(vuelve bool) {
	s.enFondo.Store(!vuelve)
	s.mu.Lock()
	lista := s.alFondo
	if vuelve {
		lista = s.alVolver
	}
	quienes := append([]func(){}, lista...)
	s.mu.Unlock()
	if vuelve && s.PausaEnFondo() {
		select {
		case s.senalVuelta() <- struct{}{}:
		default: // ya había una vuelta sin atender
		}
	}
	for _, f := range quienes {
		f()
	}
}

// # Lo que dice que hay alguien sigue al primer plano (iOS)
//
// En iOS una app mandada al fondo se suspende, y hasta la tarea de refresco del widget
// (fondo.go) no volvía a correr. Con ella, iOS la reanuda en segundo plano cada 30 min o
// menos, y todo lo que estaba esperando vuelve a correr: el latido vencido publicaba «En
// línea» de madrugada y el SSE de la barra se reconectaba. Por eso allí la presencia
// (latir) y el SSE de la barra (ui.BarraSuite) se paran al dejar el primer plano y
// vuelven con él, no con la escena: un despertar, frío o caliente, nunca late.
//
// En el escritorio «fondo» es solo que la ventana pierde el foco —se sigue estando en
// línea y los avisos tienen que seguir llegando—, y en Android el SSE de la barra es por
// donde llegan los avisos con la app al fondo (`eu.sytue.nativo.Avisos`): allí no se para.

// plataformaQueDespierta es la que reanuda en segundo plano una app suspendida para su
// widget (BGAppRefreshTask).
const plataformaQueDespierta = runtime.GOOS == "ios"

// PausaEnFondo dice si en esta plataforma lo que dice que hay alguien —la presencia, el
// SSE de los avisos— se para con la app fuera del primer plano (AlFondo) y vuelve con
// ella (AlVolver). Es la pregunta de ui.BarraSuite.
func (s *Suite) PausaEnFondo() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pausaEnFondo
}

// latidoEnPausa: la presencia no se publica ahora.
func (s *Suite) latidoEnPausa() bool { return s.PausaEnFondo() && s.enFondo.Load() }

// senalVuelta es el aviso a latir de que la app ha vuelto al primer plano.
func (s *Suite) senalVuelta() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vuelta == nil {
		s.vuelta = make(chan struct{}, 1)
	}
	return s.vuelta
}
