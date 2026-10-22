// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
)

// Sin sesión, una app de la suite enseña «Iniciar sesión» en lugar de su contenido, y
// al salir de la sesión vuelve a ella. Antes cada app pintaba su malentendido —Mail y
// Rooms «Cargando…» para siempre, Contacts una libreta vacía— (visto en una ventana
// real el 2026-09-27). Los editores, que abren ficheros del equipo, lo dicen ellos.
//
// VISTO EN ROJO sin la llamada a `montarSinSesion` en Montar: el contenido sigue ahí.
func TestSinSesionLaAppPideIniciarSesion(t *testing.T) {
	s := suiteConSesion(t)
	contenido := container.NewStack(canvas.NewRectangle(nil))
	barra := NuevaBarraSuite(traductorPrueba(t), s, iconos.Mail)
	t.Cleanup(barra.Parar)
	c := barra.Montar(contenido)
	if ContenidoMontado(c) != contenido {
		t.Fatal("con sesión se ve el contenido de la app")
	}

	s.Salir()
	var vacio *Vacio
	for limite := time.Now().Add(5 * time.Second); vacio == nil; time.Sleep(10 * time.Millisecond) {
		fyne.DoAndWait(func() { vacio, _ = ContenidoMontado(c).(*Vacio) })
		if time.Now().After(limite) {
			t.Fatalf("al cerrar la sesión sigue %T, quiero la página «Iniciar sesión»", ContenidoMontado(c))
		}
	}
	tr := traductorPrueba(t)
	if vacio.Titulo != tr.T("account.signedOutTitle", nil) || vacio.Accion == nil {
		t.Errorf("la página dice %q y botón %v", vacio.Titulo, vacio.Accion != nil)
	}
	if b, ok := vacio.Accion.(*Boton); !ok || b.Texto != tr.T("account.signIn", nil) {
		t.Errorf("el botón no es «%s»", tr.T("account.signIn", nil))
	}

	// Un editor sin sesión sigue enseñando lo suyo.
	editor := NuevaBarraSuite(traductorPrueba(t), s, iconos.Docs).ConSinSesionPropia()
	t.Cleanup(editor.Parar)
	propio := editor.Montar(contenido)
	if ContenidoMontado(propio) != contenido {
		t.Error("con ConSinSesionPropia la app pinta ella lo que ve quien no ha entrado")
	}
}
