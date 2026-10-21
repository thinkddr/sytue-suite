// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package offline

import "sync"

// State es si hay red, como `Connectivity` en native.ts.
type State uint8

// Online es el valor cero: hasta que una petición falle, se supone que hay red.
const (
	Online State = iota
	Offline
)

// Monitor guarda el estado y avisa a quien se suscriba cuando CAMBIA. Quien lo
// alimenta es Transport, con el resultado de cada petición: es la única señal fiable —
// el «hay red» del sistema dice que hay wifi, no que el servidor conteste.
type Monitor struct {
	mu      sync.Mutex
	estado  State
	oyentes map[int]func(State)
	sig     int
}

// State es el último estado conocido.
func (m *Monitor) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.estado
}

// Report anota el estado y, si ha cambiado, avisa. Los oyentes se llaman fuera del
// cerrojo: pueden volver a entrar en el Monitor sin bloquearse.
func (m *Monitor) Report(s State) {
	m.mu.Lock()
	if m.estado == s {
		m.mu.Unlock()
		return
	}
	m.estado = s
	oyentes := make([]func(State), 0, len(m.oyentes))
	for _, f := range m.oyentes {
		oyentes = append(oyentes, f)
	}
	m.mu.Unlock()
	for _, f := range oyentes {
		f(s)
	}
}

// Subscribe llama a f en cada cambio. En Gio, f invalida la ventana. Devuelve cómo
// darse de baja.
func (m *Monitor) Subscribe(f func(State)) (baja func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.oyentes == nil {
		m.oyentes = map[int]func(State){}
	}
	id := m.sig
	m.sig++
	m.oyentes[id] = f
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.oyentes, id)
	}
}
