// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"strings"
	"testing"
	"time"
)

// TestResponderEsBuildReplyDraft: los prefijos no se duplican ni se traducen, la cita
// lleva la cabecera del catálogo y «> » por línea, y «Responder a todos» deja fuera a
// quien responde y al remitente.
func TestResponderEsBuildReplyDraft(t *testing.T) {
	tr := traductor(t)
	m := Mensaje{
		MessageID: "<a1@example.org>", Asunto: "Re: Obra",
		De:       []Direccion{{Nombre: "Grace", Correo: "grace@example.org"}},
		Para:     []Direccion{{Correo: "ada@example.org"}, {Correo: "linus@example.org"}},
		CC:       []Direccion{{Correo: "GRACE@example.org"}, {Correo: "linus@example.org"}},
		Recibido: time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC),
	}
	b := BorradorDeRespuesta(tr, "es", m, ResponderATodos, "Ada@example.org", "Hola\nequipo")
	if b.Asunto != "Re: Obra" || b.Para != "Grace <grace@example.org>" || b.Cc != "linus@example.org" {
		t.Errorf("responder a todos: %+v", b)
	}
	if !strings.Contains(b.Cuerpo, "> Hola\n> equipo") || !strings.HasPrefix(b.Cuerpo, "\n\n") {
		t.Errorf("la cita no es la de la web: %q", b.Cuerpo)
	}
	if b.EnRespuestaA != "<a1@example.org>" {
		t.Error("la respuesta no dice a qué mensaje contesta")
	}
	f := BorradorDeRespuesta(tr, "es", Mensaje{Asunto: "Obra", Extracto: "resumen"}, Reenviar, "", "")
	if f.Asunto != "Fwd: Obra" || f.Para != "" || !strings.Contains(f.Cuerpo, "> resumen") {
		t.Errorf("reenviar: %+v", f)
	}
	if got := TituloRedactor(tr, f.Asunto, false); got != tr.T("compose.forwardTitle", nil) {
		t.Errorf("el título de un reenvío es %q", got)
	}
}

// TestElAvisoDeAdjuntoNoCuentaLaCita: «te adjunto» sin adjunto pregunta antes de
// enviar, pero el «adjunto» de lo citado es de otro.
func TestElAvisoDeAdjuntoNoCuentaLaCita(t *testing.T) {
	if !MencionaAdjunto("Te adjunto el presupuesto.") || !MencionaAdjunto("See attached.") {
		t.Error("no se ve la mención de un adjunto")
	}
	if MencionaAdjunto("Gracias.\n\n> Te adjunto el presupuesto.") {
		t.Error("la cita cuenta como mención propia")
	}
	if got := ConFirma("Hola", "Ada", false); got != "Hola\n\n-- \nAda" || ConFirma("Hola", "Ada", true) != "Hola" {
		t.Errorf("firma: %q", got)
	}
}
