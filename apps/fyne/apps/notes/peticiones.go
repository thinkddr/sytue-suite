// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"

	"fyne.io/fyne/v2"

	pet "git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La franja de un cuaderno ajeno que se lee pero no se escribe, con su «Pedir acceso de
// edición» a su dueño; y, para el dueño que llega desde el aviso, la de quién se lo
// pide. Es la `FranjaPeticion` de `NotesApp.tsx`; la pieza es la común (`peticion`).

func (p *Pantalla) tPeticion(k string, v i18n.Vars) string { return p.tr.T(k, v) }

// engancharPeticion rehace la franja al abrir un cuaderno.
func (p *Pantalla) engancharPeticion(c Cuaderno) {
	if p.svc == nil {
		return
	}
	pedida := c.Crudo.EditRequested
	pet.Enganchar(&p.peticion, p.svc.Peticiones(c.ID), pet.Opciones{
		SinPermiso:    c.Crudo.ReadOnly,
		PuedeConceder: !c.Crudo.ReadOnly && c.ID == p.cuadernoDelAviso,
		YaPedida:      func(context.Context) (bool, error) { return pedida, nil },
		Recomponer:    p.Repintar,
		T:             p.tPeticion,
		Concedido:     "notes.access.granted", Recurso: c.ID,
		AlConceder: func() { go p.cargarCuadernos() },
	})
}

// franjaPeticion es la franja que toca en el cuaderno abierto, o nil.
func (p *Pantalla) franjaPeticion() fyne.CanvasObject {
	if !p.hayRed() || p.activo.ID == "" {
		return nil
	}
	f := pet.Franja(&p.peticion, p.activo.Crudo.ReadOnly,
		p.tr.T("readOnly.sharedWithYou", i18n.Vars{"name": p.activo.Titulo}), p.tPeticion)
	if f == nil {
		return nil // un *FranjaSoloLectura nil en la interfaz no sería nil
	}
	return f
}
