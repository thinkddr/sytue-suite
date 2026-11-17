// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix/cierre"
)

// suiteConDispositivoMatrix deja una sesión de «ana» y, en el llavero, la del dispositivo
// de almacenamiento privado de Mail (la que guarda apps/nucleo/matrix) y la de otra
// persona que comparte el equipo.
func suiteConDispositivoMatrix(t *testing.T, homeserver *httptest.Server) *Suite {
	t.Helper()
	aparte(t)
	viejoCliente, viejoPlazo := clienteCierreMatrix, plazoCierreMatrix
	clienteCierreMatrix = homeserver.Client
	t.Cleanup(func() { clienteCierreMatrix, plazoCierreMatrix = viejoCliente, viejoPlazo })
	guardar := func(cuenta string, s cierre.Sesion) {
		datos, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := auth.GuardarSecreto(cuenta, datos); err != nil {
			t.Fatal(err)
		}
	}
	guardar("rooms-matrix-session-mail", cierre.Sesion{Homeserver: homeserver.URL, DeviceID: "MAIL", AccessToken: "token-mail-ana", Subject: "ana"})
	guardar("rooms-matrix-session-bea", cierre.Sesion{Homeserver: homeserver.URL, DeviceID: "BEA", AccessToken: "token-bea", Subject: "bea"})
	a := test.NewApp()
	t.Cleanup(a.Quit)
	s := Abrir(a, "sytue.eu")
	s.almacen = s.oidc.Almacen()
	tokens := auth.Tokens{Access: "synthetic-access", Subject: "ana", Expiry: time.Now().Add(time.Hour)}
	if err := s.almacen.Guardar(tokens); err != nil {
		t.Fatal(err)
	}
	s.Sesion = auth.NewSession(tokens, nil, s.almacen.Guardar)
	return s
}

// TestSalirCierraElDispositivoMatrixAntesDeOlvidarlo: la salida cierra en el homeserver,
// una vez y con su propio token, el dispositivo de Matrix de quien sale, mientras su
// sesión sigue en el llavero; después la olvida. El de otra persona no se cierra.
//
// VISTO EN ROJO sin cerrarDispositivosMatrix: el homeserver no recibía nada y el
// dispositivo de Mail quedaba vivo con cada salida.
func TestSalirCierraElDispositivoMatrixAntesDeOlvidarlo(t *testing.T) {
	var mu sync.Mutex
	var tokens []string
	guardadaAlCerrar := false
	homeserver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/_matrix/client/v3/logout" {
			t.Errorf("petición inesperada %s %s", r.Method, r.URL.Path)
		}
		datos, _ := auth.LeerSecreto("rooms-matrix-session-mail")
		mu.Lock()
		tokens = append(tokens, r.Header.Get("Authorization"))
		guardadaAlCerrar = len(datos) > 0
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer homeserver.Close()
	s := suiteConDispositivoMatrix(t, homeserver)
	if err := s.salirClasico(t.Context()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(tokens) != 1 || tokens[0] != "Bearer token-mail-ana" {
		t.Fatalf("al homeserver han llegado %v; quiero un cierre con el token del dispositivo de ana", tokens)
	}
	if !guardadaAlCerrar {
		t.Error("el dispositivo se cerró cuando su sesión ya se había olvidado")
	}
	if datos, err := auth.LeerSecreto("rooms-matrix-session-mail"); err != nil || len(datos) != 0 {
		t.Error("la sesión del dispositivo sobrevivió a la salida", err)
	}
}

// TestSalirNoEsperaAUnHomeserverCaido: sin respuesta del homeserver la salida acaba en
// el plazo y olvida igual la sesión del dispositivo.
func TestSalirNoEsperaAUnHomeserverCaido(t *testing.T) {
	colgado := make(chan struct{})
	homeserver := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-colgado }))
	defer homeserver.Close()
	defer close(colgado)
	s := suiteConDispositivoMatrix(t, homeserver)
	plazoCierreMatrix = 200 * time.Millisecond
	inicio := time.Now()
	if err := s.salirClasico(t.Context()); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(inicio); d > 5*time.Second {
		t.Errorf("la salida esperó %v a un homeserver caído", d)
	}
	if s.Estado().Dentro() {
		t.Error("la salida no se completó")
	}
	if datos, err := auth.LeerSecreto("rooms-matrix-session-mail"); err != nil || len(datos) != 0 {
		t.Error("sin homeserver, la sesión del dispositivo sobrevivió a la salida", err)
	}
}
