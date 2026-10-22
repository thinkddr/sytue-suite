// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// La hoja protegida es el caso (a) de la franja: el bloqueo es de quien edita, así que
// la franja ofrece «Editar» y, sin contraseña, pulsarlo la desprotege al momento. Sin
// protección no hay franja. Es `FranjaHojaProtegida.tsx`.
func TestLaHojaProtegidaSeDesbloqueaConEditar(t *testing.T) {
	lz := NuevoLienzo(NuevoLibro())
	ctx := &editor.Contexto{Tipo: editor.Hoja, Lienzo: lz, Estado: editor.Listo}
	if _, f := lz.bloqueoPropio(ctx); f != nil {
		t.Fatal("una hoja sin proteger ofrece desbloquearla")
	}
	lz.Mandar(CmdProteger, `{}`)
	aviso, desbloquear := lz.bloqueoPropio(ctx)
	if desbloquear == nil || aviso == "" {
		t.Fatal("la hoja protegida no tiene franja con «Editar»")
	}
	desbloquear()
	if lz.Estado().Protegida {
		t.Error("«Editar» no ha desprotegido la hoja")
	}
}
