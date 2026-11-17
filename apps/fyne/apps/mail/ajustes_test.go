// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElLateralTieneUnaSolaPuertaALosAjustes: las ocho entradas sueltas de antes se
// van del lateral; queda «Ajustes», que navega a la página (no despliega nada).
func TestElLateralTieneUnaSolaPuertaALosAjustes(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	if len(b.P.Menu.Carpetas) == 0 {
		t.Fatal("sin carpetas cargadas, la prueba no mira nada")
	}
	sec := b.P.Menu.seccionAjustes(tema.Claro)
	if len(sec.Filas) != 1 || sec.Titulo != "" {
		t.Fatalf("la sección de ajustes tiene %d filas y título %q: debe ser una fila sin título", len(sec.Filas), sec.Titulo)
	}
	f, ok := sec.Filas[0].(*ui.FilaLateral)
	if !ok || f.Activa {
		t.Fatal("«Ajustes» no es una fila del lateral, o arranca activa")
	}
	f.Tapped(&fyne.PointEvent{})
	if b.P.Ajustes == nil || !b.P.Menu.EnAjustes {
		t.Fatal("pulsar «Ajustes» no abre la página")
	}
	if !b.P.Menu.seccionAjustes(tema.Claro).Filas[0].(*ui.FilaLateral).Activa {
		t.Error("con la página abierta, «Ajustes» no es la entrada activa")
	}
	for _, c := range b.P.Menu.Carpetas {
		if b.P.Menu.filaCarpeta(c, tema.Claro).Activa {
			t.Errorf("con la página abierta, «%s» sigue siendo la entrada activa", c.Nombre)
		}
	}
}

// TestLaPaginaDeAjustesOcupaLaListaYDiceElEstado: Cmd+, abre la página en lugar de la
// lista y el lector, con las ocho filas en sus cinco secciones; la firma y la
// autorespuesta dicen su estado sin abrir nada, y cada botón abre su editor.
func TestLaPaginaDeAjustesOcupaLaListaYDiceElEstado(t *testing.T) {
	s := servicioConTodas()
	fin := time.Now().AddDate(0, 0, 10)
	s.vacaciones = api.MailVacation{Enabled: true, Subject: "Fuera", EndsAt: &fin}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.firma = "Ana\nSytue"
	tr := traductor(t)

	if !b.AtajoPersonalizado(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}) {
		t.Fatal("el atajo Cmd+, no se reconoce")
	}
	p := b.P.Ajustes
	if p == nil {
		t.Fatal("Cmd+, no abre la página de ajustes")
	}
	if b.P.armazon.Panel != nil || b.P.armazon.Contenido == fyne.CanvasObject(b.P.Lector) {
		t.Fatal("la página no ocupa el sitio de la lista y el lector")
	}
	if s.pidio("Vacation") == 0 {
		t.Fatal("la página no lee el estado de la autorespuesta")
	}
	if e := p.estado("signature"); e == nil || e.Rotulo != tr.T("settings.signatureOn", nil) {
		t.Errorf("con firma guardada, la fila dice %+v", e)
	}
	if e := p.estado("vacation"); e == nil || e.Rotulo != tr.T("vacation.statusOnLabel", nil) || e.Tono != ui.InsigniaExito {
		t.Errorf("con la autorespuesta encendida, la fila dice %+v", e)
	}
	for _, clave := range []string{"signature", "vacation", "aliases", "shared", "rules", "smime-anchor", "smime-identity", "mail-clients"} {
		if p.Boton(clave) == nil {
			t.Errorf("falta la fila %q en la página", clave)
		}
	}
	// El nombre accesible dice QUÉ se edita: «Editar» solo se repite en varias filas.
	if got, want := p.Boton("vacation").AccessibilityLabel(), tr.T("settings.edit", nil)+": "+tr.T("nav.vacation", nil); got != want {
		t.Errorf("nombre accesible %q, quería %q", got, want)
	}

	test.Tap(p.Boton("vacation"))
	if !hayDialogo(w.Canvas(), tr.T("vacation.title", nil)) {
		t.Fatal("«Editar» de Vacaciones no abre su editor")
	}
}

// TestGuardarEnUnEditorRefrescaLaPagina: lo recién guardado es lo primero que se mira.
func TestGuardarEnUnEditorRefrescaLaPagina(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	b.verAjustes()
	if e := b.P.Ajustes.estado("vacation"); e == nil || e.Rotulo != tr.T("vacation.statusOffLabel", nil) {
		t.Fatalf("apagada, la fila dice %+v", e)
	}
	g := b.verVacaciones()
	g.conmutador.Encendido = true
	g.salvar()
	if e := b.P.Ajustes.estado("vacation"); e == nil || e.Rotulo != tr.T("vacation.statusOnLabel", nil) {
		t.Errorf("tras encenderla, la fila sigue diciendo %+v", e)
	}
}

// TestElegirUnaCarpetaCierraLosAjustes: el lateral es navegación; volver al correo
// devuelve la lista y el lector.
func TestElegirUnaCarpetaCierraLosAjustes(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.verAjustes()
	b.ElegirCarpeta(Carpeta{ID: "in", Nombre: "INBOX", Rol: "inbox"})
	if b.P.Ajustes != nil || b.P.Menu.EnAjustes {
		t.Fatal("elegir una carpeta no cierra la página de ajustes")
	}
	if b.P.armazon.Panel == nil || b.P.armazon.Contenido != fyne.CanvasObject(b.P.Lector) {
		t.Fatal("al volver no están la lista y el lector")
	}
}

// TestLaPaginaDeAjustesNoDesbordaEnElTelefono: en estrecho el botón baja bajo su texto
// (`flex-wrap` con `basis-64`) y nada sale de la página; en ancho va a la derecha.
func TestLaPaginaDeAjustesNoDesbordaEnElTelefono(t *testing.T) {
	for _, ancho := range []float32{390, 1280} {
		b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(ancho, 800))
		b.verAjustes()
		p := b.P.Ajustes
		w := test.NewWindow(p.Objeto(tema.Claro))
		w.SetPadded(false)
		w.Resize(fyne.NewSize(ancho, 800))
		marco, ok := p.desplaza.Content.(*ui.MarcoAjustes)
		if !ok {
			t.Fatalf("la página es %T y tiene que ser el marco de ajustes de la suite", p.desplaza.Content)
		}
		marco.Resize(fyne.NewSize(ancho, marco.AltoPara(ancho)))
		canalon := ui.MargenMarcoAjustesX
		if ancho >= ui.CorteSM {
			canalon = ui.MargenMarcoAjustesXSM
		}
		for _, o := range marco.Secciones {
			s, ok := o.(*ui.SeccionAjustes)
			if !ok {
				t.Fatalf("una sección es %T y tiene que ser la de la suite", o)
			}
			for _, f := range s.Cuerpo.(*fyne.Container).Layout.(*listaAjustes).filas {
				// El botón, en la página: lo pone su sección, que pone el marco.
				der := marco.Position().X + s.Position().X + s.Cuerpo.Position().X + f.boton.Position().X + f.boton.Size().Width
				if der <= canalon || der > ancho-canalon+0.5 {
					t.Errorf("%v: el botón de %q acaba en %v y la página en %v", ancho, f.clave, der, ancho-canalon)
				}
				if f.apilada != (ancho < ui.CorteSM) {
					t.Errorf("%v: la fila %q apilada=%v", ancho, f.clave, f.apilada)
				}
			}
		}
		w.Close()
	}
}

// TestEnAjustesLosAtajosDeLaListaCallan: la página no enseña lista ni lector, así que
// j/k o archivar no pueden mover ni tocar una conversación escondida; «c» sigue
// redactando, como en la web.
func TestEnAjustesLosAtajosDeLaListaCallan(t *testing.T) {
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	if len(b.P.Bandeja.Conversaciones) == 0 {
		t.Fatal("sin conversaciones, la prueba no mira nada")
	}
	b.verAjustes()
	for _, r := range []rune{'j', 'k', 'e', 's', '#', 'x', '/'} {
		if b.Atajo(w.Canvas(), r) {
			t.Errorf("en la página de ajustes, %q sigue actuando", r)
		}
	}
	if b.hiloID != "" {
		t.Fatalf("j abrió %q detrás de la página", b.hiloID)
	}
	if !b.Atajo(w.Canvas(), 'c') {
		t.Error("en la página de ajustes, «c» ya no redacta")
	}
}

// TestUnAvisoDelHubSacaDeLosAjustes: abrir una conversación desde un aviso con la
// página puesta la deja a la vista, no escondida detrás.
func TestUnAvisoDelHubSacaDeLosAjustes(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	if len(b.P.Bandeja.Conversaciones) == 0 {
		t.Fatal("sin conversaciones, la prueba no mira nada")
	}
	b.verAjustes()
	b.AbrirHilo(b.P.Bandeja.Conversaciones[0].HiloID)
	if b.P.Ajustes != nil || b.P.Menu.EnAjustes {
		t.Fatal("el aviso abre la conversación detrás de la página de ajustes")
	}
	if b.P.armazon.Contenido != fyne.CanvasObject(b.P.Lector) {
		t.Fatal("tras el aviso no está el lector")
	}
}
