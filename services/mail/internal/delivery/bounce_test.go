// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// El aviso de no-entrega tiene que ser un correo DE VERDAD: si el parser no lo
// entiende, el sobre entra con asunto vacío y quien lo recibe ve una fila en
// blanco justo cuando hay que leerla. Y una cabecera partida por un asunto con
// salto de línea sería inyección de cabeceras con datos del propio usuario.
func TestAvisoDeNoEntregaEsUnCorreoValido(t *testing.T) {
	t.Parallel()
	it := domain.OutboundMessage{
		ID:         uuid.New(),
		From:       "ana@corporate.sytue.eu",
		Recipients: []string{"cliente@ejemplo.eu", "otro@ejemplo.eu"},
		Subject:    "Presupuesto Q3",
		Attempts:   4,
	}
	raw := buildBounceNotice(it, "550 5.1.1 unknown recipient", time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC))

	parsed, err := mime.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("el aviso no es MIME válido: %v", err)
	}
	if got := parsed.Envelope.Subject; got != "No se ha podido entregar: Presupuesto Q3" {
		t.Errorf("asunto = %q", got)
	}
	if !strings.HasPrefix(parsed.Envelope.FromAddr, "mailer-daemon@") {
		t.Errorf("remitente = %q, quiero el daemon", parsed.Envelope.FromAddr)
	}
	// El motivo y los destinatarios van EN EL CUERPO: sin ellos el aviso dice que
	// algo falló y no qué, que es tan inútil como no avisar.
	cuerpo := string(raw)
	for _, quiero := range []string{"550 5.1.1 unknown recipient", "cliente@ejemplo.eu", "otro@ejemplo.eu"} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("el cuerpo no menciona %q", quiero)
		}
	}
	// RFC 3834: sin estas marcas, la respuesta de vacaciones del propio buzón
	// podría contestarle al daemon.
	if !strings.Contains(cuerpo, "Auto-Submitted: auto-replied") {
		t.Error("falta Auto-Submitted: un aviso automático sin marcar puede montar un bucle")
	}
}

func TestUnAsuntoConSaltoDeLineaNoParteLaCabecera(t *testing.T) {
	t.Parallel()
	it := domain.OutboundMessage{
		From:       "ana@corporate.sytue.eu",
		Recipients: []string{"x@ejemplo.eu"},
		Subject:    "Hola\r\nBcc: victima@ejemplo.eu",
	}
	raw := buildBounceNotice(it, "timeout", time.Now())
	// Lo que sería inyección es una LÍNEA nueva que empiece por Bcc:; el mismo texto
	// dentro del valor del asunto es inofensivo (y es lo que debe pasar al sanear).
	cabeceras, _, _ := strings.Cut(string(raw), "\r\n\r\n")
	for _, linea := range strings.Split(cabeceras, "\r\n") {
		if strings.HasPrefix(strings.ToLower(linea), "bcc:") {
			t.Fatalf("el asunto inyectó una cabecera:\n%s", cabeceras)
		}
	}
	if !strings.Contains(cabeceras, "Subject: No se ha podido entregar: Hola  Bcc:") {
		t.Errorf("el asunto debe quedar en UNA línea, con el salto convertido:\n%s", cabeceras)
	}
	if _, err := mime.Parse(bytes.NewReader(raw)); err != nil {
		t.Fatalf("el aviso saneado debe seguir siendo válido: %v", err)
	}
}

func TestSinAsuntoElAvisoSigueDiciendoAlgo(t *testing.T) {
	t.Parallel()
	raw := buildBounceNotice(domain.OutboundMessage{
		From: "ana@corporate.sytue.eu", Recipients: []string{"x@ejemplo.eu"},
	}, "conexión rechazada", time.Now())
	if !strings.Contains(string(raw), "(sin asunto)") {
		t.Error("un mensaje sin asunto debe rebotar como «(sin asunto)», no con la línea a medias")
	}
}
