// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// casaDePrueba pone la carpeta personal y la de ajustes en un temporal.
func casaDePrueba(t *testing.T) string {
	t.Helper()
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	return casa
}

func sinRegistrar(string) bool { return false }

// TestUnaCarpetaPorCuentaYOrganizacion (FYS-04, decidido por el dueño: «Una carpeta por
// organización»): «Sytue Drive – <organización>», la misma cada vez para la misma
// persona y organización, otra para otra organización y otra para otra persona.
func TestUnaCarpetaPorCuentaYOrganizacion(t *testing.T) {
	casa := casaDePrueba(t)
	if err := os.Mkdir(filepath.Join(casa, "Sytue Drive – gamma"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		c      Cuenta
		quiero string
	}{
		{Cuenta{"ana", "acme"}, "Sytue Drive – acme"},
		{Cuenta{"ana", "beta"}, "Sytue Drive – beta"},
		{Cuenta{"ana", "acme"}, "Sytue Drive – acme"}, // la apuntada
		{Cuenta{"luis", "acme"}, "Sytue Drive – acme (2)"},
		{Cuenta{"ana", "p-0123456789abcdef"}, "Sytue Drive – Personal"},
		// Lo que ya hay en el disco y no es de nadie no se hace de Drive: subiría.
		{Cuenta{"ana", "gamma"}, "Sytue Drive – gamma (2)"},
	} {
		got, err := carpeta(c.c, sinRegistrar)
		if err != nil || got != filepath.Join(casa, c.quiero) {
			t.Errorf("%+v: %q, %v; quiero %q", c.c, got, err, c.quiero)
		}
	}
	for _, c := range []Cuenta{{"ana", "../fuera"}, {"ana", `a\b`}, {"ana", ""}, {"", "acme"}} {
		if got, err := carpeta(c, sinRegistrar); err == nil {
			t.Errorf("%+v da carpeta: %q", c, got)
		}
	}
	// Lo apuntado ilegible no es «nada apuntado»: otra cuenta se quedaría la de alguien.
	fichero, err := ficheroVinculos()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fichero, []byte("{roto"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := carpeta(Cuenta{"eva", "acme"}, sinRegistrar); err == nil {
		t.Errorf("con lo apuntado ilegible da %q", got)
	}
}

// TestLaCarpetaDeAntesEsDeLaPrimeraCuenta (FYS-04): `Sytue Drive` a secas, la de antes de
// atarlas, pasa a ser de quien esté dentro la primera vez, con su nombre y con todo lo que
// tiene; otra cuenta no la hereda, y una que nunca se conectó no apunta ninguna.
func TestLaCarpetaDeAntesEsDeLaPrimeraCuenta(t *testing.T) {
	casa := casaDePrueba(t)
	vieja := filepath.Join(casa, "Sytue Drive")
	registradas := map[string]bool{vieja: true}
	registrada := func(r string) bool { return registradas[r] }
	a, b := Cuenta{"ana", "acme"}, Cuenta{"ana", "beta"}
	if dir, ok := conectada(a, registrada); !ok || dir != vieja {
		t.Fatalf("al arrancar: %q, %v; quiero la de antes", dir, ok)
	}
	if dir, ok := conectada(b, registrada); ok {
		t.Errorf("otra organización hereda %q", dir)
	}
	if dir, err := carpeta(b, registrada); err != nil || dir != filepath.Join(casa, "Sytue Drive – beta") {
		t.Errorf("la de la otra organización: %q, %v", dir, err)
	}
	if dir, ok := conectada(b, registrada); ok {
		t.Errorf("sin registrar todavía (no se ha pulsado «Conectar»): %q", dir)
	}
	registradas[filepath.Join(casa, "Sytue Drive – beta")] = true
	for _, c := range []struct {
		c      Cuenta
		quiero string
	}{{a, vieja}, {b, filepath.Join(casa, "Sytue Drive – beta")}} {
		if dir, ok := conectada(c.c, registrada); !ok || dir != c.quiero {
			t.Errorf("%+v al volver: %q, %v; quiero %q", c.c, dir, ok, c.quiero)
		}
	}
}

// sesionFija es una sesión con un token puesto.
type sesionFija struct{ token string }

func (s *sesionFija) AccessToken(context.Context) (string, error) { return s.token, nil }
func (s *sesionFija) Invalidate(string)                           {}

func jwtDe(sub, tenant string) string {
	carga := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + sub + `","tenant":"` + tenant + `"}`))
	return "e30." + carga + ".firma"
}

// TestConLaSesionDeOtraCuentaLaCarpetaNoHablaConDrive (FYS-04): desde que el token es de
// otra organización o de otra persona, ni se lista —y por tanto no se quita nada— ni se
// sube ni se borra: no sale ni una petición. Con el suyo, sí.
func TestConLaSesionDeOtraCuentaLaCarpetaNoHablaConDrive(t *testing.T) {
	var peticiones atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		peticiones.Add(1)
		http.Error(w, "no", http.StatusTeapot)
	}))
	defer srv.Close()
	s := &sesionFija{}
	d := api.NewDrive(srv.URL, Atada(s, Cuenta{"ana", "acme"}), srv.Client())
	ctx := t.Context()
	for _, otro := range []string{jwtDe("ana", "beta"), jwtDe("luis", "acme"), "opaco"} {
		s.token = otro
		if _, _, err := listar(ctx, d, "c:c1"); !errors.Is(err, ErrOtraCuenta) {
			t.Errorf("listar: %v", err)
		}
		if _, err := subirNuevo(ctx, d, "c:c1", "x.txt", "text/plain", strings.NewReader("x"), 1); !errors.Is(err, ErrOtraCuenta) {
			t.Errorf("subir: %v", err)
		}
		if err := aplicarMovimiento(ctx, d, "f:f1", Movimiento{Borrar: true}); !errors.Is(err, ErrOtraCuenta) {
			t.Errorf("borrar: %v", err)
		}
	}
	if n := peticiones.Load(); n != 0 {
		t.Fatalf("llegaron %d peticiones a Drive con la sesión de otra cuenta", n)
	}
	s.token = jwtDe("ana", "acme")
	if _, _, err := listar(ctx, d, "c:c1"); errors.Is(err, ErrOtraCuenta) || peticiones.Load() == 0 {
		t.Errorf("con la suya no habla con Drive: %v", err)
	}
}
