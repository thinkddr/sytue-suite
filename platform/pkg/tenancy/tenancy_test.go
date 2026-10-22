// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tenancy

import (
	"context"
	"errors"
	"testing"
)

func TestValidSlug(t *testing.T) {
	ok := []string{"acme", "a", "acme-corp", "t3nant", "x-y-z"}
	bad := []string{"", "-acme", "acme-", "Acme", "ácme", "a_b", "a b"}
	for _, s := range ok {
		if !ValidSlug(s) {
			t.Errorf("esperaba válido: %q", s)
		}
	}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Errorf("esperaba inválido: %q", s)
		}
	}
}

func TestDerivations(t *testing.T) {
	tn, err := New("acme")
	if err != nil {
		t.Fatal(err)
	}
	if tn.Group() != "org/acme" || tn.Namespace() != "tenant-acme" || tn.StoragePrefix() != "tenants/acme/" {
		t.Fatalf("derivaciones incorrectas: %+v", tn)
	}
}

func TestContextRoundTrip(t *testing.T) {
	tn, _ := New("acme")
	ctx := Into(context.Background(), tn)
	got, err := From(ctx)
	if err != nil || got.Slug != "acme" {
		t.Fatalf("From = %+v, %v", got, err)
	}
	if _, err := From(context.Background()); !errors.Is(err, ErrNoTenant) {
		t.Fatalf("esperaba ErrNoTenant, got %v", err)
	}
}

func TestNewInvalid(t *testing.T) {
	if _, err := New("BAD_SLUG"); err == nil {
		t.Fatal("esperaba error")
	}
}

// La lista de reservados es la de las organizaciones y la de las webs de Sytue Web
// (WEB-08): un nombre de la suite, uno que parezca oficial o uno bajo un prefijo que
// la plataforma acuña no lo toma nadie.
func TestReservado(t *testing.T) {
	for _, s := range []string{"id", "login", "WWW", " sytue ", "soporte", "ayuda", "nas-acme"} {
		if !Reservado(s) {
			t.Errorf("%q debe estar reservado", s)
		}
	}
	for _, s := range []string{"panaderia-aurora", "nasa", "acme"} {
		if Reservado(s) {
			t.Errorf("%q no debe estar reservado", s)
		}
	}
}
