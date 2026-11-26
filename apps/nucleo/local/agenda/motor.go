// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"sync"
	"time"
)

// CadaSincronizar es cada cuánto se sincroniza con la app abierta.
const CadaSincronizar = 10 * time.Minute

// Motor decide CUÁNDO se sincroniza (§5.10): al arrancar, cada [Motor.Cada] con la app
// abierta y cuando se le pide ([Motor.Ya]: al volver a primer plano, «Sincronizar
// ahora»). Nunca dos a la vez: una petición mientras corre otra se junta con la
// siguiente vuelta.
type Motor struct {
	// Hacer es una sincronización entera.
	Hacer func(ctx context.Context)
	// Cada; 0 es CadaSincronizar.
	Cada time.Duration

	una     sync.Once
	dispara chan struct{}
}

func (m *Motor) iniciar() {
	m.una.Do(func() { m.dispara = make(chan struct{}, 1) })
}

// Arrancar sincroniza ahora y luego cada Cada, hasta que ctx acabe.
func (m *Motor) Arrancar(ctx context.Context) {
	m.iniciar()
	cada := m.Cada
	if cada <= 0 {
		cada = CadaSincronizar
	}
	go func() {
		t := time.NewTicker(cada)
		defer t.Stop()
		m.Hacer(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-m.dispara:
			}
			m.Hacer(ctx)
		}
	}()
}

// Ya pide una sincronización en cuanto se pueda, sin esperar.
func (m *Motor) Ya() {
	m.iniciar()
	select {
	case m.dispara <- struct{}{}:
	default:
	}
}
