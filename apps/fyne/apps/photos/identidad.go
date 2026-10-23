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
	if a.identidadAdoptada && a.identidad == identidad {
		return
	}
	primera := !a.identidadAdoptada
	a.identidadAdoptada, a.identidad = true, identidad
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
	vistas := map[*Fototeca]bool{}
	for _, f := range []*Fototeca{a.v.Teca, a.v.TecaGrande, a.pub.teca, a.pub.grande, a.cmp.teca} {
		if f != nil && !vistas[f] {
			vistas[f] = true
			f.Vaciar()
		}
	}
	a.albumes = nil
	a.huellas.Clear()
	a.g, a.alb, a.uno = estadoGaleria{}, estadoAlbumes{}, estadoAlbum{}
	a.pap, a.rep, a.enc = estadoPapelera{}, estadoRepetidas{}, estadoEncontradas{}
	a.rec, a.pub, a.cmp = estadoRecuerdos{}, estadoPublico{}, estadoCompartidos{}
	a.aj = api.PhotosSettings{}
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
}

// enSesion captura la vida ANTES de lanzar la goroutine: un trabajo aún en cola
// tampoco puede empezar con las credenciales de la cuenta siguiente.
func (a *App) enSesion(f func(context.Context)) {
	vida := a.vida
	a.lanzar(func() {
		if vida.Err() == nil {
			f(vida)
		}
	})
}
