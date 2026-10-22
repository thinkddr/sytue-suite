// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package blobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

var acme = tenancy.Tenant{Slug: "acme"}

type registroFalso struct {
	filas    map[string]bool // hash → listo
	liberado []string
}

func (r *registroFalso) ReservarBlobCompartido(_ context.Context, hash, _ string, _ int64) (bool, bool, error) {
	if r.filas == nil {
		r.filas = map[string]bool{}
	}
	listo, existe := r.filas[hash]
	if !existe {
		r.filas[hash] = false
	}
	return existe, listo, nil
}

func (r *registroFalso) MarcarBlobListo(_ context.Context, hash string) error {
	r.filas[hash] = true
	return nil
}

func (r *registroFalso) LiberarBlobCompartido(_ context.Context, hash string) error {
	delete(r.filas, hash)
	r.liberado = append(r.liberado, hash)
	return nil
}

type almacenFalso struct {
	subidas []objectstore.PutOptions
	claves  []string
	falla   error
}

func (a *almacenFalso) Put(_ context.Context, _ tenancy.Tenant, key string, r io.Reader, _ int64, opts objectstore.PutOptions) error {
	if a.falla != nil {
		return a.falla
	}
	_, _ = io.ReadAll(r)
	a.subidas = append(a.subidas, opts)
	a.claves = append(a.claves, key)
	return nil
}

func TestElMismoMensajeSeSubeUnaVez(t *testing.T) {
	reg, alm := &registroFalso{}, &almacenFalso{}
	raw := []byte("From: a@b\r\nSubject: hola\r\n\r\ncuerpo")
	k1, err := GuardarMensaje(context.Background(), reg, alm, acme, raw)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := GuardarMensaje(context.Background(), reg, alm, acme, raw)
	if err != nil {
		t.Fatal(err)
	}
	if k1 != k2 || !strings.HasPrefix(k1, "shared/mensajes/") || !strings.HasSuffix(k1, ".eml") {
		t.Fatalf("claves = %q, %q", k1, k2)
	}
	if len(alm.subidas) != 1 {
		t.Fatalf("subidas = %d, quiero 1", len(alm.subidas))
	}
	if !alm.subidas[0].Comprimir || alm.subidas[0].ContentType != "message/rfc822" {
		t.Fatalf("opciones de subida = %+v", alm.subidas[0])
	}
	// Otro mensaje, otra clave.
	k3, _ := GuardarMensaje(context.Background(), reg, alm, acme, []byte("otro"))
	if k3 == k1 || len(alm.subidas) != 2 {
		t.Fatalf("un contenido distinto debe ir a otra clave: %q (%d subidas)", k3, len(alm.subidas))
	}
}

func TestUnaReservaSinMarcarVuelveASubir(t *testing.T) {
	reg := &registroFalso{filas: map[string]bool{}}
	alm := &almacenFalso{}
	raw := []byte("x")
	// Primera pasada: reserva y sube, pero NO se marca (simulamos el fallo).
	_, _ = GuardarMensaje(context.Background(), reg, alm, acme, raw)
	for h := range reg.filas {
		reg.filas[h] = false
	}
	_, _ = GuardarMensaje(context.Background(), reg, alm, acme, raw)
	if len(alm.subidas) != 2 {
		t.Fatalf("con la reserva sin marcar hay que volver a subir: %d subidas", len(alm.subidas))
	}
}

func TestSiLaSubidaFallaSeSueltaLaReserva(t *testing.T) {
	reg := &registroFalso{}
	alm := &almacenFalso{falla: errors.New("s3 caído")}
	if _, err := GuardarMensaje(context.Background(), reg, alm, acme, []byte("x")); err == nil {
		t.Fatal("se esperaba error")
	}
	if len(reg.liberado) != 1 || len(reg.filas) != 0 {
		t.Fatalf("la reserva debía liberarse: liberado=%v filas=%v", reg.liberado, reg.filas)
	}
}

// Decisión del dueño (2026-10-16): el mismo mensaje en dos organizaciones son dos
// blobs. Si la segunda encontrara el de la primera, no subiría nada y acabaría antes:
// eso deja averiguar si otra organización tiene un mensaje concreto.
func TestOtraOrganizacionNoEncuentraElMensaje(t *testing.T) {
	reg, alm := &registroFalso{}, &almacenFalso{}
	raw := []byte("From: a@b\r\nSubject: hola\r\n\r\ncuerpo")
	k1, err := GuardarMensaje(context.Background(), reg, alm, acme, raw)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := GuardarMensaje(context.Background(), reg, alm, tenancy.Tenant{Slug: "otra"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k2 || len(alm.subidas) != 2 || !strings.HasPrefix(k2, "shared/mensajes/otra/") {
		t.Fatalf("claves %q y %q con %d subidas; quiero una por organización", k1, k2, len(alm.subidas))
	}
	if _, err := GuardarMensaje(context.Background(), reg, alm, tenancy.Tenant{}, raw); err == nil {
		t.Fatal("sin organización no se guarda")
	}
}
