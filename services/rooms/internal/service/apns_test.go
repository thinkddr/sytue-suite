// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// claveP8DePrueba genera una clave como la que descarga Apple (PKCS#8 PEM, P-256).
func claveP8DePrueba(t *testing.T) string {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func cfgAPNS(t *testing.T, host string) config.Config {
	t.Helper()
	return config.Config{
		APNSKeyP8:    claveP8DePrueba(t),
		APNSKeyID:    "ABC123DEFG",
		APNSTeamID:   "TEAM123456",
		APNSBundleID: "eu.sytue.rooms",
		APNSHost:     host,
	}
}

func TestNewAPNSSender(t *testing.T) {
	// Sin nada: deshabilitado, no error (como el Web Push sin VAPID).
	s, err := NewAPNSSender(config.Config{})
	if s != nil || err != nil {
		t.Fatalf("sin config = (%v, %v), quiero (nil, nil)", s, err)
	}
	// A medias: error de arranque, no un push silenciosamente apagado.
	_, err = NewAPNSSender(config.Config{APNSKeyID: "ABC123DEFG"})
	if err == nil {
		t.Fatal("config a medias debe abortar el arranque")
	}
	// Clave que no es PEM.
	_, err = NewAPNSSender(config.Config{
		APNSKeyP8: "no-soy-un-pem", APNSKeyID: "A", APNSTeamID: "B", APNSBundleID: "C",
	})
	if err == nil {
		t.Fatal("una clave que no es PEM debe abortar el arranque")
	}
	// Completa: host por defecto el de producción.
	s, err = NewAPNSSender(cfgAPNS(t, ""))
	if err != nil || s == nil {
		t.Fatalf("config completa = (%v, %v)", s, err)
	}
	if s.host != apnsHostProd {
		t.Fatalf("host = %q, quiero %q", s.host, apnsHostProd)
	}
}

func TestAPNSTokenSeFirmaYSeReutiliza(t *testing.T) {
	s, err := NewAPNSSender(cfgAPNS(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	s.ahora = func() time.Time { return base }

	tok, err := s.token()
	if err != nil {
		t.Fatal(err)
	}
	partes := strings.Split(tok, ".")
	if len(partes) != 3 {
		t.Fatalf("el JWT debe tener tres partes: %q", tok)
	}
	var cab struct{ Alg, Kid, Typ string }
	crudo, err := base64.RawURLEncoding.DecodeString(partes[0])
	if err != nil {
		t.Fatalf("cabecera no es base64url: %v", err)
	}
	if err := json.Unmarshal(crudo, &cab); err != nil {
		t.Fatal(err)
	}
	if cab.Alg != "ES256" || cab.Kid != "ABC123DEFG" {
		t.Fatalf("cabecera = %+v, quiero ES256 + kid", cab)
	}
	var cla struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
	}
	crudo, _ = base64.RawURLEncoding.DecodeString(partes[1])
	if err := json.Unmarshal(crudo, &cla); err != nil {
		t.Fatal(err)
	}
	if cla.Iss != "TEAM123456" || cla.Iat != base.Unix() {
		t.Fatalf("claims = %+v", cla)
	}
	// R||S de 64 bytes: el DER de ecdsa.Sign no vale para ES256.
	firma, err := base64.RawURLEncoding.DecodeString(partes[2])
	if err != nil || len(firma) != 64 {
		t.Fatalf("firma de %d bytes (err=%v), quiero 64", len(firma), err)
	}

	// Se reutiliza mientras vive: Apple castiga pedir uno nuevo demasiado seguido.
	if otro, _ := s.token(); otro != tok {
		t.Fatal("el JWT debe reutilizarse mientras siga vigente")
	}
	s.ahora = func() time.Time { return base.Add(apnsTokenTTL + time.Minute) }
	if otro, _ := s.token(); otro == tok {
		t.Fatal("pasada la vigencia hay que firmar uno nuevo")
	}
}

// El aviso que ve Apple no puede llevar NADA del mensaje. Este test es el cerrojo:
// si alguien añade un título «solo para depurar», se pone en rojo.
func TestAPNSNoMandaNadaMasQueElDespertar(t *testing.T) {
	var visto struct {
		cuerpo  string
		cabs    http.Header
		ruta    string
		llamado bool
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		visto.cuerpo, visto.cabs, visto.ruta, visto.llamado = string(buf[:n]), r.Header.Clone(), r.URL.Path, true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	restaurar := ssrfhttp.SetIPPolicyForTest(func(_ net.IP) bool { return false })
	defer restaurar()

	s, err := NewAPNSSender(cfgAPNS(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	s.client = srv.Client()
	svc := &Service{apns: s}

	if err := svc.sendOneAPNS(context.Background(), domain.APNSTarget{Subject: "u1", Token: "abcdef0123456789abcdef0123456789"}); err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if !visto.llamado {
		t.Fatal("no se llamó a Apple")
	}
	if visto.cuerpo != `{"aps":{"content-available":1}}` {
		t.Fatalf("cuerpo = %q; a Apple no puede irle NADA del mensaje", visto.cuerpo)
	}
	if visto.ruta != "/3/device/abcdef0123456789abcdef0123456789" {
		t.Fatalf("ruta = %q", visto.ruta)
	}
	if got := visto.cabs.Get("apns-push-type"); got != "background" {
		t.Fatalf("apns-push-type = %q, quiero background", got)
	}
	if got := visto.cabs.Get("apns-priority"); got != "5" {
		t.Fatalf("apns-priority = %q, quiero 5", got)
	}
	if got := visto.cabs.Get("apns-topic"); got != "eu.sytue.rooms" {
		t.Fatalf("apns-topic = %q", got)
	}
	if !strings.HasPrefix(visto.cabs.Get("authorization"), "bearer ") {
		t.Fatalf("authorization = %q", visto.cabs.Get("authorization"))
	}
}

func TestAPNSTokenCaducoSeLimpia(t *testing.T) {
	casos := []struct {
		nombre string
		estado int
		cuerpo string
		gone   bool
	}{
		{"410 Gone", http.StatusGone, `{"reason":"Unregistered"}`, true},
		{"400 BadDeviceToken", http.StatusBadRequest, `{"reason":"BadDeviceToken"}`, true},
		{"400 de otra cosa", http.StatusBadRequest, `{"reason":"PayloadTooLarge"}`, false},
		{"500", http.StatusInternalServerError, ``, false},
	}
	restaurar := ssrfhttp.SetIPPolicyForTest(func(_ net.IP) bool { return false })
	defer restaurar()
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.estado)
				_, _ = w.Write([]byte(c.cuerpo))
			}))
			defer srv.Close()
			s, err := NewAPNSSender(cfgAPNS(t, srv.URL))
			if err != nil {
				t.Fatal(err)
			}
			s.client = srv.Client()
			svc := &Service{apns: s}
			err = svc.sendOneAPNS(context.Background(), domain.APNSTarget{Token: "abcdef0123456789abcdef0123456789"})
			if got := errors.Is(err, errPushGone); got != c.gone {
				t.Fatalf("errPushGone = %v (err=%v), quiero %v", got, err, c.gone)
			}
			if !c.gone && err == nil {
				t.Fatal("un fallo que no es «token caduco» debe seguir siendo error")
			}
		})
	}
}

func TestValidAPNSToken(t *testing.T) {
	casos := map[string]bool{
		"abcdef0123456789abcdef0123456789": true,
		"ABCDEF0123456789ABCDEF0123456789": true,
		"abcdef0123456789abcdef012345678":  false, // 31: demasiado corto
		"zzzzzz0123456789abcdef0123456789": false, // no es hex
		"":                                 false,
		strings.Repeat("a", 201):           false,
	}
	for tok, quiere := range casos {
		if got := domain.ValidAPNSToken(tok); got != quiere {
			t.Errorf("ValidAPNSToken(%q) = %v, quiero %v", tok, got, quiere)
		}
	}
}
