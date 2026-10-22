// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestCoordenadasSonLasDeSytueID: la ruta, sin token, y los nombres de los campos son
// los que escribe `clientconfig.go`. Si el servicio renombrara uno, el asistente de
// dispositivos enseñaría un servidor vacío y aquí se vería antes.
func TestCoordenadasSonLasDeSytueID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/sytue-clients.json" || r.Header.Get("Authorization") != "" {
			t.Errorf("pedido %s con autorización %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"domain":"sytue.eu","imap":{"host":"mail.sytue.eu","port":993,"tls":"implicit"},` +
			`"smtp":{"host":"mail.sytue.eu","port":465,"tls":"implicit"},"caldav_url":"https://calendar.sytue.eu/dav",` +
			`"carddav_url":"https://contacts.sytue.eu/dav","webdav_url":"https://drive.sytue.eu/dav",` +
			`"apple_profile_url":"https://id.sytue.eu/account/apple.mobileconfig"}`))
	}))
	defer srv.Close()
	c, err := CoordenadasDe(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.IMAP.Port != 993 || c.SMTP.Host != "mail.sytue.eu" || c.WebDAVURL == "" || c.AppleProfileURL == "" {
		t.Errorf("salen %+v", c)
	}

	servicio := webref.Read(t, "services/id/internal/http/clientconfig.go")
	for _, literal := range []string{
		"`json:\"host\"`", "`json:\"port\"`", "`json:\"tls\"`", "`json:\"domain\"`", "`json:\"imap\"`",
		"`json:\"smtp\"`", "`json:\"caldav_url\"`", "`json:\"carddav_url\"`", "`json:\"webdav_url\"`",
		"`json:\"apple_profile_url\"`",
	} {
		if !strings.Contains(servicio, literal) {
			t.Errorf("services/id/internal/http/clientconfig.go ya no dice %s", literal)
		}
	}
	if !strings.Contains(webref.Read(t, "services/id/internal/http/server.go"), `"/.well-known/sytue-clients.json"`) {
		t.Error("Sytue ID ya no monta /.well-known/sytue-clients.json")
	}
}
