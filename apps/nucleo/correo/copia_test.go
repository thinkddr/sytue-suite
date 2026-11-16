// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func almacen(t *testing.T) *offline.Store {
	t.Helper()
	st, err := offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestLasCopiasSonDeSuDueño: cada persona y organización ve las suyas, la última
// escrita primero, y borrar quita solo la pedida.
func TestLasCopiasSonDeSuDueño(t *testing.T) {
	st := almacen(t)
	if err := Guardar(st, "ada/acme", "r1", Copia{De: "ada@acme.eu", Crudo: []byte("uno")}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // «la última primero» se decide por la fecha de escritura
	if err := Guardar(st, "ada/acme", "r2", Copia{De: "ada@acme.eu", Crudo: []byte("dos")}); err != nil {
		t.Fatal(err)
	}
	if err := Guardar(st, "bob/acme", "r1", Copia{Crudo: []byte("de bob")}); err != nil {
		t.Fatal(err)
	}
	cs := Copias(st, "ada/acme")
	if len(cs) != 2 || cs[0].ID != "r2" || string(cs[0].Crudo) != "dos" {
		t.Fatalf("copias de ada = %+v; quiero r2 y r1, la última primero", cs)
	}
	Borrar(st, "ada/acme", "r2")
	if cs := Copias(st, "ada/acme"); len(cs) != 1 || cs[0].ID != "r1" {
		t.Fatalf("tras borrar r2 quedan %+v", cs)
	}
	if cs := Copias(st, "bob/acme"); len(cs) != 1 || string(cs[0].Crudo) != "de bob" {
		t.Fatalf("la de bob se ha tocado: %+v", cs)
	}
	if Copias(st, "") != nil || Guardar(st, "", "r1", Copia{}) == nil {
		t.Fatal("sin dueño no hay copia")
	}
}

// TestSubirLlevaLasCopiasABorradores: lo que llega se borra; lo que no, se queda; y la
// que sustituía a un borrador desaparecido se sube como uno nuevo.
func TestSubirLlevaLasCopiasABorradores(t *testing.T) {
	st := almacen(t)
	_ = Guardar(st, "ada/acme", "nueva", Copia{De: "ada@acme.eu", Crudo: []byte("nueva")})
	_ = Guardar(st, "ada/acme", "huerfana", Copia{De: "ada@acme.eu", Crudo: []byte("huerfana"), Servidor: "d-borrado"})
	_ = Guardar(st, "ada/acme", "sin-red", Copia{De: "ada@acme.eu", Crudo: []byte("sin-red")})
	var pedidas []string
	subir := func(_ context.Context, id, _ string, crudo []byte) (api.MailMessage, error) {
		pedidas = append(pedidas, string(crudo)+"@"+id)
		switch {
		case id == "d-borrado":
			return api.MailMessage{}, &api.Error{Status: http.StatusNotFound}
		case string(crudo) == "sin-red":
			return api.MailMessage{}, errors.New("dial tcp: sin red")
		}
		return api.MailMessage{ID: "d1"}, nil
	}
	if quedan := Subir(context.Background(), st, "ada/acme", subir); quedan != 1 {
		t.Fatalf("quedan %d; quiero 1 (la que no tuvo red). Pedidas: %v", quedan, pedidas)
	}
	cs := Copias(st, "ada/acme")
	if len(cs) != 1 || cs[0].ID != "sin-red" {
		t.Fatalf("en el equipo quedan %+v; solo tenía que quedar la que no llegó", cs)
	}
	reintento := false
	for _, p := range pedidas {
		reintento = reintento || p == "huerfana@"
	}
	if !reintento {
		t.Fatalf("la del borrador desaparecido no se subió como nueva: %v", pedidas)
	}
}
