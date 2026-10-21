// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import "fyne.io/fyne/v2"

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
	s.mu.Lock()
	lista := s.alFondo
	if vuelve {
		lista = s.alVolver
	}
	quienes := append([]func(){}, lista...)
	s.mu.Unlock()
	for _, f := range quienes {
		f()
	}
}
