// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// El paso de confirmación avisa de lo que se pierde y no deja restablecer sin
// marcar la casilla, como el modal de la web. No pide nada (la clave nueva se
// envuelve con la clave de desbloqueo de Sytue ID, ADR 0084) y nunca enseña ni pide
// la clave de recuperación.
func TestRestablecerPideConfirmacionExplicita(t *testing.T) {
	sa, _, _ := salaEn(t, &servicioFalso{}, 640)
	c := NuevoCifrado(sa, nil, mx.Config{}, nil)
	c.AbrirPanel()
	t.Cleanup(c.Cerrar)
	fyne.DoAndWait(c.restablecer)
	cuerpo, pie := c.Objetos()
	var casilla *ui.Eleccion
	for _, o := range cuerpo {
		if e, ok := o.(*ui.Eleccion); ok {
			casilla = e
		}
	}
	if casilla == nil || casilla.Etiqueta != c.t("matrix.resetUnderstood") || casilla.Marcada {
		t.Fatal("falta la casilla de «Entiendo…» sin marcar")
	}
	for _, o := range cuerpo {
		if e, ok := o.(*ui.Entrada); ok {
			t.Fatalf("restablecer pide %q: aquí no se teclea nada", e.AccessibilityLabel())
		}
	}
	accion := pie[0].(*ui.Boton)
	if accion.Texto != c.t("matrix.resetAction") || accion.Variante != ui.Peligro || !accion.Inerte {
		t.Fatal("el botón de restablecer no es de peligro o está activo sin confirmar")
	}
	fyne.DoAndWait(func() { casilla.Tapped(&fyne.PointEvent{}) })
	if accion.Inerte {
		t.Fatal("marcar la casilla no habilita el restablecimiento")
	}
	fyne.DoAndWait(func() { casilla.Tapped(&fyne.PointEvent{}) })
	if !accion.Inerte {
		t.Fatal("desmarcar la casilla no vuelve a bloquear el restablecimiento")
	}
	back := pie[1].(*ui.Boton)
	fyne.DoAndWait(back.AlPulsar)
	_, pie = c.Objetos()
	for _, o := range pie {
		if b, ok := o.(*ui.Boton); ok && b.Texto == c.t("matrix.resetAction") {
			t.Fatal("volver no deshizo el paso de confirmación")
		}
	}
}
