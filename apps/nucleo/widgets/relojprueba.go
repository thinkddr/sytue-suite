// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"sync"
	"time"
)

// RelojDePrueba es SOLO para tests (de este paquete y de la suite): el Reloj y el
// Programar de un Publicador sin esperas reales. Lo programado corre en la rutina de
// quien llama a Avanzar, en orden, así que una ronda entera —Fuente incluida— ha
// terminado cuando Avanzar vuelve.
type RelojDePrueba struct {
	mu     sync.Mutex
	ahora  time.Time
	tareas []*tareaDePrueba
}

type tareaDePrueba struct {
	cuando    time.Time
	f         func()
	cancelada bool
}

// NuevoRelojDePrueba arranca en `desde`.
func NuevoRelojDePrueba(desde time.Time) *RelojDePrueba { return &RelojDePrueba{ahora: desde} }

// Ahora es el Reloj.
func (r *RelojDePrueba) Ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ahora
}

// Programar es el Programar: apunta f para dentro de d y no la ejecuta hasta Avanzar.
func (r *RelojDePrueba) Programar(d time.Duration, f func()) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := &tareaDePrueba{cuando: r.ahora.Add(max(d, 0)), f: f}
	r.tareas = append(r.tareas, t)
	return func() {
		r.mu.Lock()
		t.cancelada = true
		r.mu.Unlock()
	}
}

// Avanzar mueve el reloj y ejecuta, en orden, lo que venza por el camino, incluido lo
// que se programe mientras tanto.
func (r *RelojDePrueba) Avanzar(d time.Duration) {
	r.mu.Lock()
	objetivo := r.ahora.Add(d)
	r.mu.Unlock()
	for {
		r.mu.Lock()
		siguiente := -1
		for i, t := range r.tareas {
			if t.cancelada || t.cuando.After(objetivo) {
				continue
			}
			if siguiente < 0 || t.cuando.Before(r.tareas[siguiente].cuando) {
				siguiente = i
			}
		}
		if siguiente < 0 {
			r.ahora = objetivo
			r.tareas = quitarCanceladas(r.tareas)
			r.mu.Unlock()
			return
		}
		t := r.tareas[siguiente]
		r.tareas = append(r.tareas[:siguiente], r.tareas[siguiente+1:]...)
		if t.cuando.After(r.ahora) {
			r.ahora = t.cuando
		}
		r.mu.Unlock()
		t.f()
	}
}

func quitarCanceladas(ts []*tareaDePrueba) []*tareaDePrueba {
	out := ts[:0]
	for _, t := range ts {
		if !t.cancelada {
			out = append(out, t)
		}
	}
	return out
}

// Opciones devuelve `o` con el Reloj y el Programar de esta prueba.
func (r *RelojDePrueba) Opciones(o OpcionesPublicador) OpcionesPublicador {
	o.Reloj, o.Programar = r.Ahora, r.Programar
	return o
}
