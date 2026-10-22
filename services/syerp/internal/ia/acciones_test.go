// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ia

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

func TestPlanDe(t *testing.T) {
	t.Parallel()
	respuesta := "Aquí tienes:\n```json\n" + `{"resumen":"Gasto de Claude Code","pasos":[
		{"ref":"$P1","modelo":"contacto","buscar":"Anthropic","datos":{"nombre":"Anthropic Ireland, Limited","tipo":"organizacion"}},
		{"ref":"p2","modelo":"factura_compra","datos":{"proveedor":"$p1","fecha":"2026-09-15","base":180,"iva":0,"total":180}},
		{"ref":"p1","modelo":"contacto","datos":{}},
		{"ref":"mal ref","modelo":"contacto","datos":{}},
		{"ref":"p3","modelo":"","datos":{}}
	],"avisos":["Sube la factura en PDF"]}` + "\n```"
	p, err := PlanDe(respuesta)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Pasos) != 2 || p.Pasos[0].Ref != "p1" || p.Pasos[1].Modelo != "factura_compra" || p.Avisos[0] == "" {
		t.Fatalf("se quedan los pasos legibles y sin repetir referencia: %+v", p)
	}
	if ref, ok := Referencia(" $p1 "); !ok || ref != "p1" {
		t.Fatal("«$p1» apunta al paso p1")
	}
	if _, ok := Referencia("Anthropic"); ok {
		t.Fatal("un nombre no es una referencia")
	}
	if _, err := PlanDe(`{"resumen":"nada","pasos":[]}`); err == nil {
		t.Fatal("un plan sin pasos no es un plan")
	}

	instr := InstruccionAcciones("Añade un gasto de Claude Code de 180 €", Organizacion{Nombre: "Acme SL", Pais: "ES"}, time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC))
	for _, quiero := range []string{"inversión del sujeto pasivo", "Registro de Operadores Intracomunitarios", "Hoy es 2026-09-15", "No inventes NIF"} {
		if !strings.Contains(instr, quiero) {
			t.Errorf("la instrucción no dice %q", quiero)
		}
	}
	ctx := ContextoModelos([]catalogo.ModeloVista{{Clave: "factura_compra", Etiqueta: "Facturas recibidas", Campos: []catalogo.CampoVista{
		{Clave: "proveedor", Tipo: "relacion", Etiqueta: "Proveedor", Requerido: true, Modelo: "contacto"},
	}}})
	if !strings.Contains(ctx, "proveedor:relacion*→contacto «Proveedor»") {
		t.Fatalf("contexto: %s", ctx)
	}
}

func TestPistasDe(t *testing.T) {
	t.Parallel()
	respuesta := `{"movimientos":[
		{"id":"m1","empresa":" Anthropic ","tipo":"online","url_factura":"https://claude.ai/settings/billing","email":""},
		{"id":"m2","empresa":"Ferretería López","tipo":"tienda","email":"facturas@ferreterialopez.es"},
		{"id":"m9","empresa":"Inventada"},
		{"id":"m1","empresa":"Repetida"}
	]}`
	p := PistasDe(respuesta, map[string]bool{"m1": true, "m2": true})
	if len(p) != 2 || p[0].Empresa != "Anthropic" || p[1].Tipo != "desconocido" {
		t.Fatalf("solo los ids pedidos, una vez, y un tipo raro es desconocido: %+v", p)
	}
	ctx := ContextoPistas([]MovimientoPista{{ID: "m1", Fecha: "2026-09-10", Descripcion: "CLAUDE.AI SUBSCRIPTION", Centimos: -18000}})
	if !strings.Contains(ctx, "m1 | 2026-09-10 | 180.00 | CLAUDE.AI SUBSCRIPTION") {
		t.Fatalf("contexto: %s", ctx)
	}
}
