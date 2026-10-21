// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestAccountDeviceTrust — `POST /account/devices/trust` marca de confianza el
// dispositivo de la COOKIE, no otro; sin cookie de dispositivo o desde otro
// origen, no marca nada.
func TestAccountDeviceTrust(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-disp", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	u := caOrgUser(t, l, org, "disp@id.test")
	token := "tok-confianza-dispositivo"
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(token), UserID: u.ID, OrgID: org.ID,
		AuthTime: time.Now().UTC(), AMR: []string{"pwd"}, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	secreto := "secreto-de-navegador-de-prueba"
	dev, err := l.st.UpsertDevice(l.ctx, nil, domain.Device{
		OrgID: org.ID, UserID: u.ID, DeviceID: domain.DeriveDeviceID(l.srv.deviceKey, u.ID, secreto), Name: "Prueba",
	})
	if err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	otro, err := l.st.UpsertDevice(l.ctx, nil, domain.Device{
		OrgID: org.ID, UserID: u.ID, DeviceID: domain.DeriveDeviceID(l.srv.deviceKey, u.ID, "otro-navegador"), Name: "Otro",
	})
	if err != nil {
		t.Fatalf("UpsertDevice otro: %v", err)
	}

	pedir := func(conDispositivo bool, origen string) int {
		r := httptest.NewRequest(http.MethodPost, "/account/devices/trust", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		if conDispositivo {
			r.AddCookie(&http.Cookie{Name: deviceCookieName, Value: domain.SignDeviceCookie(l.srv.deviceKey, secreto)})
		}
		r.Header.Set("Sec-Fetch-Site", origen)
		rr := httptest.NewRecorder()
		l.mux.ServeHTTP(rr, r)
		return rr.Code
	}

	if c := pedir(true, "cross-site"); c != http.StatusForbidden {
		t.Fatalf("desde otro origen: status=%d, quiero 403", c)
	}
	if c := pedir(false, "same-origin"); c != http.StatusBadRequest {
		t.Fatalf("sin cookie de dispositivo: status=%d, quiero 400", c)
	}
	if c := pedir(true, "same-origin"); c != http.StatusOK {
		t.Fatalf("con su cookie: status=%d, quiero 200", c)
	}
	if d, _ := l.st.GetDeviceForUser(l.ctx, u.ID, dev.ID); !d.Trusted {
		t.Fatal("el dispositivo de la cookie no quedó de confianza")
	}
	if d, _ := l.st.GetDeviceForUser(l.ctx, u.ID, otro.ID); d.Trusted {
		t.Fatal("se marcó de confianza un dispositivo que no es el de la cookie")
	}
}
