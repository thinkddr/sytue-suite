// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos

package mail

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Contestar marca el original `\Answered`, que es lo que Thunderbird o el iPhone leen,
// y la fila lo enseña con una flecha que tiene nombre accesible. La marca espera a que
// el envío SALGA —no se pone dentro de la ventana de deshacer— y deshacer a tiempo la
// quita. Es `respondido.test.tsx` de la web.
//
// VISTO EN ROJO (2026-11-13): sin `Responde` en BorradorDeRespuesta, sin
// `retenerRespuesta` en `enviar` o sin `anularRespuesta` en `deshacerEnvio`, fallan.

// plazosDeDeshacer aparta las esperas de la ventana de deshacer: las demás (el
// autoguardado) no son de estas pruebas.
func plazosDeDeshacer(b *Buzon) *[]func() {
	var xs []func()
	b.Esperar = func(d time.Duration, f func()) func() {
		if d == VentanaDeshacer*time.Second {
			xs = append(xs, f)
		}
		return func() {}
	}
	return &xs
}

func cumplir(xs *[]func()) {
	fs := *xs
	*xs = nil
	for _, f := range fs {
		f()
	}
}

func marcasDe(b *Buzon, id string) []string {
	for _, m := range b.mensajes {
		if m.ID == id {
			return m.Keywords
		}
	}
	return nil
}

func conversacionDe(b *Buzon, id string) (Conversacion, bool) {
	for _, c := range b.P.Bandeja.Conversaciones {
		for _, m := range c.Mensajes {
			if m.ID == id {
				return c, true
			}
		}
	}
	return Conversacion{}, false
}

// responderYEnviar contesta a m2 («Encuesta», un hilo de uno) y lo envía.
func responderYEnviar(t *testing.T, modo Respuesta) (*Buzon, *servicioFalso, *[]func()) {
	t.Helper()
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	plazos := plazosDeDeshacer(b)
	m, ok := conversacionDe(b, "m2")
	if !ok {
		t.Fatal("m2 no está en la bandeja")
	}
	b.responder(m.Ultimo, modo)
	if b.redaccion == nil || b.redaccion.r.B.Responde != "m2" {
		t.Fatalf("el borrador de la respuesta no sabe a qué contesta: %+v", b.redaccion)
	}
	b.enviar(b.redaccion, false)
	if s.pidio("Send") != 1 {
		t.Fatalf("no se ha enviado: %v", s.llamadas)
	}
	return b, s, plazos
}

func TestResponderMarcaElOriginalAlSalir(t *testing.T) {
	for _, modo := range []Respuesta{Responder, ResponderATodos} {
		b, s, plazos := responderYEnviar(t, modo)
		if s.pidio("SetFlagMany") != 0 || slices.Contains(marcasDe(b, "m2"), api.MailFlagAnswered) {
			t.Fatalf("se ha marcado respondido dentro de la ventana de deshacer: %v", s.llamadas)
		}
		cumplir(plazos)
		if !slices.Contains(s.llamadas, `SetFlagMany(\Answered)`) {
			t.Errorf("al salir el envío no se marca el original en el servidor: %v", s.llamadas)
		}
		if c, _ := conversacionDe(b, "m2"); !c.Respondida {
			t.Errorf("la conversación no queda respondida en la bandeja: %+v", c)
		}
	}
}

func TestReenviarNoMarcaRespondido(t *testing.T) {
	m := Mensaje{ID: "m2", Asunto: "Encuesta", De: []Direccion{{Correo: "m2@example.org"}}}
	if br := BorradorDeRespuesta(traductor(t), "es", m, Reenviar, "", ""); br.Responde != "" {
		t.Errorf("un reenvío no contesta a nadie y lleva Responde=%q", br.Responde)
	}
}

// Contestar desde un buzón compartido no marca: el original es de otra cuenta y la
// marca en lote solo toca las propias, así que la flecha no llegaría al servidor.
func TestResponderDesdeUnCompartidoNoMarca(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	m, ok := conversacionDe(b, "m2")
	if !ok {
		t.Fatal("m2 no está en la bandeja")
	}
	b.compartido = &api.MailShared{Address: "equipo@example.org", Permission: "send"}
	b.responder(m.Ultimo, Responder)
	if b.redaccion == nil || b.redaccion.r.B.Responde != "" {
		t.Errorf("la respuesta desde un compartido no debe marcar el original: %+v", b.redaccion)
	}
}

func TestDeshacerLaRespuestaNoDejaLaMarca(t *testing.T) {
	// Dentro de la ventana: el plazo ya no pone nada.
	b, s, plazos := responderYEnviar(t, Responder)
	av := b.P.Avisos.Avisos()
	if len(av) != 1 || av[0].AlPulsarAccion == nil {
		t.Fatalf("el aviso de enviado no ofrece deshacer: %+v", av)
	}
	av[0].AlPulsarAccion()
	cumplir(plazos)
	if s.pidio("SetFlagMany") != 0 || slices.Contains(marcasDe(b, "m2"), api.MailFlagAnswered) {
		t.Errorf("deshecho el envío, el original sigue marcándose: %v", s.llamadas)
	}

	// El plazo se cumplió un instante antes de que la cola lo soltara: se quita.
	b, s, plazos = responderYEnviar(t, Responder)
	cumplir(plazos)
	b.P.Avisos.Avisos()[0].AlPulsarAccion()
	if s.pidio("SetFlagMany") != 2 || slices.Contains(marcasDe(b, "m2"), api.MailFlagAnswered) {
		t.Errorf("deshecho el envío, la marca no se quita: %v, marcas %v", s.llamadas, marcasDe(b, "m2"))
	}
}

func TestLaFilaRespondidaEnseñaLaFlecha(t *testing.T) {
	tr := traductor(t)
	c := ejemplo()[0]
	c.Respondida = true
	f := NuevaFilaConversacion()
	f.RespondidaTexto = tr.T("list.answered", nil)
	f.Poner(c, "Sofía Nogueira Paz", "Calendario de despliegues", "11 sept")
	w := test.NewWindow(f)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, AltoFilaConversacion()))
	r := test.WidgetRenderer(f).(*filaRenderer)
	if !r.respondida.Objeto().Visible() {
		t.Error("la flecha de respondido no se ve")
	}
	if !strings.HasSuffix(f.AccessibilityLabel(), "Respondido") {
		t.Errorf("el nombre accesible no dice que está respondida: %q", f.AccessibilityLabel())
	}

	c.Respondida = false
	f.Poner(c, "Sofía Nogueira Paz", "Calendario de despliegues", "11 sept")
	if r.respondida.Objeto().Visible() || strings.Contains(f.AccessibilityLabel(), "Respondido") {
		t.Errorf("una fila sin responder enseña la flecha: %q", f.AccessibilityLabel())
	}
}
