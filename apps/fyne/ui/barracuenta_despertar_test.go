// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// casillaDespertar busca «Actualizar los widgets con la app cerrada» en las
// preferencias del menú de cuenta; nil si no está.
func casillaDespertar(c *cuentaBarra, s fyne.CanvasObject) *Eleccion {
	rotulo := c.t.T("shell.widgets.wake", nil)
	var hallada *Eleccion
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *Eleccion:
			if v.Etiqueta == rotulo {
				hallada = v
			}
		case *fyne.Container:
			for _, h := range v.Objects {
				mirar(h)
			}
		}
	}
	mirar(s)
	return hallada
}

// TestLaCasillaDelDespertarSoloConElModoDeFondo: la casilla sale solo en la compilación
// con SYTUE_DESPERTAR_APPLE (el modo de fondo `remote-notification`, aquí
// EscenaDePrueba.ProbarAvisos) y con sesión; empieza APAGADA, y si encenderla no se
// guarda, vuelve a decir lo que vale de verdad.
//
// VISTO EN ROJO de las tres maneras: enseñándola sin mirar DespertarDisponible, con
// `Marcada = true` de salida y sin devolverla tras el fallo.
func TestLaCasillaDelDespertarSoloConElModoDeFondo(t *testing.T) {
	antes := fyne.CurrentApp()
	// Envuelve la app de TestMain, con el tema de la suite: `test.NewTempApp` dejaría
	// detrás una de serie y sin Figtree (ver TestNingunTestMontaUnaAppDeSerie).
	fyne.SetCurrentApp(appRooms{antes})
	t.Cleanup(func() { fyne.SetCurrentApp(antes) })
	t.Cleanup(widgets.SimularLector(true))
	t.Cleanup(widgets.SimularInstalados(1))
	s := suiteConSesion(t)
	s.PublicadorWidget(func(context.Context) (widgets.Detail, error) { return widgets.Detail{}, nil })
	escena := s.ProbarSinEscena()
	escena.Conectar()
	c := nuevaCuentaBarra(traductorPrueba(t), s)

	// Sin el modo de fondo declarado, no está.
	if casillaDespertar(c, c.preferencias(s.Estado())) != nil {
		t.Fatal("sin SYTUE_DESPERTAR_APPLE la casilla no puede salir")
	}
	// Sin sesión, tampoco.
	escena.ProbarAvisos(strings.Repeat("ab01", 8))
	if casillaDespertar(c, c.preferencias(sesion.Estado{})) != nil {
		t.Fatal("sin sesión la casilla no puede salir")
	}

	casilla := casillaDespertar(c, c.preferencias(s.Estado()))
	if casilla == nil {
		t.Fatal("con el modo de fondo y sesión, falta la casilla")
	}
	if casilla.Marcada {
		t.Fatal("la casilla tiene que empezar apagada")
	}

	// Que no se pueda guardar: una carpeta de ajustes relativa la rechaza el sistema.
	t.Setenv("XDG_CONFIG_HOME", "relativa")
	test.NewTempWindow(t, casilla)
	var pedido atomic.Bool
	if alCambiar := casilla.AlCambiar; alCambiar != nil {
		casilla.AlCambiar = func(encender bool) { pedido.Store(encender); alCambiar(encender) }
	}
	test.Tap(casilla)
	limite := time.Now().Add(5 * time.Second)
	for {
		var marcada bool
		fyne.DoAndWait(func() { marcada = casilla.Marcada })
		if !marcada {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("el despertar no se ha guardado y la casilla sigue marcada")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !pedido.Load() {
		t.Fatal("el toque no ha pedido encender el despertar")
	}
}
