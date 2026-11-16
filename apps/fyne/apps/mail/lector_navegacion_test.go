// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestElLectorPasaDeConversacionYPideLaPaginaSiguiente es MAIL-11 (la web en
// `lector-navegacion.test.tsx`): «Más reciente» y «Más antigua» hacen lo que j/k sin
// volver a la lista, se apagan en los extremos y, desde la última conversación
// cargada, piden la página siguiente y abren la que llega.
func TestElLectorPasaDeConversacionYPideLaPaginaSiguiente(t *testing.T) {
	// Sin ventana: la lista no se pinta y su centinela no pide la segunda página
	// por su cuenta; aquí la tiene que pedir el botón.
	b := buzonConAvisos(t, servicioDeEjemplo())
	b.Arrancar(context.Background())
	l, convs := b.P.Lector, b.P.Bandeja.Conversaciones
	if len(convs) != 2 || !b.P.Bandeja.HayMas {
		t.Fatalf("la primera página son %d conversaciones (HayMas %v): el ejemplo ha cambiado", len(convs), b.P.Bandeja.HayMas)
	}
	if l.AlNavegar == nil {
		t.Fatal("el lector no lleva los botones de navegar")
	}

	b.Abrir(convs[0])
	if l.HayAnterior || !l.HaySiguiente {
		t.Errorf("en la más reciente: anterior %v (tocaba apagado), siguiente %v", l.HayAnterior, l.HaySiguiente)
	}
	l.AlNavegar(true)
	if b.hiloID != convs[1].HiloID {
		t.Fatalf("«Más antigua» abre %q y tocaba %q", b.hiloID, convs[1].HiloID)
	}
	if !l.HayAnterior || !l.HaySiguiente {
		t.Errorf("en la última cargada con más páginas los dos van encendidos: %v %v", l.HayAnterior, l.HaySiguiente)
	}

	l.AlNavegar(true) // desde la última cargada: pide «c2» y abre lo que trae
	if b.hiloID != "h4" {
		t.Fatalf("tras pedir la página siguiente está abierta %q y tocaba h4", b.hiloID)
	}
	if l.HaySiguiente || l.CargandoSiguiente || b.avanceTrasCargar != "" {
		t.Errorf("en la más antigua de todas: siguiente %v, cargando %v, pendiente %q",
			l.HaySiguiente, l.CargandoSiguiente, b.avanceTrasCargar)
	}

	l.AlNavegar(false)
	if b.hiloID != convs[1].HiloID {
		t.Errorf("«Más reciente» abre %q y tocaba volver a %q", b.hiloID, convs[1].HiloID)
	}
}

// TestLosBotonesDeNavegarVanEnLaFilaDelAsunto: dos `iconOnly` arriba a la derecha,
// con su nombre accesible, apagados en el extremo, sin pisar la cuenta de mensajes y
// sin ensanchar la barra de acciones.
func TestLosBotonesDeNavegarVanEnLaFilaDelAsunto(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioDeEjemplo(), fyne.NewSize(1280, 800))
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	l := b.P.Lector
	r := test.WidgetRenderer(l).(*lectorRenderer)
	if r.nav == nil || len(r.nav.Objects) != 2 {
		t.Fatal("no hay botones de navegar en la cabecera del lector")
	}
	tr := traductor(t)
	nuevo, viejo := r.nav.Objects[0].(*ui.Boton), r.nav.Objects[1].(*ui.Boton)
	if nuevo.Rotulo != tr.T("reader.newer", nil) || viejo.Rotulo != tr.T("reader.older", nil) {
		t.Errorf("rótulos %q y %q", nuevo.Rotulo, viejo.Rotulo)
	}
	if !nuevo.Disabled() || viejo.Disabled() {
		t.Errorf("en la más reciente: «Más reciente» inerte %v (tocaba sí), «Más antigua» %v", nuevo.Disabled(), viejo.Disabled())
	}
	ancho := l.Size().Width
	if fin := r.nav.Position().X + r.nav.Size().Width; fin > ancho-MargenLector+0.5 {
		t.Errorf("los botones acaban en %v, fuera del `px-6` (%v)", fin, ancho-MargenLector)
	}
	if r.cuenta.Visible() && r.cuenta.Position().X+r.cuenta.Size().Width > r.nav.Position().X {
		t.Error("la cuenta de mensajes pisa los botones")
	}
	if r.barra.Position().Y < r.nav.Position().Y+r.nav.Size().Height {
		t.Error("la barra de acciones empieza antes de que acaben los botones")
	}
}
