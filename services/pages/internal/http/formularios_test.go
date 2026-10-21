// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func TestFilasEnviosCSV_NeutralizaFormulasYOrdena(t *testing.T) {
	campos := []ir.CampoFormulario{
		{ID: "nombre", Kind: "text", Label: map[string]string{"es": "Nombre completo", "en": "Full name"}},
		{ID: "email", Kind: "email", Label: map[string]string{"es": "Correo electrónico", "en": "Email"}},
	}

	creado := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	envios := []domain.FormSubmission{
		{
			CreatedAt: creado,
			Language:  "es",
			Data: map[string]any{
				"nombre":   "=cmd|' /C calc'!A0", // Fórmula maliciosa para neutralizar
				"email":    "usuario@example.test",
				"telefono": "+34600000000", // Campo extra de versión anterior
			},
		},
	}

	filas := filasEnviosCSV(campos, envios, "es")
	if len(filas) != 2 {
		t.Fatalf("se esperaban 2 filas (cabecera + datos), hay %d", len(filas))
	}

	cabecera := filas[0]
	// created_at, language, Nombre completo, Correo electrónico, telefono
	if len(cabecera) != 5 {
		t.Fatalf("cabecera esperada con 5 columnas, hay %d: %v", len(cabecera), cabecera)
	}
	if cabecera[0] != "created_at" || cabecera[1] != "language" || cabecera[2] != "Nombre completo" || cabecera[3] != "Correo electrónico" || cabecera[4] != "telefono" {
		t.Errorf("cabecera inesperada: %v", cabecera)
	}

	fila := filas[1]
	if fila[0] != "2026-10-16T12:00:00Z" {
		t.Errorf("fecha inesperada: %s", fila[0])
	}
	if fila[1] != "es" {
		t.Errorf("idioma inesperado: %s", fila[1])
	}
	// El valor con '=' debe haber sido neutralizado con apóstrofe por validation.CSVSafe
	if fila[2] != "'=cmd|' /C calc'!A0" {
		t.Errorf("fórmula no neutralizada: %q", fila[2])
	}
	if fila[3] != "usuario@example.test" {
		t.Errorf("email inesperado: %s", fila[3])
	}
	// El teléfono con '+' debe haber sido neutralizado por validation.CSVSafe
	if fila[4] != "'+34600000000" {
		t.Errorf("campo extra con + no neutralizado: %q", fila[4])
	}
}

func TestValorCSV(t *testing.T) {
	casos := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"texto", "texto"},
		{42.5, "42.5"},
		{100.0, "100"},
		{true, "true"},
	}
	for _, tc := range casos {
		if got := valorCSV(tc.in); got != tc.want {
			t.Errorf("valorCSV(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
