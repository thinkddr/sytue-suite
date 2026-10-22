// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// El canal de una alerta acepta ahora dos formas. Lo que no puede pasar es que
// abrir la puerta al correo abra también la de un webhook interno: la rama http
// tiene que seguir pasando por el filtro anti-SSRF de siempre.
func TestValidateAlertChannel(t *testing.T) {
	t.Parallel()
	validos := []string{
		"https://hooks.slack.com/services/T/B/x",
		"mailto:ops@empresa.com",
		"MAILTO:Ops@Empresa.com",
		"mailto:ops@empresa.com?subject=algo",
	}
	for _, c := range validos {
		if err := ValidateAlertChannel(c); err != nil {
			t.Errorf("ValidateAlertChannel(%q) = %v; se esperaba válido", c, err)
		}
	}

	invalidos := []string{
		"",
		"mailto:",
		"mailto:sin-arroba",
		"mailto:sin@dominio",
		"ftp://ejemplo.com/x",
		// Los destinos internos siguen fuera: el correo no relaja el anti-SSRF.
		"http://localhost:8080/hook",
		"http://169.254.169.254/latest/meta-data",
	}
	for _, c := range invalidos {
		if err := ValidateAlertChannel(c); err == nil {
			t.Errorf("ValidateAlertChannel(%q) = nil; se esperaba rechazo", c)
		}
	}
}

// La parte de después de «?» no viaja al correo: un mailto puede llevar asunto y
// cuerpo, y aquí eso solo serviría para colar cabeceras en el mensaje que
// enviamos nosotros.
func TestMailChannelAddress(t *testing.T) {
	t.Parallel()
	casos := map[string]string{
		"mailto:ops@empresa.com":                     "ops@empresa.com",
		"mailto:ops@empresa.com?subject=Hola":        "ops@empresa.com",
		"  mailto:ops@empresa.com  ":                 "ops@empresa.com",
		"mailto:ops@empresa.com?cc=otro@empresa.com": "ops@empresa.com",
	}
	for in, want := range casos {
		got, ok := MailChannelAddress(in)
		if !ok || got != want {
			t.Errorf("MailChannelAddress(%q) = (%q, %v), quiero (%q, true)", in, got, ok, want)
		}
	}
	if _, ok := MailChannelAddress("https://ejemplo.com"); ok {
		t.Error("un webhook no puede leerse como canal de correo")
	}
}
