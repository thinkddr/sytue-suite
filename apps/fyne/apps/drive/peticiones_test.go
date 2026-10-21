// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Dentro de algo compartido en lectura, la vista lleva la franja con «Pedir acceso de
// edición»; con permiso de edición, no. Es `FranjaPeticion` de SharedWithMeView.
func TestLoCompartidoEnLecturaTieneSuFranja(t *testing.T) {
	c := NuevaVistaCompartido(traductor(t))
	c.Raiz = &api.Compartido{File: api.File{ID: "r1", Name: "Informes"}, IsDir: true, Permission: "view"}
	c.Permiso = "view"
	c.Peticion = peticion.Estado{Pedir: func() {}}
	f, ok := c.franja().(*ui.FranjaSoloLectura)
	if !ok || f.Accion == nil {
		t.Fatal("en lectura no hay franja con «Pedir acceso de edición»")
	}
	c.Permiso = "edit"
	if c.franja() != nil {
		t.Error("con permiso de edición sigue la franja de solo lectura")
	}
}
