// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configSuite "git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

func TestMailboxWidgetUsesServerCountAndRejectsLateIdentity(t *testing.T) {
	// Linux no tiene widgets nativos: se simula el lector para probar la publicación.
	t.Cleanup(widgets.SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
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

func TestMailboxWidgetPublishesInboxHeadersWithoutPreviewBodies(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	configSuite.MarcarSesion()

	recibido := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	service := &servicioFalso{
		carpetas: []api.Mailbox{{ID: "in", Role: "inbox", Name: "Inbox", UnreadEmails: 2}},
		paginas: map[string][]api.MailMessage{
			"": {
				{
					ID: "m1", ThreadID: "t1", MailboxID: "in",
					From: "clara@sytue.eu", FromName: "Clara Soto",
					Subject: "Informe Q4", Preview: "texto privado del cuerpo",
					ReceivedAt: recibido,
				},
			},
		},
	}
	b, _ := buzonDePrueba(t, service)
	b.Widget = func() widgets.View {
		return widgets.NewView("eu.sytue.mail", "mail", widgets.Capture(configSuite.Leer().Sesion))
	}
	b.Arrancar(context.Background())

	snapshot, err := widgets.Read("eu.sytue.mail")
	if err != nil {
		t.Fatalf("snapshot read: %v", err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Title != "Clara Soto" || snapshot.Items[0].Subtitle != "Informe Q4" || !snapshot.Items[0].Unread {
		t.Fatalf("inbox headers missing from widget items: %+v", snapshot.Items)
	}
	dir, err := configSuite.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "widgets", "eu.sytue.mail.json"))
	if err != nil || strings.Contains(string(raw), "texto privado del cuerpo") {
		t.Fatalf("widget snapshot leaked preview body: %q %v", raw, err)
	}
}
