// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func servicioConCompartidos() *servicioFalso {
	s := servicioConTodas()
	s.compartidos = []api.MailShared{
		{AccountID: "s1", Address: "soporte@example.org", Kind: "shared", Permission: "send"},
		{AccountID: "u1", Address: "jefa@example.org", Kind: "user", Permission: "read"},
	}
	s.deCompartido = []api.MailMessage{sobre("c1", "hc", "s1-in", "Pedido 42", false, 0)}
	s.sigCompartido = "k2"
	return s
}

// TestUnBuzonCompartidoSeLeeSinTocarlo: la sección sale con los dos; elegir uno pide
// su Recibidos y enseña sus mensajes en solo lectura; abrir no marca leído ni pide el
// hilo; la segunda página va a su carpeta; y volver a una carpeta propia lo deja.
func TestUnBuzonCompartidoSeLeeSinTocarlo(t *testing.T) {
	s := servicioConCompartidos()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	if len(b.P.Menu.Compartidos) != 2 || !b.P.Menu.Compartidos[1].Delegado {
		t.Fatalf("lateral: %+v", b.P.Menu.Compartidos)
	}
	b.ElegirCompartido(b.P.Menu.Compartidos[0])
	if !slices.Contains(s.llamadas, "SharedMessages(s1,s1-in,)") || len(b.P.Bandeja.Conversaciones) != 1 {
		t.Fatalf("lista: %v", s.llamadas)
	}
	if !b.P.Bandeja.SoloLectura || !b.P.Lector.SoloLectura || b.P.Acciones.Archivar != nil || b.P.Bandeja.Titulo != "soporte@example.org" {
		t.Fatalf("no está en solo lectura: %+v", b.P.Acciones)
	}
	antes := len(s.llamadas)
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	for _, l := range s.llamadas[antes:] {
		if l == "Thread(hc)" || l == "SetFlagMany(\\Seen)" {
			t.Errorf("en un compartido se ha llamado %s", l)
		}
	}
	b.MasPagina()
	if !slices.Contains(s.llamadas, "SharedMessages(s1,s1-in,k2)") {
		t.Errorf("la segunda página no va al compartido: %v", s.llamadas)
	}
	b.ElegirCarpeta(b.P.Menu.Carpetas[0])
	if b.compartido != nil || b.P.Bandeja.SoloLectura || b.P.Menu.CompartidoID != "" || b.P.Acciones.Archivar == nil {
		t.Error("volver a una carpeta propia no deja el compartido")
	}
}

// TestDesdeUnCompartidoSeEscribeConSuDireccion: con permiso de envío, redactar y
// responder salen desde él; con solo lectura, desde la propia. Y el «De» lo ofrece.
func TestDesdeUnCompartidoSeEscribeConSuDireccion(t *testing.T) {
	s := servicioConCompartidos()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	if o := b.opcionesDe(); !slices.Contains(o, "soporte@example.org") || slices.Contains(o, "jefa@example.org") {
		t.Errorf("«De»: %v", o)
	}
	b.ElegirCompartido(b.P.Menu.Compartidos[0])
	b.Redactar()
	if b.redaccion.r.B.De != "soporte@example.org" {
		t.Errorf("redactar desde el compartido sale de %q", b.redaccion.r.B.De)
	}
	b.cerrar(b.redaccion)
	b.ElegirCompartido(b.P.Menu.Compartidos[1])
	if de := b.deCompartido(); de != "" {
		t.Errorf("con solo lectura se escribiría desde %q", de)
	}
}

// TestLaDelegacionYElAccesoSeGestionan: delegar pone a la persona en la lista (y
// vuelve si falla), crear pide la dirección bien escrita, y en uno que gestiono se da
// y se quita acceso.
func TestLaDelegacionYElAccesoSeGestionan(t *testing.T) {
	s := servicioConCompartidos()
	s.compartidos[0].Permission = "manage"
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verCompartidos()
	if !hayDialogo(w.Canvas(), tr.T("shared.title", nil)) || g.estado != Listo || len(g.gestionados()) != 1 {
		t.Fatalf("diálogo: %v %+v", g.estado, g.gestionados())
	}
	g.persona.PonerTexto("asistente")
	g.delegar()
	if s.pidio("Delegate") != 0 {
		t.Fatal("se delega en una dirección a medias")
	}
	g.persona.PonerTexto("asistente@example.org")
	g.permDel = "send"
	g.delegar()
	if !slices.Contains(s.llamadas, "Delegate(asistente@example.org,send)") || len(g.delegados) != 1 || g.persona.Texto() != "" {
		t.Fatalf("delegar: %v %+v", s.llamadas, g.delegados)
	}
	s.errAcceso = api.ErrMailTooLarge
	g.retirarDelegacion(g.delegados[0])
	if len(g.delegados) != 1 {
		t.Error("la retirada rechazada ha quitado a la persona")
	}
	s.errAcceso = nil

	g.direccion.PonerTexto("ventas@example.org")
	g.nombre.PonerTexto("Ventas")
	g.crear()
	if !slices.Contains(s.llamadas, "CreateSharedMailbox(ventas@example.org,Ventas)") || len(g.gestionados()) != 2 {
		t.Fatalf("crear: %v", s.llamadas)
	}

	g.abrirACL(g.gestionados()[0])
	if g.editando == nil || g.estadoACL != Listo {
		t.Fatal("no se abre el acceso")
	}
	g.personaACL.PonerTexto("agente@example.org")
	g.permACL = "manage"
	g.pintar()
	g.darAcceso()
	if !slices.Contains(s.llamadas, "Grant(s1,agente@example.org,manage)") || len(g.accesos) != 1 {
		t.Fatalf("dar acceso: %v %+v", s.llamadas, g.accesos)
	}
	g.quitarAcceso(g.accesos[0])
	if len(g.accesos) != 0 || !slices.Contains(s.llamadas, "Revoke(s1,agente@example.org)") {
		t.Errorf("quitar acceso: %v", s.llamadas)
	}
	if got := RotuloPermiso(tr, "send", true); got != tr.T("shared.permSendOnBehalf", nil) {
		t.Errorf("«send» delegado es %q", got)
	}
}

// TestLaCapturaDeCompartidosSeGuarda (SYTUE_CAPTURAS): el lateral con la sección, el
// diálogo y el acceso a uno.
func TestLaCapturaDeCompartidosSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	s := servicioConCompartidos()
	s.compartidos[0].Permission = "manage"
	s.delegados = []api.MailGrant{{Grantee: "asistente@example.org", Permission: "send"}}
	s.accesos = map[string][]api.MailGrant{"s1": {{Grantee: "agente@example.org", Permission: "read"}}}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.ElegirCompartido(b.P.Menu.Compartidos[0])
	guardar(t, filepath.Join(dir, "compartido.png"), w.Canvas().Capture())
	g := b.verCompartidos()
	guardar(t, filepath.Join(dir, "compartidos.png"), w.Canvas().Capture())
	g.abrirACL(g.gestionados()[0])
	g.permACL = "manage"
	g.pintar()
	guardar(t, filepath.Join(dir, "compartidos-acceso.png"), w.Canvas().Capture())
}

// TestLasCarpetasCargandoLlevanSuRueda: el lateral mientras llegan las carpetas es
// la rueda de 16 con «Cargando…», como la web (no solo el texto).
func TestLasCarpetasCargandoLlevanSuRueda(t *testing.T) {
	m := NuevaPantalla(traductor(t), "es").Menu
	m.EstadoCarpetas = Cargando
	s := m.seccionCarpetas(0)
	var hay bool
	var mirar func(fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if _, es := o.(*ui.Rueda); es {
			hay = true
		}
		if c, es := o.(*fyne.Container); es {
			for _, h := range c.Objects {
				mirar(h)
			}
		}
	}
	for _, f := range s.Filas {
		mirar(f)
	}
	if !hay {
		t.Error("cargando carpetas, el lateral no lleva rueda")
	}
}
