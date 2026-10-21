// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

// La videollamada de un evento sobrevive el ciclo evento -> ICS -> evento. El ICS es
// lo que se lleva un cliente CalDAV de terceros: si CONFERENCE no sale de aquí bien
// formada, Apple Calendar y Outlook enseñan el evento SIN botón de «Unirse» y el
// usuario tiene que buscar el enlace a mano.
func TestConferenceRoundTrip(t *testing.T) {
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	const url = "https://rooms.sytue.eu/?rooms=6f1b6d0e-1e2a-4f3b-9c1d-0a1b2c3d4e5f"
	ev := domain.Event{
		UID:           "conf-1@calendar.sytue.eu",
		Summary:       "Revisión de diseño",
		DTStart:       start,
		DTEnd:         start.Add(time.Hour),
		ConferenceURL: url,
	}

	data, err := EncodeEvent(ev, "")
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if !strings.Contains(data, "CONFERENCE") || !strings.Contains(data, url) {
		t.Fatalf("el ICS no emitió la videollamada:\n%s", data)
	}
	// FEATURE=VIDEO y VALUE=URI son lo que distingue «una URL cualquiera» de «aquí se
	// entra a una videollamada» para el cliente que la pinta.
	if !strings.Contains(data, "FEATURE=VIDEO") || !strings.Contains(data, "VALUE=URI") {
		t.Fatalf("CONFERENCE sin los parámetros que la hacen accionable:\n%s", data)
	}

	po, err := DecodeCalendarObject(data, "cal-1")
	if err != nil {
		t.Fatalf("DecodeCalendarObject: %v", err)
	}
	if po.Master.ConferenceURL != url {
		t.Errorf("videollamada tras round-trip = %q, want %q", po.Master.ConferenceURL, url)
	}
}

// Un evento SIN videollamada no debe emitir la propiedad: un CONFERENCE vacío haría
// que algunos clientes pinten un botón «Unirse» que no lleva a ninguna parte.
func TestConferenceAusenteNoEmiteLaPropiedad(t *testing.T) {
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	data, err := EncodeEvent(domain.Event{
		UID: "sin-conf@calendar.sytue.eu", Summary: "Café", DTStart: start, DTEnd: start.Add(time.Hour),
	}, "")
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if strings.Contains(data, "CONFERENCE") {
		t.Fatalf("un evento sin videollamada no debe llevar CONFERENCE:\n%s", data)
	}
}

// El ICS es un formato de LÍNEAS: una URL con CR/LF, ';' o ',' cerraría la propiedad e
// inyectaría otras nuevas en un fichero que consumen clientes de terceros. El saneo
// rechaza el valor entero en vez de intentar escaparlo.
func TestSanitizeConferenceURLRechazaInyeccion(t *testing.T) {
	t.Parallel()
	malos := map[string]string{
		"salto de línea":     "https://rooms.sytue.eu/\r\nX-EVIL:1",
		"punto y coma":       "https://rooms.sytue.eu/;X-EVIL=1",
		"coma":               "https://rooms.sytue.eu/,https://otro.example",
		"esquema javascript": "javascript:alert(1)",
		"esquema de datos":   "data:text/html,<script>",
		"relativa":           "/?rooms=abc",
		"control":            "https://rooms.sytue.eu/\x07",
	}
	for nombre, v := range malos {
		t.Run(nombre, func(t *testing.T) {
			t.Parallel()
			if got := domain.SanitizeConferenceURL(v); got != "" {
				t.Fatalf("%q debía rechazarse, salió %q", v, got)
			}
		})
	}

	// Y lo legítimo pasa intacto, incluida una URL con query y fragmento.
	buena := "https://rooms.sytue.eu/?rooms=6f1b6d0e-1e2a-4f3b-9c1d-0a1b2c3d4e5f#top"
	if got := domain.SanitizeConferenceURL(buena); got != buena {
		t.Fatalf("una URL válida no debe alterarse: %q", got)
	}
}
