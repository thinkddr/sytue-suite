// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

func modeloContacto() *catalogo.Modelo {
	return &catalogo.Modelo{
		Clave: "contacto", Titulo: "nombre",
		Campos: []catalogo.Campo{
			{Clave: "nombre", Tipo: catalogo.TipoTexto, Requerido: true},
			{Clave: "tipo", Tipo: catalogo.TipoSeleccion, Opciones: []string{"organizacion", "persona"}},
			{Clave: "correo", Tipo: catalogo.TipoCorreo},
			{Clave: "web", Tipo: catalogo.TipoURL},
			{Clave: "empresa", Tipo: catalogo.TipoRelacion, Modelo: "contacto"},
			{Clave: "saldo", Tipo: catalogo.TipoDinero},
			{Clave: "cantidad", Tipo: catalogo.TipoDecimal},
			{Clave: "fecha", Tipo: catalogo.TipoFecha},
		},
	}
}

func TestValidarNormaliza(t *testing.T) {
	crudo := json.RawMessage(`{
		"nombre": "  Suministros Acme S.L. ",
		"tipo": "organizacion",
		"correo": "compras@acme.example",
		"web": "https://acme.example",
		"saldo": 1234500,
		"cantidad": "2.5",
		"fecha": "2026-09-12"
	}`)
	data, etiqueta, err := Validar(modeloContacto(), crudo)
	if err != nil {
		t.Fatalf("Validar: %v", err)
	}
	if etiqueta != "Suministros Acme S.L." {
		t.Errorf("etiqueta = %q", etiqueta)
	}
	if data["saldo"] != int64(1234500) {
		t.Errorf("saldo = %#v, se esperaba int64 en céntimos", data["saldo"])
	}
	if data["cantidad"] != "2.5" {
		t.Errorf("cantidad = %#v, se esperaba cadena exacta", data["cantidad"])
	}
}

// TestValidarRechazaCampoDesconocido: guardarlo en silencio es cómo se pierde un
// dato que nadie vuelve a encontrar.
func TestValidarRechazaCampoDesconocido(t *testing.T) {
	crudo := json.RawMessage(`{"nombre":"Acme","teléfono":"600"}`)
	if _, _, err := Validar(modeloContacto(), crudo); err == nil {
		t.Fatal("se esperaba error por campo no declarado")
	}
}

func TestValidarCampos(t *testing.T) {
	casos := map[string]string{
		"obligatorio ausente":   `{"tipo":"persona"}`,
		"obligatorio en blanco": `{"nombre":"   "}`,
		"selección inventada":   `{"nombre":"A","tipo":"cliente"}`,
		"correo inválido":       `{"nombre":"A","correo":"arroba.falta"}`,
		"url sin esquema":       `{"nombre":"A","web":"acme.example"}`,
		"relación no uuid":      `{"nombre":"A","empresa":"acme"}`,
		"dinero con decimales":  `{"nombre":"A","saldo":12.34}`,
		"decimal con letras":    `{"nombre":"A","cantidad":"2,5 kg"}`,
		"fecha con hora":        `{"nombre":"A","fecha":"2026-09-12T10:00:00Z"}`,
	}
	for nombre, cuerpo := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, _, err := Validar(modeloContacto(), json.RawMessage(cuerpo)); err == nil {
				t.Fatalf("se esperaba error para %s", cuerpo)
			}
		})
	}
}

func TestValidarAdmiteOpcionalesAusentesYNulos(t *testing.T) {
	data, _, err := Validar(modeloContacto(), json.RawMessage(`{"nombre":"Acme","correo":null}`))
	if err != nil {
		t.Fatalf("Validar: %v", err)
	}
	if _, hay := data["correo"]; hay {
		t.Error("un opcional nulo no debe guardarse como clave vacía")
	}
}

func TestRelacionesDevuelveLasQueHay(t *testing.T) {
	data := map[string]any{"empresa": "3f6c0f6e-0000-4000-8000-000000000001", "nombre": "A"}
	rel := Relaciones(modeloContacto(), data)
	if len(rel) != 1 || rel["empresa"] == "" {
		t.Fatalf("relaciones = %#v", rel)
	}
}
