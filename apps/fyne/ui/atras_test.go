// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// TestAtrasVaALaApp: el «atrás» de Android cierra primero lo de encima; sin nada encima
// lo recibe la app (volver de un vídeo a la lista), con el foco en un campo igual que
// sin foco. Y la ranura de teclas que tuviera la app sigue recibiendo lo demás.
//
// VISTO EN ROJO con `atras` llamando directamente a GoBack (lo que hacía la envoltura):
// la app no se entera y el botón manda la app al fondo desde el detalle.
func TestAtrasVaALaApp(t *testing.T) {
	campo := NuevaEntrada("")
	w := test.NewWindow(campo)
	t.Cleanup(w.Close)
	c := w.Canvas()
	var teclas []fyne.KeyName
	c.SetOnTypedKey(func(k *fyne.KeyEvent) { teclas = append(teclas, k.Name) })
	atrases := 0
	AlAtras(c, func() bool { atrases++; return true })

	c.OnTypedKey()(tecla(mobile.KeyBack))
	if atrases != 1 {
		t.Fatalf("atrás sin foco llegó %d veces a la app", atrases)
	}
	c.OnTypedKey()(tecla(fyne.KeySpace))
	if len(teclas) != 1 || teclas[0] != fyne.KeySpace {
		t.Errorf("la ranura de la app recibió %v", teclas)
	}

	c.Focus(campo)
	campo.TypedKey(tecla(mobile.KeyBack))
	if atrases != 2 {
		t.Errorf("atrás con el foco en un campo llegó %d veces a la app", atrases)
	}

	d := NuevoDialogo("Diálogo", canvas.NewText("x", nil))
	d.Mostrar(c)
	c.Unfocus()
	c.OnTypedKey()(tecla(mobile.KeyBack))
	if atrases != 2 || c.Overlays().Top() != nil {
		t.Errorf("con un diálogo abierto, atrás lo tenía que cerrar sin llegar a la app (%d, %v)", atrases, c.Overlays().Top())
	}
}

func TestAtrasVuelveAlPanelDespuesDeGirar(t *testing.T) {
	tactilAntes := tactilDePrueba
	tactil := true
	tactilDePrueba = &tactil
	t.Cleanup(func() { tactilDePrueba = tactilAntes })
	campo := NuevaEntrada("")
	a := NuevoArmazon(campo)
	a.Panel = canvas.NewRectangle(magenta)
	vueltas := 0
	a.AlVolver = func() { vueltas++ }
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	t.Cleanup(b.Parar)
	w := test.NewTempWindow(t, b.Montar(a))
	c := w.Canvas()
	for _, tam := range []fyne.Size{{Width: 390, Height: 844}, {Width: 844, Height: 390}, {Width: 390, Height: 300}} {
		w.Resize(tam)
		a.DetalleAbierto = true
		a.Refresh()
		antes := vueltas
		if c.OnTypedKey() == nil {
			t.Fatal("el montaje no instaló el retorno móvil sin foco")
		}
		c.Unfocus()
		c.OnTypedKey()(tecla(mobile.KeyBack))
		if a.DetalleAbierto || vueltas != antes+1 {
			t.Fatalf("%v: Atrás no volvió a la lista: detalle=%v vueltas=%d", tam, a.DetalleAbierto, vueltas)
		}
		a.DetalleAbierto = true
		a.Refresh()
		c.Focus(campo)
		campo.TypedKey(tecla(mobile.KeyBack))
		if a.DetalleAbierto || vueltas != antes+2 {
			t.Fatalf("%v: Atrás con foco no usa la misma acción que volver", tam)
		}
	}
}
