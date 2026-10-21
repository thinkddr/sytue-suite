// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestTransiciones(t *testing.T) {
	ok := [][2]Status{
		{StatusUploading, StatusProcessing},
		{StatusProcessing, StatusReady},
		{StatusProcessing, StatusFailed},
		{StatusFailed, StatusProcessing},
		{StatusScheduled, StatusLive},
		{StatusLive, StatusEnded},
		{StatusEnded, StatusProcessing},
	}
	for _, p := range ok {
		if !PuedePasar(p[0], p[1]) {
			t.Errorf("%s → %s debería valer", p[0], p[1])
		}
	}
	mal := [][2]Status{
		{StatusReady, StatusUploading},
		{StatusUploading, StatusReady},
		{StatusLive, StatusScheduled},
		{StatusScheduled, StatusReady},
		{StatusReady, StatusReady},
		{StatusUploading, StatusLive},
		{StatusReady, StatusProcessing},
	}
	for _, p := range mal {
		if PuedePasar(p[0], p[1]) {
			t.Errorf("%s → %s no debería valer", p[0], p[1])
		}
	}
	if EstadoInicial(KindUpload) != StatusUploading || EstadoInicial(KindLive) != StatusScheduled {
		t.Fatal("estado inicial")
	}
}

func TestTitulo(t *testing.T) {
	if s, err := Titulo("  Hola \n mundo  "); err != nil || s != "Hola mundo" {
		t.Fatalf("%q %v", s, err)
	}
	if _, err := Titulo("   "); !errors.Is(err, ErrEntradaInvalida) {
		t.Fatal("vacío debería fallar")
	}
	if _, err := Titulo(strings.Repeat("ñ", MaxTitulo)); err != nil {
		t.Fatal("200 runas caben")
	}
	if _, err := Titulo(strings.Repeat("ñ", MaxTitulo+1)); !errors.Is(err, ErrEntradaInvalida) {
		t.Fatal("201 runas no caben")
	}
}

func TestCambiosNormalizar(t *testing.T) {
	mala := Visibility("secreto")
	c := Cambios{Visibility: &mala}
	if err := c.Normalizar(); !errors.Is(err, ErrEntradaInvalida) {
		t.Fatal("visibilidad desconocida debería fallar")
	}
	tit := " Título "
	c = Cambios{Title: &tit}
	if err := c.Normalizar(); err != nil || *c.Title != "Título" {
		t.Fatalf("%v %q", err, *c.Title)
	}
	if !(Cambios{}).Vacios() || c.Vacios() {
		t.Fatal("Vacios")
	}
}
