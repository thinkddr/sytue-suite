// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// aparte manda la carpeta compartida de la suite a un temporal, para no tocar la del
// usuario que ejecuta las pruebas.
func aparte(t *testing.T) {
	t.Helper()
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
}

// Dos apps de escritorio, una entrada del llavero: la que renueva segunda tiene que
// presentar el refresh token que dejó la primera, no el que ella tenía en memoria —Sytue
// ID revocaría la familia entera—, y no puede resucitar una sesión que otra ha cerrado.
func TestCompartidaRenuevaConLoGuardado(t *testing.T) {
	var presentado string
	idp := func(_ context.Context, actuales Tokens) (Tokens, error) {
		presentado = actuales.Refresh
		return Tokens{Access: "a", Refresh: "nuevo"}, nil
	}
	casos := []struct {
		nombre     string
		guardados  Tokens
		errCargar  error
		presentado string
		err        error
	}{
		{"otra app ya lo rotó: manda lo guardado", Tokens{Refresh: "r2"}, nil, "r2", nil},
		{"nadie lo ha tocado", Tokens{Refresh: "r1"}, nil, "r1", nil},
		{"otra app cerró la sesión: no se resucita", Tokens{}, nil, "", ErrLoginRequired},
		{"el llavero no se deja leer: lo de memoria", Tokens{}, errors.New("sin llavero"), "r1", nil},
	}
	for _, c := range casos {
		presentado = ""
		renovar := Compartida(idp, func() (Tokens, error) { return c.guardados, c.errCargar }, nil)
		_, err := renovar(context.Background(), Tokens{Refresh: "r1", Subject: "ada"})
		if !errors.Is(err, c.err) || presentado != c.presentado {
			t.Errorf("%s: presentó %q, %v; quiero %q, %v", c.nombre, presentado, err, c.presentado, c.err)
		}
	}
}

// El guardián de la carrera de renovación: mientras Compartida está pidiendo el token
// nuevo, NINGUNA otra app puede entrar a renovar. Sytue ID rota el refresh token y revoca
// la familia entera si le presentan uno gastado, así que dos renovaciones a la vez dejan
// a las trece apps fuera.
//
// Visto en ROJO quitando el `ajustes.Cerrojo` del principio de Compartida: el cerrojo de
// dentro entra, `libre` sale true y la prueba falla.
func TestCompartidaRenuevaConElCerrojoPuesto(t *testing.T) {
	aparte(t)
	var libre bool
	idp := func(_ context.Context, _ Tokens) (Tokens, error) {
		ctx, parar := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer parar()
		if soltar, err := ajustes.Cerrojo(ctx); err == nil {
			libre = true
			soltar()
		}
		return Tokens{Access: "a2", Refresh: "r2"}, nil
	}
	var guardado Tokens
	renovar := Compartida(idp,
		func() (Tokens, error) { return Tokens{Refresh: "r1", Subject: "ada"}, nil },
		func(t Tokens) error { guardado = t; return nil })

	nuevos, err := renovar(context.Background(), Tokens{Refresh: "r0", Subject: "ada"})
	if err != nil {
		t.Fatalf("renovar: %v", err)
	}
	if libre {
		t.Error("otra app ha podido entrar a renovar a la vez: el cerrojo no estaba puesto")
	}
	// Y lo nuevo se guarda DENTRO del cerrojo, con el `sub` que el refresh no devuelve.
	if guardado.Refresh != "r2" || guardado.Subject != "ada" {
		t.Errorf("guardó %+v", guardado)
	}
	if nuevos.Subject != "ada" {
		t.Errorf("devolvió %+v", nuevos)
	}
}

// Cerrar sesión en una app la cierra en las trece: lo guardado se borra, y quien lo mire
// después no encuentra con qué renovar. La marca es lo que hace que lo miren pronto.
func TestCerrarSesionSeVeDesdeOtraApp(t *testing.T) {
	aparte(t)
	var almacen Tokens
	antes := ajustes.Leer().Sesion

	// La app que cierra.
	s := NewSession(Tokens{Access: "a", Refresh: "r", Subject: "ada"}, nil, func(t Tokens) error {
		almacen = t
		ajustes.MarcarSesion()
		return nil
	})
	s.Logout()

	if almacen.Refresh != "" {
		t.Errorf("la entrada compartida ha quedado con %+v", almacen)
	}
	if ajustes.Leer().Sesion == antes {
		t.Error("la marca no ha cambiado: las otras doce apps no se enterarían")
	}
	// Y la de al lado, que tenía el mismo token en memoria, no la resucita.
	otra := Compartida(
		func(context.Context, Tokens) (Tokens, error) { return Tokens{Access: "resucitada"}, nil },
		func() (Tokens, error) { return almacen, nil }, nil)
	if _, err := otra(context.Background(), Tokens{Refresh: "r"}); !errors.Is(err, ErrLoginRequired) {
		t.Errorf("la otra app ha renovado tras el cierre: %v", err)
	}
}

func TestCompartidaNoRenuevaSinCerrojo(t *testing.T) {
	aparte(t)
	soltar, err := ajustes.Cerrojo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	ctx, cancelar := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelar()
	llamadas := 0
	renovar := Compartida(func(context.Context, Tokens) (Tokens, error) {
		llamadas++
		return Tokens{Refresh: "rotado"}, nil
	}, func() (Tokens, error) { return Tokens{Refresh: "inicial"}, nil }, nil)
	if _, err := renovar(ctx, Tokens{Refresh: "inicial"}); !errors.Is(err, context.DeadlineExceeded) || llamadas != 0 {
		t.Fatalf("renovó sin exclusión: llamadas=%d error=%v", llamadas, err)
	}
}

func TestCompartidaAdoptaElDuenoDelRefresh(t *testing.T) {
	aparte(t)
	renovar := Compartida(func(_ context.Context, actuales Tokens) (Tokens, error) {
		if actuales.Subject != "nueva" || actuales.Refresh != "r-nueva" || actuales.Access != "" {
			t.Fatalf("mezcló la cuenta anterior con el refresh compartido: %+v", actuales)
		}
		return Tokens{Access: "a-nueva", Refresh: "rotado"}, nil
	}, func() (Tokens, error) { return Tokens{Refresh: "r-nueva", Subject: "nueva"}, nil }, nil)
	nuevos, err := renovar(context.Background(), Tokens{Refresh: "r-vieja", Access: "a-vieja", Subject: "vieja"})
	if err != nil || nuevos.Subject != "nueva" {
		t.Fatalf("no adoptó la cuenta de otra app: %+v %v", nuevos, err)
	}
}

func TestSincronizarNoReescribeSesionYActualizaIdentidad(t *testing.T) {
	guardados := 0
	s := NewSession(Tokens{Access: "viejo", Refresh: "r1", Subject: "anterior"}, nil, func(Tokens) error { guardados++; return nil })
	s.Sincronizar(Tokens{Refresh: "r2", Subject: "nueva", Organizacion: "org"})
	if s.Subject() != "nueva" || s.Organizacion() != "org" || s.tokens.Access != "" {
		t.Fatal("otra app cambió de cuenta pero se conservó su identidad o access anterior")
	}
	s.Sincronizar(Tokens{})
	if s.Subject() != "" || guardados != 0 {
		t.Fatalf("el cierre se volvió a persistir: sujeto=%q escrituras=%d", s.Subject(), guardados)
	}
}
