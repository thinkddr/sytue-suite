// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"net/url"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
)

// TestRecuerdaVistaYOcultosPorPersona es el «hecho cuando» de la pieza: ocultar
// «Personal» y pasar a semana, volver a abrir la app → semana y «Personal» oculto; otra
// cuenta en el mismo equipo no lo hereda; y un calendario que no estaba (aquí, «Equipo»
// la primera vez que se guarda) aparece encendido porque se guardan los OCULTOS.
func TestRecuerdaVistaYOcultosPorPersona(t *testing.T) {
	prefs := test.NewApp().Preferences()
	quien := "ana"
	almacen := sesion.NuevoPorPersona(prefs, func() sesion.Alcance { return sesion.Alcance{Sujeto: quien} })

	a := appDePrueba(t, &servicioFalso{})
	a.Almacen = almacen
	a.Arrancar()
	if a.P.Vista != VistaMes || !a.P.visible("mio") || !a.P.visible("suyo") {
		t.Fatalf("sin nada guardado: vista %d, visibles %v", a.P.Vista, a.P.Visibles)
	}
	a.P.AlConmutarCalendario("mio")
	a.P.AlCambiarVista(VistaSemana)

	// Volver a abrir la app: una App nueva sobre el mismo almacén.
	b := appDePrueba(t, &servicioFalso{})
	b.Almacen = almacen
	b.Arrancar()
	if b.P.Vista != VistaSemana {
		t.Errorf("al volver, la vista es %d y se dejó en semana", b.P.Vista)
	}
	if b.P.visible("mio") || !b.P.visible("suyo") {
		t.Errorf("al volver, visibles = %v: «Personal» tenía que seguir oculto", b.P.Visibles)
	}

	// Otra cuenta en el mismo equipo empieza como siempre.
	quien = "beto"
	c := appDePrueba(t, &servicioFalso{})
	c.Almacen = almacen
	c.Arrancar()
	if c.P.Vista != VistaMes || !c.P.visible("mio") || !c.P.visible("suyo") {
		t.Errorf("Beto hereda lo de Ana: vista %d, visibles %v", c.P.Vista, c.P.Visibles)
	}

	// Y si Ana vuelve a encenderlo, se olvida.
	quien = "ana"
	b.P.AlConmutarCalendario("mio")
	if got := almacen.Leer(ClaveOcultos); got != "" {
		t.Errorf("con todo encendido quedan ocultos guardados: %q", got)
	}
}

// TestRecuerdaElCambioDePersonaEnLaMismaVentana: `Arrancar` corre al cambiar quién ha
// entrado, sin cerrar la ventana. Lo que ocultó la anterior no sigue oculto para la
// nueva, y la nueva recupera lo suyo.
func TestRecuerdaElCambioDePersonaEnLaMismaVentana(t *testing.T) {
	prefs := test.NewApp().Preferences()
	quien := "beto"
	almacen := sesion.NuevoPorPersona(prefs, func() sesion.Alcance { return sesion.Alcance{Sujeto: quien} })
	almacen.Poner(ClaveVista, "agenda")
	almacen.Poner(ClaveOcultos, `["suyo"]`)

	quien = "ana"
	a := appDePrueba(t, &servicioFalso{})
	a.Almacen = almacen
	a.Arrancar()
	a.P.AlConmutarCalendario("mio")

	quien = "beto"
	a.Arrancar()
	if a.P.Vista != VistaAgenda || !a.P.visible("mio") || a.P.visible("suyo") {
		t.Errorf("Beto entra con vista %d y visibles %v", a.P.Vista, a.P.Visibles)
	}
}

// TestRecuerdaPeroElEnlaceManda: un `?date=` que llega antes de entrar abre el día, y lo
// guardado no lo pisa ni el enlace cambia lo guardado.
func TestRecuerdaPeroElEnlaceManda(t *testing.T) {
	prefs := test.NewApp().Preferences()
	almacen := sesion.NuevoPorPersona(prefs, func() sesion.Alcance { return sesion.Alcance{Sujeto: "ana"} })
	almacen.Poner(ClaveVista, "week")

	a := appDePrueba(t, &servicioFalso{})
	a.Almacen = almacen
	a.AtenderEnlace(&url.URL{RawQuery: "date=2026-09-10"})
	a.Arrancar()
	if a.P.Vista != VistaDia {
		t.Errorf("el enlace pedía el día y la vista es %d", a.P.Vista)
	}
	if got := almacen.Leer(ClaveVista); got != "week" {
		t.Errorf("el enlace cambió la vista guardada a %q", got)
	}
	// La siguiente vez que entra alguien, ya sin enlace, vuelve lo guardado.
	a.Arrancar()
	if a.P.Vista != VistaSemana {
		t.Errorf("sin enlace, la vista es %d y la guardada es semana", a.P.Vista)
	}
}

// TestRecuerdaSinAlmacenEsComoSiempre: sin almacén, mes y todos visibles.
func TestRecuerdaSinAlmacenEsComoSiempre(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{})
	a.Arrancar()
	a.P.AlConmutarCalendario("mio")
	a.P.AlCambiarVista(VistaSemana)
	b := appDePrueba(t, &servicioFalso{})
	b.Arrancar()
	if b.P.Vista != VistaMes || !b.P.visible("mio") {
		t.Errorf("sin almacén: vista %d, visibles %v", b.P.Vista, b.P.Visibles)
	}
}

// TestRecuerdaQueOtraPersonaSinNadaGuardadoEmpiezaEnElMes: Ana deja la semana y, sin
// cerrar la ventana, entra Beto, que no tiene nada guardado → el mes, no la semana de Ana.
func TestRecuerdaQueOtraPersonaSinNadaGuardadoEmpiezaEnElMes(t *testing.T) {
	prefs := test.NewApp().Preferences()
	quien := "ana"
	almacen := sesion.NuevoPorPersona(prefs, func() sesion.Alcance { return sesion.Alcance{Sujeto: quien} })

	a := appDePrueba(t, &servicioFalso{})
	a.Almacen = almacen
	a.Arrancar()
	a.P.AlCambiarVista(VistaSemana)

	quien = "beto"
	a.Arrancar()
	if a.P.Vista != VistaMes {
		t.Errorf("Beto entra con la vista %d de Ana", a.P.Vista)
	}
}
