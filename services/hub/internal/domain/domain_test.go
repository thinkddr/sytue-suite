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

// Un tipo partido en varios (`cloud.deploy` → `.ready` y `.failed`) hereda la regla
// del de antes hasta que el suyo tenga la propia, que manda.
func TestMutedHeredaLaReglaDelTipoPartido(t *testing.T) {
	t.Parallel()
	prefs := []NotificationPref{
		{Source: SourceCloud, Kind: "cloud.deploy", Muted: true},
		{Source: SourceCloud, Kind: "cloud.deploy.failed", Muted: false},
	}
	if !Muted(prefs, SourceCloud, "cloud.deploy.ready") {
		t.Error("cloud.deploy.ready no hereda la regla de cloud.deploy")
	}
	if Muted(prefs, SourceCloud, "cloud.deploy.failed") {
		t.Error("la regla propia de cloud.deploy.failed no manda sobre la heredada")
	}
	// Solo los de la tabla: `rooms.invitation.direct` no salió de `rooms.invitation`.
	salas := []NotificationPref{{Source: SourceRooms, Kind: "rooms.invitation", Muted: true}}
	if Muted(salas, SourceRooms, "rooms.invitation.direct") {
		t.Error("rooms.invitation.direct heredó la regla de rooms.invitation")
	}
	// La heredada gana a la del origen entero, como la exacta.
	conOrigen := []NotificationPref{{Source: SourceCloud, Muted: true}, {Source: SourceCloud, Kind: "cloud.deploy", Muted: false}}
	if Muted(conOrigen, SourceCloud, "cloud.deploy.ready") {
		t.Error("la regla del tipo de antes no gana a la del origen")
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

// Una regla vieja que silenciaba Sytue ID (de antes de que el Hub la rechazara) no
// calla los avisos de seguridad: se entregan siempre.
func TestMutedNuncaCallaLosAvisosDeSeguridad(t *testing.T) {
	prefs := []NotificationPref{{Source: SourceID, Muted: true}, {Source: SourceID, Kind: "login", Muted: true}}
	if Muted(prefs, SourceID, "") || Muted(prefs, SourceID, "login") {
		t.Fatal("un aviso de Sytue ID quedó silenciado")
	}
	if Silenciable(SourceID) || !Silenciable(SourceMail) {
		t.Fatal("Silenciable no distingue Sytue ID del resto")
	}
}

// Los tipos críticos (un cobro duplicado, un desistimiento, una denuncia) se entregan
// aunque haya una regla que los nombre o se haya callado el origen entero; los demás
// tipos del mismo origen sí se callan.
func TestMutedNuncaCallaLosTiposCriticos(t *testing.T) {
	prefs := []NotificationPref{
		{Source: SourcePages, Muted: true},
		{Source: SourcePages, Kind: "pages.withdrawal.requested", Muted: true},
		{Source: SourceRooms, Kind: "rooms.report", Muted: true},
	}
	for _, c := range []struct {
		source Source
		kind   string
	}{
		{SourcePages, "pages.payment.duplicated"},
		{SourcePages, "pages.withdrawal.requested"},
		{SourceRooms, "rooms.report"},
	} {
		if Muted(prefs, c.source, c.kind) {
			t.Errorf("el aviso crítico %s quedó silenciado", c.kind)
		}
	}
	if !Muted(prefs, SourcePages, "pages.form.received") {
		t.Error("callar Sytue Web ya no calla sus formularios")
	}
	if !TipoCritico("cast.report") || TipoCritico("cast.live") || TipoCritico("") {
		t.Error("TipoCritico no distingue los críticos del resto")
	}
}
