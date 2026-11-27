// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fallo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

var (
	errNoListo   = errors.New("mail: private source import is not complete")
	errRechazado = errors.New("webport: scoped host operation rejected")
)

func init() {
	Conocido(errNoListo, "no_listo")
	Conocido(errRechazado, "rechazado")
}

func TestMotivoDiceElPasoYElErrorConocido(t *testing.T) {
	secreto := errors.New("https://mail.example/ana@example.com informe.pdf")
	casos := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{secreto, ""},
		{errNoListo, "no_listo"},
		{En("preparar", En("capacidades", errNoListo)), "preparar.capacidades.no_listo"},
		// Con fmt.Errorf en medio (lo que ya envolvía el núcleo) y su texto con datos.
		{En("preparar", fmt.Errorf("mail vault import %v: %w", secreto.Error(), En("importar", errRechazado))), "preparar.importar.rechazado"},
		// Un paso sin error conocido debajo: al menos el paso.
		{En("producto", En("autoridad", secreto)), "producto.autoridad"},
		// De un Join (el cierre que falla tras el fallo), la primera rama que diga algo.
		{errors.Join(En("abrir", errNoListo), En("cerrar", errRechazado)), "abrir.no_listo"},
		{errors.Join(secreto, En("cerrar", errRechazado)), "cerrar.rechazado"},
		{En("abrir", context.Canceled), "abrir.cancelado"},
		{fmt.Errorf("x: %w", context.DeadlineExceeded), "plazo"},
	}
	for _, c := range casos {
		if got := Motivo(c.err); got != c.want {
			t.Errorf("Motivo(%v) = %q, quería %q", c.err, got, c.want)
		}
	}
}

func TestUnNombreQueNoEsFijoNoSeCopia(t *testing.T) {
	err := En("https://mail.example", errNoListo)
	if got := Motivo(err); got != "no_listo" {
		t.Fatalf("Motivo = %q", got)
	}
	Conocido(errors.New("otro"), "Con Espacios")
	if got := Motivo(errors.New("otro")); got != "" {
		t.Fatalf("Motivo = %q", got)
	}
	if En("paso", nil) != nil {
		t.Fatal("sin error no hay paso")
	}
}

func TestElMotivoTieneUnLargoAcotado(t *testing.T) {
	err := errNoListo
	for range 20 {
		err = En(strings.Repeat("p", 32), err)
	}
	got := Motivo(err)
	if len(got) > maxLargo || strings.Count(got, ".") >= maxTrozos {
		t.Fatalf("Motivo demasiado largo: %d %q", len(got), got)
	}
	if got == "" {
		t.Fatal("Motivo vacío")
	}
}

func TestElErrorSigueSiendoElMismo(t *testing.T) {
	err := En("preparar", fmt.Errorf("envuelto: %w", errNoListo))
	if !errors.Is(err, errNoListo) {
		t.Fatal("errors.Is no ve el error de dentro")
	}
	if !strings.HasPrefix(err.Error(), "preparar: envuelto") {
		t.Fatalf("Error() = %q", err.Error())
	}
}
