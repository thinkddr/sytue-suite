// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// La regla MÁS ESPECÍFICA gana: silenciar un origen entero pero dejar pasar un tipo
// concreto (o al revés) es exactamente lo que la gente quiere de estas preferencias.
// Y lo que no se ha configurado se entrega: una app nueva no puede nacer muda.
func TestMutedAplicaLaReglaMasEspecifica(t *testing.T) {
	t.Parallel()
	prefs := []NotificationPref{
		{Source: SourceMail, Kind: "", Muted: true},         // todo el correo, callado
		{Source: SourceMail, Kind: "mention", Muted: false}, // …salvo que me mencionen
		{Source: SourceDrive, Kind: "comment", Muted: true}, // solo los comentarios
	}
	casos := []struct {
		nombre string
		source Source
		kind   string
		want   bool
	}{
		{"origen silenciado", SourceMail, "digest", true},
		{"excepción del tipo gana sobre el origen", SourceMail, "mention", false},
		{"tipo silenciado dentro de un origen que no lo está", SourceDrive, "comment", true},
		{"otro tipo del mismo origen sigue llegando", SourceDrive, "share", false},
		{"origen sin ninguna regla", SourceCalendar, "reminder", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			if got := Muted(prefs, c.source, c.kind); got != c.want {
				t.Fatalf("Muted(%s,%s) = %v, quiero %v", c.source, c.kind, got, c.want)
			}
		})
	}
}

func TestValidSource(t *testing.T) {
	t.Parallel()
	if !ValidSource(SourceRooms) {
		t.Errorf("ValidSource(SourceRooms) debería ser true")
	}
	if !ValidSource(SourceMail) || !ValidSource(SourceDrive) || !ValidSource(SourceCalendar) {
		t.Errorf("orígenes estándar deberían ser válidos")
	}
	// Pages federa su búsqueda y además avisa; no puede contarse dos veces entre los
	// orígenes que dejan avisos.
	if !ValidSource(SourcePages) || !ValidNotificationSource(SourcePages) {
		t.Errorf("Pages debe ser origen de búsqueda y de avisos")
	}
	vistos := map[Source]bool{}
	for _, s := range NotificationSources() {
		if vistos[s] {
			t.Errorf("NotificationSources repite %q", s)
		}
		vistos[s] = true
	}
	if ValidSource(Source("desconocido")) {
		t.Errorf("origen desconocido no debería ser válido")
	}
}

// Entre organizaciones solo valen el `sub` y el `mailbox` (ADR 0053): el `email` de
// entrar puede llevar `email_verified` sin que nadie haya probado que es suyo.
func TestBuzonesPersonalesSinElCorreoDeEntrar(t *testing.T) {
	if d := BuzonesPersonales("bea", "Bea@Corp.test"); len(d) != 2 || d[1] != "email:bea@corp.test" {
		t.Errorf("buzones personales: %v", d)
	}
	if d := BuzonesPersonales("", "bea@corp.test"); len(d) != 0 {
		t.Errorf("sin sub hay buzones personales: %v", d)
	}
	if d := Destinatarios("", "a@b.test", ""); len(d) != 0 {
		t.Errorf("sin sub hay destinatarios: %v", d)
	}
}
