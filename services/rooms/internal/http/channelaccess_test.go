// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/rooms/internal/config"
)

// TestChannelAccessRequiresInternalSecret: la verja que consulta Sytue Drive para
// servir un adjunto está detrás del secreto compartido y VALIDA sus parámetros ANTES
// de tocar el servicio (svc nil: si lo tocara, el test entraría en pánico). Sin
// secreto configurado la ruta no se monta siquiera, y con ella los adjuntos de canal
// no se sirven a nadie (fail-closed).
func TestChannelAccessRequiresInternalSecret(t *testing.T) {
	const secret = "s3cr3to-interno"
	r := NewRouter(Deps{Cfg: config.Config{InternalSecret: secret}})
	base := "/internal/channel-access?tenant=acme&channel=" + uuid.New().String() + "&subject=bob"

	for _, tc := range []struct {
		name, url, auth string
		want            int
	}{
		{"sin secreto", base, "", http.StatusUnauthorized},
		{"secreto erróneo", base, "Bearer otro", http.StatusUnauthorized},
		{"tenant inválido", "/internal/channel-access?tenant=&channel=" + uuid.New().String() + "&subject=bob", "Bearer " + secret, http.StatusBadRequest},
		{"canal inválido", "/internal/channel-access?tenant=acme&channel=nope&subject=bob", "Bearer " + secret, http.StatusBadRequest},
		{"sujeto vacío", "/internal/channel-access?tenant=acme&channel=" + uuid.New().String(), "Bearer " + secret, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, http.NoBody)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s => %d, quiero %d", tc.url, rec.Code, tc.want)
			}
		})
	}
}

// TestChannelAccessNotMountedWithoutSecret: sin CHAT_INTERNAL_SECRET la ruta NO existe
// (404), de modo que nadie puede sondear la membresía sin secreto.
func TestChannelAccessNotMountedWithoutSecret(t *testing.T) {
	r := NewRouter(Deps{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/internal/channel-access?tenant=acme&channel="+uuid.New().String()+"&subject=bob", http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sin secreto la ruta responde %d, quiero 404", rec.Code)
	}
}
