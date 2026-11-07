// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package apns

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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// aparatoDePrueba tiene la forma de un token de aparato (hex de 32 caracteres).
var aparatoDePrueba = strings.Repeat("ab01", 8)

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

func configDePrueba(t *testing.T, host string) Config {
	t.Helper()
	return Config{KeyP8: claveP8DePrueba(t), KeyID: "ABC123DEFG", TeamID: "TEAM123456", Host: host}
}

// contra es un enviador que habla con `srv` (TLS de prueba, en 127.0.0.1).
func contra(t *testing.T, srv *httptest.Server) *Enviador {
	t.Helper()
	t.Cleanup(ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false }))
	e, err := Nuevo(configDePrueba(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	e.client = srv.Client()
	return e
}

func TestNuevo(t *testing.T) {
	// Sin nada: deshabilitado, no error (como el Web Push sin VAPID).
	e, err := Nuevo(Config{})
	if e != nil || err != nil {
		t.Fatalf("sin config = (%v, %v), quiero (nil, nil)", e, err)
	}
	// A medias: error de arranque, no un despertar silenciosamente apagado.
	if _, err := Nuevo(Config{KeyID: "ABC123DEFG"}); err == nil {
		t.Fatal("config a medias debe abortar el arranque")
	}
	if _, err := Nuevo(Config{KeyP8: "no-soy-un-pem", KeyID: "A", TeamID: "B"}); err == nil {
		t.Fatal("una clave que no es PEM debe abortar el arranque")
	}
	// Completa: host por defecto el de producción.
	e, err = Nuevo(configDePrueba(t, ""))
	if err != nil || e == nil {
		t.Fatalf("config completa = (%v, %v)", e, err)
	}
	if e.host != HostProduccion {
		t.Fatalf("host = %q, quiero %q", e.host, HostProduccion)
	}
}

func TestAPNSTokenSeFirmaYSeReutiliza(t *testing.T) {
	e, err := Nuevo(configDePrueba(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	e.ahora = func() time.Time { return base }

	tok, err := e.token()
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
	// R||S de 64 bytes: el DER no vale para ES256.
	firma, err := base64.RawURLEncoding.DecodeString(partes[2])
	if err != nil || len(firma) != 64 {
		t.Fatalf("firma de %d bytes (err=%v), quiero 64", len(firma), err)
	}

	// Se reutiliza mientras vive: Apple castiga pedir uno nuevo demasiado seguido.
	if otro, _ := e.token(); otro != tok {
		t.Fatal("el JWT debe reutilizarse mientras siga vigente")
	}
	e.ahora = func() time.Time { return base.Add(vidaJWT + time.Minute) }
	if otro, _ := e.token(); otro == tok {
		t.Fatal("pasada la vigencia hay que firmar uno nuevo")
	}
}

// El aviso que ve Apple no puede llevar NADA de lo que ha cambiado. Este test es el
// cerrojo: si alguien añade un título «solo para depurar», se pone en rojo.
func TestAPNSNoMandaNadaMasQueElDespertar(t *testing.T) {
	var visto struct {
		cuerpo  string
		cabs    http.Header
		ruta    string
		llamado bool
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		visto.cuerpo, visto.cabs, visto.ruta, visto.llamado = string(b), r.Header.Clone(), r.URL.Path, true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	e := contra(t, srv)
	base := time.Unix(1_800_000_000, 0)
	e.ahora = func() time.Time { return base }

	if err := e.Despertar(context.Background(), "eu.sytue.calendar", aparatoDePrueba); err != nil {
		t.Fatalf("enviar: %v", err)
	}
	if !visto.llamado {
		t.Fatal("no se llamó a Apple")
	}
	if visto.cuerpo != `{"aps":{"content-available":1}}` {
		t.Fatalf("cuerpo = %q; a Apple no puede irle NADA de lo que ha cambiado", visto.cuerpo)
	}
	if visto.ruta != "/3/device/"+aparatoDePrueba {
		t.Fatalf("ruta = %q", visto.ruta)
	}
	quiere := map[string]string{
		"apns-push-type":  "background",
		"apns-priority":   "5",
		"apns-topic":      "eu.sytue.calendar", // el de cada envío: el Hub despierta a las quince
		"apns-expiration": strconv.FormatInt(base.Add(time.Hour).Unix(), 10),
	}
	for k, v := range quiere {
		if got := visto.cabs.Get(k); got != v {
			t.Errorf("%s = %q, quiero %q", k, got, v)
		}
	}
	if !strings.HasPrefix(visto.cabs.Get("authorization"), "bearer ") {
		t.Fatalf("authorization = %q", visto.cabs.Get("authorization"))
	}
	// Y ninguna cabecera de más que pueda llevar algo: apns-collapse-id o apns-id
	// compuestos con un identificador del recurso le dirían a Apple QUÉ cambió.
	for k := range visto.cabs {
		if strings.HasPrefix(strings.ToLower(k), "apns-") {
			if _, ok := quiere[strings.ToLower(k)]; !ok {
				t.Errorf("cabecera %s de más hacia Apple", k)
			}
		}
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
		{"400 DeviceTokenNotForTopic", http.StatusBadRequest, `{"reason":"DeviceTokenNotForTopic"}`, true},
		{"400 de otra cosa", http.StatusBadRequest, `{"reason":"PayloadTooLarge"}`, false},
		{"500", http.StatusInternalServerError, ``, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.estado)
				_, _ = w.Write([]byte(c.cuerpo))
			}))
			defer srv.Close()
			err := contra(t, srv).Despertar(context.Background(), "eu.sytue.rooms", aparatoDePrueba)
			if got := errors.Is(err, ErrTokenCaduco); got != c.gone {
				t.Fatalf("ErrTokenCaduco = %v (err=%v), quiero %v", got, err, c.gone)
			}
			if !c.gone && err == nil {
				t.Fatal("un fallo que no es «token caduco» debe seguir siendo error")
			}
		})
	}
}

// Un token con otra forma no sale hacia Apple: iría en la ruta de la petición.
func TestUnTokenMalFormadoNoSale(t *testing.T) {
	llamado := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		llamado = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	err := contra(t, srv).Despertar(context.Background(), "eu.sytue.rooms", "../../3/otra/"+aparatoDePrueba)
	if !errors.Is(err, ErrTokenCaduco) || llamado {
		t.Fatalf("err = %v, llamado = %v: un token mal formado se descarta sin salir", err, llamado)
	}
}

// Un fallo de transporte (timeout, corte de red, la barrera SSRF) no lleva la URL en
// el error: su ruta es `/3/device/<token>`, y quien lo registra lo haría en claro.
func TestUnFalloDeRedNoLlevaElToken(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	e := contra(t, srv)
	srv.Close() // ya no escucha nadie: el Do falla antes de que conteste Apple
	err := e.Despertar(context.Background(), "eu.sytue.rooms", aparatoDePrueba)
	if err == nil {
		t.Fatal("contra un host cerrado el despertar debe fallar")
	}
	if strings.Contains(err.Error(), aparatoDePrueba) || strings.Contains(err.Error(), "/3/device/") {
		t.Fatalf("el error lleva la URL con el token: %v", err)
	}
	if errors.Is(err, ErrTokenCaduco) {
		t.Fatalf("un fallo de red no es «token caduco»: %v", err)
	}
}

func TestTokenValido(t *testing.T) {
	casos := map[string]bool{
		aparatoDePrueba:                      true,
		strings.ToUpper(aparatoDePrueba):     true,
		aparatoDePrueba[1:]:                  false, // 31: demasiado corto
		"zz" + aparatoDePrueba[2:]:           false, // no es hex
		"":                                   false,
		strings.Repeat("a", maxLargoToken+1): false,
	}
	for tok, quiere := range casos {
		if got := TokenValido(tok); got != quiere {
			t.Errorf("TokenValido(%q) = %v, quiero %v", tok, got, quiere)
		}
	}
}
