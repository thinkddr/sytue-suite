// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/sieve/sieveprueba"
)

// tokens es un extremo de tokens de mentira: cuenta las renovaciones y da token-bueno.
type tokens struct{ pedidas atomic.Int32 }

func (t *tokens) RoundTrip(r *http.Request) (*http.Response, error) {
	t.pedidas.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Request: r,
		Body: io.NopCloser(strings.NewReader(`{"access_token":"token-bueno","refresh_token":"refresco","expires_in":3600}`)),
	}, nil
}

// Una cuenta OAuth entra con el token que ya usa Mail (XOAUTH2 u OAUTHBEARER, el que
// anuncie el servidor), y el token solo se renueva cuando ya se sabe que al otro lado hay
// un servidor ManageSieve: preguntar a un puerto vacío no gasta una renovación.
func TestRespuestaConTokenOAuth(t *testing.T) {
	s := sieveDePrueba(t, func(s *sieveprueba.Servidor) {
		s.SinTLS = true // en este equipo, sin cifrar
		s.SASL = []string{"PLAIN", "XOAUTH2"}
		s.Token = "token-bueno"
	})
	tk := &tokens{}
	b, err := Abrir(t.TempDir(), &secretosEnMemoria{m: map[string]string{}}, Red{Marcar: marcarDirecto, HTTP: &http.Client{Transport: tk}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Cerrar)
	c, err := b.Anadir(Cuenta{
		Proveedor: ProveedorMicrosoft, Acceso: AccesoMicrosoft, Direccion: usuario,
		IMAP:  Servidor{Host: "127.0.0.1", Puerto: 1143, Seguridad: SeguridadNinguna},
		SMTP:  Servidor{Host: "127.0.0.1", Puerto: 1025, Seguridad: SeguridadNinguna},
		Sieve: Servidor{Host: "127.0.0.1", Puerto: s.Puerto()},
	}, "refresco")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.LeerRespuesta(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if n := tk.pedidas.Load(); n != 1 {
		t.Fatalf("renovaciones: %d", n)
	}
	if _, entro := s.EntroCifrada(usuario); !entro {
		t.Fatal("no entró con el token")
	}

	// Nada en el puerto: no se renueva nada.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.Sieve.Puerto = ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	if _, err := b.Anadir(c, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.LeerRespuesta(context.Background(), c.ID); !errors.Is(err, api.ErrMailSinRespuesta) {
		t.Fatalf("%v", err)
	}
	if n := tk.pedidas.Load(); n != 1 {
		t.Fatalf("se renovó el token para un puerto vacío: %d", n)
	}
}
