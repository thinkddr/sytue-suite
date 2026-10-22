// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package sesion

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// TestLaConexionEsDeLaSesion: cualquier app pide su cliente «red primero» con su
// producto y sus rutas privadas, y el monitor que devuelve es el que alimenta ese
// transporte. Era código de `cmd/drive`; sin esto, cada app lo copiaba.
func TestLaConexionEsDeLaSesion(t *testing.T) {
	aparte(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")

	red, cliente := s.Conexion("mail", "/api/v1/adjuntos/")
	tr, es := cliente.Transport.(*offline.Transport)
	if !es {
		t.Fatalf("el cliente lleva un %T y tiene que llevar la caché de la suite", cliente.Transport)
	}
	if tr.Product != "mail" || !slices.Equal(tr.Private, []string{"/api/v1/adjuntos/"}) {
		t.Errorf("el transporte es de %q con privadas %v", tr.Product, tr.Private)
	}
	if tr.Monitor != red {
		t.Error("el monitor devuelto no es el que alimenta el transporte: la pantalla no se enteraría")
	}
}

// viaDePrueba apunta lo que llega a la red.
type viaDePrueba struct{ vistas []string }

func (v *viaDePrueba) RoundTrip(r *http.Request) (*http.Response, error) {
	v.vistas = append(v.vistas, r.Method+" "+r.URL.Path)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
}

// TestSoloLecturaNoDejaEscribir: con `SYTUE_SOLO_LECTURA` un clic del recorrido de
// ventana real puede caer en cualquier botón y no llega nada a producción. Lo que lee
// pasa, y el POST al `/token` del IdP también: sin él no hay sesión.
//
// VISTO EN ROJO dejando pasar todo POST: «Nueva nota» llegaba a la red.
func TestSoloLecturaNoDejaEscribir(t *testing.T) {
	red := &viaDePrueba{}
	c := &http.Client{Transport: soloLectura{red}}
	for _, p := range []struct {
		metodo, url string
		pasa        bool
	}{
		{http.MethodGet, "https://notes.sytue.eu/api/v1/notebooks", true},
		{http.MethodPost, "https://id.sytue.eu/oauth2/token", true},
		{http.MethodPost, "https://notes.sytue.eu/api/v1/notebooks/1/notes", false},
		{http.MethodPut, "https://docs.sytue.eu/api/v1/documents/1", false},
		{http.MethodPatch, "https://mail.sytue.eu/api/v1/messages/1", false},
		{http.MethodDelete, "https://drive.sytue.eu/api/v1/files/1", false},
	} {
		req, err := http.NewRequest(p.metodo, p.url, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		if pasa := err == nil; pasa != p.pasa {
			t.Errorf("%s %s: pasa=%v y quiero %v (%v)", p.metodo, p.url, pasa, p.pasa, err)
		}
		if err != nil && !errors.Is(err, ErrSoloLectura) {
			t.Errorf("%s %s: el error es %v; quiero ErrSoloLectura", p.metodo, p.url, err)
		}
	}
	if len(red.vistas) != 2 {
		t.Errorf("a la red han llegado %v; quiero solo la lectura y el canje del token", red.vistas)
	}
}
