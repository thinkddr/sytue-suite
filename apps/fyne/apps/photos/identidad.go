// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// CambiarSesion adopta la identidad issuer/sujeto/organización que usa la suite.
// Una renovación de tokens mantiene la pantalla; otra cuenta empieza en Fotos.
// Se llama en el hilo de la interfaz, igual que Ir.
func (a *App) CambiarSesion(identidad string) {
	if a.cerrada || a.identidadAdoptada && a.identidad == identidad {
		return
	}
	primera := !a.identidadAdoptada
	a.identidadAdoptada, a.identidad = true, identidad
	a.refrescando = false
	a.reanudandoImportaciones = false
	a.cuenta.Store(identidad)
	a.cancelarVida()
	a.vida, a.cancelarVida = context.WithCancel(context.Background())
	if primera {
		return
	}
	a.gen++
	if a.temporizado != nil {
		a.temporizado.Stop()
		a.temporizado = nil
	}
	a.cerrarVisor()
	a.cerrarEditor()
	a.cerrarPublica()
	// Los avisos hablan de las fotos de la otra cuenta, y un «Deshacer» suyo ya no
	// tiene a quién devolver nada.
	for _, av := range a.avisos.Avisos() {
		a.avisos.Descartar(av)
	}
	a.vaciarFototecas()
	a.albumes = nil
	a.huellas.Clear()
	a.g, a.alb, a.uno = estadoGaleria{}, estadoAlbumes{}, estadoAlbum{}
	a.pap, a.rep, a.enc = estadoPapelera{}, estadoRepetidas{}, estadoEncontradas{}
	a.rec, a.pub, a.cmp = estadoRecuerdos{}, estadoPublico{}, estadoCompartidos{}
	a.aj = api.PhotosSettings{}
	// La ronda de la cuenta anterior se corta con su vida; la de esta empieza al acabar
	// aquella (finRondaCopia).
	a.cop.progreso, a.cop.fallo = nil, nil
	a.arrastrando = false
	a.campoAlbum.PonerTexto("")
	if a.w != nil {
		c := a.w.Canvas()
		for _, capa := range c.Overlays().List() {
			c.Overlays().Remove(capa)
		}
		c.Unfocus()
	}
	a.Ir(RutaFotos)
	a.rondaCopia()
}

// enSesion captura la vida ANTES de lanzar la goroutine: un trabajo aún en cola
// tampoco puede empezar con las credenciales de la cuenta siguiente.
//
//nolint:contextcheck // el trabajo va ligado a la vida de la sesión capturada aquí, no al contexto de quien lo pide.
func (a *App) enSesion(f func(context.Context)) {
	vida := a.vida
	a.lanzar(func() {
		if vida.Err() == nil {
			f(vida)
		}
	})
}

// Cerrar retires this presenter permanently before its private repository drains.
// It runs on the UI thread and clears image caches without deleting storage WAL.
func (a *App) Cerrar() {
	if a.cerrada {
		return
	}
	a.cerrada = true
	a.gen++
	a.cancelarVida()
	if a.temporizado != nil {
		a.temporizado.Stop()
		a.temporizado = nil
	}
	a.cerrarVisor()
	a.cerrarEditor()
	a.cerrarPublica()
	a.vaciarFototecas()
	a.huellas.Clear()
	if a.w != nil {
		a.w.Canvas().SetOnTypedKey(nil)
		a.w.Canvas().SetOnTypedRune(nil)
		a.w.SetOnDropped(nil)
	}
}

func (a *App) vaciarFototecas() {
	vistas := map[*Fototeca]bool{}
	for _, view := range []*Fototeca{a.v.Teca, a.v.TecaGrande, a.pub.teca, a.pub.grande, a.cmp.teca} {
		if view != nil && !vistas[view] {
			vistas[view] = true
			view.Vaciar()
		}
	}
}
