// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import "context"

type cargaContacts struct {
	ctx        context.Context
	generacion uint64
}

// capturar ties reads and optional synchronization to one authenticated identity.
// The caller captures it before leaving the UI; a later account can never adopt it.
func (a *App) capturar() cargaContacts {
	a.vidaMu.Lock()
	defer a.vidaMu.Unlock()
	if a.cerrada {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return cargaContacts{ctx, a.generacion}
	}
	if a.vida == nil {
		a.vida, a.cancelarVida = context.WithCancel(context.Background())
	}
	return cargaContacts{a.vida, a.generacion}
}

func (a *App) vigente(c cargaContacts) bool {
	if c.ctx.Err() != nil {
		return false
	}
	a.vidaMu.Lock()
	defer a.vidaMu.Unlock()
	return c.generacion == a.generacion && !a.cerrada
}

func (a *App) abierta() bool {
	a.vidaMu.Lock()
	defer a.vidaMu.Unlock()
	return !a.cerrada
}

func (a *App) invalidar(cerrar bool) {
	a.vidaMu.Lock()
	defer a.vidaMu.Unlock()
	if a.cancelarVida != nil {
		a.cancelarVida()
	}
	a.vida, a.cancelarVida = nil, nil
	a.generacion++
	a.cerrada = a.cerrada || cerrar
	a.organizacionEnCurso = false
}

// Cerrar cancels in-flight reads and optional directory work before the window dies.
func (a *App) Cerrar() {
	a.invalidar(true)
	a.Vaciar()
}

// lanzar admits work before starting the goroutine, so Cerrar and Esperar cannot
// race with an operation that has been scheduled but has not started yet.
func (a *App) lanzar(f func()) {
	scope := a.capturar()
	a.vidaMu.Lock()
	if a.cerrada || scope.ctx.Err() != nil || scope.generacion != a.generacion {
		a.vidaMu.Unlock()
		return
	}
	if a.trabajos == 0 {
		a.trabajosTerminados = make(chan struct{})
	}
	a.trabajos++
	a.vidaMu.Unlock()
	go func() {
		defer func() {
			a.vidaMu.Lock()
			a.trabajos--
			if a.trabajos == 0 {
				close(a.trabajosTerminados)
			}
			a.vidaMu.Unlock()
		}()
		if a.vigente(scope) {
			f()
		}
	}()
}

// Esperar joins the presenter after Cerrar has stopped admission and canceled it.
// It must run away from the UI thread because admitted work can use DoAndWait.
func (a *App) Esperar(ctx context.Context) error {
	a.vidaMu.Lock()
	if !a.cerrada {
		a.vidaMu.Unlock()
		return context.Canceled
	}
	if a.trabajos == 0 {
		a.vidaMu.Unlock()
		return ctx.Err()
	}
	done := a.trabajosTerminados
	a.vidaMu.Unlock()
	select {
	case <-done:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// programar captures the identity before scheduling. The task must use this
// scope for every operation instead of capturing a possibly newer identity.
func (a *App) programar(f func(cargaContacts)) {
	a.programarEn(a.capturar(), f)
}

// programarEn keeps the original identity when a task schedules more work.
func (a *App) programarEn(scope cargaContacts, f func(cargaContacts)) {
	a.enFondo(func() {
		if a.vigente(scope) {
			f(scope)
		}
	})
}

// IniciarCarga and SolicitarContacto bind external entry points to the identity
// that requested them, even if the presenter is cleared before a task starts.
func (a *App) IniciarCarga() {
	a.restaurarOrden()
	a.programar(a.cargarEn)
}

func (a *App) SolicitarContacto(id string) {
	a.programar(func(scope cargaContacts) { a.abrirContactoEn(scope, id) })
}

func (a *App) pararImportador() {
	a.importadorCargado = false
	if d := a.Pantalla.Importar; d != nil {
		d.Parar()
	}
}
