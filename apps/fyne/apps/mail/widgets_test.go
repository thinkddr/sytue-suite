// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"testing"

	configSuite "git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func TestMailboxWidgetUsesServerCountAndRejectsLateIdentity(t *testing.T) {
	// Linux no tiene widgets nativos: se simula el lector para probar la publicación.
	t.Cleanup(widgets.SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configSuite.MarcarSesion()
	service := &servicioFalso{carpetas: []api.Mailbox{
		{Role: "sent", UnreadEmails: 30}, {Role: "inbox", UnreadEmails: 7},
	}}
	b := &Buzon{S: service, Widget: func() widgets.View {
		return widgets.NewView("eu.sytue.mail", "mail", widgets.Capture(configSuite.Leer().Sesion))
	}}
	fetch := b.carpetasParaWidget()
	if _, err := fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := widgets.Read("eu.sytue.mail")
	if err != nil || len(snapshot.Counters) != 1 || snapshot.Counters[0].Value != 7 || service.pidio("Mailboxes") != 1 {
		t.Fatalf("widget must reuse the authoritative inbox fetch: %+v %v", snapshot, err)
	}
	late := b.carpetasParaWidget()
	configSuite.MarcarSesion()
	service.carpetas[1].UnreadEmails = 99
	if _, err := late(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := widgets.Read("eu.sytue.mail"); !errors.Is(err, widgets.ErrPrivate) {
		t.Fatalf("late mailbox response acquired the new identity: %v", err)
	}
}
