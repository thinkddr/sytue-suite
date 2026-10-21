// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"strings"
	"testing"
)

// Las coordenadas publicadas deben salir de la configuración (host + puerto ya
// separados) y NO llevar nada del usuario.
func TestClientEndpointsOf(t *testing.T) {
	got, err := clientEndpointsOf(Config{
		Issuer:         "https://id.sytue.eu",
		MailIMAPAddr:   "imap.sytue.eu:993",
		MailSMTPAddr:   "smtp.sytue.eu:465",
		CalDAVURL:      "https://calendar.sytue.eu/dav/",
		CardDAVURL:     "https://contacts.sytue.eu/dav/",
		DriveWebDAVURL: "https://drive.sytue.eu/dav/",
	})
	if err != nil {
		t.Fatalf("clientEndpointsOf: %v", err)
	}
	if got.Domain != "sytue.eu" {
		t.Fatalf("domain=%q", got.Domain)
	}
	if got.IMAP.Host != "imap.sytue.eu" || got.IMAP.Port != 993 {
		t.Fatalf("imap=%+v", got.IMAP)
	}
	if got.SMTP.Host != "smtp.sytue.eu" || got.SMTP.Port != 465 {
		t.Fatalf("smtp=%+v", got.SMTP)
	}
	if got.AppleProfileURL != "https://id.sytue.eu/account/apple.mobileconfig" {
		t.Fatalf("appleProfileUrl=%q", got.AppleProfileURL)
	}
	if got.WebDAVURL == "" || !strings.HasSuffix(got.WebDAVURL, "/dav/") {
		t.Fatalf("webdavUrl=%q", got.WebDAVURL)
	}
}

// Una dirección mal configurada NO debe publicarse a medias: falla explícito.
func TestClientEndpointsOfRejectsBadAddr(t *testing.T) {
	if _, err := clientEndpointsOf(Config{Issuer: "https://id.sytue.eu"}); err == nil {
		t.Fatal("se esperaba error con MAIL_IMAP_ADDR vacío")
	}
}
