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

func (a *App) invalidar(cerrar bool) {
	a.vidaMu.Lock()
	defer a.vidaMu.Unlock()
	if a.cancelarVida != nil {
		a.cancelarVida()
	}
	a.vida, a.cancelarVida = nil, nil
	a.generacion++
	a.cerrada = cerrar
	a.organizacionEnCurso = false
}

// Cerrar cancels in-flight reads and optional directory work before the window dies.
func (a *App) Cerrar() {
	a.invalidar(true)
	a.pararImportador()
}

func (a *App) pararImportador() {
	a.importadorCargado = false
	if d := a.Pantalla.Importar; d != nil {
		d.Parar()
	}
}
